package signal

import (
	"testing"
	"time"
)

func TestDetectProtocolViolations_UnreportedPrescription(t *testing.T) {
	t.Parallel()

	entries := []Entry{
		{EventID: "P1", IsPrescription: true},
		{EventID: "P2", IsPrescription: true},
		{EventID: "R1", IsReport: true, PrescriptionID: "P1"},
	}
	result := DetectProtocolViolations(entries)
	if result.Count != 1 {
		t.Errorf("count = %d, want 1 (P2 unreported)", result.Count)
	}
	assertEventID(t, result.EventIDs, "P2")
}

func TestDetectProtocolViolations_UnprescribedReport(t *testing.T) {
	t.Parallel()

	entries := []Entry{
		{EventID: "R1", IsReport: true, PrescriptionID: "UNKNOWN"},
	}
	result := DetectProtocolViolations(entries)
	if result.Count != 1 {
		t.Errorf("count = %d, want 1 (orphan report)", result.Count)
	}
}

func TestDetectProtocolViolations_AllMatched(t *testing.T) {
	t.Parallel()

	entries := []Entry{
		{EventID: "P1", IsPrescription: true},
		{EventID: "R1", IsReport: true, PrescriptionID: "P1"},
	}
	result := DetectProtocolViolations(entries)
	if result.Count != 0 {
		t.Errorf("count = %d, want 0", result.Count)
	}
}

func TestDetectArtifactDrift_DriftDetected(t *testing.T) {
	t.Parallel()

	entries := []Entry{
		{EventID: "P1", IsPrescription: true, ArtifactDigest: "abc123"},
		{EventID: "R1", IsReport: true, PrescriptionID: "P1", ArtifactDigest: "xyz789"},
	}
	result := DetectArtifactDrift(entries)
	if result.Count != 1 {
		t.Errorf("count = %d, want 1", result.Count)
	}
}

func TestDetectArtifactDrift_NoDrift(t *testing.T) {
	t.Parallel()

	entries := []Entry{
		{EventID: "P1", IsPrescription: true, ArtifactDigest: "abc123"},
		{EventID: "R1", IsReport: true, PrescriptionID: "P1", ArtifactDigest: "abc123"},
	}
	result := DetectArtifactDrift(entries)
	if result.Count != 0 {
		t.Errorf("count = %d, want 0", result.Count)
	}
}

func TestDetectRetryLoops_LoopDetected(t *testing.T) {
	t.Parallel()

	now := time.Now()
	entries := []Entry{
		{EventID: "P1", IsPrescription: true, IntentDigest: "abc", ShapeHash: "def", Timestamp: now},
		{EventID: "P2", IsPrescription: true, IntentDigest: "abc", ShapeHash: "def", Timestamp: now.Add(1 * time.Minute)},
		{EventID: "P3", IsPrescription: true, IntentDigest: "abc", ShapeHash: "def", Timestamp: now.Add(2 * time.Minute)},
	}
	result := DetectRetryLoops(entries)
	if result.Count != 3 {
		t.Errorf("count = %d, want 3", result.Count)
	}
}

func TestDetectRetryLoops_BelowThreshold(t *testing.T) {
	t.Parallel()

	now := time.Now()
	entries := []Entry{
		{EventID: "P1", IsPrescription: true, IntentDigest: "abc", ShapeHash: "def", Timestamp: now},
		{EventID: "P2", IsPrescription: true, IntentDigest: "abc", ShapeHash: "def", Timestamp: now.Add(1 * time.Minute)},
	}
	result := DetectRetryLoops(entries)
	if result.Count != 0 {
		t.Errorf("count = %d, want 0", result.Count)
	}
}

func TestDetectRetryLoops_OutsideWindow(t *testing.T) {
	t.Parallel()

	now := time.Now()
	entries := []Entry{
		{EventID: "P1", IsPrescription: true, IntentDigest: "abc", ShapeHash: "def", Timestamp: now},
		{EventID: "P2", IsPrescription: true, IntentDigest: "abc", ShapeHash: "def", Timestamp: now.Add(30 * time.Minute)},
		{EventID: "P3", IsPrescription: true, IntentDigest: "abc", ShapeHash: "def", Timestamp: now.Add(60 * time.Minute)},
	}
	result := DetectRetryLoops(entries)
	if result.Count != 0 {
		t.Errorf("count = %d, want 0 (outside 10min window)", result.Count)
	}
}

func TestDetectBlastRadius_Destructive(t *testing.T) {
	t.Parallel()

	entries := []Entry{
		{EventID: "P1", IsPrescription: true, OperationClass: "destroy", ResourceCount: 15},
	}
	result := DetectBlastRadius(entries)
	if result.Count != 1 {
		t.Errorf("count = %d, want 1", result.Count)
	}
}

func TestDetectBlastRadius_BelowThreshold(t *testing.T) {
	t.Parallel()

	entries := []Entry{
		{EventID: "P1", IsPrescription: true, OperationClass: "destroy", ResourceCount: 5},
	}
	result := DetectBlastRadius(entries)
	if result.Count != 0 {
		t.Errorf("count = %d, want 0", result.Count)
	}
}

func TestDetectBlastRadius_Mutating(t *testing.T) {
	t.Parallel()

	entries := []Entry{
		{EventID: "P1", IsPrescription: true, OperationClass: "mutate", ResourceCount: 60},
	}
	result := DetectBlastRadius(entries)
	if result.Count != 1 {
		t.Errorf("count = %d, want 1", result.Count)
	}
}

func TestDetectNewScope_FirstOccurrences(t *testing.T) {
	t.Parallel()

	entries := []Entry{
		{EventID: "P1", IsPrescription: true, Tool: "kubectl", OperationClass: "mutate"},
		{EventID: "P2", IsPrescription: true, Tool: "kubectl", OperationClass: "mutate"},
		{EventID: "P3", IsPrescription: true, Tool: "terraform", OperationClass: "plan"},
	}
	result := DetectNewScope(entries)
	if result.Count != 2 {
		t.Errorf("count = %d, want 2 (kubectl.mutate + terraform.plan)", result.Count)
	}
	assertEventID(t, result.EventIDs, "P1")
	assertEventID(t, result.EventIDs, "P3")
}

func TestDetectNewScope_AllSame(t *testing.T) {
	t.Parallel()

	entries := []Entry{
		{EventID: "P1", IsPrescription: true, Tool: "kubectl", OperationClass: "mutate"},
		{EventID: "P2", IsPrescription: true, Tool: "kubectl", OperationClass: "mutate"},
	}
	result := DetectNewScope(entries)
	if result.Count != 1 {
		t.Errorf("count = %d, want 1", result.Count)
	}
}

func TestAllSignals_ReturnsAllFive(t *testing.T) {
	t.Parallel()

	results := AllSignals(nil)
	if len(results) != 5 {
		t.Fatalf("AllSignals returned %d results, want 5", len(results))
	}
	names := map[string]bool{}
	for _, r := range results {
		names[r.Name] = true
	}
	for _, want := range []string{"protocol_violation", "artifact_drift", "retry_loop", "blast_radius", "new_scope"} {
		if !names[want] {
			t.Errorf("missing signal %q", want)
		}
	}
}

func assertEventID(t *testing.T, ids []string, want string) {
	t.Helper()
	for _, id := range ids {
		if id == want {
			return
		}
	}
	t.Errorf("event IDs %v does not contain %q", ids, want)
}
