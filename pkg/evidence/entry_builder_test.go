package evidence

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestFormatDigest(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		input   string
		want    string
		wantErr bool
	}{
		{name: "bare hex gets prefix", input: strings.Repeat("a", 64), want: "sha256:" + strings.Repeat("a", 64)},
		{name: "already prefixed is idempotent", input: "sha256:" + strings.Repeat("b", 64), want: "sha256:" + strings.Repeat("b", 64)},
		{name: "empty stays empty", input: "", want: ""},
		{name: "rejects non hex", input: "not-a-digest", wantErr: true},
		{name: "rejects wrong length", input: "sha256:cafebabe", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := FormatDigest(tt.input)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("FormatDigest(%q) error = nil, want non-nil", tt.input)
				}
				return
			}
			if err != nil {
				t.Fatalf("FormatDigest(%q) unexpected error: %v", tt.input, err)
			}
			if got != tt.want {
				t.Errorf("FormatDigest(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestBuildEntry_RequiresSigner(t *testing.T) {
	t.Parallel()
	_, err := BuildEntry(EntryBuildParams{
		Type:    EntryTypePrescribe,
		TraceID: "trace-1",
		Payload: json.RawMessage(`{}`),
	})
	if err == nil {
		t.Fatal("expected error when Signer is nil")
	}
}

func TestBuildEntry_Prescribe(t *testing.T) {
	t.Parallel()
	signer := newTestSigner(t)

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
		IntentDigest:   strings.Repeat("d", 64),
		ArtifactDigest: "sha256:" + strings.Repeat("c", 64),
		Payload:        payload,
		SpecVersion:    "0.3.0",
		CanonVersion:   "1.0.0",
		AdapterVersion: "k8s-1.0.0",
		Signer:         signer,
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
	if entry.IntentDigest != "sha256:"+strings.Repeat("d", 64) {
		t.Errorf("intent_digest = %q, want %q", entry.IntentDigest, "sha256:"+strings.Repeat("d", 64))
	}
	if entry.ArtifactDigest != "sha256:"+strings.Repeat("c", 64) {
		t.Errorf("artifact_digest = %q, want %q", entry.ArtifactDigest, "sha256:"+strings.Repeat("c", 64))
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
	signer := newTestSigner(t)

	payload := json.RawMessage(`{"prescription_id":"rx-001"}`)

	entry1, err := BuildEntry(EntryBuildParams{
		Type:           EntryTypePrescribe,
		TraceID:        "TRACE1",
		Actor:          Actor{Type: "agent", ID: "a1", Provenance: "mcp"},
		Payload:        payload,
		SpecVersion:    "0.3.0",
		CanonVersion:   "1.0.0",
		AdapterVersion: "k8s-1.0.0",
		Signer:         signer,
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
		Signer:         signer,
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

func TestBuildEntry_OperationIDAndAttempt(t *testing.T) {
	t.Parallel()
	signer := newTestSigner(t)
	entry, err := BuildEntry(EntryBuildParams{
		Type:        EntryTypePrescribe,
		TraceID:     "trace-1",
		OperationID: "op-123",
		Attempt:     2,
		Payload:     json.RawMessage(`{}`),
		Signer:      signer,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if entry.OperationID != "op-123" {
		t.Errorf("expected OperationID op-123, got %s", entry.OperationID)
	}
	if entry.Attempt != 2 {
		t.Errorf("expected Attempt 2, got %d", entry.Attempt)
	}
}

func TestBuildEntry_RejectsInvalidDigestFormat(t *testing.T) {
	t.Parallel()

	signer := newTestSigner(t)
	_, err := BuildEntry(EntryBuildParams{
		Type:           EntryTypeReport,
		TraceID:        "trace-1",
		Actor:          Actor{Type: "agent", ID: "claude", Provenance: "mcp"},
		ArtifactDigest: "sha256:not-hex",
		Payload:        json.RawMessage(`{}`),
		Signer:         signer,
	})
	if err == nil {
		t.Fatal("expected invalid digest error")
	}
}

func TestBuildEntry_InvalidType(t *testing.T) {
	t.Parallel()
	signer := newTestSigner(t)

	_, err := BuildEntry(EntryBuildParams{
		Type:           EntryType("bogus"),
		TraceID:        "TRACE1",
		Actor:          Actor{Type: "agent", ID: "a1", Provenance: "mcp"},
		Payload:        json.RawMessage(`{}`),
		SpecVersion:    "0.3.0",
		CanonVersion:   "1.0.0",
		AdapterVersion: "k8s-1.0.0",
		Signer:         signer,
	})
	if err == nil {
		t.Fatal("expected error for invalid entry type, got nil")
	}
}

func TestBuildEntry_UsesProvidedEntryID(t *testing.T) {
	t.Parallel()

	signer := newTestSigner(t)
	entry, err := BuildEntry(EntryBuildParams{
		EntryID:        "01TESTENTRYID0000000000000",
		Type:           EntryTypePrescribe,
		TraceID:        "TRACE1",
		Actor:          Actor{Type: "agent", ID: "a1", Provenance: "mcp"},
		Payload:        json.RawMessage(`{}`),
		SpecVersion:    "0.3.0",
		CanonVersion:   "1.0.0",
		AdapterVersion: "k8s-1.0.0",
		Signer:         signer,
	})
	if err != nil {
		t.Fatalf("BuildEntry: %v", err)
	}
	if entry.EntryID != "01TESTENTRYID0000000000000" {
		t.Fatalf("entry_id=%q, want 01TESTENTRYID0000000000000", entry.EntryID)
	}
}
