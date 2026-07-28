package analyzer

import (
	"os"
	"path/filepath"
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
	if lastProgress.FilesProcessed != 4 {
		t.Fatalf("last progress files = %d, want 4", lastProgress.FilesProcessed)
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

func writeTestFile(t *testing.T, path string, content string) {
	t.Helper()

	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatalf("MkdirAll(%q) error = %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("WriteFile(%q) error = %v", path, err)
	}
}
