package sarif

import (
	"os"
	"testing"
)

func TestParseSARIF_Trivy(t *testing.T) {
	t.Parallel()

	data, err := os.ReadFile("../../tests/testdata/sarif_trivy.json")
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}

	findings, err := Parse(data)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}

	if len(findings) == 0 {
		t.Fatal("expected findings")
	}

	f := findings[0]
	if f.Tool != "trivy" {
		t.Errorf("tool: got %q, want trivy", f.Tool)
	}
	if f.RuleID != "AVD-AWS-0001" {
		t.Errorf("rule_id: got %q, want AVD-AWS-0001", f.RuleID)
	}
	if f.Severity != "high" {
		t.Errorf("severity: got %q, want high", f.Severity)
	}
	if f.Resource != "main.tf" {
		t.Errorf("resource: got %q", f.Resource)
	}
}

func TestParseSARIF_Kubescape(t *testing.T) {
	t.Parallel()

	data, err := os.ReadFile("../../tests/testdata/sarif_kubescape.json")
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}

	findings, err := Parse(data)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}

	if len(findings) == 0 {
		t.Fatal("expected findings")
	}

	f := findings[0]
	if f.Tool != "kubescape" {
		t.Errorf("tool: got %q, want kubescape", f.Tool)
	}
	if f.RuleID != "KSV001" {
		t.Errorf("rule_id: got %q, want KSV001", f.RuleID)
	}
	if f.Severity != "medium" {
		t.Errorf("severity: got %q, want medium", f.Severity)
	}
	if f.Resource != "deployment.yaml" {
		t.Errorf("resource: got %q", f.Resource)
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

func TestParseSARIF_ToolVersion(t *testing.T) {
	t.Parallel()

	data := []byte(`{
		"version": "2.1.0",
		"runs": [{
			"tool": {"driver": {"name": "Trivy", "version": "0.50.1"}},
			"results": [{
				"ruleId": "CVE-2024-0001",
				"level": "error",
				"message": {"text": "test vuln"},
				"locations": [{"physicalLocation": {"artifactLocation": {"uri": "Dockerfile"}}}]
			}]
		}]
	}`)

	findings, err := Parse(data)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if len(findings) != 1 {
		t.Fatalf("expected 1 finding, got %d", len(findings))
	}
	if findings[0].ToolVersion != "0.50.1" {
		t.Errorf("tool_version: got %q, want %q", findings[0].ToolVersion, "0.50.1")
	}
}

func TestParseSARIF_ToolVersionEmpty(t *testing.T) {
	t.Parallel()

	data := []byte(`{
		"version": "2.1.0",
		"runs": [{
			"tool": {"driver": {"name": "scanner"}},
			"results": [{
				"ruleId": "R1",
				"level": "warning",
				"message": {"text": "msg"}
			}]
		}]
	}`)

	findings, err := Parse(data)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if len(findings) != 1 {
		t.Fatalf("expected 1 finding, got %d", len(findings))
	}
	if findings[0].ToolVersion != "" {
		t.Errorf("tool_version: got %q, want empty", findings[0].ToolVersion)
	}
}

func TestParseSARIF_MissingToolNameDefaultsUnknown(t *testing.T) {
	t.Parallel()

	data := []byte(`{"version":"2.1.0","runs":[{"tool":{"driver":{}},"results":[{"ruleId":"X","level":"warning","message":{"text":"m"}}]}]}`)
	findings, err := Parse(data)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if len(findings) != 1 {
		t.Fatalf("expected 1 finding, got %d", len(findings))
	}
	if findings[0].Tool != "unknown" {
		t.Fatalf("tool: got %q, want unknown", findings[0].Tool)
	}
}

func TestMapSeverity(t *testing.T) {
	t.Parallel()

	cases := []struct {
		input string
		want  string
	}{
		{"critical", "critical"},
		{"high", "high"},
		{"error", "high"},
		{"medium", "medium"},
		{"warning", "medium"},
		{"low", "low"},
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
