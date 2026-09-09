package signal

import "time"

func init() {
	registerSignal(signalDefinition{
		name:  "unprescribed_mutation",
		order: 90,
		detect: func(entries []Entry, _ time.Duration) SignalResult {
			return DetectUnprescribedMutations(entries)
		},
	})
}

// DetectUnprescribedMutations counts mutations that executed with no prior
// model-issued claim. The Evidra server-side capture (run_command
// auto-evidence) writes an explicit prescription flagged
// auto_prescribed=true when it could not link the action to a prescription
// the agent had recorded itself, so the evidence chain stays complete
// regardless of model compliance — and non-compliance stops being a silent
// hole and becomes measured data:
//
//	compliance_rate = 1 - unprescribed_mutations / total_mutations
//
// The default scoring profile gives this signal weight 0 on purpose: the
// number is for observation and calibration first. Whether (and how hard) it
// should penalize the score is a policy decision per profile, not a fact
// about the format.
func DetectUnprescribedMutations(entries []Entry) SignalResult {
	var eventIDs []string
	for _, e := range entries {
		if e.IsPrescription && e.AutoPrescribed {
			eventIDs = append(eventIDs, e.EventID)
		}
	}
	return SignalResult{
		Name:     "unprescribed_mutation",
		Count:    len(eventIDs),
		EventIDs: eventIDs,
	}
}
