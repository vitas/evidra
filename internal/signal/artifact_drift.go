package signal

import "time"

func init() {
	registerSignal(signalDefinition{
		name:  "artifact_drift",
		order: 20,
		detect: func(entries []Entry, _ time.Duration) SignalResult {
			return DetectArtifactDrift(entries)
		},
	})
}

// DetectArtifactDrift finds reports where the artifact digest does not match
// the prescription's artifact digest.
func DetectArtifactDrift(entries []Entry) SignalResult {
	// Build map: prescription event_id → artifact_digest
	prescriptionDigest := make(map[string]string)
	for _, e := range entries {
		if e.IsPrescription && e.ArtifactDigest != "" {
			prescriptionDigest[e.EventID] = e.ArtifactDigest
		}
	}

	var eventIDs []string
	for _, e := range entries {
		if !e.IsReport || e.PrescriptionID == "" || e.ArtifactDigest == "" {
			continue
		}
		presDigest, ok := prescriptionDigest[e.PrescriptionID]
		if !ok {
			continue
		}
		if presDigest != e.ArtifactDigest {
			eventIDs = append(eventIDs, e.EventID)
		}
	}

	return SignalResult{
		Name:     "artifact_drift",
		Count:    len(eventIDs),
		EventIDs: eventIDs,
	}
}
