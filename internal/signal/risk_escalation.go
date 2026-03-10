package signal

import (
	"fmt"
	"time"

	"samebits.com/evidra-benchmark/internal/risk"
)

func init() {
	registerSignal(signalDefinition{
		name:  "risk_escalation",
		order: 80,
		detect: func(entries []Entry, _ time.Duration) SignalResult {
			return DetectRiskEscalation(entries)
		},
	})
}

// BaselineWindow is the rolling window for computing an actor's baseline risk level.
const BaselineWindow = 30 * 24 * time.Hour

// MinBaselineSamples is the minimum number of prior prescriptions required to establish a baseline.
const MinBaselineSamples = 3

// riskSeverityOrder maps risk levels to numeric severity for comparison.
var riskSeverityOrder = map[string]int{
	"low":      0,
	"medium":   1,
	"high":     2,
	"critical": 3,
}

// severityLabel maps numeric severity back to risk level string.
var severityLabel = map[int]string{
	0: "low",
	1: "medium",
	2: "high",
	3: "critical",
}

// behaviorKey identifies a distinct behavior stream for baseline computation.
type behaviorKey struct {
	actor string
	tool  string
}

// DetectRiskEscalation flags prescriptions whose computed risk level exceeds the
// actor+tool baseline (mode) risk level within BaselineWindow. Entries must be
// sorted chronologically. Baseline is computed causally — only prior entries
// contribute to the baseline for each entry.
func DetectRiskEscalation(entries []Entry) SignalResult {
	events := DetectRiskEscalationEvents(entries)
	var eventIDs []string
	for _, e := range events {
		if e.SubSignal == "risk_escalation" {
			eventIDs = append(eventIDs, e.EntryRef)
		}
	}
	return SignalResult{
		Name:     "risk_escalation",
		Count:    len(eventIDs),
		EventIDs: eventIDs,
	}
}

// DetectRiskEscalationEvents returns detailed signal events for risk escalations
// and demotions. Escalations are counted in SignalResult; demotions are
// informational only.
func DetectRiskEscalationEvents(entries []Entry) []SignalEvent {
	history := make(map[behaviorKey][]severityEntry)
	var events []SignalEvent

	for _, e := range entries {
		if !e.IsPrescription {
			continue
		}

		k := behaviorKey{actor: e.ActorID, tool: e.Tool}
		level := risk.ElevateRiskLevel(risk.RiskLevel(e.OperationClass, e.ScopeClass), e.RiskTags)
		sev := riskSeverityOrder[level]

		// Filter history to entries within BaselineWindow of current entry.
		prior := withinWindow(history[k], e.Timestamp)

		if len(prior) >= MinBaselineSamples {
			baseline := baselineSeverity(prior)
			baselineLabel := severityLabel[baseline]

			if sev > baseline {
				events = append(events, SignalEvent{
					Signal:    "risk_escalation",
					SubSignal: "risk_escalation",
					Timestamp: e.Timestamp,
					EntryRef:  e.EventID,
					Details:   fmt.Sprintf("baseline=%s, actual=%s", baselineLabel, level),
				})
			} else if sev < baseline {
				events = append(events, SignalEvent{
					Signal:    "risk_escalation",
					SubSignal: "risk_demotion",
					Timestamp: e.Timestamp,
					EntryRef:  e.EventID,
					Details:   fmt.Sprintf("baseline=%s, actual=%s", baselineLabel, level),
				})
			}
		}

		// Append current entry to history AFTER baseline check (causal ordering).
		history[k] = append(history[k], severityEntry{ts: e.Timestamp, severity: sev})
	}

	return events
}

type severityEntry struct {
	ts       time.Time
	severity int
}

// withinWindow returns entries from hist where ts is within BaselineWindow of anchor.
// Only entries strictly before anchor qualify (causal).
func withinWindow(hist []severityEntry, anchor time.Time) []severityEntry {
	cutoff := anchor.Add(-BaselineWindow)
	var result []severityEntry
	for _, h := range hist {
		if !h.ts.Before(cutoff) && h.ts.Before(anchor) {
			result = append(result, h)
		}
	}
	return result
}

// baselineSeverity returns the mode severity from a slice of severity entries.
// On tie, picks the lower severity value (conservative — more sensitive to escalation).
func baselineSeverity(entries []severityEntry) int {
	counts := make(map[int]int)
	for _, e := range entries {
		counts[e.severity]++
	}

	bestSev := -1
	bestCount := 0
	for sev, count := range counts {
		if count > bestCount || (count == bestCount && (bestSev == -1 || sev < bestSev)) {
			bestSev = sev
			bestCount = count
		}
	}
	return bestSev
}
