package evidence

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestFormatDigest(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		input string
		want  string
	}{
		{name: "bare hex gets prefix", input: "abc123", want: "sha256:abc123"},
		{name: "already prefixed is idempotent", input: "sha256:abc123", want: "sha256:abc123"},
		{name: "empty stays empty", input: "", want: ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := FormatDigest(tt.input)
			if got != tt.want {
				t.Errorf("FormatDigest(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestBuildEntry_Prescribe(t *testing.T) {
	t.Parallel()

	payload, err := json.Marshal(PrescriptionPayload{
		PrescriptionID: "rx-001",
		RiskLevel:      "low",
		TTLMs:          DefaultTTLMs,
		CanonSource:    "k8s",
	})
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}

	entry, err := BuildEntry(EntryBuildParams{
		Type:           EntryTypePrescribe,
		TenantID:       "tenant-1",
		TraceID:        "TRACE123",
		Actor:          Actor{Type: "agent", ID: "claude", Provenance: "mcp"},
		IntentDigest:   "deadbeef",
		ArtifactDigest: "sha256:cafebabe",
		Payload:        payload,
		SpecVersion:    "0.3.0",
		CanonVersion:   "1.0.0",
		AdapterVersion: "k8s-1.0.0",
	})
	if err != nil {
		t.Fatalf("BuildEntry: %v", err)
	}

	if entry.EntryID == "" {
		t.Error("entry_id must not be empty")
	}
	if entry.Type != EntryTypePrescribe {
		t.Errorf("type = %q, want %q", entry.Type, EntryTypePrescribe)
	}
	if !strings.HasPrefix(entry.Hash, "sha256:") {
		t.Errorf("hash must have sha256: prefix, got %q", entry.Hash)
	}
	if entry.IntentDigest != "sha256:deadbeef" {
		t.Errorf("intent_digest = %q, want %q", entry.IntentDigest, "sha256:deadbeef")
	}
	if entry.ArtifactDigest != "sha256:cafebabe" {
		t.Errorf("artifact_digest = %q, want %q", entry.ArtifactDigest, "sha256:cafebabe")
	}
	if entry.TraceID != "TRACE123" {
		t.Errorf("trace_id = %q, want %q", entry.TraceID, "TRACE123")
	}
	if entry.Timestamp.IsZero() {
		t.Error("timestamp must not be zero")
	}
}

func TestBuildEntry_HashChain(t *testing.T) {
	t.Parallel()

	payload := json.RawMessage(`{"prescription_id":"rx-001"}`)

	entry1, err := BuildEntry(EntryBuildParams{
		Type:           EntryTypePrescribe,
		TraceID:        "TRACE1",
		Actor:          Actor{Type: "agent", ID: "a1", Provenance: "mcp"},
		Payload:        payload,
		SpecVersion:    "0.3.0",
		CanonVersion:   "1.0.0",
		AdapterVersion: "k8s-1.0.0",
	})
	if err != nil {
		t.Fatalf("BuildEntry entry1: %v", err)
	}

	entry2, err := BuildEntry(EntryBuildParams{
		Type:           EntryTypeReport,
		TraceID:        "TRACE1",
		Actor:          Actor{Type: "agent", ID: "a1", Provenance: "mcp"},
		Payload:        json.RawMessage(`{"report_id":"rpt-001","prescription_id":"rx-001","exit_code":0,"verdict":"success"}`),
		PreviousHash:   entry1.Hash,
		SpecVersion:    "0.3.0",
		CanonVersion:   "1.0.0",
		AdapterVersion: "k8s-1.0.0",
	})
	if err != nil {
		t.Fatalf("BuildEntry entry2: %v", err)
	}

	if entry2.PreviousHash != entry1.Hash {
		t.Errorf("entry2.PreviousHash = %q, want %q", entry2.PreviousHash, entry1.Hash)
	}
	if entry1.Hash == entry2.Hash {
		t.Error("entry1 and entry2 must have different hashes")
	}
}

func TestBuildEntry_InvalidType(t *testing.T) {
	t.Parallel()

	_, err := BuildEntry(EntryBuildParams{
		Type:           EntryType("bogus"),
		TraceID:        "TRACE1",
		Actor:          Actor{Type: "agent", ID: "a1", Provenance: "mcp"},
		Payload:        json.RawMessage(`{}`),
		SpecVersion:    "0.3.0",
		CanonVersion:   "1.0.0",
		AdapterVersion: "k8s-1.0.0",
	})
	if err == nil {
		t.Fatal("expected error for invalid entry type, got nil")
	}
}
