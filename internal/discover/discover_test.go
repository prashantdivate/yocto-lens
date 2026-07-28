package discover

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLayersFromInputsDiscoversLayerMetadata(t *testing.T) {
	root := t.TempDir()
	layer := filepath.Join(root, "meta-test")
	writeDiscoverTestFile(t, filepath.Join(layer, "conf", "layer.conf"), `BBFILE_COLLECTIONS = "test"
BBFILE_PRIORITY_test = "7"
LAYERSERIES_COMPAT_test = "scarthgap"
BBFILES = "${LAYERDIR}/recipes-*/*/*.bb"
`)

	layers, err := LayersFromInputs([]string{root})
	if err != nil {
		t.Fatalf("LayersFromInputs() error = %v", err)
	}
	if len(layers) != 1 {
		t.Fatalf("len(layers) = %d, want 1", len(layers))
	}

	layerInfo := layers[0]
	if layerInfo.Name != "meta-test" {
		t.Fatalf("layer name = %q, want meta-test", layerInfo.Name)
	}
	if layerInfo.Priority != "7" {
		t.Fatalf("layer priority = %q, want 7", layerInfo.Priority)
	}
	if layerInfo.Series != "scarthgap" {
		t.Fatalf("layer series = %q, want scarthgap", layerInfo.Series)
	}
	if layerInfo.BBFiles == "" {
		t.Fatal("layer BBFiles was not parsed")
	}
}

func TestShouldSkipDir(t *testing.T) {
	if !ShouldSkipDir("tmp") {
		t.Fatal("ShouldSkipDir(tmp) = false, want true")
	}
	if ShouldSkipDir("recipes-core") {
		t.Fatal("ShouldSkipDir(recipes-core) = true, want false")
	}
}

func writeDiscoverTestFile(t *testing.T, path string, content string) {
	t.Helper()

	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatalf("MkdirAll(%q) error = %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("WriteFile(%q) error = %v", path, err)
	}
}
