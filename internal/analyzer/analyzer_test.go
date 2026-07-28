package analyzer

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/example/yocto-lens/internal/model"
)

func TestAnalyzeParsesLayerFiles(t *testing.T) {
	root := t.TempDir()
	layer := filepath.Join(root, "meta-test")

	writeTestFile(t, filepath.Join(layer, "conf", "layer.conf"), `BBFILE_COLLECTIONS = "test"
LAYERSERIES_COMPAT_test = "scarthgap"
BBFILE_PATTERN_test = "^${LAYERDIR}/"
BBFILE_PRIORITY_test = "6"
`)
	writeTestFile(t, filepath.Join(layer, "recipes-a", "bar", "bar_1.0.bb"), `SUMMARY = "bar"
LICENSE = "MIT"
LIC_FILES_CHKSUM = "file://COPYING;md5=abc"
`)
	writeTestFile(t, filepath.Join(layer, "recipes-z", "foo", "foo_1.0.bb"), `SUMMARY = "foo"
LICENSE = "MIT"
LIC_FILES_CHKSUM = "file://COPYING;md5=abc"
`)
	writeTestFile(t, filepath.Join(layer, "recipes-z", "foo", "foo_%.bbappend"), `FILESEXTRAPATHS:prepend := "${THISDIR}/files:"
`)
	writeTestFile(t, filepath.Join(layer, "classes", "image-policy.bbclass"), `IMAGE_FEATURES += "ssh-server-dropbear"
`)
	writeTestFile(t, filepath.Join(layer, "recipes-z", "foo", "fix.patch"), `From 0000000000000000000000000000000000000000 Mon Sep 17 00:00:00 2001
Subject: [PATCH] fix foo
Upstream-Status: Pending
Signed-off-by: Test User <test@example.com>
---
diff --git a/foo b/foo
--- a/foo
+++ b/foo
@@ -1 +1 @@
-old
+new
`)

	var lastProgress model.ScanProgress
	report, err := AnalyzeWithProgress([]string{root}, func(p model.ScanProgress) {
		lastProgress = p
	})
	if err != nil {
		t.Fatalf("AnalyzeWithProgress() error = %v", err)
	}

	if len(report.Layers) != 1 {
		t.Fatalf("len(report.Layers) = %d, want 1", len(report.Layers))
	}
	if len(report.Recipes) != 2 {
		t.Fatalf("len(report.Recipes) = %d, want 2", len(report.Recipes))
	}
	if report.Recipes[0].Name != "bar_1.0" || report.Recipes[1].Name != "foo_1.0" {
		t.Fatalf("recipes were not kept in walk order: got %q, %q", report.Recipes[0].Name, report.Recipes[1].Name)
	}
	if len(report.Appends) != 1 {
		t.Fatalf("len(report.Appends) = %d, want 1", len(report.Appends))
	}
	if len(report.Patches) != 1 {
		t.Fatalf("len(report.Patches) = %d, want 1", len(report.Patches))
	}
	if len(report.MetadataFiles) != 1 {
		t.Fatalf("len(report.MetadataFiles) = %d, want 1", len(report.MetadataFiles))
	}
	if report.MetadataFiles[0].Kind != "bbclass" {
		t.Fatalf("report.MetadataFiles[0].Kind = %q, want bbclass", report.MetadataFiles[0].Kind)
	}
	if lastProgress.FilesProcessed != 5 {
		t.Fatalf("last progress files = %d, want 5", lastProgress.FilesProcessed)
	}
}

func TestResolveIncludePathUsesLayerFileIndex(t *testing.T) {
	root := t.TempDir()
	currentFile := filepath.Join(root, "recipes-test", "foo", "foo_1.0.bb")
	includeFile := filepath.Join(root, "shared", "common.inc")
	writeTestFile(t, currentFile, `require common.inc
`)
	writeTestFile(t, includeFile, `SUMMARY = "from include"
`)

	index := layerFileIndex{}
	index.add(includeFile)

	resolved, ok := resolveIncludePath("common.inc", currentFile, filepath.Join(root, "missing-layer-root"), nil, index)
	if !ok {
		t.Fatal("resolveIncludePath() did not resolve include from index")
	}
	if resolved != includeFile {
		t.Fatalf("resolveIncludePath() = %q, want %q", resolved, includeFile)
	}
}

func TestApplySuppressions(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "recipes-test", "foo", "foo_1.0.bb")
	writeTestFile(t, path, `# yocto-lens-disable-next-line static/license-missing
LICENSE = "CLOSED"
SUMMARY = "foo" # yocto-lens-disable-line
DESCRIPTION = "foo"
`)

	findings := []model.Finding{
		{RuleID: "static/license-missing", File: path, Line: 2},
		{RuleID: "static/license-closed", File: path, Line: 2},
		{RuleID: "style/line-length", File: path, Line: 3},
		{RuleID: "style/variable-order", File: path, Line: 4},
	}

	filtered := applySuppressions(findings)
	if len(filtered) != 2 {
		t.Fatalf("len(applySuppressions()) = %d, want 2", len(filtered))
	}
	if filtered[0].RuleID != "static/license-closed" {
		t.Fatalf("filtered[0].RuleID = %q, want static/license-closed", filtered[0].RuleID)
	}
	if filtered[1].RuleID != "style/variable-order" {
		t.Fatalf("filtered[1].RuleID = %q, want style/variable-order", filtered[1].RuleID)
	}
}

func TestAnalyzeUsesConfigFile(t *testing.T) {
	root := t.TempDir()
	layer := filepath.Join(root, "meta-test")

	writeTestFile(t, filepath.Join(root, ".yocto-lens.json"), `{
  "target_release": "scarthgap",
  "exclude": ["recipes-skip/**"],
  "severity": {
    "static/license-closed": "HIGH"
  }
}
`)
	writeTestFile(t, filepath.Join(layer, "conf", "layer.conf"), `BBFILE_COLLECTIONS = "test"
LAYERSERIES_COMPAT_test = "scarthgap"
BBFILE_PATTERN_test = "^${LAYERDIR}/"
BBFILE_PRIORITY_test = "6"
`)
	writeTestFile(t, filepath.Join(layer, "recipes-keep", "foo", "foo_1.0.bb"), `SUMMARY = "foo"
DESCRIPTION = "foo"
LICENSE = "CLOSED"
`)
	writeTestFile(t, filepath.Join(layer, "recipes-skip", "bar", "bar_1.0.bb"), `SUMMARY = "bar"
DESCRIPTION = "bar"
LICENSE = "MIT"
`)

	report, err := Analyze([]string{root})
	if err != nil {
		t.Fatalf("Analyze() error = %v", err)
	}

	if report.TargetRelease != "scarthgap" {
		t.Fatalf("report.TargetRelease = %q, want scarthgap", report.TargetRelease)
	}
	if len(report.Recipes) != 1 {
		t.Fatalf("len(report.Recipes) = %d, want 1", len(report.Recipes))
	}
	if report.Recipes[0].Name != "foo_1.0" {
		t.Fatalf("report.Recipes[0].Name = %q, want foo_1.0", report.Recipes[0].Name)
	}

	foundOverride := false
	for _, finding := range report.Findings {
		if finding.RuleID == "static/license-closed" {
			foundOverride = true
			if finding.Severity != model.SeverityHigh {
				t.Fatalf("license-closed severity = %s, want HIGH", finding.Severity)
			}
		}
		if strings.Contains(filepath.ToSlash(finding.File), "recipes-skip") {
			t.Fatalf("finding for excluded path was not filtered: %s", finding.File)
		}
	}
	if !foundOverride {
		t.Fatal("did not find static/license-closed finding")
	}
}

func TestCheckPatchReferences(t *testing.T) {
	root := t.TempDir()
	recipePath := filepath.Join(root, "meta-test", "recipes-test", "foo", "foo_1.0.bb")
	usedPatch := filepath.Join(root, "meta-test", "recipes-test", "foo", "files", "used.patch")
	unusedPatch := filepath.Join(root, "meta-test", "recipes-test", "foo", "files", "unused.patch")

	writeTestFile(t, recipePath, `SRC_URI = "file://used.patch file://missing.patch"
`)
	writeTestFile(t, usedPatch, "")
	writeTestFile(t, unusedPatch, "")

	report := model.Report{
		Recipes: []model.Recipe{
			{
				Path:  recipePath,
				Layer: "meta-test",
				Variables: map[string]string{
					"SRC_URI": "file://used.patch file://missing.patch",
				},
				Lines: []string{`SRC_URI = "file://used.patch file://missing.patch"`},
			},
		},
		Patches: []model.Patch{
			{Path: usedPatch, Layer: "meta-test"},
			{Path: unusedPatch, Layer: "meta-test"},
		},
	}

	findings := checkPatchReferences(report)
	if !hasFinding(findings, "static/patch-reference-missing", recipePath) {
		t.Fatal("did not find missing patch reference")
	}
	if !hasFinding(findings, "static/patch-unreferenced", unusedPatch) {
		t.Fatal("did not find unreferenced patch")
	}
	if hasFinding(findings, "static/patch-unreferenced", usedPatch) {
		t.Fatal("referenced patch was reported as unreferenced")
	}
}

func TestAnalyzeChecksTargetReleaseCompatibility(t *testing.T) {
	root := t.TempDir()
	layer := filepath.Join(root, "meta-test")

	writeTestFile(t, filepath.Join(root, ".yocto-lens.json"), `{
  "target_release": "walnascar"
}
`)
	writeTestFile(t, filepath.Join(layer, "conf", "layer.conf"), `BBFILE_COLLECTIONS = "test"
LAYERSERIES_COMPAT_test = "scarthgap"
BBFILE_PATTERN_test = "^${LAYERDIR}/"
BBFILE_PRIORITY_test = "6"
`)

	report, err := Analyze([]string{root})
	if err != nil {
		t.Fatalf("Analyze() error = %v", err)
	}

	if !hasFinding(report.Findings, "static/layer-target-release-incompatible", filepath.Join(layer, "conf", "layer.conf")) {
		t.Fatal("did not find target release compatibility finding")
	}
}

func TestCheckDuplicateProviders(t *testing.T) {
	recipes := []model.Recipe{
		{
			Path:  "foo_1.0.bb",
			Layer: "meta-a",
			PN:    "foo",
			Variables: map[string]string{
				"PROVIDES": "virtual/foo",
			},
			Lines: []string{`PROVIDES = "virtual/foo"`},
		},
		{
			Path:  "foo_2.0.bb",
			Layer: "meta-b",
			PN:    "foo",
			Variables: map[string]string{
				"PROVIDES": "virtual/foo",
			},
			Lines: []string{`PROVIDES = "virtual/foo"`},
		},
	}

	findings := checkDuplicateProviders(recipes)
	if !hasFinding(findings, "static/duplicate-provider", "foo_1.0.bb") {
		t.Fatal("did not find duplicate provider for first recipe")
	}
	if !hasFinding(findings, "static/duplicate-provider", "foo_2.0.bb") {
		t.Fatal("did not find duplicate provider for second recipe")
	}
}

func TestMetadataParseCacheReturnsCopies(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "foo.inc")
	writeTestFile(t, path, `SUMMARY = "foo"
`)

	cache := &metadataParseCache{}
	lines, vars, err := parseMetadataFileCached(path, cache)
	if err != nil {
		t.Fatalf("parseMetadataFileCached() error = %v", err)
	}

	lines[0] = "changed"
	vars["SUMMARY"] = "changed"

	lines, vars, err = parseMetadataFileCached(path, cache)
	if err != nil {
		t.Fatalf("parseMetadataFileCached() second error = %v", err)
	}
	if lines[0] != `SUMMARY = "foo"` {
		t.Fatalf("cached line = %q, want original", lines[0])
	}
	if vars["SUMMARY"] != "foo" {
		t.Fatalf("cached SUMMARY = %q, want foo", vars["SUMMARY"])
	}
}

func hasFinding(findings []model.Finding, ruleID string, path string) bool {
	for _, finding := range findings {
		if finding.RuleID == ruleID && finding.File == path {
			return true
		}
	}

	return false
}

func writeTestFile(t *testing.T, path string, content string) {
	t.Helper()

	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatalf("MkdirAll(%q) error = %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("WriteFile(%q) error = %v", path, err)
	}
}
