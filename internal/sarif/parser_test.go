package sarif

import (
	"os"
	"testing"
)

func TestParseSARIF_Checkov(t *testing.T) {
	t.Parallel()

	data, err := os.ReadFile("../../tests/testdata/sarif_checkov.json")
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}

	findings, err := Parse(data)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}

	if len(findings) != 2 {
		t.Fatalf("expected 2 findings, got %d", len(findings))
	}

	f := findings[0]
	if f.Tool != "checkov" {
		t.Errorf("tool: got %q, want checkov", f.Tool)
	}
	if f.RuleID != "CKV_K8S_1" {
		t.Errorf("rule_id: got %q, want CKV_K8S_1", f.RuleID)
	}
	if f.Severity != "high" {
		t.Errorf("severity: got %q, want high", f.Severity)
	}
	if f.Resource != "deployment.yaml" {
		t.Errorf("resource: got %q", f.Resource)
	}

	f2 := findings[1]
	if f2.Severity != "medium" {
		t.Errorf("second finding severity: got %q, want medium", f2.Severity)
	}
}

func TestParseSARIF_Empty(t *testing.T) {
	t.Parallel()

	data := []byte(`{"version":"2.1.0","runs":[]}`)
	findings, err := Parse(data)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if len(findings) != 0 {
		t.Errorf("expected 0 findings for empty runs, got %d", len(findings))
	}
}

func TestParseSARIF_InvalidJSON(t *testing.T) {
	t.Parallel()

	_, err := Parse([]byte(`not json`))
	if err == nil {
		t.Error("expected error for invalid JSON")
	}
}

func TestMapSeverity(t *testing.T) {
	t.Parallel()

	cases := []struct {
		input string
		want  string
	}{
		{"error", "high"},
		{"warning", "medium"},
		{"note", "low"},
		{"unknown", "info"},
		{"", "info"},
	}
	for _, tc := range cases {
		got := mapSeverity(tc.input)
		if got != tc.want {
			t.Errorf("mapSeverity(%q): got %q, want %q", tc.input, got, tc.want)
		}
	}
}
