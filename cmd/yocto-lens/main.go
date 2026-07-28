package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"golang.org/x/term"

	"github.com/example/yocto-lens/internal/analyzer"
	"github.com/example/yocto-lens/internal/model"
	"github.com/example/yocto-lens/internal/tui"
)

type scanProfile struct {
	started   time.Time
	lastAt    time.Time
	lastPhase model.ScanPhase
	durations map[model.ScanPhase]time.Duration
	progress  model.ScanProgress
}

func main() {
	jsonPath := flag.String("json", "", "write JSON report to file")
	sarifPath := flag.String("sarif", "", "write SARIF report to file")
	markdownPath := flag.String("markdown", "", "write Markdown report to file")
	noTUI := flag.Bool("no-tui", false, "disable TUI and print console output")
	profile := flag.Bool("profile", false, "print scan phase timing profile")
	failOn := flag.String("fail-on", "", "exit non-zero when findings are at or above severity: critical, high, medium, low, info")
	flag.Parse()

	paths := flag.Args()
	if len(paths) == 0 {
		paths = []string{"."}
	}

	failSeverity, failOnEnabled, err := parseFailOnSeverity(*failOn)
	if err != nil {
		fmt.Fprintf(os.Stderr, "invalid --fail-on value: %v\n", err)
		os.Exit(2)
	}

	if *noTUI || *profile || failOnEnabled {
		scanProfile := newScanProfile()
		if *profile && !*noTUI {
			fmt.Println("Scanning with profiling enabled...")
		}

		report, err := analyzer.AnalyzeWithProgress(paths, func(p model.ScanProgress) {
			scanProfile.Observe(p)
			if *noTUI {
				fmt.Printf(
					"\r[%s] layers=%d recipes=%d bbappends=%d patches=%d files=%d findings=%d %s",
					p.Phase,
					p.LayersFound,
					p.RecipesFound,
					p.AppendsFound,
					p.PatchesFound,
					p.FilesProcessed,
					p.FindingsFound,
					compactConsolePath(p.CurrentPath, 70),
				)
			}
		})
		scanProfile.Finish()
		if *noTUI {
			fmt.Println()
		}

		if err != nil {
			fmt.Fprintf(os.Stderr, "analysis failed: %v\n", err)
			os.Exit(1)
		}

		if err := writeOutputs(report, *jsonPath, *sarifPath, *markdownPath); err != nil {
			fmt.Fprintf(os.Stderr, "export failed: %v\n", err)
			os.Exit(1)
		}

		printSummary(report)
		if *profile {
			printScanProfile(scanProfile)
		}
		if failOnEnabled && reportHasSeverityAtLeast(report, failSeverity) {
			fmt.Fprintf(os.Stderr, "quality gate failed: findings at or above %s were found\n", failSeverity)
			os.Exit(1)
		}
		return
	}

	showSplash(paths)

	program := tea.NewProgram(
		tui.NewLoading(paths),
		tea.WithAltScreen(),
	)

	if _, err := program.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "tui failed: %v\n", err)
		os.Exit(1)
	}

	if *jsonPath != "" || *sarifPath != "" || *markdownPath != "" {
		report, err := analyzer.Analyze(paths)
		if err != nil {
			fmt.Fprintf(os.Stderr, "analysis failed during export: %v\n", err)
			os.Exit(1)
		}

		if err := writeOutputs(report, *jsonPath, *sarifPath, *markdownPath); err != nil {
			fmt.Fprintf(os.Stderr, "export failed: %v\n", err)
			os.Exit(1)
		}
	}
}

func newScanProfile() *scanProfile {
	now := time.Now()
	return &scanProfile{
		started:   now,
		lastAt:    now,
		durations: map[model.ScanPhase]time.Duration{},
	}
}

func (p *scanProfile) Observe(progress model.ScanProgress) {
	now := time.Now()
	if p.lastPhase == "" {
		p.lastPhase = progress.Phase
		p.lastAt = now
	} else if progress.Phase != p.lastPhase {
		p.durations[p.lastPhase] += now.Sub(p.lastAt)
		p.lastPhase = progress.Phase
		p.lastAt = now
	}

	p.progress = progress
	if progress.Done {
		p.Finish()
	}
}

func (p *scanProfile) Finish() {
	if p.lastPhase == "" {
		return
	}

	now := time.Now()
	p.durations[p.lastPhase] += now.Sub(p.lastAt)
	p.lastPhase = ""
	p.lastAt = now
}

func showSplash(paths []string) {
	width, height := terminalSize()

	gold := lipgloss.Color("#d8a657")
	green := lipgloss.Color("#a9b665")
	cream := lipgloss.Color("#d4be98")
	muted := lipgloss.Color("#928374")
	bg := lipgloss.Color("#1d2021")
	border := lipgloss.Color("#504945")

	titleStyle := lipgloss.NewStyle().
		Foreground(gold).
		Background(bg).
		Bold(true)

	subtitleStyle := lipgloss.NewStyle().
		Foreground(cream).
		Background(bg)

	accentStyle := lipgloss.NewStyle().
		Foreground(green).
		Background(bg).
		Bold(true)

	mutedStyle := lipgloss.NewStyle().
		Foreground(muted).
		Background(bg)

	boxWidth := responsiveSplashWidth(width)

	workspace := strings.Join(paths, ", ")
	if workspace == "" {
		workspace = "."
	}

	steps := []string{
		"Loading analyzers...",
		"Discovering Yocto layers...",
		"Preparing metadata scanner...",
		"Starting dashboard...",
	}

	for i, step := range steps {
		clearScreen()

		progress := (i + 1) * 100 / len(steps)
		barWidth := responsiveBarWidth(boxWidth)

		body := lipgloss.JoinVertical(
			lipgloss.Center,
			"",
			titleStyle.Render(">>> YOCTO LENS <<<"),
			"",
			subtitleStyle.Render("Static Analysis & Style Review"),
			subtitleStyle.Render("for Yocto/OpenEmbedded Metadata"),
			"",
			accentStyle.Render("Catch Issues Early • Build Better"),
			"",
			progressBar(progress, barWidth),
			"",
			mutedStyle.Render(step),
			mutedStyle.Render("Workspace: "+truncatePlain(workspace, boxWidth-18)),
			"",
		)

		box := lipgloss.NewStyle().
			Width(boxWidth).
			Align(lipgloss.Center).
			Background(bg).
			Foreground(cream).
			Border(lipgloss.RoundedBorder()).
			BorderForeground(border).
			Padding(1, 2).
			Render(body)

		screen := lipgloss.Place(
			width,
			height,
			lipgloss.Center,
			lipgloss.Center,
			box,
		)

		fmt.Print(screen)
		time.Sleep(420 * time.Millisecond)
	}

	time.Sleep(250 * time.Millisecond)
	clearScreen()
}

func responsiveSplashWidth(terminalWidth int) int {
	if terminalWidth <= 0 {
		return 64
	}

	maxWidth := 72
	minWidth := 44

	width := terminalWidth * 55 / 100

	if width > maxWidth {
		width = maxWidth
	}

	if width < minWidth {
		width = minWidth
	}

	if terminalWidth < width+4 {
		width = terminalWidth - 4
	}

	if width < 32 {
		width = 32
	}

	return width
}

func responsiveBarWidth(boxWidth int) int {
	barWidth := boxWidth - 24

	if barWidth > 40 {
		barWidth = 40
	}

	if barWidth < 18 {
		barWidth = 18
	}

	return barWidth
}

func progressBar(percent int, width int) string {
	if percent < 0 {
		percent = 0
	}
	if percent > 100 {
		percent = 100
	}

	filled := percent * width / 100
	empty := width - filled

	green := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#a9b665")).
		Render(strings.Repeat("█", filled))

	gray := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#3c3836")).
		Render(strings.Repeat("░", empty))

	text := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#d8a657")).
		Bold(true).
		Render(fmt.Sprintf(" %3d%%", percent))

	return "[" + green + gray + "]" + text
}

func terminalSize() (int, int) {
	width, height, err := term.GetSize(int(os.Stdout.Fd()))
	if err != nil || width <= 0 || height <= 0 {
		return 120, 36
	}

	return width, height
}

func clearScreen() {
	fmt.Print("\033[2J\033[H")
}

func writeOutputs(report model.Report, jsonPath string, sarifPath string, markdownPath string) error {
	if jsonPath != "" {
		if err := writeJSON(report, jsonPath); err != nil {
			return err
		}
	}

	if sarifPath != "" {
		if err := writeSARIF(report, sarifPath); err != nil {
			return err
		}
	}

	if markdownPath != "" {
		if err := writeMarkdown(report, markdownPath); err != nil {
			return err
		}
	}

	return nil
}

func writeJSON(report model.Report, path string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil && filepath.Dir(path) != "." {
		return err
	}

	data, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(path, data, 0644)
}

func writeMarkdown(report model.Report, path string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil && filepath.Dir(path) != "." {
		return err
	}

	var b strings.Builder
	high, medium, low := severityCounts(report)

	b.WriteString("# Yocto Lens Report\n\n")
	b.WriteString(fmt.Sprintf("- Root: `%s`\n", markdownEscape(report.Root)))
	if report.TargetRelease != "" {
		b.WriteString(fmt.Sprintf("- Target release: `%s`\n", markdownEscape(report.TargetRelease)))
	}
	b.WriteString(fmt.Sprintf("- Layers: %d\n", len(report.Layers)))
	b.WriteString(fmt.Sprintf("- Recipes: %d\n", len(report.Recipes)))
	b.WriteString(fmt.Sprintf("- bbappends: %d\n", len(report.Appends)))
	b.WriteString(fmt.Sprintf("- Patches: %d\n", len(report.Patches)))
	b.WriteString(fmt.Sprintf("- Metadata files: %d\n", len(report.MetadataFiles)))
	b.WriteString(fmt.Sprintf("- Findings: %d\n", len(report.Findings)))
	b.WriteString(fmt.Sprintf("- High: %d\n", high))
	b.WriteString(fmt.Sprintf("- Medium: %d\n", medium))
	b.WriteString(fmt.Sprintf("- Low/info: %d\n\n", low))

	if len(report.Findings) == 0 {
		b.WriteString("No findings.\n")
		return os.WriteFile(path, []byte(b.String()), 0644)
	}

	findings := sortedFindings(report.Findings)
	b.WriteString("## Findings\n\n")
	b.WriteString("| Severity | Rule | File | Line | Message |\n")
	b.WriteString("| --- | --- | --- | ---: | --- |\n")
	for _, finding := range findings {
		b.WriteString(fmt.Sprintf(
			"| %s | `%s` | `%s` | %d | %s |\n",
			finding.Severity,
			markdownEscape(finding.RuleID),
			markdownEscape(sarifURI(finding.File)),
			finding.Line,
			markdownEscape(finding.Message),
		))
	}

	return os.WriteFile(path, []byte(b.String()), 0644)
}

func writeSARIF(report model.Report, path string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil && filepath.Dir(path) != "." {
		return err
	}

	rulesByID := map[string]map[string]any{}
	results := []map[string]any{}

	for _, finding := range sortedFindings(report.Findings) {
		rulesByID[finding.RuleID] = map[string]any{
			"id":   finding.RuleID,
			"name": finding.Title,
			"shortDescription": map[string]any{
				"text": finding.Title,
			},
			"fullDescription": map[string]any{
				"text": finding.WhyItMatters,
			},
			"help": map[string]any{
				"text": finding.Remediation,
			},
		}

		results = append(results, map[string]any{
			"ruleId": finding.RuleID,
			"level":  sarifLevel(finding.Severity),
			"message": map[string]any{
				"text": finding.Message,
			},
			"locations": []map[string]any{
				{
					"physicalLocation": map[string]any{
						"artifactLocation": map[string]any{
							"uri": sarifURI(finding.File),
						},
						"region": map[string]any{
							"startLine": finding.Line,
						},
					},
				},
			},
		})
	}

	rules := []map[string]any{}
	ruleIDs := make([]string, 0, len(rulesByID))
	for ruleID := range rulesByID {
		ruleIDs = append(ruleIDs, ruleID)
	}
	sort.Strings(ruleIDs)
	for _, ruleID := range ruleIDs {
		rules = append(rules, rulesByID[ruleID])
	}

	doc := map[string]any{
		"version": "2.1.0",
		"$schema": "https://json.schemastore.org/sarif-2.1.0.json",
		"runs": []map[string]any{
			{
				"tool": map[string]any{
					"driver": map[string]any{
						"name":           "yocto-lens",
						"informationUri": "https://github.com/prashantdivate/yocto-lens",
						"rules":          rules,
					},
				},
				"results": results,
			},
		},
	}

	data, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(path, data, 0644)
}

func sarifLevel(sev model.Severity) string {
	switch sev {
	case model.SeverityCritical, model.SeverityHigh:
		return "error"
	case model.SeverityMedium:
		return "warning"
	default:
		return "note"
	}
}

func sortedFindings(findings []model.Finding) []model.Finding {
	sorted := append([]model.Finding(nil), findings...)
	sort.SliceStable(sorted, func(i, j int) bool {
		if severityRank(sorted[i].Severity) != severityRank(sorted[j].Severity) {
			return severityRank(sorted[i].Severity) > severityRank(sorted[j].Severity)
		}
		if sorted[i].RuleID != sorted[j].RuleID {
			return sorted[i].RuleID < sorted[j].RuleID
		}
		if sorted[i].File != sorted[j].File {
			return sorted[i].File < sorted[j].File
		}
		return sorted[i].Line < sorted[j].Line
	})

	return sorted
}

func severityCounts(report model.Report) (int, int, int) {
	high, medium, low := 0, 0, 0

	for _, finding := range report.Findings {
		switch finding.Severity {
		case model.SeverityCritical, model.SeverityHigh:
			high++
		case model.SeverityMedium:
			medium++
		default:
			low++
		}
	}

	return high, medium, low
}

func markdownEscape(value string) string {
	value = strings.ReplaceAll(value, "\r\n", " ")
	value = strings.ReplaceAll(value, "\n", " ")
	value = strings.ReplaceAll(value, "|", "\\|")
	return strings.TrimSpace(value)
}

func parseFailOnSeverity(value string) (model.Severity, bool, error) {
	value = strings.TrimSpace(strings.ToLower(value))
	if value == "" || value == "none" || value == "off" {
		return "", false, nil
	}

	switch value {
	case "critical":
		return model.SeverityCritical, true, nil
	case "high":
		return model.SeverityHigh, true, nil
	case "medium", "warning", "warn":
		return model.SeverityMedium, true, nil
	case "low":
		return model.SeverityLow, true, nil
	case "info", "note":
		return model.SeverityInfo, true, nil
	default:
		return "", false, fmt.Errorf("%q, expected critical, high, medium, low, info, or none", value)
	}
}

func reportHasSeverityAtLeast(report model.Report, threshold model.Severity) bool {
	thresholdRank := severityRank(threshold)
	for _, finding := range report.Findings {
		if severityRank(finding.Severity) >= thresholdRank {
			return true
		}
	}

	return false
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
	case model.SeverityInfo:
		return 1
	default:
		return 0
	}
}

func sarifURI(path string) string {
	abs, err := filepath.Abs(path)
	if err != nil {
		return filepath.ToSlash(path)
	}

	wd, err := os.Getwd()
	if err != nil {
		return filepath.ToSlash(abs)
	}

	rel, err := filepath.Rel(wd, abs)
	if err == nil && !strings.HasPrefix(rel, "..") && !filepath.IsAbs(rel) {
		return filepath.ToSlash(rel)
	}

	return filepath.ToSlash(abs)
}

func printSummary(report model.Report) {
	high, medium, low := severityCounts(report)

	fmt.Printf("Layers: %d\n", len(report.Layers))
	fmt.Printf("Recipes: %d\n", len(report.Recipes))
	fmt.Printf("bbappends: %d\n", len(report.Appends))
	fmt.Printf("Patches: %d\n", len(report.Patches))
	fmt.Printf("Findings: %d\n", len(report.Findings))
	fmt.Printf("High: %d\n", high)
	fmt.Printf("Medium: %d\n", medium)
	fmt.Printf("Low: %d\n", low)
}

func printScanProfile(profile *scanProfile) {
	fmt.Println()
	fmt.Println("Profile:")
	fmt.Printf("Total: %s\n", time.Since(profile.started).Round(time.Millisecond))

	for _, phase := range []model.ScanPhase{
		model.PhaseStarting,
		model.PhaseDiscovering,
		model.PhaseParsing,
		model.PhaseRules,
		model.PhaseDone,
	} {
		duration := profile.durations[phase]
		if duration == 0 {
			continue
		}
		fmt.Printf("%s: %s\n", phase, duration.Round(time.Millisecond))
	}

	fmt.Printf(
		"Counts: layers=%d recipes=%d bbappends=%d patches=%d files=%d findings=%d\n",
		profile.progress.LayersFound,
		profile.progress.RecipesFound,
		profile.progress.AppendsFound,
		profile.progress.PatchesFound,
		profile.progress.FilesProcessed,
		profile.progress.FindingsFound,
	)
}

func compactConsolePath(path string, max int) string {
	if max < 12 {
		max = 12
	}

	clean := filepath.ToSlash(path)
	if len(clean) <= max {
		return clean
	}

	parts := strings.Split(clean, "/")
	if len(parts) >= 3 {
		tail := strings.Join(parts[len(parts)-3:], "/")
		out := ".../" + tail
		if len(out) <= max {
			return out
		}
	}

	return "..." + clean[len(clean)-max+3:]
}

func truncatePlain(s string, max int) string {
	if max <= 0 {
		return ""
	}

	runes := []rune(s)
	if len(runes) <= max {
		return s
	}

	if max <= 1 {
		return "…"
	}

	return string(runes[:max-1]) + "…"
}
