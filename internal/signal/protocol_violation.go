package signal

import (
	"fmt"
	"time"
)

// DetectProtocolViolations finds prescriptions without matching reports
// (unreported operations) and reports without matching prescriptions
// (unprescribed actions). Also detects duplicate reports and cross-actor reports.
func DetectProtocolViolations(entries []Entry) SignalResult {
	events := DetectProtocolViolationEvents(entries)
	eventIDs := make([]string, len(events))
	for i, e := range events {
		eventIDs[i] = e.EntryRef
	}
	return SignalResult{
		Name:     "protocol_violation",
		Count:    len(eventIDs),
		EventIDs: eventIDs,
	}
}

// DetectProtocolViolationEvents returns detailed signal events for protocol violations.
func DetectProtocolViolationEvents(entries []Entry) []SignalEvent {
	prescriptions := make(map[string]Entry)
	firstReport := make(map[string]Entry) // prescription_id → first report
	reportedIDs := make(map[string]bool)

	for _, e := range entries {
		if e.IsPrescription {
			prescriptions[e.EventID] = e
		}
	}

	var events []SignalEvent

	for _, e := range entries {
		if !e.IsReport || e.PrescriptionID == "" {
			continue
		}

		// Unprescribed report
		if _, found := prescriptions[e.PrescriptionID]; !found {
			events = append(events, SignalEvent{
				Signal:    "protocol_violation",
				SubSignal: "unprescribed_action",
				Timestamp: e.Timestamp,
				EntryRef:  e.EventID,
				Details:   fmt.Sprintf("report %s references unknown prescription %s", e.EventID, e.PrescriptionID),
			})
			continue
		}

		// Duplicate report
		if reportedIDs[e.PrescriptionID] {
			events = append(events, SignalEvent{
				Signal:    "protocol_violation",
				SubSignal: "duplicate_report",
				Timestamp: e.Timestamp,
				EntryRef:  e.EventID,
				Details:   fmt.Sprintf("duplicate report for prescription %s (first: %s)", e.PrescriptionID, firstReport[e.PrescriptionID].EventID),
			})
			continue
		}

		// Cross-actor report
		rx := prescriptions[e.PrescriptionID]
		if rx.ActorID != "" && e.ActorID != "" && rx.ActorID != e.ActorID {
			events = append(events, SignalEvent{
				Signal:    "protocol_violation",
				SubSignal: "cross_actor_report",
				Timestamp: e.Timestamp,
				EntryRef:  e.EventID,
				Details:   fmt.Sprintf("report actor %s != prescription actor %s", e.ActorID, rx.ActorID),
			})
		}

		reportedIDs[e.PrescriptionID] = true
		firstReport[e.PrescriptionID] = e
	}

	// Unreported prescriptions (without TTL — for basic detection)
	for _, e := range entries {
		if e.IsPrescription && !reportedIDs[e.EventID] {
			events = append(events, SignalEvent{
				Signal:    "protocol_violation",
				SubSignal: "unreported_prescription",
				Timestamp: e.Timestamp,
				EntryRef:  e.EventID,
				Details:   fmt.Sprintf("prescription %s has no matching report", e.EventID),
			})
		}
	}

	return events
}

// DetectUnreported scans evidence chain for prescriptions without matching
// reports within TTL. Called at scorecard computation time, not at
// prescribe/report time.
func DetectUnreported(entries []Entry, ttl time.Duration) []SignalEvent {
	reportedIDs := make(map[string]bool)
	for _, e := range entries {
		if e.IsReport && e.PrescriptionID != "" {
			reportedIDs[e.PrescriptionID] = true
		}
	}

	var events []SignalEvent
	now := time.Now()
	for _, e := range entries {
		if !e.IsPrescription {
			continue
		}
		if reportedIDs[e.EventID] {
			continue
		}
		if now.Sub(e.Timestamp) <= ttl {
			continue // still within TTL, not yet a violation
		}

		sub := classifyUnreported(e, entries)
		events = append(events, SignalEvent{
			Signal:    "protocol_violation",
			SubSignal: sub,
			Timestamp: e.Timestamp,
			EntryRef:  e.EventID,
			Details:   fmt.Sprintf("prescription %s unreported after %v", e.EventID, ttl),
		})
	}
	return events
}

// classifyUnreported determines whether an unreported prescription is:
//   - crash_before_report: the last report from the same actor had exit_code != 0
//   - stalled_operation: no crash indicator — agent simply stopped working
func classifyUnreported(p Entry, entries []Entry) string {
	if p.ActorID == "" {
		return "stalled_operation"
	}

	// Find the most recent report from the same actor before or near this prescription.
	var lastReport *Entry
	for i := range entries {
		e := &entries[i]
		if e.IsReport && e.ActorID == p.ActorID {
			if lastReport == nil || e.Timestamp.After(lastReport.Timestamp) {
				lastReport = e
			}
		}
	}

	if lastReport != nil && lastReport.ExitCode != nil && *lastReport.ExitCode != 0 {
		return "crash_before_report"
	}
	return "stalled_operation"
}
