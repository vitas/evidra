package evidence

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
)

func ComputeHash(record EvidenceRecord) (string, error) {
	payload := canonicalEvidenceRecord{
		EventID:         record.EventID,
		Timestamp:       record.Timestamp.UTC(),
		PolicyRef:       record.PolicyRef,
		Actor:           record.Actor,
		Tool:            record.Tool,
		Operation:       record.Operation,
		Params:          record.Params,
		PolicyDecision:  record.PolicyDecision,
		ExecutionResult: record.ExecutionResult,
		PreviousHash:    record.PreviousHash,
	}

	b, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("marshal canonical record: %w", err)
	}

	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:]), nil
}
