package analyticsdb

import (
	"encoding/json"
	"fmt"

	"samebits.com/evidra-benchmark/internal/analytics"
	"samebits.com/evidra-benchmark/pkg/evidence"
)

// StoredRow is the minimum stored-entry shape needed to replay analytics from DB-backed evidence.
type StoredRow struct {
	ID      string
	Payload json.RawMessage
}

// EvidenceEntriesFromStoredRows decodes stored JSON payloads back into canonical evidence entries.
func EvidenceEntriesFromStoredRows(rows []StoredRow) ([]evidence.EvidenceEntry, error) {
	result := make([]evidence.EvidenceEntry, 0, len(rows))
	for _, row := range rows {
		var entry evidence.EvidenceEntry
		if err := json.Unmarshal(row.Payload, &entry); err != nil {
			return nil, fmt.Errorf("stored row %s: decode evidence payload: %w", row.ID, err)
		}
		result = append(result, entry)
	}
	return result, nil
}

// ComputeScorecardFromStoredRows replays DB-backed evidence through the shared analytics engine.
func ComputeScorecardFromStoredRows(rows []StoredRow, filters analytics.Filters) (analytics.ScorecardOutput, error) {
	entries, err := EvidenceEntriesFromStoredRows(rows)
	if err != nil {
		return analytics.ScorecardOutput{}, err
	}
	return analytics.ComputeScorecard(entries, filters)
}

// ComputeExplainFromStoredRows replays DB-backed evidence through the shared explain path.
func ComputeExplainFromStoredRows(rows []StoredRow, filters analytics.Filters) (analytics.ExplainOutput, error) {
	entries, err := EvidenceEntriesFromStoredRows(rows)
	if err != nil {
		return analytics.ExplainOutput{}, err
	}
	return analytics.ComputeExplain(entries, filters)
}
