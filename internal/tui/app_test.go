package tui

import (
	"testing"

	"github.com/example/yocto-lens/internal/model"
)

func TestFilterFindingsByMode(t *testing.T) {
	findings := []model.Finding{
		{RuleID: "static/license-missing"},
		{RuleID: "style/line-length"},
	}

	staticFindings := filterFindingsByMode(findings, ModeStaticAnalysis)
	if len(staticFindings) != 1 || staticFindings[0].RuleID != "static/license-missing" {
		t.Fatalf("static findings = %#v", staticFindings)
	}

	styleFindings := filterFindingsByMode(findings, ModeStyleCheck)
	if len(styleFindings) != 1 || styleFindings[0].RuleID != "style/line-length" {
		t.Fatalf("style findings = %#v", styleFindings)
	}
}

func TestHealthScoreAndRiskLabel(t *testing.T) {
	score := healthScore([]model.Finding{
		{Severity: model.SeverityHigh},
		{Severity: model.SeverityMedium},
	})
	if score != 77 {
		t.Fatalf("healthScore() = %d, want 77", score)
	}

	if riskLabel(0) != "clean" {
		t.Fatalf("riskLabel(0) = %q, want clean", riskLabel(0))
	}
	if riskLabel(100) != "critical" {
		t.Fatalf("riskLabel(100) = %q, want critical", riskLabel(100))
	}
}

func TestRecipeNameFromPath(t *testing.T) {
	if got := recipeNameFromPath("recipes-test/foo/foo_1.0.bb"); got != "foo" {
		t.Fatalf("recipeNameFromPath(.bb) = %q, want foo", got)
	}
	if got := recipeNameFromPath("recipes-test/foo/foo_%.bbappend"); got != "foo" {
		t.Fatalf("recipeNameFromPath(.bbappend) = %q, want foo", got)
	}
}
