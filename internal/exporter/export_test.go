package exporter

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/example/yocto-lens/internal/model"
)

func TestWriteJSON(t *testing.T) {
	path := filepath.Join(t.TempDir(), "report.json")
	report := model.Report{
		Root: "meta-test",
		Layers: []model.Layer{
			{Name: "meta-test", Path: "/layers/meta-test"},
		},
	}

	if err := WriteJSON(path, report); err != nil {
		t.Fatalf("WriteJSON() error = %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}

	var got model.Report
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	if got.Root != report.Root {
		t.Fatalf("Root = %q, want %q", got.Root, report.Root)
	}
}

func TestWriteSARIF(t *testing.T) {
	path := filepath.Join(t.TempDir(), "report.sarif")
	report := model.Report{
		Findings: []model.Finding{
			{
				RuleID:      "static/test",
				Title:       "Test finding",
				Severity:    model.SeverityHigh,
				File:        "recipes-test/foo.bb",
				Line:        12,
				Message:     "Problem",
				Remediation: "Fix it",
			},
		},
	}

	if err := WriteSARIF(path, report); err != nil {
		t.Fatalf("WriteSARIF() error = %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}

	var got sarif
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	if got.Version != "2.1.0" {
		t.Fatalf("SARIF version = %q, want 2.1.0", got.Version)
	}
	if len(got.Runs) != 1 || len(got.Runs[0].Results) != 1 {
		t.Fatalf("SARIF results shape = %#v", got.Runs)
	}
	if got.Runs[0].Results[0].Level != "error" {
		t.Fatalf("SARIF level = %q, want error", got.Runs[0].Results[0].Level)
	}
}
