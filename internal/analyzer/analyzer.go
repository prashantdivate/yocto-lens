package analyzer

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/example/yocto-lens/internal/model"
)

type ProgressFunc func(model.ScanProgress)

var assignmentPattern = regexp.MustCompile(`^([A-Za-z0-9_:+${}./-]+)\s*(\?=|\+=|=|:=|\.=|=\+)\s*"(.*)"`)

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

			filesProcessed++

			if filesProcessed%50 == 0 {
				emit(progress, model.ScanProgress{
					Phase:          model.PhaseParsing,
					CurrentPath:    path,
					LayersFound:    len(report.Layers),
					RecipesFound:   len(report.Recipes),
					AppendsFound:   len(report.Appends),
					PatchesFound:   len(report.Patches),
					FilesProcessed: filesProcessed,
					FindingsFound:  len(report.Findings),
				})
			}

			switch {
			case strings.HasSuffix(path, ".bb"):
				recipe, parseErr := parseRecipe(path, layer)
				if parseErr == nil {
					report.Recipes = append(report.Recipes, recipe)
				}

			case strings.HasSuffix(path, ".bbappend"):
				appendFile, parseErr := parseAppend(path, layer)
				if parseErr == nil {
					report.Appends = append(report.Appends, appendFile)
				}

			case strings.HasSuffix(path, ".patch"):
				patch, parseErr := parsePatch(path, layer)
				if parseErr == nil {
					report.Patches = append(report.Patches, patch)
				}
			}

			return nil
		})

		if err != nil {
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

	sort.Slice(report.Findings, func(i, j int) bool {
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
			if walkErr != nil {
				return nil
			}

			if !d.IsDir() {
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
	lines, vars, err := parseMetadataFile(path)
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
	lines, vars, err := parseMetadataFile(path)
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
			vars[key] = matches[3]
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

	for _, recipe := range report.Recipes {
		findings = append(findings, checkRecipeBasics(recipe)...)
		findings = append(findings, checkLines(recipe.Path, recipe.Layer, recipe.Lines)...)
	}

	for _, appendFile := range report.Appends {
		findings = append(findings, checkAppendBasics(appendFile, report.Recipes)...)
		findings = append(findings, checkLines(appendFile.Path, appendFile.Layer, appendFile.Lines)...)
	}

	for _, patch := range report.Patches {
		findings = append(findings, checkPatchMetadata(patch)...)
	}

	findings = append(findings, checkDuplicateRecipes(report.Recipes)...)

	return findings
}

func checkRecipeBasics(recipe model.Recipe) []model.Finding {
	var findings []model.Finding

	if !hasVar(recipe.Variables, "LICENSE") {
		findings = append(findings, finding(
			"missing-license",
			"Missing LICENSE",
			model.SeverityHigh,
			recipe.Layer,
			recipe.Path,
			1,
			"Recipe does not define LICENSE.",
			"Yocto recipes must clearly declare licensing so generated images and license manifests are auditable.",
			"Add a valid LICENSE value, for example LICENSE = \"MIT\".",
		))
	}

	if !hasVar(recipe.Variables, "LIC_FILES_CHKSUM") {
		findings = append(findings, finding(
			"missing-lic-files-chksum",
			"Missing LIC_FILES_CHKSUM",
			model.SeverityHigh,
			recipe.Layer,
			recipe.Path,
			1,
			"Recipe does not define LIC_FILES_CHKSUM.",
			"Yocto uses license file checksums to detect upstream license changes during builds.",
			"Add LIC_FILES_CHKSUM pointing to the upstream license file.",
		))
	}

	if !hasVar(recipe.Variables, "SUMMARY") {
		findings = append(findings, finding(
			"missing-summary",
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

	if srcRev, ok := recipe.Variables["SRCREV"]; ok && strings.Contains(srcRev, "AUTOREV") {
		findings = append(findings, finding(
			"floating-srcrev",
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
			"insecure-src-uri",
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

	return findings
}

func checkAppendBasics(appendFile model.Append, recipes []model.Recipe) []model.Finding {
	var findings []model.Finding

	if strings.Contains(appendFile.Target, "%") {
		findings = append(findings, finding(
			"wildcard-bbappend",
			"Wildcard bbappend",
			model.SeverityLow,
			appendFile.Layer,
			appendFile.Path,
			1,
			"This .bbappend uses a wildcard target.",
			"Wildcard appends are sometimes correct, but they can silently affect newer recipe versions.",
			"Confirm the wildcard is intentional and documented.",
		))
	}

	if !appendMatchesAnyRecipe(appendFile, recipes) {
		findings = append(findings, finding(
			"orphan-bbappend",
			"Possibly orphaned bbappend",
			model.SeverityHigh,
			appendFile.Layer,
			appendFile.Path,
			1,
			"This .bbappend does not match any recipe discovered in the scanned layers.",
			"Orphan appends are ignored by BitBake or cause layer compatibility problems depending on configuration.",
			"Check recipe name/version, BBFILES patterns, layer dependencies, and whether the target recipe exists.",
		))
	}

	if modifiesRiskyVariables(appendFile.Lines) {
		findings = append(findings, finding(
			"bbappend-modifies-source-or-install",
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

func checkLines(path string, layer string, lines []string) []model.Finding {
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

		if strings.HasPrefix(trimmed, "#") {
			continue
		}

		if strings.Contains(line, "_append") || strings.Contains(line, "_prepend") || strings.Contains(line, "_remove") {
			findings = append(findings, finding(
				"old-override-syntax",
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

		if strings.Contains(line, "AUTOREV") {
			findings = append(findings, finding(
				"autorev-used",
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
					"host-absolute-path",
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
					"possible-hardcoded-secret",
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

func checkPatchMetadata(patch model.Patch) []model.Finding {
	var findings []model.Finding

	content := strings.Join(patch.Lines, "\n")

	if !strings.Contains(content, "Upstream-Status:") {
		findings = append(findings, finding(
			"patch-missing-upstream-status",
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

	upperName := strings.ToUpper(filepath.Base(patch.Path))
	if strings.Contains(upperName, "CVE") && !strings.Contains(content, "CVE-") {
		findings = append(findings, finding(
			"patch-cve-metadata-missing",
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

	return findings
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
				"duplicate-recipe",
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

func emit(progress ProgressFunc, p model.ScanProgress) {
	if progress != nil {
		progress(p)
	}
}
