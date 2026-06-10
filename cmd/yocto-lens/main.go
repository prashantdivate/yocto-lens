package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/example/yocto-lens/internal/analyzer"
	"github.com/example/yocto-lens/internal/model"
	"github.com/example/yocto-lens/internal/tui"
)

func main() {
	jsonPath := flag.String("json", "", "write JSON report to file")
	sarifPath := flag.String("sarif", "", "write SARIF report to file")
	noTUI := flag.Bool("no-tui", false, "disable TUI and print console output")
	mode := flag.String("mode", "all", "finding mode for console/export: all, static, or style")
	flag.Parse()

	paths := flag.Args()
	if len(paths) == 0 {
		paths = []string{"."}
	}

	if *noTUI {
		report, err := analyzer.AnalyzeWithProgress(paths, func(p model.ScanProgress) {
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
		})
		fmt.Println()
		if err != nil {
			fmt.Fprintf(os.Stderr, "analysis failed: %v\n", err)
			os.Exit(1)
		}

		report, err = filterReportByMode(report, *mode)
		if err != nil {
			fmt.Fprintf(os.Stderr, "invalid mode: %v\n", err)
			os.Exit(1)
		}

		if err := writeOutputs(report, *jsonPath, *sarifPath); err != nil {
			fmt.Fprintf(os.Stderr, "export failed: %v\n", err)
			os.Exit(1)
		}

		printSummary(report)
		return
	}

	program := tea.NewProgram(
		tui.NewLoading(paths),
		tea.WithAltScreen(),
	)

	if _, err := program.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "tui failed: %v\n", err)
		os.Exit(1)
	}

	if *jsonPath != "" || *sarifPath != "" {
		report, err := analyzer.Analyze(paths)
		if err != nil {
			fmt.Fprintf(os.Stderr, "analysis failed during export: %v\n", err)
			os.Exit(1)
		}
		report, err = filterReportByMode(report, *mode)
		if err != nil {
			fmt.Fprintf(os.Stderr, "invalid mode: %v\n", err)
			os.Exit(1)
		}
		if err := writeOutputs(report, *jsonPath, *sarifPath); err != nil {
			fmt.Fprintf(os.Stderr, "export failed: %v\n", err)
			os.Exit(1)
		}
	}
}

func filterReportByMode(report model.Report, mode string) (model.Report, error) {
	mode = strings.ToLower(strings.TrimSpace(mode))
	if mode == "" {
		mode = "all"
	}

	switch mode {
	case "all":
		return report, nil
	case "static", "static-analysis", "analysis":
		filtered := make([]model.Finding, 0, len(report.Findings))
		for _, finding := range report.Findings {
			if !isStyleFinding(finding) {
				filtered = append(filtered, finding)
			}
		}
		report.Findings = filtered
		return report, nil
	case "style", "style-check":
		filtered := make([]model.Finding, 0, len(report.Findings))
		for _, finding := range report.Findings {
			if isStyleFinding(finding) {
				filtered = append(filtered, finding)
			}
		}
		report.Findings = filtered
		return report, nil
	default:
		return report, fmt.Errorf("%q, expected all, static, or style", mode)
	}
}

func isStyleFinding(f model.Finding) bool {
	rule := strings.ToLower(f.RuleID)
	return strings.HasPrefix(rule, "style/") || strings.HasPrefix(rule, "style-")
}

func writeOutputs(report model.Report, jsonPath string, sarifPath string) error {
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

func writeSARIF(report model.Report, path string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil && filepath.Dir(path) != "." {
		return err
	}

	rulesByID := map[string]map[string]any{}
	results := []map[string]any{}

	for _, finding := range report.Findings {
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
							"uri": finding.File,
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
	for _, rule := range rulesByID {
		rules = append(rules, rule)
	}

	doc := map[string]any{
		"version": "2.1.0",
		"$schema": "https://json.schemastore.org/sarif-2.1.0.json",
		"runs": []map[string]any{
			{
				"tool": map[string]any{
					"driver": map[string]any{
						"name":           "yocto-lens",
						"informationUri": "https://github.com/example/yocto-lens",
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

func printSummary(report model.Report) {
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

	fmt.Printf("Layers: %d\n", len(report.Layers))
	fmt.Printf("Recipes: %d\n", len(report.Recipes))
	fmt.Printf("bbappends: %d\n", len(report.Appends))
	fmt.Printf("Patches: %d\n", len(report.Patches))
	fmt.Printf("Findings: %d\n", len(report.Findings))
	fmt.Printf("High: %d\n", high)
	fmt.Printf("Medium: %d\n", medium)
	fmt.Printf("Low: %d\n", low)
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
