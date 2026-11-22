package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunRedactsValuesAndDoesNotModifyInputs(t *testing.T) {
	dir := t.TempDir()
	baselinePath := filepath.Join(dir, "baseline.yaml")
	configPath := filepath.Join(dir, "config.yaml")
	baseline := []byte("schema_version: 1\nrules:\n  - path: /credentials/password\n    type: equals\n    value: approved-placeholder\n")
	config := []byte("credentials:\n  password: SYNTHETIC-SECRET-NOT-FOR-USE\n")
	if err := os.WriteFile(baselinePath, baseline, 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(configPath, config, 0600); err != nil {
		t.Fatal(err)
	}

	var stdout, stderr bytes.Buffer
	code := run([]string{"-baseline", baselinePath, "-config", configPath}, &stdout, &stderr)
	if code != 1 {
		t.Fatalf("run exit code = %d, want 1; stderr=%q", code, stderr.String())
	}
	if strings.Contains(stdout.String(), "SYNTHETIC-SECRET-NOT-FOR-USE") || strings.Contains(stdout.String(), "approved-placeholder") {
		t.Fatalf("report exposed a value: %q", stdout.String())
	}
	if !strings.Contains(stdout.String(), "MODIFIED /credentials/password") {
		t.Fatalf("report did not identify drift: %q", stdout.String())
	}

	afterBaseline, err := os.ReadFile(baselinePath)
	if err != nil {
		t.Fatal(err)
	}
	afterConfig, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(afterBaseline, baseline) || !bytes.Equal(afterConfig, config) {
		t.Fatal("comparison modified an input file")
	}
}

func TestRunReportsMissingBaselineAndUnsupportedRule(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if code := run([]string{"-baseline", "missing.yaml", "-config", "config.json"}, &stdout, &stderr); code != 2 {
		t.Fatalf("missing baseline exit code = %d, want 2", code)
	}

	dir := t.TempDir()
	baselinePath := filepath.Join(dir, "baseline.yaml")
	configPath := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(baselinePath, []byte("schema_version: 1\nrules:\n  - path: /x\n    type: run\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(configPath, []byte("x: 1\n"), 0600); err != nil {
		t.Fatal(err)
	}
	stderr.Reset()
	if code := run([]string{"-baseline", baselinePath, "-config", configPath}, &stdout, &stderr); code != 2 {
		t.Fatalf("unsupported rule exit code = %d, want 2", code)
	}
	if !strings.Contains(stderr.String(), "unsupported rule type") {
		t.Fatalf("stderr = %q, want unsupported rule error", stderr.String())
	}
}

func TestRunExampleMatchesExpectedReport(t *testing.T) {
	examples := filepath.Join("..", "..", "examples")
	want, err := os.ReadFile(filepath.Join(examples, "expected-report.txt"))
	if err != nil {
		t.Fatal(err)
	}

	var stdout, stderr bytes.Buffer
	code := run([]string{
		"-baseline", filepath.Join(examples, "baseline.yaml"),
		"-config", filepath.Join(examples, "config-drift.yaml"),
	}, &stdout, &stderr)
	if code != 1 {
		t.Fatalf("run exit code = %d, want 1; stderr=%q", code, stderr.String())
	}
	normalizedWant := strings.ReplaceAll(string(want), "\r\n", "\n")
	if stdout.String() != normalizedWant {
		t.Fatalf("example report changed; update examples/expected-report.txt and README.\ngot:\n%s\nwant:\n%s", stdout.String(), normalizedWant)
	}
	if strings.Contains(stdout.String(), "EXAMPLE-ONLY-NOT-A-REAL-SECRET") {
		t.Fatal("example report exposed the synthetic token")
	}
}

func TestDisplayPath(t *testing.T) {
	tests := map[string]string{
		"":                   "(root)",
		"/":                  "/",
		"/service/port":      "/service/port",
		"/a b/ä":             "/a b/ä",
		"/a\nMODIFIED /fake": `"/a\nMODIFIED /fake"`,
		"/tab\there":         `"/tab\there"`,
		"/escape\x1b[31mred": `"/escape\x1b[31mred"`,
	}
	for path, want := range tests {
		if got := displayPath(path); got != want {
			t.Errorf("displayPath(%q) = %q, want %q", path, got, want)
		}
	}
}

func TestRunTextReportCannotBeForgedByKeys(t *testing.T) {
	dir := t.TempDir()
	baselinePath := filepath.Join(dir, "baseline.json")
	configPath := filepath.Join(dir, "config.json")
	if err := os.WriteFile(baselinePath, []byte(`{"schema_version":1,"rules":[]}`), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(configPath, []byte(`{"a\nMODIFIED /fake":1}`), 0600); err != nil {
		t.Fatal(err)
	}

	var stdout, stderr bytes.Buffer
	if code := run([]string{"-baseline", baselinePath, "-config", configPath}, &stdout, &stderr); code != 1 {
		t.Fatalf("run exit code = %d, want 1; stderr=%q", code, stderr.String())
	}
	for _, line := range strings.Split(stdout.String(), "\n") {
		if strings.HasPrefix(line, "MODIFIED") {
			t.Fatalf("key injected a report line: %q", stdout.String())
		}
	}
	if !strings.Contains(stdout.String(), `UNEXPECTED "/a\nMODIFIED ~1fake"`) {
		t.Fatalf("report did not quote the key: %q", stdout.String())
	}
}

func TestRunVersion(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if code := run([]string{"-version"}, &stdout, &stderr); code != 0 {
		t.Fatalf("run -version exit code = %d, want 0", code)
	}
	if !strings.HasPrefix(stdout.String(), "config-baseline ") {
		t.Fatalf("unexpected version output %q", stdout.String())
	}
}

func TestRunJSONOutputIsDeterministic(t *testing.T) {
	dir := t.TempDir()
	baselinePath := filepath.Join(dir, "baseline.json")
	configPath := filepath.Join(dir, "config.json")
	if err := os.WriteFile(baselinePath, []byte(`{"schema_version":1,"rules":[]}`), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(configPath, []byte(`{"z":1,"a":2}`), 0600); err != nil {
		t.Fatal(err)
	}

	var first, second, stderr bytes.Buffer
	args := []string{"-baseline", baselinePath, "-config", configPath, "-format", "json"}
	if code := run(args, &first, &stderr); code != 1 {
		t.Fatalf("first run exit code = %d, want 1; stderr=%q", code, stderr.String())
	}
	if code := run(args, &second, &stderr); code != 1 {
		t.Fatalf("second run exit code = %d, want 1; stderr=%q", code, stderr.String())
	}
	if first.String() != second.String() {
		t.Fatalf("reports differ:\n%s\n%s", first.String(), second.String())
	}
	if strings.Index(first.String(), `"/a"`) > strings.Index(first.String(), `"/z"`) {
		t.Fatalf("JSON findings are not sorted: %s", first.String())
	}
}
