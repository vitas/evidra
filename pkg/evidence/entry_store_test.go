package evidence

import (
	"encoding/json"
	"testing"
)

func buildTestEntry(t *testing.T, typ EntryType, previousHash string) EvidenceEntry {
	t.Helper()
	payload, err := json.Marshal(PrescriptionPayload{
		PrescriptionID:  "presc-001",
		CanonicalAction: json.RawMessage(`{"tool":"kubectl","operation":"apply"}`),
		RiskLevel:       "low",
		TTLMs:           DefaultTTLMs,
		CanonSource:     "test",
	})
	if err != nil {
		t.Fatalf("marshal test payload: %v", err)
	}
	entry, err := BuildEntry(EntryBuildParams{
		Type:           typ,
		TraceID:        GenerateTraceID(),
		Actor:          Actor{Type: "agent", ID: "test-agent", Provenance: "unit-test"},
		Payload:        payload,
		PreviousHash:   previousHash,
		SpecVersion:    "0.3.0",
		CanonVersion:   "1",
		AdapterVersion: "k8s-1",
	})
	if err != nil {
		t.Fatalf("BuildEntry: %v", err)
	}
	return entry
}

func TestAppendEntryAtPath_Basic(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	entry := buildTestEntry(t, EntryTypePrescribe, "")

	if err := AppendEntryAtPath(dir, entry); err != nil {
		t.Fatalf("AppendEntryAtPath: %v", err)
	}

	entries, err := ReadAllEntriesAtPath(dir)
	if err != nil {
		t.Fatalf("ReadAllEntriesAtPath: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}

	got := entries[0]
	if got.EntryID != entry.EntryID {
		t.Errorf("entry_id mismatch: got %s, want %s", got.EntryID, entry.EntryID)
	}
	if got.Hash != entry.Hash {
		t.Errorf("hash mismatch: got %s, want %s", got.Hash, entry.Hash)
	}
	if got.Type != EntryTypePrescribe {
		t.Errorf("type mismatch: got %s, want %s", got.Type, EntryTypePrescribe)
	}
	if got.Actor.ID != "test-agent" {
		t.Errorf("actor.id mismatch: got %s, want test-agent", got.Actor.ID)
	}
}

func TestReadAllEntriesAtPath(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	e1 := buildTestEntry(t, EntryTypePrescribe, "")
	if err := AppendEntryAtPath(dir, e1); err != nil {
		t.Fatalf("append e1: %v", err)
	}

	e2 := buildTestEntry(t, EntryTypeReport, e1.Hash)
	if err := AppendEntryAtPath(dir, e2); err != nil {
		t.Fatalf("append e2: %v", err)
	}

	e3 := buildTestEntry(t, EntryTypeSignal, e2.Hash)
	if err := AppendEntryAtPath(dir, e3); err != nil {
		t.Fatalf("append e3: %v", err)
	}

	entries, err := ReadAllEntriesAtPath(dir)
	if err != nil {
		t.Fatalf("ReadAllEntriesAtPath: %v", err)
	}
	if len(entries) != 3 {
		t.Fatalf("expected 3 entries, got %d", len(entries))
	}

	// Verify order is preserved.
	if entries[0].EntryID != e1.EntryID {
		t.Errorf("entries[0] entry_id mismatch: got %s, want %s", entries[0].EntryID, e1.EntryID)
	}
	if entries[1].EntryID != e2.EntryID {
		t.Errorf("entries[1] entry_id mismatch: got %s, want %s", entries[1].EntryID, e2.EntryID)
	}
	if entries[2].EntryID != e3.EntryID {
		t.Errorf("entries[2] entry_id mismatch: got %s, want %s", entries[2].EntryID, e3.EntryID)
	}
}

func TestLastHashAtPath(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	e1 := buildTestEntry(t, EntryTypePrescribe, "")
	if err := AppendEntryAtPath(dir, e1); err != nil {
		t.Fatalf("append e1: %v", err)
	}

	hash1, err := LastHashAtPath(dir)
	if err != nil {
		t.Fatalf("LastHashAtPath after e1: %v", err)
	}
	if hash1 != e1.Hash {
		t.Errorf("after e1: got %s, want %s", hash1, e1.Hash)
	}

	e2 := buildTestEntry(t, EntryTypeReport, e1.Hash)
	if err := AppendEntryAtPath(dir, e2); err != nil {
		t.Fatalf("append e2: %v", err)
	}

	hash2, err := LastHashAtPath(dir)
	if err != nil {
		t.Fatalf("LastHashAtPath after e2: %v", err)
	}
	if hash2 != e2.Hash {
		t.Errorf("after e2: got %s, want %s", hash2, e2.Hash)
	}
}

func TestFindEntryByID(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	e1 := buildTestEntry(t, EntryTypePrescribe, "")
	if err := AppendEntryAtPath(dir, e1); err != nil {
		t.Fatalf("append e1: %v", err)
	}

	e2 := buildTestEntry(t, EntryTypeReport, e1.Hash)
	if err := AppendEntryAtPath(dir, e2); err != nil {
		t.Fatalf("append e2: %v", err)
	}

	e3 := buildTestEntry(t, EntryTypeSignal, e2.Hash)
	if err := AppendEntryAtPath(dir, e3); err != nil {
		t.Fatalf("append e3: %v", err)
	}

	got, found, err := FindEntryByID(dir, e2.EntryID)
	if err != nil {
		t.Fatalf("FindEntryByID: %v", err)
	}
	if !found {
		t.Fatal("expected to find entry, but not found")
	}
	if got.EntryID != e2.EntryID {
		t.Errorf("entry_id mismatch: got %s, want %s", got.EntryID, e2.EntryID)
	}
	if got.Hash != e2.Hash {
		t.Errorf("hash mismatch: got %s, want %s", got.Hash, e2.Hash)
	}
}

func TestFindEntryByID_NotFound(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	e1 := buildTestEntry(t, EntryTypePrescribe, "")
	if err := AppendEntryAtPath(dir, e1); err != nil {
		t.Fatalf("append e1: %v", err)
	}

	_, found, err := FindEntryByID(dir, "nonexistent-id")
	if err != nil {
		t.Fatalf("FindEntryByID: %v", err)
	}
	if found {
		t.Error("expected not found, but found")
	}
}
