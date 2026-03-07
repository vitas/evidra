package automationevent

import (
	"encoding/json"
	"fmt"
	"strings"

	"samebits.com/evidra-benchmark/pkg/evidence"
)

const (
	// RecordContractVersionV1 is the only supported version for v1 ingestion.
	RecordContractVersionV1 = "v1"
)

// RecordInput is the v1 structured ingestion contract for completed automation executions.
// It is consumed by the `evidra record` command and mapped into the existing lifecycle path.
type RecordInput struct {
	ContractVersion string          `json:"contract_version"`
	SessionID       string          `json:"session_id"`
	OperationID     string          `json:"operation_id"`
	Tool            string          `json:"tool"`
	Operation       string          `json:"operation"`
	Environment     string          `json:"environment"`
	Actor           evidence.Actor  `json:"actor"`
	ExitCode        int             `json:"exit_code"`
	DurationMs      int64           `json:"duration_ms"`
	Attempt         int             `json:"attempt,omitempty"`
	RawArtifact     string          `json:"raw_artifact,omitempty"`
	CanonicalAction json.RawMessage `json:"canonical_action,omitempty"`
}

// ValidationError contains one or more contract violations.
type ValidationError struct {
	Violations []string
}

func (e *ValidationError) Error() string {
	return fmt.Sprintf("record contract validation failed: %s", strings.Join(e.Violations, "; "))
}

// ValidateRecordInput validates the v1 record contract input.
func ValidateRecordInput(in RecordInput) error {
	var violations []string

	if strings.TrimSpace(in.ContractVersion) == "" {
		violations = append(violations, "contract_version is required")
	} else if strings.TrimSpace(in.ContractVersion) != RecordContractVersionV1 {
		violations = append(violations, "contract_version must be v1")
	}

	if strings.TrimSpace(in.SessionID) == "" {
		violations = append(violations, "session_id is required")
	}
	if strings.TrimSpace(in.OperationID) == "" {
		violations = append(violations, "operation_id is required")
	}
	if strings.TrimSpace(in.Tool) == "" {
		violations = append(violations, "tool is required")
	}
	if strings.TrimSpace(in.Operation) == "" {
		violations = append(violations, "operation is required")
	}
	if strings.TrimSpace(in.Environment) == "" {
		violations = append(violations, "environment is required")
	}
	if strings.TrimSpace(in.Actor.Type) == "" {
		violations = append(violations, "actor.type is required")
	}
	if strings.TrimSpace(in.Actor.ID) == "" {
		violations = append(violations, "actor.id is required")
	}
	if in.DurationMs < 0 {
		violations = append(violations, "duration_ms must be >= 0")
	}
	if in.Attempt < 0 {
		violations = append(violations, "attempt must be >= 0")
	}

	hasArtifact := strings.TrimSpace(in.RawArtifact) != ""
	hasCanonical := len(bytesTrimSpace(in.CanonicalAction)) > 0
	if !hasArtifact && !hasCanonical {
		violations = append(violations, "one of raw_artifact or canonical_action is required")
	}

	if len(violations) > 0 {
		return &ValidationError{Violations: violations}
	}
	return nil
}

func bytesTrimSpace(v []byte) []byte {
	return []byte(strings.TrimSpace(string(v)))
}
