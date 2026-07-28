package exporter

import (
	"encoding/json"
	"os"

	"github.com/example/yocto-lens/internal/model"
)

func WriteJSON(path string, r model.Report) error {
	b, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, b, 0644)
}

type sarif struct {
	Version string `json:"version"`
	Schema  string `json:"$schema"`
	Runs    []run  `json:"runs"`
}
type run struct {
	Tool    tool     `json:"tool"`
	Results []result `json:"results"`
}
type tool struct {
	Driver driver `json:"driver"`
}
type driver struct {
	Name           string `json:"name"`
	InformationURI string `json:"informationUri,omitempty"`
	Rules          []rule `json:"rules"`
}
type rule struct {
	ID               string `json:"id"`
	Name             string `json:"name"`
	ShortDescription text   `json:"shortDescription"`
}
type result struct {
	RuleID    string     `json:"ruleId"`
	Level     string     `json:"level"`
	Message   text       `json:"message"`
	Locations []location `json:"locations"`
}
type text struct {
	Text string `json:"text"`
}
type location struct {
	PhysicalLocation physical `json:"physicalLocation"`
}
type physical struct {
	ArtifactLocation artifact `json:"artifactLocation"`
	Region           region   `json:"region"`
}
type artifact struct {
	URI string `json:"uri"`
}
type region struct {
	StartLine int `json:"startLine"`
}

func WriteSARIF(path string, r model.Report) error {
	ruleSeen := map[string]bool{}
	var rules []rule
	var results []result
	for _, f := range r.Findings {
		if !ruleSeen[f.RuleID] {
			ruleSeen[f.RuleID] = true
			rules = append(rules, rule{ID: f.RuleID, Name: f.Title, ShortDescription: text{Text: f.Title}})
		}
		results = append(results, result{RuleID: f.RuleID, Level: level(f.Severity), Message: text{Text: f.Message + " Fix: " + f.Remediation}, Locations: []location{{PhysicalLocation: physical{ArtifactLocation: artifact{URI: f.File}, Region: region{StartLine: f.Line}}}}})
	}
	doc := sarif{Version: "2.1.0", Schema: "https://json.schemastore.org/sarif-2.1.0.json", Runs: []run{{Tool: tool{Driver: driver{Name: "yocto-lens", Rules: rules}}, Results: results}}}
	b, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, b, 0644)
}

func level(s model.Severity) string {
	switch s {
	case model.SeverityCritical, model.SeverityHigh:
		return "error"
	case model.SeverityMedium:
		return "warning"
	default:
		return "note"
	}
}
