package signal

// DetectProtocolViolations finds prescriptions without matching reports
// (unreported operations) and reports without matching prescriptions
// (unprescribed actions).
func DetectProtocolViolations(entries []Entry) SignalResult {
	prescriptions := make(map[string]bool)
	reportedIDs := make(map[string]bool)

	for _, e := range entries {
		if e.IsPrescription {
			prescriptions[e.EventID] = true
		}
		if e.IsReport && e.PrescriptionID != "" {
			reportedIDs[e.PrescriptionID] = true
		}
	}

	var eventIDs []string

	// Unreported prescriptions
	for _, e := range entries {
		if e.IsPrescription && !reportedIDs[e.EventID] {
			eventIDs = append(eventIDs, e.EventID)
		}
	}

	// Unprescribed reports
	for _, e := range entries {
		if e.IsReport && e.PrescriptionID != "" && !prescriptions[e.PrescriptionID] {
			eventIDs = append(eventIDs, e.EventID)
		}
	}

	return SignalResult{
		Name:     "protocol_violation",
		Count:    len(eventIDs),
		EventIDs: eventIDs,
	}
}
