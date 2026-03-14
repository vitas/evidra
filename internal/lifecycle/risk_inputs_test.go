package lifecycle

import (
	"testing"

	"samebits.com/evidra/pkg/evidence"
)

func TestBuildSARIFRiskInput_CriticalFindings(t *testing.T) {
	t.Parallel()

	got := buildSARIFRiskInput(ExternalFindingsSource{
		Source: "trivy/0.58.0",
		Findings: []evidence.FindingPayload{
			{Tool: "Trivy", ToolVersion: "0.58.0", RuleID: "DS002", Severity: "critical"},
			{Tool: "Trivy", ToolVersion: "0.58.0", RuleID: "DS005", Severity: "high"},
			{Tool: "Trivy", ToolVersion: "0.58.0", RuleID: "DS009", Severity: "medium"},
		},
	})

	if got.Source != "trivy/0.58.0" {
		t.Fatalf("source = %q, want trivy/0.58.0", got.Source)
	}
	if got.RiskLevel != "critical" {
		t.Fatalf("risk_level = %q, want critical", got.RiskLevel)
	}
	if len(got.RiskTags) != 2 {
		t.Fatalf("risk_tags len = %d, want 2", len(got.RiskTags))
	}
	if got.Detail != "3 findings (1 critical, 1 high, 1 medium)" {
		t.Fatalf("detail = %q", got.Detail)
	}
}

func TestBuildSARIFRiskInput_OnlyHighCriticalBecomeTags(t *testing.T) {
	t.Parallel()

	got := buildSARIFRiskInput(ExternalFindingsSource{
		Source: "scanner/1.0",
		Findings: []evidence.FindingPayload{
			{Tool: "Scanner", RuleID: "LOW1", Severity: "low"},
			{Tool: "Scanner", RuleID: "MED1", Severity: "medium"},
		},
	})

	if len(got.RiskTags) != 0 {
		t.Fatalf("risk_tags = %v, want none", got.RiskTags)
	}
	if got.RiskLevel != "medium" {
		t.Fatalf("risk_level = %q, want medium", got.RiskLevel)
	}
}

func TestBuildSARIFRiskInput_AutoDetectsSource(t *testing.T) {
	t.Parallel()

	got := buildSARIFRiskInput(ExternalFindingsSource{
		Findings: []evidence.FindingPayload{
			{Tool: "Trivy", ToolVersion: "0.58.0", RuleID: "DS002", Severity: "critical"},
		},
	})

	if got.Source != "trivy/0.58.0" {
		t.Fatalf("source = %q, want trivy/0.58.0", got.Source)
	}
}

func TestBuildSARIFRiskInput_EmptyFindings(t *testing.T) {
	t.Parallel()

	got := buildSARIFRiskInput(ExternalFindingsSource{})
	if got.RiskLevel != "low" {
		t.Fatalf("risk_level = %q, want low", got.RiskLevel)
	}
	if got.Detail != "" {
		t.Fatalf("detail = %q, want empty", got.Detail)
	}
}

func TestComputeEffectiveRisk_MaxWins(t *testing.T) {
	t.Parallel()

	got := computeEffectiveRisk([]evidence.RiskInput{
		{Source: "evidra/native", RiskLevel: "medium"},
		{Source: "trivy/0.58.0", RiskLevel: "critical"},
	})
	if got != "critical" {
		t.Fatalf("computeEffectiveRisk = %q, want critical", got)
	}
}

func TestComputeEffectiveRisk_Empty(t *testing.T) {
	t.Parallel()

	if got := computeEffectiveRisk(nil); got != "low" {
		t.Fatalf("computeEffectiveRisk(nil) = %q, want low", got)
	}
}

func TestComputeEffectiveRisk_UnrecognizedLevel(t *testing.T) {
	t.Parallel()

	got := computeEffectiveRisk([]evidence.RiskInput{
		{Source: "evidra/native", RiskLevel: "low"},
		{Source: "scanner/1.0", RiskLevel: "catastrophic"},
	})
	if got != "low" {
		t.Fatalf("computeEffectiveRisk = %q, want low", got)
	}
}
