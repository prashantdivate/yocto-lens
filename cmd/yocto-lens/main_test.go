package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/example/yocto-lens/internal/model"
)

func TestCompactConsolePath(t *testing.T) {
	path := filepath.Join("one", "two", "three", "four", "five.bb")
	got := compactConsolePath(path, 24)
	if !strings.HasPrefix(got, "...") {
		t.Fatalf("compactConsolePath() = %q, want compact path with ellipsis", got)
	}
	if !strings.Contains(filepath.ToSlash(got), "five.bb") {
		t.Fatalf("compactConsolePath() = %q, want file name preserved", got)
	}
}

func TestSarifLevel(t *testing.T) {
	tests := map[model.Severity]string{
		model.SeverityCritical: "error",
		model.SeverityHigh:     "error",
		model.SeverityMedium:   "warning",
		model.SeverityLow:      "note",
		model.SeverityInfo:     "note",
	}

	for severity, want := range tests {
		if got := sarifLevel(severity); got != want {
			t.Fatalf("sarifLevel(%s) = %q, want %q", severity, got, want)
		}
	}
}

func TestParseFailOnSeverity(t *testing.T) {
	sev, enabled, err := parseFailOnSeverity("HIGH")
	if err != nil {
		t.Fatalf("parseFailOnSeverity(HIGH) error = %v", err)
	}
	if !enabled || sev != model.SeverityHigh {
		t.Fatalf("parseFailOnSeverity(HIGH) = %s, %v; want HIGH, true", sev, enabled)
	}

	_, enabled, err = parseFailOnSeverity("none")
	if err != nil {
		t.Fatalf("parseFailOnSeverity(none) error = %v", err)
	}
	if enabled {
		t.Fatal("parseFailOnSeverity(none) enabled gate")
	}

	if _, _, err := parseFailOnSeverity("loud"); err == nil {
		t.Fatal("parseFailOnSeverity(loud) error = nil, want error")
	}
}

func TestReportHasSeverityAtLeast(t *testing.T) {
	report := model.Report{
		Findings: []model.Finding{
			{Severity: model.SeverityLow},
			{Severity: model.SeverityHigh},
		},
	}

	if !reportHasSeverityAtLeast(report, model.SeverityMedium) {
		t.Fatal("reportHasSeverityAtLeast(medium) = false, want true")
	}
	if reportHasSeverityAtLeast(report, model.SeverityCritical) {
		t.Fatal("reportHasSeverityAtLeast(critical) = true, want false")
	}
}

func TestWriteMarkdown(t *testing.T) {
	path := filepath.Join(t.TempDir(), "report.md")
	report := model.Report{
		Root:          "meta-test",
		TargetRelease: "scarthgap",
		Layers:        []model.Layer{{Name: "meta-test", Path: "/layers/meta-test"}},
		Findings: []model.Finding{
			{
				RuleID:   "static/test",
				Severity: model.SeverityHigh,
				File:     "recipes-test/foo.bb",
				Line:     7,
				Message:  "Value contains | pipe",
			},
		},
	}

	if err := writeMarkdown(report, path); err != nil {
		t.Fatalf("writeMarkdown() error = %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	content := string(data)
	for _, want := range []string{
		"# Yocto Lens Report",
		"Target release: `scarthgap`",
		"| HIGH | `static/test` |",
		"Value contains \\| pipe",
	} {
		if !strings.Contains(content, want) {
			t.Fatalf("Markdown report missing %q in:\n%s", want, content)
		}
	}
}

func TestSortedFindingsSeverityFirst(t *testing.T) {
	findings := sortedFindings([]model.Finding{
		{RuleID: "style/a", Severity: model.SeverityInfo, File: "b.bb"},
		{RuleID: "static/a", Severity: model.SeverityHigh, File: "a.bb"},
		{RuleID: "static/b", Severity: model.SeverityMedium, File: "c.bb"},
	})

	if findings[0].Severity != model.SeverityHigh {
		t.Fatalf("first severity = %s, want HIGH", findings[0].Severity)
	}
	if findings[2].Severity != model.SeverityInfo {
		t.Fatalf("last severity = %s, want INFO", findings[2].Severity)
	}
}

func TestScanProfileRecordsProgress(t *testing.T) {
	profile := newScanProfile()
	profile.Observe(model.ScanProgress{Phase: model.PhaseStarting})
	time.Sleep(time.Millisecond)
	profile.Observe(model.ScanProgress{Phase: model.PhaseParsing, FilesProcessed: 3})
	profile.Finish()

	if profile.progress.FilesProcessed != 3 {
		t.Fatalf("FilesProcessed = %d, want 3", profile.progress.FilesProcessed)
	}
	if profile.durations[model.PhaseStarting] == 0 {
		t.Fatal("starting phase duration was not recorded")
	}
}
