package model

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestLayerOptionalMetadataJSON(t *testing.T) {
	layer := Layer{Name: "meta-test", Path: "/layers/meta-test"}
	data, err := json.Marshal(layer)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}

	raw := string(data)
	for _, field := range []string{"priority", "series", "bbfiles"} {
		if strings.Contains(raw, field) {
			t.Fatalf("empty optional field %q was serialized in %s", field, raw)
		}
	}

	layer.Priority = "7"
	layer.Series = "scarthgap"
	layer.BBFiles = "${LAYERDIR}/recipes-*/*/*.bb"
	data, err = json.Marshal(layer)
	if err != nil {
		t.Fatalf("json.Marshal() with metadata error = %v", err)
	}
	raw = string(data)
	for _, field := range []string{"priority", "series", "bbfiles"} {
		if !strings.Contains(raw, field) {
			t.Fatalf("non-empty optional field %q was not serialized in %s", field, raw)
		}
	}
}

func TestSeverityValues(t *testing.T) {
	if SeverityHigh != "HIGH" {
		t.Fatalf("SeverityHigh = %q, want HIGH", SeverityHigh)
	}
	if SeverityInfo != "INFO" {
		t.Fatalf("SeverityInfo = %q, want INFO", SeverityInfo)
	}
}
