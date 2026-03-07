package signal

import (
	"testing"
	"time"
)

func TestRepairLoop_FailThenFixThenSucceed(t *testing.T) {
	t.Parallel()

	now := time.Now()
	entries := []Entry{
		{
			EventID:        "p1",
			Timestamp:      now,
			ActorID:        "agent-1",
			IsPrescription: true,
			IntentDigest:   "intent-deploy-web",
			ArtifactDigest: "artifact-v1-bad",
		},
		{
			EventID:        "r1",
			Timestamp:      now.Add(1 * time.Second),
			IsReport:       true,
			PrescriptionID: "p1",
			ExitCode:       intPtr(1),
		},
		{
			EventID:        "p2",
			Timestamp:      now.Add(2 * time.Second),
			ActorID:        "agent-1",
			IsPrescription: true,
			IntentDigest:   "intent-deploy-web",
			ArtifactDigest: "artifact-v2-fixed",
		},
		{
			EventID:        "r2",
			Timestamp:      now.Add(3 * time.Second),
			IsReport:       true,
			PrescriptionID: "p2",
			ExitCode:       intPtr(0),
		},
	}

	result := DetectRepairLoop(entries)
	if result.Count != 1 {
		t.Fatalf("expected 1 repair, got %d", result.Count)
	}
	if len(result.EventIDs) != 1 || result.EventIDs[0] != "p2" {
		t.Fatalf("expected event p2, got %v", result.EventIDs)
	}
}

func TestRepairLoop_RetryIsNotRepair(t *testing.T) {
	t.Parallel()

	now := time.Now()
	entries := []Entry{
		{
			EventID:        "p1",
			Timestamp:      now,
			ActorID:        "agent-1",
			IsPrescription: true,
			IntentDigest:   "intent-x",
			ArtifactDigest: "same-artifact",
		},
		{
			EventID:        "r1",
			Timestamp:      now.Add(1 * time.Second),
			IsReport:       true,
			PrescriptionID: "p1",
			ExitCode:       intPtr(1),
		},
		{
			EventID:        "p2",
			Timestamp:      now.Add(2 * time.Second),
			ActorID:        "agent-1",
			IsPrescription: true,
			IntentDigest:   "intent-x",
			ArtifactDigest: "same-artifact",
		},
		{
			EventID:        "r2",
			Timestamp:      now.Add(3 * time.Second),
			IsReport:       true,
			PrescriptionID: "p2",
			ExitCode:       intPtr(0),
		},
	}

	result := DetectRepairLoop(entries)
	if result.Count != 0 {
		t.Fatalf("retry with same artifact should not count as repair, got %d", result.Count)
	}
}

func TestRepairLoop_NoFailureMeansNoRepair(t *testing.T) {
	t.Parallel()

	now := time.Now()
	entries := []Entry{
		{
			EventID:        "p1",
			Timestamp:      now,
			ActorID:        "agent-1",
			IsPrescription: true,
			IntentDigest:   "intent-x",
			ArtifactDigest: "art-a",
		},
		{
			EventID:        "r1",
			Timestamp:      now.Add(1 * time.Second),
			IsReport:       true,
			PrescriptionID: "p1",
			ExitCode:       intPtr(0),
		},
		{
			EventID:        "p2",
			Timestamp:      now.Add(2 * time.Second),
			ActorID:        "agent-1",
			IsPrescription: true,
			IntentDigest:   "intent-x",
			ArtifactDigest: "art-b",
		},
		{
			EventID:        "r2",
			Timestamp:      now.Add(3 * time.Second),
			IsReport:       true,
			PrescriptionID: "p2",
			ExitCode:       intPtr(0),
		},
	}

	result := DetectRepairLoop(entries)
	if result.Count != 0 {
		t.Fatalf("two successes with different artifacts is not repair, got %d", result.Count)
	}
}

func TestRepairLoop_EmptyEntries(t *testing.T) {
	t.Parallel()
	result := DetectRepairLoop(nil)
	if result.Count != 0 {
		t.Fatalf("expected 0, got %d", result.Count)
	}
}
