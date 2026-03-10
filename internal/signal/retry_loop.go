package signal

import (
	"sort"
	"time"
)

func init() {
	registerSignal(signalDefinition{
		name:  "retry_loop",
		order: 30,
		detect: func(entries []Entry, _ time.Duration) SignalResult {
			return DetectRetryLoops(entries)
		},
	})
}

const (
	// DefaultRetryThreshold is the minimum number of same-intent prescriptions
	// within the retry window to fire the signal.
	DefaultRetryThreshold = 3

	// DefaultRetryWindow is the time window for retry loop detection.
	DefaultRetryWindow = 30 * time.Minute
)

// DefaultVariantRetryThreshold is the minimum number of same-scope prescriptions
// within the retry window to fire the variant retry signal. Higher than the exact
// threshold to reduce false positives on legitimate investigative retries.
const DefaultVariantRetryThreshold = 5

// DetectRetryLoops finds retry loop patterns using both exact and variant detection:
//   - Exact: same (actor, intent_digest, shape_hash) repeated >= DefaultRetryThreshold times
//   - Variant: same (actor, tool, operation_class, scope_class) repeated >= DefaultVariantRetryThreshold
//     times regardless of artifact content — catches agents that mutate the artifact
//     between attempts without making real progress.
//
// Results are merged and deduplicated by event ID.
func DetectRetryLoops(entries []Entry) SignalResult {
	exact := DetectRetryLoopsWithConfig(entries, DefaultRetryThreshold, DefaultRetryWindow)
	variant := DetectVariantRetryLoopsWithConfig(entries, DefaultVariantRetryThreshold, DefaultRetryWindow)

	seen := make(map[string]bool, len(exact.EventIDs)+len(variant.EventIDs))
	var merged []string
	for _, id := range append(exact.EventIDs, variant.EventIDs...) {
		if !seen[id] {
			seen[id] = true
			merged = append(merged, id)
		}
	}
	return SignalResult{
		Name:     "retry_loop",
		Count:    len(merged),
		EventIDs: merged,
	}
}

// DetectRetryLoopsWithConfig allows configurable threshold and window.
func DetectRetryLoopsWithConfig(entries []Entry, threshold int, window time.Duration) SignalResult {
	return detectRetryLoopChains(entries, threshold, window, func(entry Entry) (string, bool) {
		if !entry.IsPrescription || entry.IntentDigest == "" {
			return "", false
		}
		return entry.ActorID + "|" + entry.IntentDigest + "|" + entry.ShapeHash, true
	})
}

// DetectVariantRetryLoopsWithConfig detects retry loops where the agent mutates
// the artifact between attempts (different intent_digest / shape_hash) but keeps
// operating in the same (actor, tool, operation_class, scope_class) space after a
// failure. This captures investigative-looking loops that escape exact-match detection.
//
// A higher threshold (DefaultVariantRetryThreshold = 5) reduces false positives from
// legitimate troubleshooting where the operator is genuinely making different attempts.
func DetectVariantRetryLoopsWithConfig(entries []Entry, threshold int, window time.Duration) SignalResult {
	return detectRetryLoopChains(entries, threshold, window, func(entry Entry) (string, bool) {
		if !entry.IsPrescription {
			return "", false
		}
		return entry.ActorID + "|" + entry.Tool + "|" + entry.OperationClass + "|" + entry.ScopeClass, true
	})
}

func detectRetryLoopChains(entries []Entry, threshold int, window time.Duration, groupKey func(entry Entry) (string, bool)) SignalResult {
	reportExitCode := make(map[string]*int)
	for _, entry := range entries {
		if entry.IsReport && entry.PrescriptionID != "" {
			reportExitCode[entry.PrescriptionID] = entry.ExitCode
		}
	}

	groups := make(map[string][]Entry)
	for _, entry := range entries {
		key, ok := groupKey(entry)
		if !ok {
			continue
		}
		groups[key] = append(groups[key], entry)
	}

	var eventIDs []string
	for _, group := range groups {
		sort.Slice(group, func(i, j int) bool {
			return group[i].Timestamp.Before(group[j].Timestamp)
		})

		var chain []Entry
		failSeen := false
		for _, entry := range group {
			exitCode, hasReport := reportExitCode[entry.EventID]
			if !failSeen {
				if hasReport && exitCode != nil && *exitCode != 0 {
					failSeen = true
					chain = append(chain, entry)
				}
				continue
			}
			if entry.Timestamp.Sub(chain[0].Timestamp) <= window {
				chain = append(chain, entry)
				continue
			}
			if len(chain) >= threshold {
				for _, chained := range chain {
					eventIDs = append(eventIDs, chained.EventID)
				}
			}
			chain = nil
			failSeen = false
			if hasReport && exitCode != nil && *exitCode != 0 {
				failSeen = true
				chain = append(chain, entry)
			}
		}

		if len(chain) >= threshold {
			for _, chained := range chain {
				eventIDs = append(eventIDs, chained.EventID)
			}
		}
	}

	return SignalResult{
		Name:     "retry_loop",
		Count:    len(eventIDs),
		EventIDs: eventIDs,
	}
}
