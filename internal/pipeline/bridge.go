package pipeline

import (
	"encoding/json"
	"fmt"

	"samebits.com/evidra-benchmark/internal/canon"
	"samebits.com/evidra-benchmark/internal/signal"
	"samebits.com/evidra-benchmark/pkg/evidence"
)

// EvidenceToSignalEntries converts evidence entries to signal detector input.
// Only prescribe and report entries produce signal entries; other types are skipped.
func EvidenceToSignalEntries(entries []evidence.EvidenceEntry) ([]signal.Entry, error) {
	var result []signal.Entry
	prescriptions := make(map[string]canon.CanonicalAction, len(entries))

	for _, e := range entries {
		if e.Type != evidence.EntryTypePrescribe {
			continue
		}
		var p evidence.PrescriptionPayload
		if err := json.Unmarshal(e.Payload, &p); err != nil {
			return nil, fmt.Errorf("pipeline: unmarshal prescription %s: %w", e.EntryID, err)
		}
		if ca, err := extractCanonicalAction(p.CanonicalAction); err == nil {
			prescriptions[e.EntryID] = ca
		}
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
			// Canonical contract is risk_details with legacy fallback to risk_tags.
			se.RiskTags = p.EffectiveRiskDetails()
			// Extract fields from canonical_action.
			if ca, err := extractCanonicalAction(p.CanonicalAction); err == nil {
				se.Tool = ca.Tool
				se.Operation = ca.Operation
				se.OperationClass = ca.OperationClass
				se.ScopeClass = ca.ScopeClass
				se.ResourceCount = ca.ResourceCount
				se.ShapeHash = ca.ResourceShapeHash
			}

		case evidence.EntryTypeReport:
			se.IsReport = true
			var r evidence.ReportPayload
			if err := json.Unmarshal(e.Payload, &r); err != nil {
				return nil, fmt.Errorf("pipeline: unmarshal report %s: %w", e.EntryID, err)
			}
			se.PrescriptionID = r.PrescriptionID
			se.ExitCode = r.ExitCode
			if ca, ok := prescriptions[r.PrescriptionID]; ok {
				se.Tool = ca.Tool
				se.Operation = ca.Operation
				se.OperationClass = ca.OperationClass
				se.ScopeClass = ca.ScopeClass
				se.ResourceCount = ca.ResourceCount
				se.ShapeHash = ca.ResourceShapeHash
			}

		default:
			// Skip finding, signal, receipt, canonicalization_failure, session_start, session_end, annotation entries
			continue
		}

		result = append(result, se)
	}

	return result, nil
}

func extractCanonicalAction(raw json.RawMessage) (canon.CanonicalAction, error) {
	var ca canon.CanonicalAction
	if err := json.Unmarshal(raw, &ca); err != nil {
		return canon.CanonicalAction{}, err
	}
	return ca, nil
}
