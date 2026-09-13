package proxy

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/oklog/ulid/v2"
)

// epRecorder receives lifecycle and observation notes. Step 2 wires a no-op;
// the v2 store of §59 step 4 replaces it. Keeping the seam narrow now means the
// relay does not have to be reopened when durable evidence arrives.
type epRecorder interface {
	note(eventType string, fields map[string]any)
}

// epNopRecorder discards notes; used until the v2 store lands.
type epNopRecorder struct{}

func (epNopRecorder) note(string, map[string]any) {}

// epOperation is one open prescribed operation (§31).
type epOperation struct {
	ID              string    `json:"operation_id"`
	Objective       string    `json:"objective"`
	ExpectedOutcome string    `json:"expected_outcome,omitempty"`
	RecordedAt      time.Time `json:"recorded_at"`
}

// epSession holds the single-open-operation rule of §11 for one endpoint
// process, which is also one MCP session.
type epSession struct {
	open         *epOperation
	lastClosed   *epOperation
	closedAt     time.Time
	prescribes   int
	reports      int
	replacements int
	blocks       int
}

// Enforcement modes (§7). Exactly two: an annotation-based exception would route
// enforcement through untrusted server metadata.
const (
	epEnforceAll = "all"
	epEnforceOff = "off"
)

// epLocalTools are the namespaced protocol tools advertised alongside the
// upstream list. Descriptions carry the contract because §9 makes tool
// descriptions the primary instruction surface.
var epLocalTools = []epTool{
	{
		Name:  "evidra_prescribe",
		Title: "Prescribe an operation",
		Description: `Open an operation before using any upstream tool of this server.

Call this once per intended piece of work, with the objective you actually intend to achieve. When an operation is already open, the call fails with operation_already_open; pass continue_current=true to keep working under the existing operation, or abandon_and_replace=true to close the record of the old one and open this one instead.

No risk score, resource list, or tool selection is required or inferred here.`,
		InputSchema: json.RawMessage(`{
  "type": "object",
  "properties": {
    "objective": {
      "type": "string",
      "description": "What you intend to achieve, in your own words."
    },
    "expected_outcome": {
      "type": "string",
      "description": "The observable state that will mean this operation succeeded."
    },
    "continue_current": {
      "type": "boolean",
      "description": "Keep the already-open operation instead of opening a new one."
    },
    "abandon_and_replace": {
      "type": "boolean",
      "description": "Replace the open operation with this one. The old operation is recorded as replaced without a report."
    }
  },
  "required": ["objective"]
}`),
		Annotations: map[string]any{
			"readOnlyHint":    false,
			"destructiveHint": false,
			"idempotentHint":  false,
			"openWorldHint":   false,
		},
	},
	{
		Name:  "evidra_report",
		Title: "Report an operation outcome",
		Description: `Close the open operation with what actually happened.

Call exactly once per operation, whether it completed, failed, was cancelled, or was abandoned. Status and outcome are your claims about the operation; they are recorded as claims and are never upgraded to verified facts because a tool call succeeded.

Use the operation_id returned by evidra_prescribe.`,
		InputSchema: json.RawMessage(`{
  "type": "object",
  "properties": {
    "operation_id": {
      "type": "string",
      "description": "The id returned by evidra_prescribe."
    },
    "status": {
      "type": "string",
      "enum": ["completed", "failed", "cancelled", "abandoned"],
      "description": "How the operation ended."
    },
    "outcome": {
      "type": "string",
      "enum": ["achieved", "not_achieved", "unknown"],
      "description": "Whether the expected outcome was reached."
    },
    "summary": {
      "type": "string",
      "description": "Optional short note about what happened."
    }
  },
  "required": ["operation_id", "status", "outcome"]
}`),
		Annotations: map[string]any{
			"readOnlyHint":    false,
			"destructiveHint": false,
			"idempotentHint":  true,
			"openWorldHint":   false,
		},
	},
}

var epStatuses = []string{"completed", "failed", "cancelled", "abandoned"}
var epOutcomes = []string{"achieved", "not_achieved", "unknown"}

// handlePrescribe implements §31 plus the second-prescribe fork of §11.
func (e *epEndpoint) handlePrescribe(raw json.RawMessage) (any, *epError) {
	var in struct {
		Objective         string `json:"objective"`
		ExpectedOutcome   string `json:"expected_outcome"`
		ContinueCurrent   bool   `json:"continue_current"`
		AbandonAndReplace bool   `json:"abandon_and_replace"`
		// ReplaceOpen is accepted as an alias: §11 names the field
		// replace_open in one place and abandon_and_replace in the choices
		// list, and an agent that echoes either back must not be punished.
		ReplaceOpen bool `json:"replace_open"`
	}
	if err := json.Unmarshal(raw, &in); err != nil {
		return nil, &epError{Code: epCodeInvalidRequest, Message: "invalid params",
			Data: map[string]any{"error": "invalid_params", "detail": err.Error()}}
	}
	objective := strings.TrimSpace(in.Objective)
	if objective == "" {
		return nil, &epError{Code: epCodeInvalidRequest, Message: "objective is required",
			Data: map[string]any{"error": "objective_required"}}
	}
	replace := in.AbandonAndReplace || in.ReplaceOpen

	e.stateMu.Lock()
	defer e.stateMu.Unlock()

	if e.state.open != nil {
		current := *e.state.open
		switch {
		case in.ContinueCurrent && replace:
			return nil, &epError{Code: epCodeInvalidRequest,
				Message: "continue_current and abandon_and_replace are mutually exclusive",
				Data:    map[string]any{"error": "conflicting_choices"}}
		case in.ContinueCurrent:
			// No new operation, and no second prescribe event (§11).
			return map[string]any{
				"operation_id":     current.ID,
				"state":            "open",
				"objective":        current.Objective,
				"expected_outcome": current.ExpectedOutcome,
				"note":             "Continuing the open operation. Use evidra_report when it ends.",
			}, nil
		case replace:
			next := &epOperation{
				ID:              epNewOperationID(),
				Objective:       objective,
				ExpectedOutcome: strings.TrimSpace(in.ExpectedOutcome),
				RecordedAt:      time.Now().UTC(),
			}
			e.state.replacements++
			// The old operation gets no synthetic terminal report; its derived
			// state becomes replaced_without_report (§11).
			e.recorder.note("operation_replaced", map[string]any{
				"old_operation_id": current.ID,
				"new_operation_id": next.ID,
				"authorized_by":    "agent",
				"provenance":       "agent_declared",
			})
			e.state.open = next
			e.state.prescribes++
			return epPrescribeOpened(next), nil
		default:
			return nil, &epError{Code: epCodeInvalidRequest,
				Message: "an operation is already open",
				Data: map[string]any{
					"error":        "operation_already_open",
					"operation_id": current.ID,
					"choices":      []string{"continue_current", "abandon_and_replace"},
				}}
		}
	}

	op := &epOperation{
		ID:              epNewOperationID(),
		Objective:       objective,
		ExpectedOutcome: strings.TrimSpace(in.ExpectedOutcome),
		RecordedAt:      time.Now().UTC(),
	}
	e.state.open = op
	e.state.prescribes++
	e.recorder.note("operation_prescribed", map[string]any{
		"operation_id":     op.ID,
		"objective":        op.Objective,
		"expected_outcome": op.ExpectedOutcome,
		"provenance":       "agent_declared",
	})
	return epPrescribeOpened(op), nil
}

func epPrescribeOpened(op *epOperation) map[string]any {
	out := map[string]any{
		"operation_id": op.ID,
		"state":        "open",
		"recorded_at":  op.RecordedAt.Format(time.RFC3339Nano),
		"note":         "Use evidra_report when this operation is finished.",
	}
	if op.ExpectedOutcome != "" {
		out["expected_outcome"] = op.ExpectedOutcome
	}
	return out
}

// handleReport implements §32's deterministic report-state rules.
func (e *epEndpoint) handleReport(raw json.RawMessage) (any, *epError) {
	var in struct {
		OperationID string `json:"operation_id"`
		Status      string `json:"status"`
		Outcome     string `json:"outcome"`
		Summary     string `json:"summary"`
	}
	if err := json.Unmarshal(raw, &in); err != nil {
		return nil, &epError{Code: epCodeInvalidRequest, Message: "invalid params",
			Data: map[string]any{"error": "invalid_params", "detail": err.Error()}}
	}
	if !epInList(in.Status, epStatuses) {
		return nil, &epError{Code: epCodeInvalidRequest,
			Message: fmt.Sprintf("status must be one of %s", strings.Join(epStatuses, ", ")),
			Data:    map[string]any{"error": "invalid_status", "allowed": epStatuses}}
	}
	if !epInList(in.Outcome, epOutcomes) {
		return nil, &epError{Code: epCodeInvalidRequest,
			Message: fmt.Sprintf("outcome must be one of %s", strings.Join(epOutcomes, ", ")),
			Data:    map[string]any{"error": "invalid_outcome", "allowed": epOutcomes}}
	}

	e.stateMu.Lock()
	defer e.stateMu.Unlock()

	if e.state.open == nil {
		if e.state.lastClosed != nil && e.state.lastClosed.ID == in.OperationID {
			// Terminal report coverage must not punish an agent for a duplicate
			// closing call on the operation it already closed (§32).
			return map[string]any{
				"ok":           true,
				"state":        "already_reported",
				"operation_id": in.OperationID,
			}, nil
		}
		return nil, &epError{Code: epCodeInvalidRequest,
			Message: "no operation is open in this session",
			Data:    map[string]any{"error": "no_open_operation"}}
	}
	if e.state.open.ID != in.OperationID {
		return nil, &epError{Code: epCodeInvalidRequest,
			Message: "operation_id does not match the open operation",
			Data: map[string]any{
				"error":             "operation_id_mismatch",
				"open_operation_id": e.state.open.ID,
				"submitted_id":      in.OperationID,
			}}
	}

	closed := *e.state.open
	e.state.open = nil
	e.state.lastClosed = &closed
	e.state.closedAt = time.Now().UTC()
	e.state.reports++
	e.recorder.note("operation_reported", map[string]any{
		"operation_id": closed.ID,
		"status":       in.Status,
		"outcome":      in.Outcome,
		"summary":      in.Summary,
		"provenance":   "agent_declared",
	})
	return map[string]any{
		"ok":           true,
		"state":        "reported",
		"operation_id": closed.ID,
		"note":         "Call evidra_prescribe before the next operation.",
	}, nil
}

func epNewOperationID() string { return "EV-" + ulid.Make().String() }

func epInList(v string, allowed []string) bool {
	for _, a := range allowed {
		if v == a {
			return true
		}
	}
	return false
}
