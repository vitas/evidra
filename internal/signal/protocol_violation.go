package signal

import (
	"fmt"
	"time"
)

func init() {
	registerSignal(signalDefinition{
		name:  "protocol_violation",
		order: 10,
		detect: func(entries []Entry, ttl time.Duration) SignalResult {
			return DetectProtocolViolations(entries, ttl)
		},
	})
}

// DetectProtocolViolations finds prescriptions without matching reports
// (unreported operations) and reports without matching prescriptions
// (unprescribed actions). Also detects duplicate reports and cross-actor reports.
// TTL controls the window for unreported prescription detection.
func DetectProtocolViolations(entries []Entry, ttl time.Duration) SignalResult {
	events := DetectProtocolViolationEvents(entries, ttl)
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
// TTL controls unreported prescription detection — only prescriptions older than
// TTL without a matching report are flagged.
func DetectProtocolViolationEvents(entries []Entry, ttl time.Duration) []SignalEvent {
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

		// Missing artifact digest — prescription had a digest but report omits it.
		// Artifact drift detection is disabled for this report pair.
		if rx.ArtifactDigest != "" && e.ArtifactDigest == "" && e.ExitCode != nil {
			events = append(events, SignalEvent{
				Signal:    "protocol_violation",
				SubSignal: "report_without_digest",
				Timestamp: e.Timestamp,
				EntryRef:  e.EventID,
				Details:   fmt.Sprintf("report %s omits artifact_digest; drift detection unavailable for prescription %s", e.EventID, e.PrescriptionID),
			})
		}

		reportedIDs[e.PrescriptionID] = true
		firstReport[e.PrescriptionID] = e
	}

	// Unreported prescriptions — TTL-aware with sub-signal classification
	events = append(events, DetectUnreported(entries, ttl)...)

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
