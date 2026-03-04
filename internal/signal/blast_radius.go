package signal

// Blast radius thresholds by operation class.
const (
	BlastRadiusDestructive = 10
	BlastRadiusMutating    = 50
)

// DetectBlastRadius finds operations with resource_count exceeding the
// threshold for their operation class.
func DetectBlastRadius(entries []Entry) SignalResult {
	var eventIDs []string
	for _, e := range entries {
		if !e.IsPrescription || e.ResourceCount == 0 {
			continue
		}
		threshold := blastThreshold(e.OperationClass)
		if threshold > 0 && e.ResourceCount > threshold {
			eventIDs = append(eventIDs, e.EventID)
		}
	}

	return SignalResult{
		Name:     "blast_radius",
		Count:    len(eventIDs),
		EventIDs: eventIDs,
	}
}

func blastThreshold(opClass string) int {
	switch opClass {
	case "destroy":
		return BlastRadiusDestructive
	case "mutate":
		return BlastRadiusMutating
	}
	return 0
}
