package signal

// BlastRadiusThreshold is the resource count above which a destroy operation
// triggers the blast_radius signal.
const BlastRadiusThreshold = 5

// DetectBlastRadius finds destroy operations with resource_count exceeding
// the threshold. Only destroy operations are considered.
func DetectBlastRadius(entries []Entry) SignalResult {
	var eventIDs []string
	for _, e := range entries {
		if !e.IsPrescription || e.ResourceCount == 0 {
			continue
		}
		if e.OperationClass == "destroy" && e.ResourceCount > BlastRadiusThreshold {
			eventIDs = append(eventIDs, e.EventID)
		}
	}

	return SignalResult{
		Name:     "blast_radius",
		Count:    len(eventIDs),
		EventIDs: eventIDs,
	}
}
