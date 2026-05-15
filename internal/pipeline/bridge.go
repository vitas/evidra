package pipeline

import (
	"encoding/json"
	"fmt"

	"samebits.com/evidra/internal/canon"
	"samebits.com/evidra/internal/signal"
	"samebits.com/evidra/pkg/evidence"
)

// EvidenceToSignalEntries converts evidence entries to signal detector input.
// Only prescribe and report entries produce signal entries; other types are skipped.
func EvidenceToSignalEntries(entries []evidence.EvidenceEntry) ([]signal.Entry, error) {
	var result []signal.Entry
	prescriptions := make(map[string]signal.Entry, len(entries))

	for _, e := range entries {
		if e.Type != evidence.EntryTypePrescribe {
			continue
		}
		var p evidence.PrescriptionPayload
		if err := json.Unmarshal(e.Payload, &p); err != nil {
			return nil, fmt.Errorf("pipeline: unmarshal prescription %s: %w", e.EntryID, err)
		}
		prescriptions[e.EntryID] = signalIdentityFromPrescription(p)
	}

	for _, e := range entries {
		se := signal.Entry{
			EventID:        e.EntryID,
			Timestamp:      e.Timestamp,
			ActorID:        e.Actor.ID,
			ArtifactDigest: e.ArtifactDigest,
			IntentDigest:   e.IntentDigest,
		}

		switch e.Type {
		case evidence.EntryTypePrescribe:
			se.IsPrescription = true
			var p evidence.PrescriptionPayload
			if err := json.Unmarshal(e.Payload, &p); err != nil {
				return nil, fmt.Errorf("pipeline: unmarshal prescription %s: %w", e.EntryID, err)
			}
			// Signals only consume Evidra-native risk tags.
			se.RiskTags = p.NativeRiskTags()
			applySignalIdentity(&se, signalIdentityFromPrescription(p))

		case evidence.EntryTypeReport:
			se.IsReport = true
			var r evidence.ReportPayload
			if err := json.Unmarshal(e.Payload, &r); err != nil {
				return nil, fmt.Errorf("pipeline: unmarshal report %s: %w", e.EntryID, err)
			}
			se.PrescriptionID = r.PrescriptionID
			se.ExitCode = r.ExitCode
			if identity, ok := prescriptions[r.PrescriptionID]; ok {
				applySignalIdentity(&se, identity)
			}

		default:
			// Skip finding, signal, receipt, canonicalization_failure, session_start, session_end, annotation entries
			continue
		}

		result = append(result, se)
	}

	return result, nil
}

func signalIdentityFromPrescription(p evidence.PrescriptionPayload) signal.Entry {
	var identity signal.Entry
	if ca, err := extractCanonicalAction(p.CanonicalAction); err == nil {
		identity.Tool = ca.Tool
		identity.Operation = ca.Operation
		identity.OperationClass = ca.OperationClass
		identity.ScopeClass = ca.ScopeClass
		identity.ResourceCount = ca.ResourceCount
		identity.ShapeHash = ca.ResourceShapeHash
		return identity
	}
	if p.Intent != nil {
		identity.Tool = p.Intent.Tool
		identity.Operation = p.Intent.Operation
	}
	return identity
}

func applySignalIdentity(entry *signal.Entry, identity signal.Entry) {
	entry.Tool = identity.Tool
	entry.Operation = identity.Operation
	entry.OperationClass = identity.OperationClass
	entry.ScopeClass = identity.ScopeClass
	entry.ResourceCount = identity.ResourceCount
	entry.ShapeHash = identity.ShapeHash
}

func extractCanonicalAction(raw json.RawMessage) (canon.CanonicalAction, error) {
	var ca canon.CanonicalAction
	if err := json.Unmarshal(raw, &ca); err != nil {
		return canon.CanonicalAction{}, err
	}
	return ca, nil
}
