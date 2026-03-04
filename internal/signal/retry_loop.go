package signal

import (
	"sort"
	"time"
)

const (
	// DefaultRetryThreshold is the minimum number of same-intent prescriptions
	// within the retry window to fire the signal.
	DefaultRetryThreshold = 3

	// DefaultRetryWindow is the time window for retry loop detection.
	DefaultRetryWindow = 10 * time.Minute
)

// DetectRetryLoops finds cases where the same (intent_digest, shape_hash) appears
// N or more times within a time window, indicating a retry loop.
func DetectRetryLoops(entries []Entry) SignalResult {
	return DetectRetryLoopsWithConfig(entries, DefaultRetryThreshold, DefaultRetryWindow)
}

// DetectRetryLoopsWithConfig allows configurable threshold and window.
func DetectRetryLoopsWithConfig(entries []Entry, threshold int, window time.Duration) SignalResult {
	type key struct{ intent, shape string }

	groups := make(map[key][]Entry)
	for _, e := range entries {
		if !e.IsPrescription || e.IntentDigest == "" {
			continue
		}
		k := key{e.IntentDigest, e.ShapeHash}
		groups[k] = append(groups[k], e)
	}

	var eventIDs []string
	for _, group := range groups {
		if len(group) < threshold {
			continue
		}
		sort.Slice(group, func(i, j int) bool {
			return group[i].Timestamp.Before(group[j].Timestamp)
		})
		// Sliding window: check if any N consecutive entries fit within the window
		for i := 0; i <= len(group)-threshold; i++ {
			if group[i+threshold-1].Timestamp.Sub(group[i].Timestamp) <= window {
				for j := i; j < i+threshold; j++ {
					eventIDs = append(eventIDs, group[j].EventID)
				}
				break
			}
		}
	}

	return SignalResult{
		Name:     "retry_loop",
		Count:    len(eventIDs),
		EventIDs: eventIDs,
	}
}
