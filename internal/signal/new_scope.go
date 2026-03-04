package signal

// DetectNewScope flags prescriptions that introduce a (tool, operation_class)
// combination not seen in earlier entries. Entries must be sorted chronologically.
func DetectNewScope(entries []Entry) SignalResult {
	type scopeKey struct{ tool, opClass string }

	seen := make(map[scopeKey]bool)
	var eventIDs []string

	for _, e := range entries {
		if !e.IsPrescription {
			continue
		}
		k := scopeKey{e.Tool, e.OperationClass}
		if !seen[k] {
			seen[k] = true
			eventIDs = append(eventIDs, e.EventID)
		}
	}

	return SignalResult{
		Name:     "new_scope",
		Count:    len(eventIDs),
		EventIDs: eventIDs,
	}
}
