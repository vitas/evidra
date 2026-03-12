package analyticsdb_test

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"samebits.com/evidra/internal/analytics"
	"samebits.com/evidra/internal/analyticsdb"
	"samebits.com/evidra/internal/canon"
	"samebits.com/evidra/internal/store"
	testutil "samebits.com/evidra/internal/testutil"
	"samebits.com/evidra/pkg/evidence"
)

func TestEvidenceEntriesFromStoredRows_RoundTripRawPayload(t *testing.T) {
	t.Parallel()

	entry := buildStoredTestPrescription(t, storedEntryFixture{
		actorID:        "agent-a",
		sessionID:      "session-a",
		tool:           "kubectl",
		scopeClass:     "production",
		artifactDigest: "artifact-a",
	})

	got, err := analyticsdb.EvidenceEntriesFromStoredRows([]analyticsdb.StoredRow{{
		ID:      entry.ID,
		Payload: entry.Payload,
	}})
	if err != nil {
		t.Fatalf("EvidenceEntriesFromStoredRows: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(got))
	}
	if got[0].EntryID != entry.ID {
		t.Fatalf("entry_id = %q, want %q", got[0].EntryID, entry.ID)
	}
	if got[0].Actor.ID != "agent-a" {
		t.Fatalf("actor_id = %q, want agent-a", got[0].Actor.ID)
	}
	if got[0].SessionID != "session-a" {
		t.Fatalf("session_id = %q, want session-a", got[0].SessionID)
	}
}

func TestComputeScorecardFromStoredRows_MatchesCanonicalEvidence(t *testing.T) {
	t.Parallel()

	storedEntries := buildStoredFixtureEntries(t)
	rows := toStoredRows(storedEntries)
	canonical := decodeStoredEntriesForParity(t, storedEntries)
	filters := analytics.Filters{
		Period:        "30d",
		Actor:         "agent-b",
		Tool:          "terraform",
		Scope:         "staging",
		SessionID:     "session-b",
		MinOperations: 1,
	}

	want, err := analytics.ComputeScorecard(canonical, filters)
	if err != nil {
		t.Fatalf("analytics.ComputeScorecard: %v", err)
	}
	got, err := analyticsdb.ComputeScorecardFromStoredRows(rows, filters)
	if err != nil {
		t.Fatalf("ComputeScorecardFromStoredRows: %v", err)
	}

	if got.Score != want.Score {
		t.Fatalf("score = %.2f, want %.2f", got.Score, want.Score)
	}
	if got.Band != want.Band {
		t.Fatalf("band = %q, want %q", got.Band, want.Band)
	}
	if got.TotalOperations != want.TotalOperations {
		t.Fatalf("total_operations = %d, want %d", got.TotalOperations, want.TotalOperations)
	}
}

func TestComputeExplainFromStoredRows_MatchesCanonicalEvidence(t *testing.T) {
	t.Parallel()

	storedEntries := buildStoredFixtureEntries(t)
	rows := toStoredRows(storedEntries)
	canonical := decodeStoredEntriesForParity(t, storedEntries)
	filters := analytics.Filters{
		Period:        "30d",
		Actor:         "agent-b",
		Tool:          "terraform",
		Scope:         "staging",
		SessionID:     "session-b",
		MinOperations: 1,
	}

	want, err := analytics.ComputeExplain(canonical, filters)
	if err != nil {
		t.Fatalf("analytics.ComputeExplain: %v", err)
	}
	got, err := analyticsdb.ComputeExplainFromStoredRows(rows, filters)
	if err != nil {
		t.Fatalf("ComputeExplainFromStoredRows: %v", err)
	}

	if got.Score != want.Score {
		t.Fatalf("score = %.2f, want %.2f", got.Score, want.Score)
	}
	if got.Band != want.Band {
		t.Fatalf("band = %q, want %q", got.Band, want.Band)
	}
	if got.TotalOps != want.TotalOps {
		t.Fatalf("total_operations = %d, want %d", got.TotalOps, want.TotalOps)
	}
}

func TestComputeScorecardFromStoredRows_ReplaysChronologically(t *testing.T) {
	t.Parallel()

	storedEntries := buildChronologySensitiveStoredEntries(t)
	rows := toStoredRows([]store.StoredEntry{
		storedEntries[3],
		storedEntries[2],
		storedEntries[1],
		storedEntries[0],
	})
	canonical := decodeStoredEntriesForParity(t, storedEntries)
	filters := analytics.Filters{
		Period:        "30d",
		Actor:         "agent-a",
		Tool:          "kubectl",
		Scope:         "production",
		SessionID:     "session-a",
		MinOperations: 1,
	}

	want, err := analytics.ComputeScorecard(canonical, filters)
	if err != nil {
		t.Fatalf("analytics.ComputeScorecard: %v", err)
	}
	got, err := analyticsdb.ComputeScorecardFromStoredRows(rows, filters)
	if err != nil {
		t.Fatalf("ComputeScorecardFromStoredRows: %v", err)
	}

	if got.Score != want.Score {
		t.Fatalf("score = %.2f, want %.2f", got.Score, want.Score)
	}
	if got.Signals["risk_escalation"] != want.Signals["risk_escalation"] {
		t.Fatalf("risk_escalation = %d, want %d", got.Signals["risk_escalation"], want.Signals["risk_escalation"])
	}
}

func TestEvidenceEntriesFromStoredRows_InvalidPayloadIncludesRowID(t *testing.T) {
	t.Parallel()

	_, err := analyticsdb.EvidenceEntriesFromStoredRows([]analyticsdb.StoredRow{{
		ID:      "broken-row",
		Payload: json.RawMessage(`{"broken":`),
	}})
	if err == nil {
		t.Fatal("expected decode error")
	}
	if got := err.Error(); got == "" || !strings.Contains(got, "broken-row") {
		t.Fatalf("error = %q, want row id included", got)
	}
}

type storedEntryFixture struct {
	actorID        string
	sessionID      string
	tool           string
	operation      string
	operationClass string
	scopeClass     string
	artifactDigest string
}

func toStoredRows(entries []store.StoredEntry) []analyticsdb.StoredRow {
	rows := make([]analyticsdb.StoredRow, 0, len(entries))
	for _, entry := range entries {
		rows = append(rows, analyticsdb.StoredRow{
			ID:      entry.ID,
			Payload: entry.Payload,
		})
	}
	return rows
}

func buildStoredFixtureEntries(t *testing.T) []store.StoredEntry {
	t.Helper()

	prescribeA := buildStoredTestPrescription(t, storedEntryFixture{
		actorID:        "agent-a",
		sessionID:      "session-a",
		tool:           "kubectl",
		scopeClass:     "production",
		artifactDigest: "artifact-a",
	})
	reportA := buildStoredTestReport(t, prescribeA, storedEntryFixture{
		actorID:        "agent-a",
		sessionID:      "session-a",
		tool:           "kubectl",
		scopeClass:     "production",
		artifactDigest: "artifact-a",
	})

	prescribeB := buildStoredTestPrescription(t, storedEntryFixture{
		actorID:        "agent-b",
		sessionID:      "session-b",
		tool:           "terraform",
		scopeClass:     "staging",
		artifactDigest: "artifact-b",
	})
	reportB := buildStoredTestReport(t, prescribeB, storedEntryFixture{
		actorID:        "agent-b",
		sessionID:      "session-b",
		tool:           "terraform",
		scopeClass:     "staging",
		artifactDigest: "artifact-b-drifted",
	})

	return []store.StoredEntry{prescribeA, reportA, prescribeB, reportB}
}

func buildChronologySensitiveStoredEntries(t *testing.T) []store.StoredEntry {
	t.Helper()

	base := time.Date(2026, time.March, 12, 8, 0, 0, 0, time.UTC)
	return []store.StoredEntry{
		buildStoredTimedPrescription(t, storedEntryFixture{
			actorID:        "agent-a",
			sessionID:      "session-a",
			tool:           "kubectl",
			operation:      "get",
			operationClass: "read",
			scopeClass:     "production",
			artifactDigest: "artifact-1",
		}, base),
		buildStoredTimedPrescription(t, storedEntryFixture{
			actorID:        "agent-a",
			sessionID:      "session-a",
			tool:           "kubectl",
			operation:      "get",
			operationClass: "read",
			scopeClass:     "production",
			artifactDigest: "artifact-2",
		}, base.Add(1*time.Minute)),
		buildStoredTimedPrescription(t, storedEntryFixture{
			actorID:        "agent-a",
			sessionID:      "session-a",
			tool:           "kubectl",
			operation:      "get",
			operationClass: "read",
			scopeClass:     "production",
			artifactDigest: "artifact-3",
		}, base.Add(2*time.Minute)),
		buildStoredTimedPrescription(t, storedEntryFixture{
			actorID:        "agent-a",
			sessionID:      "session-a",
			tool:           "kubectl",
			operation:      "apply",
			operationClass: "mutate",
			scopeClass:     "production",
			artifactDigest: "artifact-4",
		}, base.Add(3*time.Minute)),
	}
}

func decodeStoredEntriesForParity(t *testing.T, entries []store.StoredEntry) []evidence.EvidenceEntry {
	t.Helper()

	decoded := make([]evidence.EvidenceEntry, 0, len(entries))
	for _, entry := range entries {
		var raw evidence.EvidenceEntry
		if err := json.Unmarshal(entry.Payload, &raw); err != nil {
			t.Fatalf("decode stored payload %s: %v", entry.ID, err)
		}
		decoded = append(decoded, raw)
	}
	return decoded
}

func buildStoredTestPrescription(t *testing.T, fixture storedEntryFixture) store.StoredEntry {
	t.Helper()

	operation := fixture.operation
	if operation == "" {
		operation = "apply"
	}
	operationClass := fixture.operationClass
	if operationClass == "" {
		operationClass = "mutate"
	}

	canonicalAction, err := json.Marshal(map[string]any{
		"tool":                fixture.tool,
		"operation":           operation,
		"operation_class":     operationClass,
		"scope_class":         fixture.scopeClass,
		"resource_count":      1,
		"resource_shape_hash": "shape-" + fixture.scopeClass,
	})
	if err != nil {
		t.Fatalf("marshal canonical action: %v", err)
	}
	payload, err := json.Marshal(evidence.PrescriptionPayload{
		PrescriptionID:  "presc-" + fixture.actorID,
		CanonicalAction: canonicalAction,
		RiskLevel:       "medium",
		RiskDetails:     []string{"risk.example"},
		TTLMs:           evidence.DefaultTTLMs,
		CanonSource:     "unit-test",
	})
	if err != nil {
		t.Fatalf("marshal prescription payload: %v", err)
	}

	entry, err := evidence.BuildEntry(evidence.EntryBuildParams{
		Type:           evidence.EntryTypePrescribe,
		SessionID:      fixture.sessionID,
		TraceID:        "trace-" + fixture.actorID,
		Actor:          evidence.Actor{Type: "agent", ID: fixture.actorID, Provenance: "unit-test"},
		ArtifactDigest: canon.SHA256Hex([]byte(fixture.artifactDigest)),
		Payload:        payload,
		SpecVersion:    "0.4.6",
		CanonVersion:   "k8s/v1",
		AdapterVersion: "unit-test",
		ScoringVersion: "v1.1.0",
		Signer:         testutil.TestSigner(t),
	})
	if err != nil {
		t.Fatalf("BuildEntry prescribe: %v", err)
	}

	raw, err := json.Marshal(entry)
	if err != nil {
		t.Fatalf("marshal entry: %v", err)
	}

	return store.StoredEntry{
		ID:          entry.EntryID,
		TenantID:    "tenant-1",
		EntryType:   string(entry.Type),
		SessionID:   entry.SessionID,
		OperationID: entry.OperationID,
		Hash:        entry.Hash,
		Signature:   entry.Signature,
		Payload:     raw,
	}
}

func buildStoredTimedPrescription(t *testing.T, fixture storedEntryFixture, ts time.Time) store.StoredEntry {
	t.Helper()

	entry := buildStoredTestPrescription(t, fixture)

	var raw evidence.EvidenceEntry
	if err := json.Unmarshal(entry.Payload, &raw); err != nil {
		t.Fatalf("decode timed prescription payload: %v", err)
	}
	raw.Timestamp = ts

	payload, err := json.Marshal(raw)
	if err != nil {
		t.Fatalf("marshal timed prescription payload: %v", err)
	}
	entry.Payload = payload
	return entry
}

func buildStoredTestReport(t *testing.T, prescribe store.StoredEntry, fixture storedEntryFixture) store.StoredEntry {
	t.Helper()

	var prescribeEntry evidence.EvidenceEntry
	if err := json.Unmarshal(prescribe.Payload, &prescribeEntry); err != nil {
		t.Fatalf("unmarshal prescribe entry: %v", err)
	}

	payload, err := json.Marshal(evidence.ReportPayload{
		ReportID:       "report-" + fixture.actorID,
		PrescriptionID: prescribeEntry.EntryID,
		Verdict:        evidence.VerdictFailure,
		ExitCode:       intPtr(1),
	})
	if err != nil {
		t.Fatalf("marshal report payload: %v", err)
	}

	entry, err := evidence.BuildEntry(evidence.EntryBuildParams{
		Type:           evidence.EntryTypeReport,
		SessionID:      fixture.sessionID,
		TraceID:        "trace-" + fixture.actorID,
		Actor:          evidence.Actor{Type: "agent", ID: fixture.actorID, Provenance: "unit-test"},
		ArtifactDigest: canon.SHA256Hex([]byte(fixture.artifactDigest)),
		Payload:        payload,
		PreviousHash:   prescribeEntry.Hash,
		SpecVersion:    "0.4.6",
		CanonVersion:   "k8s/v1",
		AdapterVersion: "unit-test",
		ScoringVersion: "v1.1.0",
		Signer:         testutil.TestSigner(t),
	})
	if err != nil {
		t.Fatalf("BuildEntry report: %v", err)
	}

	raw, err := json.Marshal(entry)
	if err != nil {
		t.Fatalf("marshal entry: %v", err)
	}

	return store.StoredEntry{
		ID:          entry.EntryID,
		TenantID:    "tenant-1",
		EntryType:   string(entry.Type),
		SessionID:   entry.SessionID,
		OperationID: entry.OperationID,
		Hash:        entry.Hash,
		Signature:   entry.Signature,
		Payload:     raw,
	}
}

func intPtr(v int) *int { return &v }
