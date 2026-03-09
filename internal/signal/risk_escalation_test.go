package signal

import (
	"fmt"
	"testing"
	"time"
)

func TestDetectRiskEscalation_BaselineMedium_OneHighEscalation(t *testing.T) {
	t.Parallel()

	now := time.Now()
	var entries []Entry
	// 10 medium-risk prescriptions establish baseline (mutate + staging = medium)
	for i := 0; i < 10; i++ {
		entries = append(entries, Entry{
			EventID:        fmt.Sprintf("P%d", i),
			IsPrescription: true,
			ActorID:        "alice",
			Tool:           "kubectl",
			OperationClass: "mutate",
			ScopeClass:     "staging",
			Timestamp:      now.Add(time.Duration(i) * time.Minute),
		})
	}
	// 1 high-risk prescription (mutate + production = high)
	entries = append(entries, Entry{
		EventID:        "P10",
		IsPrescription: true,
		ActorID:        "alice",
		Tool:           "kubectl",
		OperationClass: "mutate",
		ScopeClass:     "production",
		Timestamp:      now.Add(10 * time.Minute),
	})

	result := DetectRiskEscalation(entries)
	if result.Name != "risk_escalation" {
		t.Errorf("name = %q, want risk_escalation", result.Name)
	}
	if result.Count != 1 {
		t.Errorf("count = %d, want 1", result.Count)
	}
	assertEventID(t, result.EventIDs, "P10")
}

func TestDetectRiskEscalation_BelowMinSamples(t *testing.T) {
	t.Parallel()

	now := time.Now()
	entries := []Entry{
		{EventID: "P1", IsPrescription: true, ActorID: "alice", Tool: "kubectl", OperationClass: "read", ScopeClass: "production", Timestamp: now},
		{EventID: "P2", IsPrescription: true, ActorID: "alice", Tool: "kubectl", OperationClass: "mutate", ScopeClass: "production", Timestamp: now.Add(1 * time.Minute)},
	}

	result := DetectRiskEscalation(entries)
	if result.Count != 0 {
		t.Errorf("count = %d, want 0 (below MinBaselineSamples)", result.Count)
	}
}

func TestDetectRiskEscalation_TieBreakLower(t *testing.T) {
	t.Parallel()

	now := time.Now()
	entries := []Entry{
		// P4: prior=[P1(low),P2(low),P3(med)] -> baseline=low, P4(med) escalates.
		// P5: prior=[P1(low),P2(low),P3(med),P4(med)] -> 2 low, 2 med -> tie -> baseline=low, P5(med) escalates.
		{EventID: "P1", IsPrescription: true, ActorID: "alice", Tool: "kubectl", OperationClass: "read", ScopeClass: "production", Timestamp: now},
		{EventID: "P2", IsPrescription: true, ActorID: "alice", Tool: "kubectl", OperationClass: "read", ScopeClass: "production", Timestamp: now.Add(1 * time.Minute)},
		{EventID: "P3", IsPrescription: true, ActorID: "alice", Tool: "kubectl", OperationClass: "mutate", ScopeClass: "staging", Timestamp: now.Add(2 * time.Minute)},
		{EventID: "P4", IsPrescription: true, ActorID: "alice", Tool: "kubectl", OperationClass: "mutate", ScopeClass: "staging", Timestamp: now.Add(3 * time.Minute)},
		// P5: medium. Prior=[P1(low),P2(low),P3(med),P4(med)] -> 2 low, 2 medium -> tie -> baseline=low.
		// P5(medium) > low -> escalation.
		{EventID: "P5", IsPrescription: true, ActorID: "alice", Tool: "kubectl", OperationClass: "mutate", ScopeClass: "staging", Timestamp: now.Add(4 * time.Minute)},
	}

	result := DetectRiskEscalation(entries)
	// P4 escalates (prior=[P1(low),P2(low),P3(med)], baseline=low, actual=medium)
	// P5 escalates (prior=[P1(low),P2(low),P3(med),P4(med)], tie -> baseline=low, actual=medium)
	if result.Count != 2 {
		t.Errorf("count = %d, want 2 (tie-break picks lower baseline)", result.Count)
	}
	assertEventID(t, result.EventIDs, "P4")
	assertEventID(t, result.EventIDs, "P5")
}

func TestDetectRiskEscalation_DemotionNotCounted(t *testing.T) {
	t.Parallel()

	now := time.Now()
	var entries []Entry
	// 10 high-risk prescriptions establish baseline (mutate + production = high)
	for i := 0; i < 10; i++ {
		entries = append(entries, Entry{
			EventID:        fmt.Sprintf("P%d", i),
			IsPrescription: true,
			ActorID:        "alice",
			Tool:           "kubectl",
			OperationClass: "mutate",
			ScopeClass:     "production",
			Timestamp:      now.Add(time.Duration(i) * time.Minute),
		})
	}
	// 1 medium-risk (demotion)
	entries = append(entries, Entry{
		EventID:        "P10",
		IsPrescription: true,
		ActorID:        "alice",
		Tool:           "kubectl",
		OperationClass: "mutate",
		ScopeClass:     "staging",
		Timestamp:      now.Add(10 * time.Minute),
	})

	result := DetectRiskEscalation(entries)
	if result.Count != 0 {
		t.Errorf("count = %d, want 0 (demotion not counted)", result.Count)
	}
}

func TestDetectRiskEscalation_ColdStart(t *testing.T) {
	t.Parallel()

	result := DetectRiskEscalation(nil)
	if result.Count != 0 {
		t.Errorf("count = %d, want 0 (empty entries)", result.Count)
	}
	if result.Name != "risk_escalation" {
		t.Errorf("name = %q, want risk_escalation", result.Name)
	}
}

func TestDetectRiskEscalation_OutsideWindow(t *testing.T) {
	t.Parallel()

	now := time.Now()
	var entries []Entry
	// 10 medium-risk prescriptions outside the 30-day window
	for i := 0; i < 10; i++ {
		entries = append(entries, Entry{
			EventID:        fmt.Sprintf("P%d", i),
			IsPrescription: true,
			ActorID:        "alice",
			Tool:           "kubectl",
			OperationClass: "mutate",
			ScopeClass:     "staging",
			Timestamp:      now.Add(-35 * 24 * time.Hour).Add(time.Duration(i) * time.Minute),
		})
	}
	// 1 high-risk within window but only 1 in-window entry -> below min samples
	entries = append(entries, Entry{
		EventID:        "P10",
		IsPrescription: true,
		ActorID:        "alice",
		Tool:           "kubectl",
		OperationClass: "mutate",
		ScopeClass:     "production",
		Timestamp:      now,
	})

	result := DetectRiskEscalation(entries)
	if result.Count != 0 {
		t.Errorf("count = %d, want 0 (old entries outside window, only 1 in window < MinBaselineSamples)", result.Count)
	}
}

func TestDetectRiskEscalation_MultipleActorsAndTools(t *testing.T) {
	t.Parallel()

	now := time.Now()
	entries := []Entry{
		// alice+kubectl: 3 low-risk, then 1 high-risk escalation
		{EventID: "PA1", IsPrescription: true, ActorID: "alice", Tool: "kubectl", OperationClass: "read", ScopeClass: "production", Timestamp: now},
		{EventID: "PA2", IsPrescription: true, ActorID: "alice", Tool: "kubectl", OperationClass: "read", ScopeClass: "production", Timestamp: now.Add(1 * time.Minute)},
		{EventID: "PA3", IsPrescription: true, ActorID: "alice", Tool: "kubectl", OperationClass: "read", ScopeClass: "production", Timestamp: now.Add(2 * time.Minute)},
		{EventID: "PA4", IsPrescription: true, ActorID: "alice", Tool: "kubectl", OperationClass: "mutate", ScopeClass: "production", Timestamp: now.Add(3 * time.Minute)},
		// bob+kubectl: only 2 entries -> below min samples, no detection
		{EventID: "PB1", IsPrescription: true, ActorID: "bob", Tool: "kubectl", OperationClass: "read", ScopeClass: "production", Timestamp: now},
		{EventID: "PB2", IsPrescription: true, ActorID: "bob", Tool: "kubectl", OperationClass: "mutate", ScopeClass: "production", Timestamp: now.Add(1 * time.Minute)},
		// alice+terraform: separate behavior stream, 3 medium, no escalation
		{EventID: "PT1", IsPrescription: true, ActorID: "alice", Tool: "terraform", OperationClass: "mutate", ScopeClass: "staging", Timestamp: now},
		{EventID: "PT2", IsPrescription: true, ActorID: "alice", Tool: "terraform", OperationClass: "mutate", ScopeClass: "staging", Timestamp: now.Add(1 * time.Minute)},
		{EventID: "PT3", IsPrescription: true, ActorID: "alice", Tool: "terraform", OperationClass: "mutate", ScopeClass: "staging", Timestamp: now.Add(2 * time.Minute)},
	}

	result := DetectRiskEscalation(entries)
	// Only alice+kubectl PA4 flagged
	if result.Count != 1 {
		t.Errorf("count = %d, want 1 (only alice+kubectl PA4)", result.Count)
	}
	assertEventID(t, result.EventIDs, "PA4")
}

func TestDetectRiskEscalation_RiskTagElevation(t *testing.T) {
	t.Parallel()

	now := time.Now()
	entries := []Entry{
		// 3 medium-risk prescriptions (mutate + staging = medium)
		{EventID: "P1", IsPrescription: true, ActorID: "alice", Tool: "kubectl", OperationClass: "mutate", ScopeClass: "staging", Timestamp: now},
		{EventID: "P2", IsPrescription: true, ActorID: "alice", Tool: "kubectl", OperationClass: "mutate", ScopeClass: "staging", Timestamp: now.Add(1 * time.Minute)},
		{EventID: "P3", IsPrescription: true, ActorID: "alice", Tool: "kubectl", OperationClass: "mutate", ScopeClass: "staging", Timestamp: now.Add(2 * time.Minute)},
		// Same op/scope but risk tag elevates medium -> critical.
		{EventID: "P4", IsPrescription: true, ActorID: "alice", Tool: "kubectl", OperationClass: "mutate", ScopeClass: "staging", RiskTags: []string{"k8s.privileged_container"}, Timestamp: now.Add(3 * time.Minute)},
	}

	result := DetectRiskEscalation(entries)
	if result.Count != 1 {
		t.Errorf("count = %d, want 1 (P4 elevated by risk tag)", result.Count)
	}
	assertEventID(t, result.EventIDs, "P4")
}

func TestDetectRiskEscalation_CausalityCheck(t *testing.T) {
	t.Parallel()

	now := time.Now()
	entries := []Entry{
		// 3 low-risk
		{EventID: "P1", IsPrescription: true, ActorID: "alice", Tool: "kubectl", OperationClass: "read", ScopeClass: "production", Timestamp: now},
		{EventID: "P2", IsPrescription: true, ActorID: "alice", Tool: "kubectl", OperationClass: "read", ScopeClass: "production", Timestamp: now.Add(1 * time.Minute)},
		{EventID: "P3", IsPrescription: true, ActorID: "alice", Tool: "kubectl", OperationClass: "read", ScopeClass: "production", Timestamp: now.Add(2 * time.Minute)},
		// P4: high-risk. Baseline from P1-P3 = low, so P4 is escalation.
		{EventID: "P4", IsPrescription: true, ActorID: "alice", Tool: "kubectl", OperationClass: "mutate", ScopeClass: "production", Timestamp: now.Add(3 * time.Minute)},
		// P5: low-risk. Baseline from P1-P4 (3 low, 1 high) = mode low. P5 == baseline, not flagged.
		{EventID: "P5", IsPrescription: true, ActorID: "alice", Tool: "kubectl", OperationClass: "read", ScopeClass: "production", Timestamp: now.Add(4 * time.Minute)},
	}

	result := DetectRiskEscalation(entries)
	if result.Count != 1 {
		t.Errorf("count = %d, want 1 (only P4, causality preserved)", result.Count)
	}
	assertEventID(t, result.EventIDs, "P4")
}

func TestDetectRiskEscalationEvents_DemotionEmitted(t *testing.T) {
	t.Parallel()

	now := time.Now()
	var entries []Entry
	for i := 0; i < 5; i++ {
		entries = append(entries, Entry{
			EventID:        fmt.Sprintf("P%d", i),
			IsPrescription: true,
			ActorID:        "alice",
			Tool:           "kubectl",
			OperationClass: "mutate",
			ScopeClass:     "production",
			Timestamp:      now.Add(time.Duration(i) * time.Minute),
		})
	}
	entries = append(entries, Entry{
		EventID:        "P5",
		IsPrescription: true,
		ActorID:        "alice",
		Tool:           "kubectl",
		OperationClass: "mutate",
		ScopeClass:     "staging",
		Timestamp:      now.Add(5 * time.Minute),
	})

	events := DetectRiskEscalationEvents(entries)
	assertSubSignal(t, events, "risk_demotion")
}

func TestDetectRiskEscalationEvents_EscalationEmitted(t *testing.T) {
	t.Parallel()

	now := time.Now()
	var entries []Entry
	for i := 0; i < 5; i++ {
		entries = append(entries, Entry{
			EventID:        fmt.Sprintf("P%d", i),
			IsPrescription: true,
			ActorID:        "alice",
			Tool:           "kubectl",
			OperationClass: "mutate",
			ScopeClass:     "staging",
			Timestamp:      now.Add(time.Duration(i) * time.Minute),
		})
	}
	entries = append(entries, Entry{
		EventID:        "P5",
		IsPrescription: true,
		ActorID:        "alice",
		Tool:           "kubectl",
		OperationClass: "mutate",
		ScopeClass:     "production",
		Timestamp:      now.Add(5 * time.Minute),
	})

	events := DetectRiskEscalationEvents(entries)
	assertSubSignal(t, events, "risk_escalation")
}
