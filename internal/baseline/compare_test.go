package baseline

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestCompareReportsMissingUnexpectedAndModified(t *testing.T) {
	dir := t.TempDir()
	baselinePath := filepath.Join(dir, "baseline.yaml")
	configPath := filepath.Join(dir, "config.json")

	writeTestFile(t, baselinePath, `schema_version: 1
rules:
  - path: /server/port
    type: equals
    value: 8443
  - path: /debug
    type: absent
  - path: /credentials/token
    type: exists
  - path: /mode
    type: one_of
    values: [safe, balanced]
  - path: /required/name
    type: exists
`)
	writeTestFile(t, configPath, `{
  "server": {"port": 8080},
  "debug": true,
  "credentials": {"token": "SYNTHETIC-SECRET-NOT-FOR-USE"},
  "mode": "unsafe",
  "extra": "untracked"
}`)

	document, err := LoadBaseline(baselinePath)
	if err != nil {
		t.Fatal(err)
	}
	config, err := LoadConfig(configPath)
	if err != nil {
		t.Fatal(err)
	}
	report := Compare(document, config)

	want := []Finding{
		{Path: "/debug", Status: "UNEXPECTED"},
		{Path: "/extra", Status: "UNEXPECTED"},
		{Path: "/mode", Status: "MODIFIED"},
		{Path: "/required/name", Status: "MISSING"},
		{Path: "/server/port", Status: "MODIFIED"},
	}
	if !reflect.DeepEqual(report.Findings, want) {
		t.Fatalf("findings = %#v, want %#v", report.Findings, want)
	}
	if report.Summary != (Summary{Missing: 1, Unexpected: 2, Modified: 2, Total: 5}) {
		t.Fatalf("summary = %#v", report.Summary)
	}
	if strings.Contains(string(mustJSON(t, report)), "SYNTHETIC-SECRET-NOT-FOR-USE") {
		t.Fatal("report exposed a configuration value")
	}
}

func TestEqualsRuleClassifiesObjectDrift(t *testing.T) {
	document := Document{
		Rules: []Rule{{
			Path:  "/server",
			Type:  "equals",
			Value: map[string]any{"host": "edge", "port": 8443},
		}},
	}
	config := map[string]any{
		"server": map[string]any{
			"host":  "edge",
			"port":  8080,
			"extra": true,
		},
	}

	report := Compare(document, config)
	want := []Finding{
		{Path: "/server/extra", Status: "UNEXPECTED"},
		{Path: "/server/port", Status: "MODIFIED"},
	}
	if !reflect.DeepEqual(report.Findings, want) {
		t.Fatalf("findings = %#v, want %#v", report.Findings, want)
	}
}

func TestCompareJSONPointerAndNumericEquivalence(t *testing.T) {
	dir := t.TempDir()
	baselinePath := filepath.Join(dir, "baseline.json")
	configPath := filepath.Join(dir, "config.yaml")
	writeTestFile(t, baselinePath, `{"schema_version":1,"allow_unexpected":true,"rules":[{"path":"/items/0/a~1b","type":"equals","value":0.1}]}`)
	writeTestFile(t, configPath, "items:\n  - a/b: 0.1\n")

	document, err := LoadBaseline(baselinePath)
	if err != nil {
		t.Fatal(err)
	}
	config, err := LoadConfig(configPath)
	if err != nil {
		t.Fatal(err)
	}
	report := Compare(document, config)
	if report.Summary.Total != 0 {
		t.Fatalf("expected no drift, got %#v", report.Findings)
	}
}

func TestLoadBaselineRejectsUnsupportedRuleAndDuplicateJSONKeys(t *testing.T) {
	dir := t.TempDir()
	badRule := filepath.Join(dir, "unsupported.yaml")
	writeTestFile(t, badRule, "schema_version: 1\nrules:\n  - path: /x\n    type: execute\n")
	if _, err := LoadBaseline(badRule); err == nil || !strings.Contains(err.Error(), "unsupported rule type") {
		t.Fatalf("LoadBaseline error = %v, want unsupported rule type", err)
	}

	duplicate := filepath.Join(dir, "duplicate.json")
	writeTestFile(t, duplicate, `{"schema_version":1,"schema_version":1,"rules":[]}`)
	if _, err := LoadBaseline(duplicate); err == nil || !strings.Contains(err.Error(), "invalid JSON") {
		t.Fatalf("LoadBaseline error = %v, want invalid JSON", err)
	}
}

func TestLoadDocumentReportsMalformedAndUnsupportedFiles(t *testing.T) {
	dir := t.TempDir()
	malformed := filepath.Join(dir, "config.yaml")
	writeTestFile(t, malformed, "password: SYNTHETIC-SECRET: invalid\n")
	if _, err := LoadConfig(malformed); err == nil || strings.Contains(err.Error(), "SYNTHETIC-SECRET") {
		t.Fatalf("LoadConfig error must be safe and report malformed YAML; got %v", err)
	}

	unsupported := filepath.Join(dir, "config.toml")
	writeTestFile(t, unsupported, "key = 1\n")
	if _, err := LoadConfig(unsupported); err == nil || !strings.Contains(err.Error(), "unsupported file extension") {
		t.Fatalf("LoadConfig error = %v, want unsupported format", err)
	}

	badJSON := filepath.Join(dir, "config.json")
	writeTestFile(t, badJSON, `{"key":1]`)
	if _, err := LoadConfig(badJSON); err == nil || !strings.Contains(err.Error(), "invalid JSON") {
		t.Fatalf("LoadConfig error = %v, want invalid JSON", err)
	}

	multipleYAMLDocuments := filepath.Join(dir, "multiple.yaml")
	writeTestFile(t, multipleYAMLDocuments, "key: one\n---\nkey: two\n")
	if _, err := LoadConfig(multipleYAMLDocuments); err == nil || !strings.Contains(err.Error(), "invalid YAML") {
		t.Fatalf("LoadConfig error = %v, want invalid multi-document YAML", err)
	}
}

func TestLoadDocumentReportsInaccessiblePath(t *testing.T) {
	if _, err := LoadConfig(filepath.Join(t.TempDir(), "missing.json")); err == nil || !strings.Contains(err.Error(), "could not be read") {
		t.Fatalf("LoadConfig error = %v, want inaccessible path", err)
	}
	if _, err := LoadConfig(t.TempDir()); err == nil {
		t.Fatal("LoadConfig should reject a directory")
	}
}

func writeTestFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		t.Fatal(err)
	}
}

func mustJSON(t *testing.T, value any) []byte {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return data
}
