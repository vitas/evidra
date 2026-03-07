package signal

import (
	"testing"
	"time"
)

func TestThrashing_ThreeDifferentIntentsAllFail(t *testing.T) {
	t.Parallel()

	now := time.Now()
	entries := []Entry{
		{
			EventID:        "p1",
			Timestamp:      now,
			ActorID:        "agent-1",
			IsPrescription: true,
			IntentDigest:   "intent-A",
			ArtifactDigest: "art-a",
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
			IntentDigest:   "intent-B",
			ArtifactDigest: "art-b",
		},
		{
			EventID:        "r2",
			Timestamp:      now.Add(3 * time.Second),
			IsReport:       true,
			PrescriptionID: "p2",
			ExitCode:       intPtr(1),
		},
		{
			EventID:        "p3",
			Timestamp:      now.Add(4 * time.Second),
			ActorID:        "agent-1",
			IsPrescription: true,
			IntentDigest:   "intent-C",
			ArtifactDigest: "art-c",
		},
		{
			EventID:        "r3",
			Timestamp:      now.Add(5 * time.Second),
			IsReport:       true,
			PrescriptionID: "p3",
			ExitCode:       intPtr(1),
		},
	}

	result := DetectThrashing(entries)
	if result.Count != 3 {
		t.Fatalf("expected 3 events in thrashing window, got %d", result.Count)
	}
}

func TestThrashing_SuccessResetsWindow(t *testing.T) {
	t.Parallel()

	now := time.Now()
	entries := []Entry{
		{
			EventID:        "p1",
			Timestamp:      now,
			ActorID:        "agent-1",
			IsPrescription: true,
			IntentDigest:   "intent-A",
		},
		{EventID: "r1", Timestamp: now.Add(1 * time.Second), IsReport: true, PrescriptionID: "p1", ExitCode: intPtr(1)},
		{
			EventID:        "p2",
			Timestamp:      now.Add(2 * time.Second),
			ActorID:        "agent-1",
			IsPrescription: true,
			IntentDigest:   "intent-B",
		},
		{EventID: "r2", Timestamp: now.Add(3 * time.Second), IsReport: true, PrescriptionID: "p2", ExitCode: intPtr(0)},
		{
			EventID:        "p3",
			Timestamp:      now.Add(4 * time.Second),
			ActorID:        "agent-1",
			IsPrescription: true,
			IntentDigest:   "intent-C",
		},
		{EventID: "r3", Timestamp: now.Add(5 * time.Second), IsReport: true, PrescriptionID: "p3", ExitCode: intPtr(1)},
	}

	result := DetectThrashing(entries)
	if result.Count != 0 {
		t.Fatalf("success should reset window, expected 0, got %d", result.Count)
	}
}

func TestThrashing_TwoIntentsNotEnough(t *testing.T) {
	t.Parallel()

	now := time.Now()
	entries := []Entry{
		{
			EventID:        "p1",
			Timestamp:      now,
			ActorID:        "agent-1",
			IsPrescription: true,
			IntentDigest:   "intent-A",
		},
		{EventID: "r1", Timestamp: now.Add(1 * time.Second), IsReport: true, PrescriptionID: "p1", ExitCode: intPtr(1)},
		{
			EventID:        "p2",
			Timestamp:      now.Add(2 * time.Second),
			ActorID:        "agent-1",
			IsPrescription: true,
			IntentDigest:   "intent-B",
		},
		{EventID: "r2", Timestamp: now.Add(3 * time.Second), IsReport: true, PrescriptionID: "p2", ExitCode: intPtr(1)},
	}

	result := DetectThrashing(entries)
	if result.Count != 0 {
		t.Fatalf("2 distinct intents below threshold=3, expected 0, got %d", result.Count)
	}
}

func TestThrashing_SameIntentRepeatedIsNotThrashing(t *testing.T) {
	t.Parallel()

	now := time.Now()
	entries := []Entry{
		{EventID: "p1", Timestamp: now, ActorID: "a", IsPrescription: true, IntentDigest: "same"},
		{EventID: "r1", Timestamp: now.Add(1 * time.Second), IsReport: true, PrescriptionID: "p1", ExitCode: intPtr(1)},
		{EventID: "p2", Timestamp: now.Add(2 * time.Second), ActorID: "a", IsPrescription: true, IntentDigest: "same"},
		{EventID: "r2", Timestamp: now.Add(3 * time.Second), IsReport: true, PrescriptionID: "p2", ExitCode: intPtr(1)},
		{EventID: "p3", Timestamp: now.Add(4 * time.Second), ActorID: "a", IsPrescription: true, IntentDigest: "same"},
		{EventID: "r3", Timestamp: now.Add(5 * time.Second), IsReport: true, PrescriptionID: "p3", ExitCode: intPtr(1)},
	}

	result := DetectThrashing(entries)
	if result.Count != 0 {
		t.Fatalf("same intent repeated is retry not thrashing, expected 0, got %d", result.Count)
	}
}

func TestThrashing_EmptyEntries(t *testing.T) {
	t.Parallel()
	result := DetectThrashing(nil)
	if result.Count != 0 {
		t.Fatalf("expected 0, got %d", result.Count)
	}
}

func TestThrashing_CustomThreshold(t *testing.T) {
	t.Parallel()

	now := time.Now()
	entries := []Entry{
		{EventID: "p1", Timestamp: now, IsPrescription: true, IntentDigest: "A"},
		{EventID: "r1", Timestamp: now.Add(1 * time.Second), IsReport: true, PrescriptionID: "p1", ExitCode: intPtr(1)},
		{EventID: "p2", Timestamp: now.Add(2 * time.Second), IsPrescription: true, IntentDigest: "B"},
		{EventID: "r2", Timestamp: now.Add(3 * time.Second), IsReport: true, PrescriptionID: "p2", ExitCode: intPtr(1)},
	}

	result := DetectThrashingWithThreshold(entries, 2)
	if result.Count != 2 {
		t.Fatalf("expected 2 events with threshold=2, got %d", result.Count)
	}
}
