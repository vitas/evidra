package store

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"samebits.com/evidra/internal/analytics"
	testutil "samebits.com/evidra/internal/testutil"
	"samebits.com/evidra/pkg/evidence"
)

func TestCollectAnalyticsReplayEntries_PaginatesUntilExhausted(t *testing.T) {
	t.Parallel()

	calls := 0
	gotOffsets := make([]int, 0, 3)
	fetch := func(_ context.Context, _ string, opts ListOptions) ([]StoredEntry, int, error) {
		calls++
		gotOffsets = append(gotOffsets, opts.Offset)
		switch opts.Offset {
		case 0:
			return []StoredEntry{{ID: "a"}, {ID: "b"}}, 5, nil
		case 2:
			return []StoredEntry{{ID: "c"}, {ID: "d"}}, 5, nil
		case 4:
			return []StoredEntry{{ID: "e"}}, 5, nil
		default:
			t.Fatalf("unexpected offset %d", opts.Offset)
			return nil, 0, nil
		}
	}

	got, err := collectAnalyticsReplayEntries(context.Background(), "tenant-1", ListOptions{Period: "30d"}, 2, fetch)
	if err != nil {
		t.Fatalf("collectAnalyticsReplayEntries: %v", err)
	}
	if calls != 3 {
		t.Fatalf("calls = %d, want 3", calls)
	}
	if want := []int{0, 2, 4}; len(gotOffsets) != len(want) || gotOffsets[0] != want[0] || gotOffsets[1] != want[1] || gotOffsets[2] != want[2] {
		t.Fatalf("offsets = %v, want %v", gotOffsets, want)
	}
	if len(got) != 5 {
		t.Fatalf("entries len = %d, want 5", len(got))
	}
}

func TestListOptions_Defaults(t *testing.T) {
	t.Parallel()
	opts := ListOptions{}
	opts = opts.withDefaults()
	if opts.Limit != 100 {
		t.Fatalf("expected default limit=100, got %d", opts.Limit)
	}
}

func TestListOptions_MaxLimit(t *testing.T) {
	t.Parallel()
	opts := ListOptions{Limit: 5000}
	opts = opts.withDefaults()
	if opts.Limit != 1000 {
		t.Fatalf("expected max limit=1000, got %d", opts.Limit)
	}
}

func TestParsePeriod(t *testing.T) {
	t.Parallel()
	tests := []struct {
		input string
		want  time.Duration
	}{
		{"30d", 30 * 24 * time.Hour},
		{"7d", 7 * 24 * time.Hour},
		{"1d", 24 * time.Hour},
		{"", 30 * 24 * time.Hour},
	}
	for _, tt := range tests {
		got := parsePeriod(tt.input)
		if got != tt.want {
			t.Errorf("parsePeriod(%q) = %v, want %v", tt.input, got, tt.want)
		}
	}
}

func TestStoredEntriesToEvidenceEntries_RoundTripRawPayload(t *testing.T) {
	t.Parallel()

	entry := buildStoredTestPrescription(t, storedEntryFixture{
		actorID:        "agent-a",
		sessionID:      "session-a",
		tool:           "kubectl",
		scopeClass:     "production",
		artifactDigest: "artifact-a",
	})

	got, err := storedEntriesToEvidenceEntries([]StoredEntry{entry})
	if err != nil {
		t.Fatalf("storedEntriesToEvidenceEntries: %v", err)
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
	if got[0].Type != evidence.EntryTypePrescribe {
		t.Fatalf("type = %q, want %q", got[0].Type, evidence.EntryTypePrescribe)
	}
}

func TestComputeScorecardFromStoredEntries_AppliesFilters(t *testing.T) {
	t.Parallel()

	entries := buildStoredFixtureEntries(t)

	got, err := computeScorecardFromStoredEntries(entries, analytics.Filters{
		Period:        "30d",
		Actor:         "agent-a",
		Tool:          "kubectl",
		Scope:         "production",
		SessionID:     "session-a",
		MinOperations: 1,
	})
	if err != nil {
		t.Fatalf("computeScorecardFromStoredEntries: %v", err)
	}
	if got.ActorID != "agent-a" {
		t.Fatalf("actor_id = %q, want agent-a", got.ActorID)
	}
	if got.SessionID != "session-a" {
		t.Fatalf("session_id = %q, want session-a", got.SessionID)
	}
	if got.TotalOperations != 1 {
		t.Fatalf("total_operations = %d, want 1", got.TotalOperations)
	}
	if got.Period != "30d" {
		t.Fatalf("period = %q, want 30d", got.Period)
	}
}

func TestComputeExplainFromStoredEntries_AppliesFilters(t *testing.T) {
	t.Parallel()

	entries := buildStoredFixtureEntries(t)

	got, err := computeExplainFromStoredEntries(entries, analytics.Filters{
		Period:        "30d",
		Actor:         "agent-b",
		Tool:          "terraform",
		Scope:         "staging",
		SessionID:     "session-b",
		MinOperations: 1,
	})
	if err != nil {
		t.Fatalf("computeExplainFromStoredEntries: %v", err)
	}
	if got.TotalOps != 1 {
		t.Fatalf("total_operations = %d, want 1", got.TotalOps)
	}
	foundDrift := false
	for _, sig := range got.Signals {
		if sig.Signal == "artifact_drift" {
			if sig.Count != 1 {
				t.Fatalf("artifact_drift count = %d, want 1", sig.Count)
			}
			foundDrift = true
		}
	}
	if !foundDrift {
		t.Fatalf("artifact_drift signal missing: %+v", got.Signals)
	}
}

func TestComputeScorecardFromStoredEntries_MatchesCanonicalEvidence(t *testing.T) {
	t.Parallel()

	stored := buildStoredFixtureEntries(t)
	canonical := decodeStoredEntriesForParity(t, stored)
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
	got, err := computeScorecardFromStoredEntries(stored, filters)
	if err != nil {
		t.Fatalf("computeScorecardFromStoredEntries: %v", err)
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
	if got.Signals["artifact_drift"] != want.Signals["artifact_drift"] {
		t.Fatalf("artifact_drift = %d, want %d", got.Signals["artifact_drift"], want.Signals["artifact_drift"])
	}
	if got.Confidence != want.Confidence {
		t.Fatalf("confidence = %+v, want %+v", got.Confidence, want.Confidence)
	}
}

func TestComputeExplainFromStoredEntries_MatchesCanonicalEvidence(t *testing.T) {
	t.Parallel()

	stored := buildStoredFixtureEntries(t)
	canonical := decodeStoredEntriesForParity(t, stored)
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
	got, err := computeExplainFromStoredEntries(stored, filters)
	if err != nil {
		t.Fatalf("computeExplainFromStoredEntries: %v", err)
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
	if len(got.Signals) != len(want.Signals) {
		t.Fatalf("signals len = %d, want %d", len(got.Signals), len(want.Signals))
	}
	gotCounts := explainSignalCounts(got.Signals)
	wantCounts := explainSignalCounts(want.Signals)
	if gotCounts["artifact_drift"] != wantCounts["artifact_drift"] {
		t.Fatalf("artifact_drift = %d, want %d", gotCounts["artifact_drift"], wantCounts["artifact_drift"])
	}
}

type storedEntryFixture struct {
	actorID        string
	sessionID      string
	tool           string
	scopeClass     string
	artifactDigest string
}

func buildStoredFixtureEntries(t *testing.T) []StoredEntry {
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

	return []StoredEntry{prescribeA, reportA, prescribeB, reportB}
}

func decodeStoredEntriesForParity(t *testing.T, entries []StoredEntry) []evidence.EvidenceEntry {
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

func explainSignalCounts(details []analytics.SignalDetail) map[string]int {
	counts := make(map[string]int, len(details))
	for _, detail := range details {
		counts[detail.Signal] = detail.Count
	}
	return counts
}

func buildStoredTestPrescription(t *testing.T, fixture storedEntryFixture) StoredEntry {
	t.Helper()

	canonicalAction, err := json.Marshal(map[string]any{
		"tool":                fixture.tool,
		"operation":           "apply",
		"operation_class":     "mutate",
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
		ArtifactDigest: fixture.artifactDigest,
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

	return StoredEntry{
		ID:          entry.EntryID,
		TenantID:    "tenant-1",
		EntryType:   string(entry.Type),
		SessionID:   entry.SessionID,
		OperationID: entry.OperationID,
		Hash:        entry.Hash,
		Signature:   entry.Signature,
		Payload:     raw,
		CreatedAt:   entry.Timestamp,
	}
}

func buildStoredTestReport(t *testing.T, prescribe StoredEntry, fixture storedEntryFixture) StoredEntry {
	t.Helper()

	var prescribeEntry evidence.EvidenceEntry
	if err := json.Unmarshal(prescribe.Payload, &prescribeEntry); err != nil {
		t.Fatalf("unmarshal prescribe entry: %v", err)
	}
	exitCode := 1
	payload, err := json.Marshal(evidence.ReportPayload{
		ReportID:       "report-" + fixture.actorID,
		PrescriptionID: prescribeEntry.EntryID,
		ExitCode:       &exitCode,
		Verdict:        evidence.VerdictFailure,
	})
	if err != nil {
		t.Fatalf("marshal report payload: %v", err)
	}

	entry, err := evidence.BuildEntry(evidence.EntryBuildParams{
		Type:           evidence.EntryTypeReport,
		SessionID:      fixture.sessionID,
		TraceID:        "trace-" + fixture.actorID,
		Actor:          evidence.Actor{Type: "agent", ID: fixture.actorID, Provenance: "unit-test"},
		ArtifactDigest: fixture.artifactDigest,
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

	return StoredEntry{
		ID:          entry.EntryID,
		TenantID:    "tenant-1",
		EntryType:   string(entry.Type),
		SessionID:   entry.SessionID,
		OperationID: entry.OperationID,
		Hash:        entry.Hash,
		Signature:   entry.Signature,
		Payload:     raw,
		CreatedAt:   entry.Timestamp,
	}
}
