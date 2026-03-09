package signal

import (
	"testing"
	"time"
)

func toIntPtr(v int) *int { return &v }

func TestAllSignals_EndToEnd(t *testing.T) {
	t.Parallel()

	now := time.Now()
	entries := []Entry{
		// Normal prescribe/report pair.
		{EventID: "p1", IsPrescription: true, Tool: "kubectl", OperationClass: "mutate",
			ScopeClass: "staging", ActorID: "agent-1", IntentDigest: "sha256:a",
			ShapeHash: "sha256:s1", ArtifactDigest: "sha256:art1", ResourceCount: 1, Timestamp: now},
		{EventID: "r1", IsReport: true, PrescriptionID: "p1", ActorID: "agent-1",
			ArtifactDigest: "sha256:art1", ExitCode: toIntPtr(0), Timestamp: now.Add(1 * time.Minute)},

		// Unreported prescription (protocol violation -- will be stalled after TTL).
		{EventID: "p2", IsPrescription: true, Tool: "kubectl", OperationClass: "mutate",
			ScopeClass: "staging", ActorID: "agent-1", IntentDigest: "sha256:b",
			ArtifactDigest: "sha256:art2", ResourceCount: 1, Timestamp: now.Add(2 * time.Minute)},

		// Drift: report has different artifact_digest.
		{EventID: "p3", IsPrescription: true, Tool: "terraform", OperationClass: "mutate",
			ScopeClass: "production", ActorID: "agent-1", IntentDigest: "sha256:c",
			ArtifactDigest: "sha256:art3", ResourceCount: 1, Timestamp: now.Add(3 * time.Minute)},
		{EventID: "r3", IsReport: true, PrescriptionID: "p3", ActorID: "agent-1",
			ArtifactDigest: "sha256:art3_DIFFERENT", ExitCode: toIntPtr(0), Timestamp: now.Add(4 * time.Minute)},
	}

	results := AllSignals(entries, DefaultTTL)

	resultMap := make(map[string]SignalResult)
	for _, r := range results {
		resultMap[r.Name] = r
	}

	// Should have all 8 signals.
	if len(results) != 8 {
		t.Errorf("expected 8 signal results, got %d", len(results))
	}

	// artifact_drift should fire for p3/r3.
	if resultMap["artifact_drift"].Count == 0 {
		t.Error("expected artifact_drift for p3/r3 digest mismatch")
	}

	// Note: protocol_violation for p2 depends on TTL window.
	// p2 is only 2 minutes old, and DefaultTTL is 10 minutes,
	// so it won't be flagged as stalled yet. This is correct behavior.
}
