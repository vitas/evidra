package pipeline

import (
	"encoding/json"
	"fmt"

	"samebits.com/evidra-benchmark/internal/signal"
	"samebits.com/evidra-benchmark/pkg/evidence"
)

// EvidenceToSignalEntries converts evidence entries to signal detector input.
// Only prescribe and report entries produce signal entries; other types are skipped.
func EvidenceToSignalEntries(entries []evidence.EvidenceEntry) ([]signal.Entry, error) {
	var result []signal.Entry

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
			se.RiskTags = p.RiskTags
			// Extract fields from canonical_action
			var ca struct {
				Tool           string `json:"tool"`
				Operation      string `json:"operation"`
				OperationClass string `json:"operation_class"`
				ScopeClass     string `json:"scope_class"`
				ResourceCount  int    `json:"resource_count"`
				ShapeHash      string `json:"resource_shape_hash"`
			}
			if err := json.Unmarshal(p.CanonicalAction, &ca); err == nil {
				se.Tool = ca.Tool
				se.Operation = ca.Operation
				se.OperationClass = ca.OperationClass
				se.ScopeClass = ca.ScopeClass
				se.ResourceCount = ca.ResourceCount
				se.ShapeHash = ca.ShapeHash
			}

		case evidence.EntryTypeReport:
			se.IsReport = true
			var r evidence.ReportPayload
			if err := json.Unmarshal(e.Payload, &r); err != nil {
				return nil, fmt.Errorf("pipeline: unmarshal report %s: %w", e.EntryID, err)
			}
			se.PrescriptionID = r.PrescriptionID
			exitCode := r.ExitCode
			se.ExitCode = &exitCode

		default:
			// Skip finding, signal, receipt, canonicalization_failure entries
			continue
		}

		result = append(result, se)
	}

	return result, nil
}
