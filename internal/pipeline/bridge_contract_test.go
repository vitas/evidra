package pipeline

import (
	"encoding/json"
	"testing"
	"time"

	"samebits.com/evidra/pkg/evidence"
)

func TestRiskContract_PrescribeUsesNativeRiskInputOnly(t *testing.T) {
	t.Parallel()

	payload, err := json.Marshal(evidence.PrescriptionPayload{
		PrescriptionID:  "P1",
		CanonicalAction: json.RawMessage(`{"tool":"kubectl","operation":"apply","operation_class":"mutate","scope_class":"production","resource_count":1,"resource_shape_hash":"sha256:shape"}`),
		RiskInputs: []evidence.RiskInput{
			{Source: "evidra/native", RiskLevel: "high", RiskTags: []string{"k8s.privileged_container"}},
			{Source: "trivy/0.58.0", RiskLevel: "critical", RiskTags: []string{"trivy.DS002"}},
		},
		EffectiveRisk: "critical",
		TTLMs:         evidence.DefaultTTLMs,
		CanonSource:   "adapter",
	})
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}

	entries := []evidence.EvidenceEntry{
		{
			EntryID:   "E1",
			Type:      evidence.EntryTypePrescribe,
			Timestamp: time.Now().UTC(),
			Actor:     evidence.Actor{Type: "agent", ID: "a1", Provenance: "mcp"},
			Payload:   payload,
		},
	}

	got, err := EvidenceToSignalEntries(entries)
	if err != nil {
		t.Fatalf("EvidenceToSignalEntries: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("entry count = %d, want 1", len(got))
	}
	if len(got[0].RiskTags) != 1 || got[0].RiskTags[0] != "k8s.privileged_container" {
		t.Fatalf("RiskTags = %v, want native-only value", got[0].RiskTags)
	}
}

func TestRiskContract_PrescribeFallsBackToLegacyRiskTags(t *testing.T) {
	t.Parallel()

	payload, err := json.Marshal(evidence.PrescriptionPayload{
		PrescriptionID:  "P1",
		CanonicalAction: json.RawMessage(`{"tool":"kubectl","operation":"apply","operation_class":"mutate","scope_class":"production","resource_count":1,"resource_shape_hash":"sha256:shape"}`),
		RiskLevel:       "high",
		RiskTags:        []string{"legacy.value"},
		TTLMs:           evidence.DefaultTTLMs,
		CanonSource:     "adapter",
	})
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}

	entries := []evidence.EvidenceEntry{
		{
			EntryID:   "E1",
			Type:      evidence.EntryTypePrescribe,
			Timestamp: time.Now().UTC(),
			Actor:     evidence.Actor{Type: "agent", ID: "a1", Provenance: "mcp"},
			Payload:   payload,
		},
	}

	got, err := EvidenceToSignalEntries(entries)
	if err != nil {
		t.Fatalf("EvidenceToSignalEntries: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("entry count = %d, want 1", len(got))
	}
	if len(got[0].RiskTags) != 1 || got[0].RiskTags[0] != "legacy.value" {
		t.Fatalf("RiskTags = %v, want risk_tags fallback", got[0].RiskTags)
	}
}
