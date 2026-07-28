package analyzer

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strings"
	"sync"

	"github.com/example/yocto-lens/internal/model"
)

type ProgressFunc func(model.ScanProgress)

var assignmentPattern = regexp.MustCompile(`^([A-Za-z0-9_:+${}./-]+)\s*(\?=|\+=|=|:=|\.=|=\+)\s*"(.*)"`)
var looseAssignmentPattern = regexp.MustCompile(`^([A-Za-z0-9_:+${}./-]+)\s*(\?=|\+=|=|:=|\.=|=\+)\s*(.*)$`)
var recipeNamePattern = regexp.MustCompile(`^[a-z0-9][a-z0-9+.-]*(_[A-Za-z0-9.+~:-]+)?$`)
var variableNamePattern = regexp.MustCompile(`^[A-Z0-9_:+${}./-]+$`)

var skipDirs = map[string]bool{
	".git":                 true,
	".repo":                true,
	"tmp":                  true,
	"downloads":            true,
	"sstate-cache":         true,
	"cache":                true,
	"deploy":               true,
	"work":                 true,
	"sysroots":             true,
	"sysroots-components":  true,
	"pkgdata":              true,
	"stamps":               true,
	"logs":                 true,
	"log":                  true,
	"node_modules":         true,
	"__pycache__":          true,
	"buildhistory":         true,
	"lost+found":           true,
	"pseudo":               true,
	"temp":                 true,
	"runqueue":             true,
	"saved_tmpdir":         true,
	"yocto-lens-reports":   true,
	"yocto_static_reports": true,
}

type parsedFileKind int

const (
	parsedFileUnknown parsedFileKind = iota
	parsedRecipe
	parsedAppend
	parsedPatch
)

type parseJob struct {
	Index int
	Path  string
	Layer model.Layer
}

type parseResult struct {
	Index  int
	Path   string
	Kind   parsedFileKind
	Recipe model.Recipe
	Append model.Append
	Patch  model.Patch
	Err    error
}

func Analyze(paths []string) (model.Report, error) {
	return AnalyzeWithProgress(paths, nil)
}

func AnalyzeWithProgress(paths []string, progress ProgressFunc) (model.Report, error) {
	if len(paths) == 0 {
		paths = []string{"."}
	}

	report := model.Report{
		Root:     strings.Join(paths, ", "),
		Layers:   []model.Layer{},
		Recipes:  []model.Recipe{},
		Appends:  []model.Append{},
		Patches:  []model.Patch{},
		Findings: []model.Finding{},
	}

	emit(progress, model.ScanProgress{
		Phase:       model.PhaseStarting,
		CurrentPath: report.Root,
	})

	layers, err := discoverLayers(paths, progress)
	if err != nil {
		return report, err
	}
	report.Layers = layers

	filesProcessed := 0

	for _, layer := range layers {
		emit(progress, model.ScanProgress{
			Phase:          model.PhaseParsing,
			CurrentPath:    layer.Path,
			LayersFound:    len(report.Layers),
			RecipesFound:   len(report.Recipes),
			AppendsFound:   len(report.Appends),
			PatchesFound:   len(report.Patches),
			FilesProcessed: filesProcessed,
			FindingsFound:  len(report.Findings),
		})

		if err := parseLayerFiles(layer, &report, &filesProcessed, progress); err != nil {
			return report, err
		}
	}

	emit(progress, model.ScanProgress{
		Phase:          model.PhaseRules,
		CurrentPath:    "running rules",
		LayersFound:    len(report.Layers),
		RecipesFound:   len(report.Recipes),
		AppendsFound:   len(report.Appends),
		PatchesFound:   len(report.Patches),
		FilesProcessed: filesProcessed,
		FindingsFound:  len(report.Findings),
	})

	report.Findings = runAllRules(report)

	sort.SliceStable(report.Findings, func(i, j int) bool {
		if severityRank(report.Findings[i].Severity) == severityRank(report.Findings[j].Severity) {
			return report.Findings[i].RuleID < report.Findings[j].RuleID
		}
		return severityRank(report.Findings[i].Severity) > severityRank(report.Findings[j].Severity)
	})

	emit(progress, model.ScanProgress{
		Phase:          model.PhaseDone,
		CurrentPath:    "done",
		LayersFound:    len(report.Layers),
		RecipesFound:   len(report.Recipes),
		AppendsFound:   len(report.Appends),
		PatchesFound:   len(report.Patches),
		FilesProcessed: filesProcessed,
		FindingsFound:  len(report.Findings),
		Done:           true,
	})

	return report, nil
}

func parseLayerFiles(layer model.Layer, report *model.Report, filesProcessed *int, progress ProgressFunc) error {
	jobs, err := collectParseJobs(layer)
	if err != nil {
		return err
	}
	if len(jobs) == 0 {
		return nil
	}

	parsedRecipes := 0
	parsedAppends := 0
	parsedPatches := 0

	results := parseFilesConcurrently(jobs, func(result parseResult) {
		*filesProcessed++

		if result.Err == nil {
			switch result.Kind {
			case parsedRecipe:
				parsedRecipes++
			case parsedAppend:
				parsedAppends++
			case parsedPatch:
				parsedPatches++
			}
		}

		if *filesProcessed%50 == 0 {
			emit(progress, model.ScanProgress{
				Phase:          model.PhaseParsing,
				CurrentPath:    result.Path,
				LayersFound:    len(report.Layers),
				RecipesFound:   len(report.Recipes) + parsedRecipes,
				AppendsFound:   len(report.Appends) + parsedAppends,
				PatchesFound:   len(report.Patches) + parsedPatches,
				FilesProcessed: *filesProcessed,
				FindingsFound:  len(report.Findings),
			})
		}
	})
	sort.SliceStable(results, func(i, j int) bool {
		return results[i].Index < results[j].Index
	})

	for _, result := range results {
		if result.Err != nil {
			continue
		}

		switch result.Kind {
		case parsedRecipe:
			report.Recipes = append(report.Recipes, result.Recipe)
		case parsedAppend:
			report.Appends = append(report.Appends, result.Append)
		case parsedPatch:
			report.Patches = append(report.Patches, result.Patch)
		}
	}

	emit(progress, model.ScanProgress{
		Phase:          model.PhaseParsing,
		CurrentPath:    layer.Path,
		LayersFound:    len(report.Layers),
		RecipesFound:   len(report.Recipes),
		AppendsFound:   len(report.Appends),
		PatchesFound:   len(report.Patches),
		FilesProcessed: *filesProcessed,
		FindingsFound:  len(report.Findings),
	})

	return nil
}

func collectParseJobs(layer model.Layer) ([]parseJob, error) {
	var jobs []parseJob

	err := filepath.WalkDir(layer.Path, func(path string, d os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return nil
		}

		if d.IsDir() {
			if shouldSkipDir(d.Name()) && path != layer.Path {
				return filepath.SkipDir
			}
			return nil
		}

		if !isInterestingFile(path) {
			return nil
		}

		jobs = append(jobs, parseJob{
			Index: len(jobs),
			Path:  path,
			Layer: layer,
		})

		return nil
	})
	if err != nil {
		return nil, err
	}

	return jobs, nil
}

func parseFilesConcurrently(jobs []parseJob, onResult func(parseResult)) []parseResult {
	results := make(chan parseResult, len(jobs))
	jobCh := make(chan parseJob)

	var wg sync.WaitGroup
	workerCount := parseWorkerCount(len(jobs))
	for i := 0; i < workerCount; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for job := range jobCh {
				results <- parseInterestingFile(job)
			}
		}()
	}

	go func() {
		for _, job := range jobs {
			jobCh <- job
		}
		close(jobCh)
		wg.Wait()
		close(results)
	}()

	parsed := make([]parseResult, 0, len(jobs))
	for result := range results {
		parsed = append(parsed, result)
		if onResult != nil {
			onResult(result)
		}
	}

	return parsed
}

func parseWorkerCount(jobCount int) int {
	if jobCount <= 0 {
		return 0
	}

	workers := runtime.GOMAXPROCS(0) * 2
	if workers < 1 {
		workers = 1
	}
	if workers > 32 {
		workers = 32
	}
	if workers > jobCount {
		workers = jobCount
	}

	return workers
}

func parseInterestingFile(job parseJob) parseResult {
	result := parseResult{
		Index: job.Index,
		Path:  job.Path,
	}

	switch {
	case strings.HasSuffix(job.Path, ".bb"):
		recipe, err := parseRecipe(job.Path, job.Layer)
		result.Kind = parsedRecipe
		result.Recipe = recipe
		result.Err = err

	case strings.HasSuffix(job.Path, ".bbappend"):
		appendFile, err := parseAppend(job.Path, job.Layer)
		result.Kind = parsedAppend
		result.Append = appendFile
		result.Err = err

	case strings.HasSuffix(job.Path, ".patch"):
		patch, err := parsePatch(job.Path, job.Layer)
		result.Kind = parsedPatch
		result.Patch = patch
		result.Err = err

	default:
		result.Kind = parsedFileUnknown
	}

	return result
}

func discoverLayers(paths []string, progress ProgressFunc) ([]model.Layer, error) {
	seen := map[string]bool{}
	var layers []model.Layer

	for _, input := range paths {
		root, err := filepath.Abs(input)
		if err != nil {
			return nil, err
		}

		info, err := os.Stat(root)
		if err != nil {
			return nil, err
		}

		if !info.IsDir() {
			return nil, fmt.Errorf("%s is not a directory", root)
		}

		if isLayerDir(root) {
			addLayer(&layers, seen, root)
			continue
		}

		err = filepath.WalkDir(root, func(path string, d os.DirEntry, walkErr error) error {
			if walkErr != nil || !d.IsDir() {
				return nil
			}

			if shouldSkipDir(d.Name()) && path != root {
				return filepath.SkipDir
			}

			if isLayerDir(path) {
				addLayer(&layers, seen, path)
				emit(progress, model.ScanProgress{
					Phase:       model.PhaseDiscovering,
					CurrentPath: path,
					LayersFound: len(layers),
				})
				return filepath.SkipDir
			}

			return nil
		})

		if err != nil {
			return nil, err
		}
	}

	sort.Slice(layers, func(i, j int) bool {
		return layers[i].Path < layers[j].Path
	})

	return layers, nil
}

func addLayer(layers *[]model.Layer, seen map[string]bool, path string) {
	clean := filepath.Clean(path)
	if seen[clean] {
		return
	}

	seen[clean] = true

	*layers = append(*layers, model.Layer{
		Path: clean,
		Name: filepath.Base(clean),
	})
}

func isLayerDir(path string) bool {
	_, err := os.Stat(filepath.Join(path, "conf", "layer.conf"))
	return err == nil
}

func shouldSkipDir(name string) bool {
	return skipDirs[name]
}

func isInterestingFile(path string) bool {
	return strings.HasSuffix(path, ".bb") ||
		strings.HasSuffix(path, ".bbappend") ||
		strings.HasSuffix(path, ".patch")
}

func parseRecipe(path string, layer model.Layer) (model.Recipe, error) {
	lines, vars, err := parseMetadataFileWithIncludes(path, layer.Path)
	if err != nil {
		return model.Recipe{}, err
	}

	name := strings.TrimSuffix(filepath.Base(path), ".bb")
	pn := recipePNFromName(name)
	pv := ""

	if idx := strings.Index(name, "_"); idx >= 0 && idx+1 < len(name) {
		pv = name[idx+1:]
	}

	if v, ok := vars["PN"]; ok && strings.TrimSpace(v) != "" {
		pn = v
	}

	if v, ok := vars["PV"]; ok && strings.TrimSpace(v) != "" {
		pv = v
	}

	return model.Recipe{
		Path:      path,
		Layer:     layer.Name,
		Name:      name,
		PN:        pn,
		PV:        pv,
		Variables: vars,
		Lines:     lines,
	}, nil
}

func parseAppend(path string, layer model.Layer) (model.Append, error) {
	lines, vars, err := parseMetadataFileWithIncludes(path, layer.Path)
	if err != nil {
		return model.Append{}, err
	}

	name := strings.TrimSuffix(filepath.Base(path), ".bbappend")

	return model.Append{
		Path:      path,
		Layer:     layer.Name,
		Name:      name,
		Target:    name,
		Variables: vars,
		Lines:     lines,
	}, nil
}

func parsePatch(path string, layer model.Layer) (model.Patch, error) {
	lines, _, err := parseMetadataFile(path)
	if err != nil {
		return model.Patch{}, err
	}

	return model.Patch{
		Path:  path,
		Layer: layer.Name,
		Lines: lines,
	}, nil
}

func parseMetadataFileWithIncludes(path string, layerRoot string) ([]string, map[string]string, error) {
	return parseMetadataFileWithIncludesSeen(path, layerRoot, map[string]bool{})
}

func parseMetadataFileWithIncludesSeen(path string, layerRoot string, seen map[string]bool) ([]string, map[string]string, error) {
	cleanPath, err := filepath.Abs(path)
	if err != nil {
		return nil, nil, err
	}

	if seen[cleanPath] {
		return []string{}, map[string]string{}, nil
	}
	seen[cleanPath] = true

	lines, vars, err := parseMetadataFile(path)
	if err != nil {
		return nil, nil, err
	}

	for _, line := range lines {
		includePath, ok := parseIncludeDirective(line)
		if !ok {
			continue
		}

		resolved, ok := resolveIncludePath(includePath, path, layerRoot, vars)
		if !ok {
			continue
		}

		includeLines, includeVars, includeErr := parseMetadataFileWithIncludesSeen(resolved, layerRoot, seen)
		if includeErr != nil {
			continue
		}

		lines = append(includeLines, lines...)
		mergeVars(vars, includeVars)
	}

	return lines, vars, nil
}

func parseIncludeDirective(line string) (string, bool) {
	trimmed := strings.TrimSpace(line)

	if trimmed == "" || strings.HasPrefix(trimmed, "#") {
		return "", false
	}

	fields := strings.Fields(trimmed)
	if len(fields) < 2 {
		return "", false
	}

	if fields[0] != "include" && fields[0] != "require" {
		return "", false
	}

	includePath := strings.Trim(fields[1], "\"'")
	includePath = strings.TrimSpace(includePath)

	if includePath == "" {
		return "", false
	}

	return includePath, true
}

func resolveIncludePath(includePath string, currentFile string, layerRoot string, vars map[string]string) (string, bool) {
	includePath = expandSimpleVars(includePath, vars)

	if filepath.IsAbs(includePath) {
		if fileExists(includePath) {
			return includePath, true
		}
		return "", false
	}

	candidates := []string{
		filepath.Join(filepath.Dir(currentFile), includePath),
		filepath.Join(layerRoot, includePath),
		filepath.Join(layerRoot, "recipes-core", includePath),
		filepath.Join(layerRoot, "recipes-bsp", includePath),
		filepath.Join(layerRoot, "recipes-kernel", includePath),
		filepath.Join(layerRoot, "recipes-app", includePath),
		filepath.Join(layerRoot, "recipes-support", includePath),
	}

	for _, candidate := range candidates {
		if fileExists(candidate) {
			return candidate, true
		}
	}

	base := filepath.Base(includePath)
	found := ""

	_ = filepath.WalkDir(layerRoot, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}

		if filepath.Base(path) == base {
			found = path
			return filepath.SkipAll
		}

		return nil
	})

	if found != "" {
		return found, true
	}

	return "", false
}

func expandSimpleVars(value string, vars map[string]string) string {
	for key, val := range vars {
		value = strings.ReplaceAll(value, "${"+key+"}", val)
	}

	return value
}

func mergeVars(target map[string]string, source map[string]string) {
	for key, value := range source {
		if strings.TrimSpace(value) == "" {
			continue
		}

		if _, exists := target[key]; !exists {
			target[key] = value
		}
	}
}

func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

func parseMetadataFile(path string) ([]string, map[string]string, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, nil, err
	}
	defer file.Close()

	lines := []string{}
	vars := map[string]string{}

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()
		lines = append(lines, line)

		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}

		matches := assignmentPattern.FindStringSubmatch(trimmed)
		if len(matches) == 4 {
			key := normalizeVar(matches[1])
			value := strings.TrimSpace(matches[3])

			if existing, ok := vars[key]; ok && existing != "" {
				vars[key] = strings.TrimSpace(existing + " " + value)
			} else {
				vars[key] = value
			}
		}
	}

	return lines, vars, scanner.Err()
}

func normalizeVar(v string) string {
	replacers := []string{
		":append", "",
		":prepend", "",
		":remove", "",
		"_append", "",
		"_prepend", "",
		"_remove", "",
	}

	r := strings.NewReplacer(replacers...)
	v = r.Replace(v)

	if idx := strings.Index(v, ":"); idx >= 0 {
		v = v[:idx]
	}

	return v
}

func runAllRules(report model.Report) []model.Finding {
	var findings []model.Finding

	findings = append(findings, checkLayerDependencyGraph(report)...)

	for _, layer := range report.Layers {
		findings = append(findings, checkLayerBasics(layer)...)
	}

	for _, recipe := range report.Recipes {
		findings = append(findings, checkRecipeStatic(recipe)...)
		findings = append(findings, checkLicenseCompliance(recipe)...)
		findings = append(findings, checkRecipeStyle(recipe)...)
		findings = append(findings, checkLinesStatic(recipe.Path, recipe.Layer, recipe.Lines)...)
		findings = append(findings, checkLinesStyle(recipe.Path, recipe.Layer, recipe.Lines)...)
	}

	for _, appendFile := range report.Appends {
		findings = append(findings, checkAppendStatic(appendFile, report.Recipes)...)
		findings = append(findings, checkAppendStyle(appendFile)...)
		findings = append(findings, checkLinesStatic(appendFile.Path, appendFile.Layer, appendFile.Lines)...)
		findings = append(findings, checkLinesStyle(appendFile.Path, appendFile.Layer, appendFile.Lines)...)
	}

	for _, patch := range report.Patches {
		findings = append(findings, checkPatchStatic(patch)...)
		findings = append(findings, checkPatchStyle(patch)...)
	}

	findings = append(findings, checkDuplicateRecipes(report.Recipes)...)

	return findings
}

func checkLayerBasics(layer model.Layer) []model.Finding {
	var findings []model.Finding

	conf := filepath.Join(layer.Path, "conf", "layer.conf")
	lines, vars, err := parseMetadataFileWithIncludes(conf, layer.Path)
	if err != nil {
		return findings
	}

	if !hasLayerSeriesCompat(vars) {
		findings = append(findings, finding(
			"static/layer-missing-series-compat",
			"Layer missing LAYERSERIES_COMPAT",
			model.SeverityMedium,
			layer.Name,
			conf,
			1,
			"Layer does not declare LAYERSERIES_COMPAT.",
			"Layer compatibility should be explicit so CI and integrators know which Yocto releases are supported.",
			"Add LAYERSERIES_COMPAT for this layer in conf/layer.conf.",
		))
	}

	if !hasVar(vars, "BBFILE_COLLECTIONS") {
		findings = append(findings, finding(
			"static/layer-missing-bbfile-collections",
			"Layer missing BBFILE_COLLECTIONS",
			model.SeverityMedium,
			layer.Name,
			conf,
			1,
			"Layer does not declare BBFILE_COLLECTIONS.",
			"A layer without BBFILE_COLLECTIONS can be misconfigured or hard to integrate cleanly.",
			"Add BBFILE_COLLECTIONS and matching BBFILE_PATTERN / BBFILE_PRIORITY entries.",
		))
	}

	findings = append(findings, checkLinesStatic(conf, layer.Name, lines)...)
	findings = append(findings, checkLinesStyle(conf, layer.Name, lines)...)

	return findings
}

type layerNode struct {
	Layer       model.Layer
	ConfigPath  string
	Collection  string
	Collections []string
	Depends     []string
	Recommends  []string
	Series      []string
}

func checkLayerDependencyGraph(report model.Report) []model.Finding {
	nodes := buildLayerNodes(report.Layers)
	if len(nodes) == 0 {
		return nil
	}

	var findings []model.Finding

	knownCollections := map[string]bool{}
	knownLayerNames := map[string]bool{}

	for _, node := range nodes {
		knownLayerNames[node.Layer.Name] = true
		for _, collection := range node.Collections {
			knownCollections[collection] = true
		}
	}

	dependents := map[string]int{}

	for _, node := range nodes {
		for _, dep := range node.Depends {
			dependents[dep]++

			if !knownCollections[dep] && !knownLayerNames[dep] {
				findings = append(findings, finding(
					"static/layer-missing-dependency",
					"Layer dependency not found",
					model.SeverityHigh,
					node.Layer.Name,
					node.ConfigPath,
					findLine(nodeLines(node.ConfigPath), "LAYERDEPENDS"),
					fmt.Sprintf("Layer declares dependency %q, but that dependency was not found in scanned layers.", dep),
					"Missing layer dependencies can cause BitBake parse failures, missing classes, missing recipes, or unexpected provider resolution.",
					"Add the missing layer to BBLAYERS or remove the stale dependency from LAYERDEPENDS.",
				))
			}
		}

		for _, rec := range node.Recommends {
			if !knownCollections[rec] && !knownLayerNames[rec] {
				findings = append(findings, finding(
					"static/layer-recommendation-not-found",
					"Layer recommendation not found",
					model.SeverityLow,
					node.Layer.Name,
					node.ConfigPath,
					findLine(nodeLines(node.ConfigPath), "LAYERRECOMMENDS"),
					fmt.Sprintf("Layer recommends %q, but that layer was not found in scanned layers.", rec),
					"Missing recommended layers might not break parsing, but optional functionality may be unavailable.",
					"Add the recommended layer if the functionality is required, or document why it is intentionally absent.",
				))
			}
		}
	}

	findings = append(findings, checkLayerDependencyCycles(nodes)...)

	for _, node := range nodes {
		hasDeps := len(node.Depends) > 0
		isDependedOn := false

		for _, collection := range node.Collections {
			if dependents[collection] > 0 {
				isDependedOn = true
				break
			}
		}

		if !hasDeps && !isDependedOn && len(nodes) > 1 {
			findings = append(findings, finding(
				"static/layer-isolated",
				"Layer appears isolated",
				model.SeverityInfo,
				node.Layer.Name,
				node.ConfigPath,
				1,
				"Layer has no LAYERDEPENDS and no other scanned layer depends on it.",
				"An isolated layer can be valid, but it may also indicate an unused or disconnected layer in a larger workspace.",
				"Confirm this layer is intentionally standalone, or add/remove it from the workspace as appropriate.",
			))
		}

		if len(node.Collections) == 0 {
			findings = append(findings, finding(
				"static/layer-no-collection-name",
				"Layer has no collection name",
				model.SeverityMedium,
				node.Layer.Name,
				node.ConfigPath,
				1,
				"Layer does not expose a usable collection name from BBFILE_COLLECTIONS.",
				"Layer dependency checks rely on collection names, and BitBake also uses them for layer configuration.",
				"Set BBFILE_COLLECTIONS in conf/layer.conf.",
			))
		}
	}

	return findings
}

func buildLayerNodes(layers []model.Layer) []layerNode {
	nodes := make([]layerNode, 0, len(layers))

	for _, layer := range layers {
		conf := filepath.Join(layer.Path, "conf", "layer.conf")
		_, vars, err := parseMetadataFileWithIncludes(conf, layer.Path)
		if err != nil {
			continue
		}

		collections := layerCollections(layer, vars)
		primary := layer.Name
		if len(collections) > 0 {
			primary = collections[0]
		}

		nodes = append(nodes, layerNode{
			Layer:       layer,
			ConfigPath:  conf,
			Collection:  primary,
			Collections: collections,
			Depends:     layerDepends(vars, collections),
			Recommends:  layerRecommends(vars, collections),
			Series:      layerSeries(vars, collections),
		})
	}

	return nodes
}

func layerCollections(layer model.Layer, vars map[string]string) []string {
	collections := splitLayerList(vars["BBFILE_COLLECTIONS"])

	if len(collections) == 0 {
		collections = append(collections, layer.Name)
	}

	return uniqueStrings(collections)
}

func layerDepends(vars map[string]string, collections []string) []string {
	return layerRelationValues(vars, "LAYERDEPENDS", collections)
}

func layerRecommends(vars map[string]string, collections []string) []string {
	return layerRelationValues(vars, "LAYERRECOMMENDS", collections)
}

func layerSeries(vars map[string]string, collections []string) []string {
	values := layerRelationValues(vars, "LAYERSERIES_COMPAT", collections)
	if len(values) == 0 {
		values = splitLayerList(vars["LAYERSERIES_COMPAT"])
	}
	return uniqueStrings(values)
}

func layerRelationValues(vars map[string]string, prefix string, collections []string) []string {
	var values []string

	if v, ok := vars[prefix]; ok {
		values = append(values, splitLayerList(v)...)
	}

	for key, value := range vars {
		if key == prefix {
			continue
		}

		if strings.HasPrefix(key, prefix+"_") {
			values = append(values, splitLayerList(value)...)
			continue
		}

		for _, collection := range collections {
			if key == prefix+"_"+collection {
				values = append(values, splitLayerList(value)...)
			}
		}
	}

	return uniqueStrings(cleanLayerRelationValues(values))
}

func splitLayerList(value string) []string {
	value = strings.ReplaceAll(value, "\\", " ")
	value = strings.ReplaceAll(value, "\"", " ")
	value = strings.ReplaceAll(value, "'", " ")

	raw := strings.Fields(value)
	out := make([]string, 0, len(raw))

	for _, item := range raw {
		item = strings.TrimSpace(item)
		item = strings.Trim(item, ",")
		item = strings.Trim(item, ")")
		item = strings.Trim(item, "(")

		if item == "" {
			continue
		}

		if strings.HasPrefix(item, "${") || strings.Contains(item, "${") {
			continue
		}

		out = append(out, item)
	}

	return out
}

func cleanLayerRelationValues(values []string) []string {
	out := make([]string, 0, len(values))

	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}

		if strings.Contains(value, ":") {
			value = strings.Split(value, ":")[0]
		}

		out = append(out, value)
	}

	return out
}

func checkLayerDependencyCycles(nodes []layerNode) []model.Finding {
	graph := map[string][]string{}
	nodeByCollection := map[string]layerNode{}

	for _, node := range nodes {
		for _, collection := range node.Collections {
			nodeByCollection[collection] = node
			graph[collection] = append(graph[collection], node.Depends...)
		}
	}

	var findings []model.Finding
	visited := map[string]bool{}
	stack := map[string]bool{}

	var visit func(string, []string)
	visit = func(name string, path []string) {
		if stack[name] {
			cycle := append(path, name)
			node, ok := nodeByCollection[name]
			if !ok {
				return
			}

			findings = append(findings, finding(
				"static/layer-dependency-cycle",
				"Layer dependency cycle",
				model.SeverityHigh,
				node.Layer.Name,
				node.ConfigPath,
				findLine(nodeLines(node.ConfigPath), "LAYERDEPENDS"),
				fmt.Sprintf("Layer dependency cycle detected: %s.", strings.Join(cycle, " -> ")),
				"Circular layer dependencies are difficult to reason about and can cause fragile integration behavior.",
				"Break the dependency cycle by moving shared metadata to a common lower-level layer.",
			))
			return
		}

		if visited[name] {
			return
		}

		visited[name] = true
		stack[name] = true

		for _, dep := range graph[name] {
			if _, ok := graph[dep]; !ok {
				continue
			}
			visit(dep, append(path, name))
		}

		stack[name] = false
	}

	names := make([]string, 0, len(graph))
	for name := range graph {
		names = append(names, name)
	}
	sort.Strings(names)

	for _, name := range names {
		visit(name, nil)
	}

	return findings
}

func nodeLines(path string) []string {
	lines, _, err := parseMetadataFile(path)
	if err != nil {
		return nil
	}
	return lines
}

func checkRecipeStatic(recipe model.Recipe) []model.Finding {
	var findings []model.Finding

	if srcRev, ok := recipe.Variables["SRCREV"]; ok && (strings.Contains(srcRev, "AUTOREV") || isFloatingRevision(srcRev)) {
		findings = append(findings, finding(
			"static/floating-srcrev",
			"Floating SRCREV",
			model.SeverityHigh,
			recipe.Layer,
			recipe.Path,
			findLine(recipe.Lines, "SRCREV"),
			"Recipe uses AUTOREV or a floating source revision.",
			"Floating source revisions break reproducible builds because the same metadata can fetch different source code later.",
			"Pin SRCREV to a fixed commit hash.",
		))
	}

	if srcURI, ok := recipe.Variables["SRC_URI"]; ok && strings.Contains(srcURI, "http://") {
		findings = append(findings, finding(
			"static/insecure-src-uri",
			"Insecure SRC_URI",
			model.SeverityMedium,
			recipe.Layer,
			recipe.Path,
			findLine(recipe.Lines, "SRC_URI"),
			"SRC_URI contains an http:// URL.",
			"Insecure fetch URLs can be intercepted or modified and may fail security review.",
			"Use https:// where supported.",
		))
	}

	if srcURI, ok := recipe.Variables["SRC_URI"]; ok && containsBranchFloatingToken(srcURI) {
		findings = append(findings, finding(
			"static/floating-source-branch",
			"Floating source branch",
			model.SeverityMedium,
			recipe.Layer,
			recipe.Path,
			findLine(recipe.Lines, "SRC_URI"),
			"SRC_URI points at a moving branch or HEAD-like source.",
			"Moving branches make rebuilds non-reproducible and can pull unreviewed code.",
			"Pin a fixed SRCREV and avoid branch names such as master/main without an immutable revision.",
		))
	}

	return findings
}

func checkRecipeStyle(recipe model.Recipe) []model.Finding {
	var findings []model.Finding

	if !recipeNamePattern.MatchString(recipe.Name) {
		findings = append(findings, finding(
			"style/recipe-name",
			"Recipe name style",
			model.SeverityLow,
			recipe.Layer,
			recipe.Path,
			1,
			"Recipe filename does not follow common lower-case Yocto naming style.",
			"Consistent recipe names make layer review and maintenance easier.",
			"Use lower-case package names and a _version suffix when the recipe is versioned.",
		))
	}

	if strings.Contains(recipe.Name, "_") && recipe.PV == "" {
		findings = append(findings, finding(
			"style/recipe-version",
			"Recipe version not clear",
			model.SeverityLow,
			recipe.Layer,
			recipe.Path,
			1,
			"Recipe filename contains an underscore but no version could be parsed.",
			"Clear recipe versioning makes upgrades and bbappend matching easier to review.",
			"Use name_version.bb, or set PV explicitly when using a special version scheme.",
		))
	}

	if !hasVar(recipe.Variables, "SUMMARY") {
		findings = append(findings, finding(
			"style/missing-summary",
			"Missing SUMMARY",
			model.SeverityLow,
			recipe.Layer,
			recipe.Path,
			1,
			"Recipe does not define SUMMARY.",
			"Good recipe metadata improves maintainability, package review, and generated package information.",
			"Add a short SUMMARY describing the package.",
		))
	}

	if !hasVar(recipe.Variables, "DESCRIPTION") {
		findings = append(findings, finding(
			"style/missing-description",
			"Missing DESCRIPTION",
			model.SeverityInfo,
			recipe.Layer,
			recipe.Path,
			1,
			"Recipe does not define DESCRIPTION.",
			"DESCRIPTION helps reviewers understand what the recipe provides beyond the short summary.",
			"Add DESCRIPTION when the package purpose is not obvious from SUMMARY.",
		))
	}

	if !hasVar(recipe.Variables, "LICENSE") {
		findings = append(findings, finding(
			"style/missing-license",
			"Missing LICENSE",
			model.SeverityMedium,
			recipe.Layer,
			recipe.Path,
			1,
			"Recipe does not define LICENSE.",
			"Clear license metadata is required for review hygiene and generated license manifests.",
			"Add a valid LICENSE value, for example LICENSE = \"MIT\".",
		))
	}

	if !hasVar(recipe.Variables, "LIC_FILES_CHKSUM") {
		findings = append(findings, finding(
			"style/missing-lic-files-chksum",
			"Missing LIC_FILES_CHKSUM",
			model.SeverityMedium,
			recipe.Layer,
			recipe.Path,
			1,
			"Recipe does not define LIC_FILES_CHKSUM.",
			"Yocto uses license file checksums to detect upstream license changes during builds.",
			"Add LIC_FILES_CHKSUM pointing to the upstream license file.",
		))
	}

	if hasVar(recipe.Variables, "LICENSE") && strings.EqualFold(strings.TrimSpace(recipe.Variables["LICENSE"]), "CLOSED") {
		findings = append(findings, finding(
			"style/closed-license",
			"CLOSED license needs review",
			model.SeverityLow,
			recipe.Layer,
			recipe.Path,
			findLine(recipe.Lines, "LICENSE"),
			"Recipe uses LICENSE = \"CLOSED\".",
			"CLOSED may be valid for proprietary components, but it should be intentional and documented.",
			"Confirm this component is proprietary and add comments explaining the choice if needed.",
		))
	}

	findings = append(findings, checkVariableOrder(recipe.Path, recipe.Layer, recipe.Lines)...)

	return findings
}

func checkAppendStatic(appendFile model.Append, recipes []model.Recipe) []model.Finding {
	var findings []model.Finding

	if !appendMatchesAnyRecipe(appendFile, recipes) {
		findings = append(findings, finding(
			"static/orphan-bbappend",
			"Possibly orphaned bbappend",
			model.SeverityLow,
			appendFile.Layer,
			appendFile.Path,
			1,
			"This .bbappend target recipe was not found in the currently scanned layers.",
			"This may be valid if the target recipe exists in another layer that was not included in the scan.",
			"Scan the provider layer as well, or verify that the target recipe exists in your build configuration.",
		))
	}

	if modifiesRiskyVariables(appendFile.Lines) {
		findings = append(findings, finding(
			"static/bbappend-modifies-source-or-install",
			"bbappend modifies source/install behavior",
			model.SeverityMedium,
			appendFile.Layer,
			appendFile.Path,
			1,
			"This .bbappend changes source, install, systemd, or file search behavior.",
			"Appends that alter SRC_URI, FILESEXTRAPATHS, do_install, or systemd units can significantly change product behavior.",
			"Review the append carefully and document why the modification is required.",
		))
	}

	return findings
}

func checkAppendStyle(appendFile model.Append) []model.Finding {
	var findings []model.Finding

	if strings.Contains(appendFile.Target, "%") {
		findings = append(findings, finding(
			"style/wildcard-bbappend",
			"Wildcard bbappend",
			model.SeverityLow,
			appendFile.Layer,
			appendFile.Path,
			1,
			"This .bbappend uses a wildcard target.",
			"Wildcard appends can be correct, but they can silently affect newer recipe versions.",
			"Confirm the wildcard is intentional and document why it is safe.",
		))
	}

	if !strings.Contains(appendFile.Target, "_") && !strings.Contains(appendFile.Target, "%") {
		findings = append(findings, finding(
			"style/broad-bbappend",
			"Broad bbappend target",
			model.SeverityLow,
			appendFile.Layer,
			appendFile.Path,
			1,
			"This .bbappend targets a recipe without a version suffix.",
			"Broad appends can unexpectedly apply to future versions of a recipe.",
			"Use a versioned .bbappend when the change is version-specific, or document why it is intentionally broad.",
		))
	}

	return findings
}

func checkLinesStatic(path string, layer string, lines []string) []model.Finding {
	var findings []model.Finding

	secretPatterns := []string{
		"password",
		"passwd",
		"token",
		"apikey",
		"api_key",
		"secret",
		"private_key",
		"begin rsa private key",
		"begin openssh private key",
	}

	hostPaths := []string{
		"/home/",
		"/users/",
		"/mnt/c/",
		"/tmp/",
		"/var/tmp/",
	}

	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		lower := strings.ToLower(line)

		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}

		if strings.Contains(line, "AUTOREV") {
			findings = append(findings, finding(
				"static/autorev-used",
				"AUTOREV used",
				model.SeverityHigh,
				layer,
				path,
				i+1,
				"Metadata uses AUTOREV.",
				"AUTOREV makes builds non-reproducible and can pull unreviewed upstream changes.",
				"Pin revisions to a commit hash.",
			))
		}

		for _, p := range hostPaths {
			if strings.Contains(lower, p) {
				findings = append(findings, finding(
					"static/host-absolute-path",
					"Host-specific absolute path",
					model.SeverityMedium,
					layer,
					path,
					i+1,
					"Metadata contains a host-specific absolute path.",
					"Host paths break reproducibility and fail on other developers' machines or CI workers.",
					"Use ${WORKDIR}, ${THISDIR}, ${S}, ${B}, ${D}, or FILESEXTRAPATHS.",
				))
				break
			}
		}

		for _, pattern := range secretPatterns {
			if strings.Contains(lower, pattern) && strings.Contains(line, "=") {
				findings = append(findings, finding(
					"static/possible-hardcoded-secret",
					"Possible hardcoded secret",
					model.SeverityHigh,
					layer,
					path,
					i+1,
					"This line appears to contain a password, token, key, or secret.",
					"Secrets in Yocto metadata can leak into source control, build logs, images, or deployed devices.",
					"Move secrets to CI secrets, runtime provisioning, secure storage, or device-specific provisioning.",
				))
				break
			}
		}
	}

	return findings
}

func checkLinesStyle(path string, layer string, lines []string) []model.Finding {
	var findings []model.Finding

	for i, line := range lines {
		trimmed := strings.TrimSpace(line)

		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}

		if strings.HasSuffix(line, " ") || strings.HasSuffix(line, "\t") {
			findings = append(findings, finding(
				"style/trailing-whitespace",
				"Trailing whitespace",
				model.SeverityInfo,
				layer,
				path,
				i+1,
				"Line contains trailing whitespace.",
				"Trailing whitespace creates noisy diffs and review churn.",
				"Remove trailing whitespace.",
			))
		}

		if strings.Contains(line, "_append") || strings.Contains(line, "_prepend") || strings.Contains(line, "_remove") {
			findings = append(findings, finding(
				"style/old-override-syntax",
				"Old override syntax",
				model.SeverityMedium,
				layer,
				path,
				i+1,
				"Metadata appears to use old underscore override syntax.",
				"Modern Yocto releases use colon override syntax. Old syntax can break or behave differently depending on release.",
				"Use VAR:append, VAR:prepend, or VAR:remove syntax.",
			))
		}

		if len(line) > 120 {
			findings = append(findings, finding(
				"style/line-length",
				"Long metadata line",
				model.SeverityInfo,
				layer,
				path,
				i+1,
				"Line is longer than 120 characters.",
				"Very long metadata lines are harder to review and maintain.",
				"Split long values across multiple lines with continuation where appropriate.",
			))
		}

		if matches := looseAssignmentPattern.FindStringSubmatch(trimmed); len(matches) == 4 {
			varName := matches[1]
			op := matches[2]
			rawValue := strings.TrimSpace(matches[3])

			if !variableNamePattern.MatchString(varName) {
				findings = append(findings, finding(
					"style/variable-name",
					"Variable name style",
					model.SeverityLow,
					layer,
					path,
					i+1,
					"Variable name is not in the usual upper-case BitBake style.",
					"Consistent variable naming makes metadata easier to scan and review.",
					"Use upper-case variable names unless this is a valid function or task name.",
				))
			}

			want := varName + " " + op + " "
			if !strings.HasPrefix(trimmed, want) {
				findings = append(findings, finding(
					"style/assignment-spacing",
					"Assignment spacing",
					model.SeverityInfo,
					layer,
					path,
					i+1,
					"Assignment spacing is not normalized.",
					"Consistent spacing makes metadata easier to read and review.",
					"Use the form VAR "+op+" \"value\".",
				))
			}

			if rawValue != "" &&
				!strings.HasPrefix(rawValue, "\"") &&
				!strings.HasPrefix(rawValue, "'") &&
				!strings.HasPrefix(rawValue, "${") {
				findings = append(findings, finding(
					"style/unquoted-assignment",
					"Unquoted assignment",
					model.SeverityInfo,
					layer,
					path,
					i+1,
					"Assignment value is not quoted.",
					"Quoted assignment values are easier to parse consistently and avoid accidental whitespace issues.",
					"Use quotes for metadata values unless BitBake syntax specifically requires otherwise.",
				))
			}
		}
	}

	return findings
}

func checkPatchStatic(patch model.Patch) []model.Finding {
	var findings []model.Finding

	content := strings.Join(patch.Lines, "\n")
	lower := strings.ToLower(content)
	upperName := strings.ToUpper(filepath.Base(patch.Path))

	if strings.Contains(upperName, "CVE") && !strings.Contains(content, "CVE-") {
		findings = append(findings, finding(
			"static/patch-cve-metadata-missing",
			"CVE patch missing CVE metadata",
			model.SeverityMedium,
			patch.Layer,
			patch.Path,
			1,
			"Patch filename suggests CVE content but patch body does not contain a CVE identifier.",
			"Security patches should be traceable for audit, maintenance, and vulnerability management.",
			"Add the CVE identifier and security context in the patch header.",
		))
	}

	if strings.Contains(upperName, "CVE") && !strings.Contains(lower, "upstream-status:") {
		findings = append(findings, finding(
			"static/patch-cve-missing-upstream-status",
			"CVE patch missing Upstream-Status",
			model.SeverityHigh,
			patch.Layer,
			patch.Path,
			1,
			"CVE-related patch does not declare Upstream-Status.",
			"Security patches should clearly show whether they are backports, submitted upstream, pending, or inappropriate for upstream.",
			"Add an Upstream-Status header such as Backport, Submitted, Pending, or Inappropriate with explanation.",
		))
	}

	if strings.Contains(lower, "password") ||
		strings.Contains(lower, "secret") ||
		strings.Contains(lower, "private key") ||
		strings.Contains(lower, "token") {
		findings = append(findings, finding(
			"static/patch-possible-secret",
			"Patch may contain secret material",
			model.SeverityHigh,
			patch.Layer,
			patch.Path,
			1,
			"Patch content contains words commonly associated with secrets.",
			"Secrets in patches can leak into source control, build logs, images, or deployed devices.",
			"Review the patch and remove credentials, private keys, tokens, or test secrets.",
		))
	}

	return findings
}

func hasPatchSubject(lines []string) bool {
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)

		if strings.HasPrefix(trimmed, "Subject:") {
			return true
		}

		if strings.HasPrefix(trimmed, "[PATCH") {
			return true
		}
	}

	return false
}

func containsDiffMarkers(lines []string) bool {
	for _, line := range lines {
		if strings.HasPrefix(line, "diff --git ") ||
			strings.HasPrefix(line, "--- ") ||
			strings.HasPrefix(line, "+++ ") ||
			strings.HasPrefix(line, "@@ ") {
			return true
		}
	}

	return false
}

func checkPatchStyle(patch model.Patch) []model.Finding {
	var findings []model.Finding

	content := strings.Join(patch.Lines, "\n")
	lower := strings.ToLower(content)

	if !strings.Contains(lower, "upstream-status:") {
		findings = append(findings, finding(
			"style/patch-missing-upstream-status",
			"Patch missing Upstream-Status",
			model.SeverityMedium,
			patch.Layer,
			patch.Path,
			1,
			"Patch does not contain Upstream-Status.",
			"Yocto patch review expects clear upstreaming state so patches do not become unmaintained technical debt.",
			"Add Upstream-Status with a value such as Pending, Submitted, Backport, Inappropriate, or Denied.",
		))
	}

	if !strings.Contains(lower, "signed-off-by:") {
		findings = append(findings, finding(
			"style/patch-missing-signed-off-by",
			"Patch missing Signed-off-by",
			model.SeverityLow,
			patch.Layer,
			patch.Path,
			1,
			"Patch does not contain a Signed-off-by trailer.",
			"Signed-off-by improves patch provenance and review hygiene.",
			"Add a Signed-off-by line to the patch trailer when appropriate for your project.",
		))
	}

	if !strings.Contains(lower, "subject:") && !hasPatchSubject(patch.Lines) {
		findings = append(findings, finding(
			"style/patch-missing-subject",
			"Patch missing subject",
			model.SeverityLow,
			patch.Layer,
			patch.Path,
			1,
			"Patch does not appear to contain a clear subject header.",
			"A clear patch subject makes review, rebasing, and upstream submission easier.",
			"Add a concise patch subject explaining what the patch changes.",
		))
	}

	if !strings.Contains(lower, "from ") && !strings.Contains(lower, "author:") {
		findings = append(findings, finding(
			"style/patch-missing-author",
			"Patch missing author information",
			model.SeverityLow,
			patch.Layer,
			patch.Path,
			1,
			"Patch does not appear to contain author metadata.",
			"Author metadata helps track patch provenance and ownership.",
			"Generate patches with git format-patch or add author metadata.",
		))
	}

	if !containsDiffMarkers(patch.Lines) {
		findings = append(findings, finding(
			"style/patch-no-diff-markers",
			"Patch has no diff markers",
			model.SeverityMedium,
			patch.Layer,
			patch.Path,
			1,
			"Patch does not contain recognizable diff markers.",
			"Malformed patches may fail during do_patch or silently become unusable.",
			"Regenerate the patch with git format-patch or diff -u.",
		))
	}

	return findings
}

func checkVariableOrder(path string, layer string, lines []string) []model.Finding {
	order := []string{
		"SUMMARY",
		"DESCRIPTION",
		"HOMEPAGE",
		"LICENSE",
		"LIC_FILES_CHKSUM",
		"SRC_URI",
		"SRCREV",
		"S",
		"DEPENDS",
		"RDEPENDS",
	}

	position := map[string]int{}
	for i, key := range order {
		position[key] = i
	}

	lastPos := -1
	lastVar := ""

	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}

		matches := looseAssignmentPattern.FindStringSubmatch(trimmed)
		if len(matches) != 4 {
			continue
		}

		key := normalizeVar(matches[1])
		pos, ok := position[key]
		if !ok {
			continue
		}

		if pos < lastPos {
			return []model.Finding{finding(
				"style/variable-order",
				"Variable order",
				model.SeverityLow,
				layer,
				path,
				i+1,
				fmt.Sprintf("%s appears after %s, which is not the usual recipe metadata order.", key, lastVar),
				"Consistent variable order makes recipes easier to review and compare.",
				"Prefer ordering common metadata as SUMMARY, DESCRIPTION, HOMEPAGE, LICENSE, LIC_FILES_CHKSUM, SRC_URI, SRCREV, S, DEPENDS, RDEPENDS.",
			)}
		}

		lastPos = pos
		lastVar = key
	}

	return nil
}

func checkDuplicateRecipes(recipes []model.Recipe) []model.Finding {
	var findings []model.Finding

	byName := map[string][]model.Recipe{}
	for _, recipe := range recipes {
		byName[recipe.Name] = append(byName[recipe.Name], recipe)
	}

	for name, matches := range byName {
		if len(matches) < 2 {
			continue
		}

		for _, recipe := range matches {
			findings = append(findings, finding(
				"static/duplicate-recipe",
				"Duplicate recipe name",
				model.SeverityMedium,
				recipe.Layer,
				recipe.Path,
				1,
				fmt.Sprintf("Recipe %s appears in multiple scanned layers.", name),
				"Duplicate recipe names can cause priority-dependent behavior and unexpected provider selection.",
				"Check layer priorities, recipe versions, and whether the duplicate is intentional.",
			))
		}
	}

	return findings
}

func hasLayerSeriesCompat(vars map[string]string) bool {
	for key, value := range vars {
		if strings.TrimSpace(value) == "" {
			continue
		}

		normalized := strings.ReplaceAll(key, ":", "_")

		if normalized == "LAYERSERIES_COMPAT" ||
			strings.HasPrefix(normalized, "LAYERSERIES_COMPAT_") {
			return true
		}
	}

	return false
}

func hasVar(vars map[string]string, key string) bool {
	v, ok := vars[key]
	return ok && strings.TrimSpace(v) != ""
}

func appendMatchesAnyRecipe(appendFile model.Append, recipes []model.Recipe) bool {
	for _, recipe := range recipes {
		if appendMatchesRecipe(appendFile.Target, recipe.Name) {
			return true
		}
	}
	return false
}

func appendMatchesRecipe(appendTarget string, recipeName string) bool {
	if appendTarget == recipeName {
		return true
	}

	if strings.Contains(appendTarget, "%") {
		prefix := strings.Split(appendTarget, "%")[0]
		return strings.HasPrefix(recipeName, prefix)
	}

	appendPN := recipePNFromName(appendTarget)
	recipePN := recipePNFromName(recipeName)

	return appendPN == recipePN
}

func recipePNFromName(name string) string {
	if idx := strings.Index(name, "_"); idx >= 0 {
		return name[:idx]
	}
	return name
}

func modifiesRiskyVariables(lines []string) bool {
	risky := []string{
		"SRC_URI",
		"FILESEXTRAPATHS",
		"do_install",
		"do_configure",
		"do_compile",
		"SYSTEMD_SERVICE",
		"FILES:",
		"RDEPENDS",
		"DEPENDS",
	}

	for _, line := range lines {
		for _, key := range risky {
			if strings.Contains(line, key) {
				return true
			}
		}
	}

	return false
}

func isFloatingRevision(value string) bool {
	v := strings.TrimSpace(strings.ToLower(value))
	if v == "" {
		return false
	}

	if strings.Contains(v, "${autorev}") || strings.Contains(v, "autorev") {
		return true
	}

	if v == "head" || v == "master" || v == "main" {
		return true
	}

	return false
}

func containsBranchFloatingToken(value string) bool {
	v := strings.ToLower(value)

	return strings.Contains(v, "branch=master") ||
		strings.Contains(v, "branch=main") ||
		strings.Contains(v, "rev=head") ||
		strings.Contains(v, "tag=latest")
}

func findLine(lines []string, needle string) int {
	for i, line := range lines {
		if strings.Contains(line, needle) {
			return i + 1
		}
	}
	return 1
}

func finding(ruleID string, title string, severity model.Severity, layer string, file string, line int, message string, why string, remediation string) model.Finding {
	return model.Finding{
		RuleID:       ruleID,
		Title:        title,
		Severity:     severity,
		Layer:        layer,
		File:         file,
		Line:         line,
		Message:      message,
		WhyItMatters: why,
		Remediation:  remediation,
	}
}

func severityRank(sev model.Severity) int {
	switch sev {
	case model.SeverityCritical:
		return 5
	case model.SeverityHigh:
		return 4
	case model.SeverityMedium:
		return 3
	case model.SeverityLow:
		return 2
	default:
		return 1
	}
}

func uniqueStrings(values []string) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(values))

	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}

		if seen[value] {
			continue
		}

		seen[value] = true
		out = append(out, value)
	}

	sort.Strings(out)
	return out
}

func emit(progress ProgressFunc, p model.ScanProgress) {
	if progress != nil {
		progress(p)
	}
}

func checkLicenseCompliance(recipe model.Recipe) []model.Finding {
	var findings []model.Finding

	license := strings.TrimSpace(recipe.Variables["LICENSE"])
	licChecksum := strings.TrimSpace(recipe.Variables["LIC_FILES_CHKSUM"])

	if license == "" {
		findings = append(findings, finding(
			"static/license-missing",
			"Missing LICENSE",
			model.SeverityHigh,
			recipe.Layer,
			recipe.Path,
			1,
			"Recipe does not define LICENSE.",
			"Missing license metadata blocks reliable license manifest generation and compliance review.",
			"Add a valid LICENSE value, for example LICENSE = \"MIT\".",
		))
		return findings
	}

	normalized := strings.ToUpper(license)

	if normalized == "CLOSED" {
		findings = append(findings, finding(
			"static/license-closed",
			"CLOSED license requires review",
			model.SeverityMedium,
			recipe.Layer,
			recipe.Path,
			findLine(recipe.Lines, "LICENSE"),
			"Recipe uses LICENSE = \"CLOSED\".",
			"CLOSED may be valid for proprietary software, but it should be explicitly reviewed and documented.",
			"Confirm the component is proprietary and document why CLOSED is required.",
		))
	}

	if strings.Contains(normalized, "GPL-3") || strings.Contains(normalized, "GPLV3") || strings.Contains(normalized, "AGPL") {
		findings = append(findings, finding(
			"static/license-gplv3-family",
			"GPLv3-family license detected",
			model.SeverityMedium,
			recipe.Layer,
			recipe.Path,
			findLine(recipe.Lines, "LICENSE"),
			"Recipe uses a GPLv3-family license.",
			"GPLv3-family packages may require additional legal and product policy review in some embedded products.",
			"Confirm GPLv3-family usage is allowed for this product and image.",
		))
	}

	if strings.Contains(normalized, "UNKNOWN") || strings.Contains(normalized, "TODO") || strings.Contains(normalized, "TBD") {
		findings = append(findings, finding(
			"static/license-placeholder",
			"Placeholder license value",
			model.SeverityHigh,
			recipe.Layer,
			recipe.Path,
			findLine(recipe.Lines, "LICENSE"),
			"Recipe appears to use a placeholder license value.",
			"Placeholder license metadata is not suitable for release or compliance review.",
			"Replace the placeholder with the correct upstream license.",
		))
	}

	if license != "CLOSED" && licChecksum == "" {
		findings = append(findings, finding(
			"static/license-missing-lic-files-chksum",
			"Missing LIC_FILES_CHKSUM",
			model.SeverityHigh,
			recipe.Layer,
			recipe.Path,
			1,
			"Recipe defines LICENSE but does not define LIC_FILES_CHKSUM.",
			"Yocto uses LIC_FILES_CHKSUM to detect upstream license text changes during builds.",
			"Add LIC_FILES_CHKSUM pointing to the upstream license file.",
		))
	}

	if licChecksum != "" {
		findings = append(findings, checkLicenseChecksumReferences(recipe, licChecksum)...)
	}

	return findings
}

func checkLicenseChecksumReferences(recipe model.Recipe, licChecksum string) []model.Finding {
	var findings []model.Finding

	entries := strings.Fields(licChecksum)
	for _, entry := range entries {
		entry = strings.TrimSpace(entry)
		if entry == "" {
			continue
		}

		if !strings.HasPrefix(entry, "file://") {
			findings = append(findings, finding(
				"static/license-invalid-reference",
				"Invalid LIC_FILES_CHKSUM reference",
				model.SeverityLow,
				recipe.Layer,
				recipe.Path,
				findLine(recipe.Lines, "LIC_FILES_CHKSUM"),
				fmt.Sprintf("LIC_FILES_CHKSUM entry %q does not use a file:// reference.", entry),
				"Yocto license checksum entries normally reference files with file:// paths and checksum parameters.",
				"Use LIC_FILES_CHKSUM entries such as file://COPYING;md5=<checksum>.",
			))
			continue
		}

		pathPart := strings.TrimPrefix(entry, "file://")

		if strings.Contains(pathPart, "${COMMON_LICENSE_DIR}") ||
			strings.Contains(pathPart, "${COREBASE}") ||
			strings.Contains(pathPart, "${S}") ||
			strings.Contains(pathPart, "${WORKDIR}") {
			continue
		}

		if !strings.Contains(entry, ";md5=") &&
			!strings.Contains(entry, ";sha256=") {
			findings = append(findings, finding(
				"static/license-missing-checksum",
				"LIC_FILES_CHKSUM entry missing checksum",
				model.SeverityMedium,
				recipe.Layer,
				recipe.Path,
				findLine(recipe.Lines, "LIC_FILES_CHKSUM"),
				fmt.Sprintf("LIC_FILES_CHKSUM entry %q does not include md5 or sha256.", entry),
				"Yocto uses the checksum to detect upstream license text changes during builds.",
				"Add a checksum parameter, for example file://COPYING;md5=<checksum>.",
			))
		}

		if idx := strings.Index(pathPart, ";"); idx >= 0 {
			pathPart = pathPart[:idx]
		}

		if strings.TrimSpace(pathPart) == "" {
			findings = append(findings, finding(
				"static/license-empty-file-reference",
				"Empty LIC_FILES_CHKSUM file reference",
				model.SeverityMedium,
				recipe.Layer,
				recipe.Path,
				findLine(recipe.Lines, "LIC_FILES_CHKSUM"),
				"LIC_FILES_CHKSUM contains an empty file:// reference.",
				"Empty license file references cannot be validated by BitBake.",
				"Point LIC_FILES_CHKSUM to a real license file, such as file://COPYING;md5=<checksum>.",
			))
		}
	}

	return findings
}
