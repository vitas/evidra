package pipeline

import (
	"encoding/json"
	"testing"
	"time"

	"samebits.com/evidra-benchmark/internal/signal"
	"samebits.com/evidra-benchmark/pkg/evidence"
)

func TestEvidenceToSignalEntries_Prescribe(t *testing.T) {
	t.Parallel()

	now := time.Now().UTC()
	canonAction := json.RawMessage(`{"tool":"kubectl","operation":"apply","operation_class":"mutate","scope_class":"production","resource_count":3,"resource_shape_hash":"sha256:shape1"}`)

	prescPayload, _ := json.Marshal(evidence.PrescriptionPayload{
		PrescriptionID:  "01PRESC",
		CanonicalAction: canonAction,
		RiskLevel:       "high",
		RiskTags:        []string{"privileged_container"},
		TTLMs:           300000,
		CanonSource:     "adapter",
	})

	entries := []evidence.EvidenceEntry{
		{
			EntryID:        "01ENTRY",
			Type:           evidence.EntryTypePrescribe,
			TraceID:        "01TRACE",
			Actor:          evidence.Actor{Type: "ai_agent", ID: "agent-1", Provenance: "mcp"},
			Timestamp:      now,
			IntentDigest:   "sha256:abc",
			ArtifactDigest: "sha256:def",
			Payload:        prescPayload,
		},
	}

	result, err := EvidenceToSignalEntries(entries)
	if err != nil {
		t.Fatalf("EvidenceToSignalEntries: %v", err)
	}

	if len(result) != 1 {
		t.Fatalf("expected 1 signal entry, got %d", len(result))
	}

	se := result[0]
	if se.EventID != "01ENTRY" {
		t.Errorf("EventID: got %q, want %q", se.EventID, "01ENTRY")
	}
	if !se.IsPrescription {
		t.Error("expected IsPrescription=true")
	}
	if se.ActorID != "agent-1" {
		t.Errorf("ActorID: got %q, want %q", se.ActorID, "agent-1")
	}
	if se.Tool != "kubectl" {
		t.Errorf("Tool: got %q, want %q", se.Tool, "kubectl")
	}
	if se.OperationClass != "mutate" {
		t.Errorf("OperationClass: got %q, want %q", se.OperationClass, "mutate")
	}
	if se.ScopeClass != "production" {
		t.Errorf("ScopeClass: got %q, want %q", se.ScopeClass, "production")
	}
	if se.ResourceCount != 3 {
		t.Errorf("ResourceCount: got %d, want %d", se.ResourceCount, 3)
	}
	if se.ShapeHash != "sha256:shape1" {
		t.Errorf("ShapeHash: got %q", se.ShapeHash)
	}
	if len(se.RiskTags) != 1 || se.RiskTags[0] != "privileged_container" {
		t.Errorf("RiskTags: got %v", se.RiskTags)
	}
	if se.IntentDigest != "sha256:abc" {
		t.Errorf("IntentDigest: got %q", se.IntentDigest)
	}
	if se.ArtifactDigest != "sha256:def" {
		t.Errorf("ArtifactDigest: got %q", se.ArtifactDigest)
	}
}

func TestEvidenceToSignalEntries_Report(t *testing.T) {
	t.Parallel()

	now := time.Now().UTC()
	reportPayload, _ := json.Marshal(evidence.ReportPayload{
		ReportID:       "01REPORT",
		PrescriptionID: "01PRESC",
		ExitCode:       1,
		Verdict:        evidence.VerdictFailure,
	})

	entries := []evidence.EvidenceEntry{
		{
			EntryID:   "02ENTRY",
			Type:      evidence.EntryTypeReport,
			TraceID:   "01TRACE",
			Actor:     evidence.Actor{Type: "ai_agent", ID: "agent-1", Provenance: "mcp"},
			Timestamp: now,
			Payload:   reportPayload,
		},
	}

	result, err := EvidenceToSignalEntries(entries)
	if err != nil {
		t.Fatalf("EvidenceToSignalEntries: %v", err)
	}

	if len(result) != 1 {
		t.Fatalf("expected 1 signal entry, got %d", len(result))
	}

	se := result[0]
	if !se.IsReport {
		t.Error("expected IsReport=true")
	}
	if se.PrescriptionID != "01PRESC" {
		t.Errorf("PrescriptionID: got %q, want %q", se.PrescriptionID, "01PRESC")
	}
	if se.ExitCode == nil || *se.ExitCode != 1 {
		t.Errorf("ExitCode: got %v, want 1", se.ExitCode)
	}
}

func TestEvidenceToSignalEntries_SkipsNonPrescribeReport(t *testing.T) {
	t.Parallel()

	entries := []evidence.EvidenceEntry{
		{
			EntryID: "01FINDING",
			Type:    evidence.EntryTypeFinding,
			Payload: json.RawMessage(`{"tool":"checkov","rule_id":"CKV_K8S_1","severity":"high","resource":"pod/test","message":"test"}`),
		},
		{
			EntryID: "02SIGNAL",
			Type:    evidence.EntryTypeSignal,
			Payload: json.RawMessage(`{"signal_name":"retry_loop","entry_refs":["x"]}`),
		},
	}

	result, err := EvidenceToSignalEntries(entries)
	if err != nil {
		t.Fatalf("EvidenceToSignalEntries: %v", err)
	}

	if len(result) != 0 {
		t.Errorf("expected 0 signal entries for non-prescribe/report, got %d", len(result))
	}
}

func TestEvidenceToSignalEntries_Empty(t *testing.T) {
	t.Parallel()

	result, err := EvidenceToSignalEntries(nil)
	if err != nil {
		t.Fatalf("EvidenceToSignalEntries: %v", err)
	}
	if len(result) != 0 {
		t.Errorf("expected 0 entries for nil input, got %d", len(result))
	}
}

// Verify the function signature matches signal.Entry type.
var _ = func() {
	var entries []evidence.EvidenceEntry
	var result []signal.Entry
	var err error
	result, err = EvidenceToSignalEntries(entries)
	_, _ = result, err
}
