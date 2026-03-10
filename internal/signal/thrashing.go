package signal

import (
	"sort"
	"time"
)

func init() {
	registerSignal(signalDefinition{
		name:  "thrashing",
		order: 70,
		detect: func(entries []Entry, _ time.Duration) SignalResult {
			return DetectThrashing(entries)
		},
	})
}

// ThrashingThreshold is the minimum number of distinct failed intents.
const ThrashingThreshold = 3

// DetectThrashing detects many distinct failed intents without success.
func DetectThrashing(entries []Entry) SignalResult {
	return DetectThrashingWithThreshold(entries, ThrashingThreshold)
}

// DetectThrashingWithThreshold allows configurable threshold for testing.
func DetectThrashingWithThreshold(entries []Entry, threshold int) SignalResult {
	reportExit := make(map[string]*int)
	for _, e := range entries {
		if e.IsReport && e.PrescriptionID != "" {
			reportExit[e.PrescriptionID] = e.ExitCode
		}
	}

	var prescriptions []Entry
	for _, e := range entries {
		if e.IsPrescription && e.IntentDigest != "" {
			prescriptions = append(prescriptions, e)
		}
	}
	sort.Slice(prescriptions, func(i, j int) bool {
		return prescriptions[i].Timestamp.Before(prescriptions[j].Timestamp)
	})

	distinctIntents := make(map[string]bool)
	var windowEntries []Entry
	var eventIDs []string
	seen := make(map[string]bool)

	for _, p := range prescriptions {
		ec := reportExit[p.EventID]

		// Success resets the window.
		if ec != nil && *ec == 0 {
			distinctIntents = make(map[string]bool)
			windowEntries = nil
			continue
		}

		// No report yet — unknown state; skip.
		if ec == nil {
			continue
		}

		distinctIntents[p.IntentDigest] = true
		windowEntries = append(windowEntries, p)
		if len(distinctIntents) >= threshold {
			for _, w := range windowEntries {
				if seen[w.EventID] {
					continue
				}
				seen[w.EventID] = true
				eventIDs = append(eventIDs, w.EventID)
			}
			distinctIntents = make(map[string]bool)
			windowEntries = nil
		}
	}

	return SignalResult{
		Name:     "thrashing",
		Count:    len(eventIDs),
		EventIDs: eventIDs,
	}
}
