package signal

import "time"

func init() {
	registerSignal(signalDefinition{
		name:  "new_scope",
		order: 50,
		detect: func(entries []Entry, _ time.Duration) SignalResult {
			return DetectNewScope(entries)
		},
	})
}

// DetectNewScope flags prescriptions that introduce an (actor, tool, operation_class, scope_class)
// combination not seen in earlier entries. Entries must be sorted chronologically.
//
// The first prescription in the entire history establishes the baseline and is never
// flagged — penalizing cold start is not useful. Only subsequent prescriptions that
// introduce a previously unseen combination are considered new scope.
func DetectNewScope(entries []Entry) SignalResult {
	type scopeKey struct {
		actor   string
		tool    string
		opClass string
		scope   string
	}

	seen := make(map[scopeKey]bool)
	var eventIDs []string
	first := true

	for _, e := range entries {
		if !e.IsPrescription {
			continue
		}
		k := scopeKey{e.ActorID, e.Tool, e.OperationClass, e.ScopeClass}
		if !seen[k] {
			seen[k] = true
			if first {
				// The very first prescription establishes the baseline scope.
				first = false
				continue
			}
			eventIDs = append(eventIDs, e.EventID)
		}
	}

	return SignalResult{
		Name:     "new_scope",
		Count:    len(eventIDs),
		EventIDs: eventIDs,
	}
}
