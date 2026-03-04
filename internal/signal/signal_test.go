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

func TestDetectProtocolViolationEvents_DuplicateReport(t *testing.T) {
	t.Parallel()

	entries := []Entry{
		{EventID: "P1", IsPrescription: true},
		{EventID: "R1", IsReport: true, PrescriptionID: "P1"},
		{EventID: "R2", IsReport: true, PrescriptionID: "P1"},
	}
	events := DetectProtocolViolationEvents(entries)
	assertSubSignal(t, events, "duplicate_report")
}

func TestDetectProtocolViolationEvents_CrossActorReport(t *testing.T) {
	t.Parallel()

	entries := []Entry{
		{EventID: "P1", IsPrescription: true, ActorID: "alice"},
		{EventID: "R1", IsReport: true, PrescriptionID: "P1", ActorID: "bob"},
	}
	events := DetectProtocolViolationEvents(entries)
	assertSubSignal(t, events, "cross_actor_report")
}

func TestDetectProtocolViolationEvents_UnprescribedAction(t *testing.T) {
	t.Parallel()

	entries := []Entry{
		{EventID: "R1", IsReport: true, PrescriptionID: "GHOST"},
	}
	events := DetectProtocolViolationEvents(entries)
	assertSubSignal(t, events, "unprescribed_action")
}

func TestDetectUnreported_TTLExpired(t *testing.T) {
	t.Parallel()

	entries := []Entry{
		{EventID: "P1", IsPrescription: true, Timestamp: time.Now().Add(-20 * time.Minute)},
	}
	events := DetectUnreported(entries, DefaultTTL)
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
	if events[0].SubSignal != "stalled_operation" {
		t.Errorf("sub_signal = %q, want stalled_operation", events[0].SubSignal)
	}
}

func TestDetectUnreported_CrashBeforeReport(t *testing.T) {
	t.Parallel()

	old := time.Now().Add(-20 * time.Minute)
	entries := []Entry{
		{EventID: "R0", IsReport: true, PrescriptionID: "P0", ActorID: "alice", ExitCode: intPtr(1), Timestamp: old.Add(-5 * time.Minute)},
		{EventID: "P1", IsPrescription: true, ActorID: "alice", Timestamp: old},
	}
	events := DetectUnreported(entries, DefaultTTL)
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
	if events[0].SubSignal != "crash_before_report" {
		t.Errorf("sub_signal = %q, want crash_before_report", events[0].SubSignal)
	}
}

func TestDetectUnreported_WithinTTL(t *testing.T) {
	t.Parallel()

	entries := []Entry{
		{EventID: "P1", IsPrescription: true, Timestamp: time.Now().Add(-5 * time.Minute)},
	}
	events := DetectUnreported(entries, DefaultTTL)
	if len(events) != 0 {
		t.Errorf("expected 0 events within TTL, got %d", len(events))
	}
}

func TestDetectUnreported_StalledOperation(t *testing.T) {
	t.Parallel()

	old := time.Now().Add(-20 * time.Minute)
	entries := []Entry{
		{EventID: "P1", IsPrescription: true, Timestamp: old, ActorID: "alice"},
		{EventID: "P2", IsPrescription: true, Timestamp: old.Add(5 * time.Minute), ActorID: "alice"},
	}
	events := DetectUnreported(entries, DefaultTTL)
	// P1 should be stalled_operation (alice has later activity)
	for _, e := range events {
		if e.EntryRef == "P1" {
			if e.SubSignal != "stalled_operation" {
				t.Errorf("P1 sub_signal = %q, want stalled_operation", e.SubSignal)
			}
			return
		}
	}
	t.Error("P1 not found in events")
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

func intPtr(i int) *int { return &i }

func TestDetectRetryLoops_LoopDetected(t *testing.T) {
	t.Parallel()

	now := time.Now()
	entries := []Entry{
		{EventID: "P1", IsPrescription: true, ActorID: "alice", IntentDigest: "abc", ShapeHash: "def", Timestamp: now},
		{EventID: "R1", IsReport: true, PrescriptionID: "P1", ExitCode: intPtr(1), Timestamp: now.Add(30 * time.Second)},
		{EventID: "P2", IsPrescription: true, ActorID: "alice", IntentDigest: "abc", ShapeHash: "def", Timestamp: now.Add(1 * time.Minute)},
		{EventID: "P3", IsPrescription: true, ActorID: "alice", IntentDigest: "abc", ShapeHash: "def", Timestamp: now.Add(2 * time.Minute)},
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
		{EventID: "P1", IsPrescription: true, ActorID: "alice", IntentDigest: "abc", ShapeHash: "def", Timestamp: now},
		{EventID: "R1", IsReport: true, PrescriptionID: "P1", ExitCode: intPtr(1), Timestamp: now.Add(30 * time.Second)},
		{EventID: "P2", IsPrescription: true, ActorID: "alice", IntentDigest: "abc", ShapeHash: "def", Timestamp: now.Add(1 * time.Minute)},
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
		{EventID: "P1", IsPrescription: true, ActorID: "alice", IntentDigest: "abc", ShapeHash: "def", Timestamp: now},
		{EventID: "R1", IsReport: true, PrescriptionID: "P1", ExitCode: intPtr(1), Timestamp: now.Add(30 * time.Second)},
		{EventID: "P2", IsPrescription: true, ActorID: "alice", IntentDigest: "abc", ShapeHash: "def", Timestamp: now.Add(35 * time.Minute)},
		{EventID: "P3", IsPrescription: true, ActorID: "alice", IntentDigest: "abc", ShapeHash: "def", Timestamp: now.Add(65 * time.Minute)},
	}
	result := DetectRetryLoops(entries)
	if result.Count != 0 {
		t.Errorf("count = %d, want 0 (outside 30min window)", result.Count)
	}
}

func TestRetryLoop_RequiresPriorFailure(t *testing.T) {
	t.Parallel()

	now := time.Now()
	entries := []Entry{
		{EventID: "P1", IsPrescription: true, ActorID: "alice", IntentDigest: "abc", ShapeHash: "def", Timestamp: now},
		{EventID: "R1", IsReport: true, PrescriptionID: "P1", ExitCode: intPtr(0), Timestamp: now.Add(30 * time.Second)},
		{EventID: "P2", IsPrescription: true, ActorID: "alice", IntentDigest: "abc", ShapeHash: "def", Timestamp: now.Add(1 * time.Minute)},
		{EventID: "R2", IsReport: true, PrescriptionID: "P2", ExitCode: intPtr(0), Timestamp: now.Add(90 * time.Second)},
		{EventID: "P3", IsPrescription: true, ActorID: "alice", IntentDigest: "abc", ShapeHash: "def", Timestamp: now.Add(2 * time.Minute)},
		{EventID: "R3", IsReport: true, PrescriptionID: "P3", ExitCode: intPtr(0), Timestamp: now.Add(150 * time.Second)},
	}
	result := DetectRetryLoops(entries)
	if result.Count != 0 {
		t.Errorf("count = %d, want 0 (all successful, no retry)", result.Count)
	}
}

func TestRetryLoop_DetectsAfterFailure(t *testing.T) {
	t.Parallel()

	now := time.Now()
	entries := []Entry{
		{EventID: "P1", IsPrescription: true, ActorID: "alice", IntentDigest: "abc", ShapeHash: "def", Timestamp: now},
		{EventID: "R1", IsReport: true, PrescriptionID: "P1", ExitCode: intPtr(1), Timestamp: now.Add(30 * time.Second)},
		{EventID: "P2", IsPrescription: true, ActorID: "alice", IntentDigest: "abc", ShapeHash: "def", Timestamp: now.Add(1 * time.Minute)},
		{EventID: "P3", IsPrescription: true, ActorID: "alice", IntentDigest: "abc", ShapeHash: "def", Timestamp: now.Add(2 * time.Minute)},
	}
	result := DetectRetryLoops(entries)
	if result.Count != 3 {
		t.Errorf("count = %d, want 3 (P1 failed, P2+P3 are retries)", result.Count)
	}
	assertEventID(t, result.EventIDs, "P1")
	assertEventID(t, result.EventIDs, "P2")
	assertEventID(t, result.EventIDs, "P3")
}

func TestRetryLoop_ScopesByActor(t *testing.T) {
	t.Parallel()

	now := time.Now()
	entries := []Entry{
		{EventID: "P1", IsPrescription: true, ActorID: "alice", IntentDigest: "abc", ShapeHash: "def", Timestamp: now},
		{EventID: "R1", IsReport: true, PrescriptionID: "P1", ExitCode: intPtr(1), Timestamp: now.Add(30 * time.Second)},
		{EventID: "P2", IsPrescription: true, ActorID: "bob", IntentDigest: "abc", ShapeHash: "def", Timestamp: now.Add(1 * time.Minute)},
		{EventID: "P3", IsPrescription: true, ActorID: "charlie", IntentDigest: "abc", ShapeHash: "def", Timestamp: now.Add(2 * time.Minute)},
	}
	result := DetectRetryLoops(entries)
	if result.Count != 0 {
		t.Errorf("count = %d, want 0 (different actors)", result.Count)
	}
}

func TestRetryLoop_30MinWindow(t *testing.T) {
	t.Parallel()

	now := time.Now()
	entries := []Entry{
		{EventID: "P1", IsPrescription: true, ActorID: "alice", IntentDigest: "abc", ShapeHash: "def", Timestamp: now},
		{EventID: "R1", IsReport: true, PrescriptionID: "P1", ExitCode: intPtr(1), Timestamp: now.Add(30 * time.Second)},
		{EventID: "P2", IsPrescription: true, ActorID: "alice", IntentDigest: "abc", ShapeHash: "def", Timestamp: now.Add(15 * time.Minute)},
		{EventID: "P3", IsPrescription: true, ActorID: "alice", IntentDigest: "abc", ShapeHash: "def", Timestamp: now.Add(29 * time.Minute)},
	}
	result := DetectRetryLoops(entries)
	if result.Count != 3 {
		t.Errorf("count = %d, want 3 (within 30min window)", result.Count)
	}
}

func TestDetectBlastRadius_Destructive(t *testing.T) {
	t.Parallel()

	entries := []Entry{
		{EventID: "P1", IsPrescription: true, OperationClass: "destroy", ResourceCount: 6},
	}
	result := DetectBlastRadius(entries)
	if result.Count != 1 {
		t.Errorf("count = %d, want 1", result.Count)
	}
}

func TestDetectBlastRadius_BelowThreshold(t *testing.T) {
	t.Parallel()

	entries := []Entry{
		{EventID: "P1", IsPrescription: true, OperationClass: "destroy", ResourceCount: 4},
	}
	result := DetectBlastRadius(entries)
	if result.Count != 0 {
		t.Errorf("count = %d, want 0", result.Count)
	}
}

func TestDetectBlastRadius_MutateDoesNotFire(t *testing.T) {
	t.Parallel()

	entries := []Entry{
		{EventID: "P1", IsPrescription: true, OperationClass: "mutate", ResourceCount: 60},
	}
	result := DetectBlastRadius(entries)
	if result.Count != 0 {
		t.Errorf("count = %d, want 0 (mutate does not fire blast_radius)", result.Count)
	}
}

func TestBlastRadius_DestroyOnlyThreshold5(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		opClass string
		count   int
		want    int
	}{
		{"destroy above threshold", "destroy", 6, 1},
		{"mutate high count", "mutate", 60, 0},
		{"destroy below threshold", "destroy", 4, 0},
		{"destroy at threshold", "destroy", 5, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			entries := []Entry{
				{EventID: "P1", IsPrescription: true, OperationClass: tt.opClass, ResourceCount: tt.count},
			}
			result := DetectBlastRadius(entries)
			if result.Count != tt.want {
				t.Errorf("count = %d, want %d", result.Count, tt.want)
			}
		})
	}
}

func TestDetectNewScope_FirstOccurrences(t *testing.T) {
	t.Parallel()

	entries := []Entry{
		{EventID: "P1", IsPrescription: true, ActorID: "alice", Tool: "kubectl", OperationClass: "mutate", ScopeClass: "namespace"},
		{EventID: "P2", IsPrescription: true, ActorID: "alice", Tool: "kubectl", OperationClass: "mutate", ScopeClass: "namespace"},
		{EventID: "P3", IsPrescription: true, ActorID: "alice", Tool: "terraform", OperationClass: "plan", ScopeClass: "account"},
	}
	result := DetectNewScope(entries)
	if result.Count != 2 {
		t.Errorf("count = %d, want 2 (kubectl.mutate.namespace + terraform.plan.account)", result.Count)
	}
	assertEventID(t, result.EventIDs, "P1")
	assertEventID(t, result.EventIDs, "P3")
}

func TestDetectNewScope_AllSame(t *testing.T) {
	t.Parallel()

	entries := []Entry{
		{EventID: "P1", IsPrescription: true, ActorID: "alice", Tool: "kubectl", OperationClass: "mutate", ScopeClass: "namespace"},
		{EventID: "P2", IsPrescription: true, ActorID: "alice", Tool: "kubectl", OperationClass: "mutate", ScopeClass: "namespace"},
	}
	result := DetectNewScope(entries)
	if result.Count != 1 {
		t.Errorf("count = %d, want 1", result.Count)
	}
}

func TestNewScope_FullKey(t *testing.T) {
	t.Parallel()

	entries := []Entry{
		// Same tool+opClass but different actor → both new.
		{EventID: "P1", IsPrescription: true, ActorID: "alice", Tool: "kubectl", OperationClass: "mutate", ScopeClass: "namespace"},
		{EventID: "P2", IsPrescription: true, ActorID: "bob", Tool: "kubectl", OperationClass: "mutate", ScopeClass: "namespace"},
		// Same tool+opClass but different scopeClass → both new.
		{EventID: "P3", IsPrescription: true, ActorID: "alice", Tool: "kubectl", OperationClass: "mutate", ScopeClass: "cluster"},
		// Same full combo repeated → not new.
		{EventID: "P4", IsPrescription: true, ActorID: "alice", Tool: "kubectl", OperationClass: "mutate", ScopeClass: "namespace"},
	}
	result := DetectNewScope(entries)
	if result.Count != 3 {
		t.Errorf("count = %d, want 3 (P1, P2, P3 are new; P4 is repeat)", result.Count)
	}
	assertEventID(t, result.EventIDs, "P1")
	assertEventID(t, result.EventIDs, "P2")
	assertEventID(t, result.EventIDs, "P3")
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

func assertSubSignal(t *testing.T, events []SignalEvent, want string) {
	t.Helper()
	for _, e := range events {
		if e.SubSignal == want {
			return
		}
	}
	subs := make([]string, len(events))
	for i, e := range events {
		subs[i] = e.SubSignal
	}
	t.Errorf("sub_signals %v does not contain %q", subs, want)
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
