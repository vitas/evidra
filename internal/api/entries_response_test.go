package api

import (
	"encoding/json"
	"testing"
	"time"

	"samebits.com/evidra/internal/store"
	"samebits.com/evidra/pkg/evidence"
)

func TestToEntryAPIResponse_UsesEffectiveRiskFromPrescriptionPayload(t *testing.T) {
	t.Parallel()

	payload, err := json.Marshal(evidence.PrescriptionPayload{
		PrescriptionID: "rx-1",
		CanonicalAction: json.RawMessage(
			`{"tool":"kubectl","operation":"apply","operation_class":"mutate","scope_class":"production"}`,
		),
		RiskInputs: []evidence.RiskInput{
			{Source: "evidra/native", RiskLevel: "high"},
		},
		EffectiveRisk: "high",
	})
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}

	got := toEntryAPIResponse(store.StoredEntry{
		ID:        "entry-1",
		EntryType: string(evidence.EntryTypePrescribe),
		Payload:   payload,
		CreatedAt: time.Now().UTC(),
	})

	if got.RiskLevel != "high" {
		t.Fatalf("RiskLevel = %q, want high", got.RiskLevel)
	}
	if got.Tool != "kubectl" {
		t.Fatalf("Tool = %q, want kubectl", got.Tool)
	}
	if got.Operation != "apply" {
		t.Fatalf("Operation = %q, want apply", got.Operation)
	}
	if got.Scope != "production" {
		t.Fatalf("Scope = %q, want production", got.Scope)
	}
}
