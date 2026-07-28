package main

import (
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
