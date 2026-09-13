package evidence

// Evidence model v2 (§14-§18 of the vNext plan).
//
// One flat envelope, one file per recorder process, a hash chain over that file,
// and provenance on every event. The shape is deliberately small: the reason v1
// grew a graph of entities is that each new question got its own table, while v2
// answers questions by reading events in order.

import (
	"encoding/json"
	"fmt"
	"time"
)

// SchemaVersion is the only schema this package writes.
const SchemaVersion = "evidra.evidence.v2"

// Provenance separates what the agent claimed from what the recorder observed.
// Mixing them is the failure this model exists to prevent: a report that says
// "achieved" is a claim, an execution that returned an error is an observation,
// and only the difference between the two is evidence.
type Provenance string

const (
	// ProvenanceAgentDeclared is text the agent chose: prescriptions and reports.
	ProvenanceAgentDeclared Provenance = "agent_declared"
	// ProvenanceProxyObserved is what the wire showed: executions.
	ProvenanceProxyObserved Provenance = "proxy_observed"
	// ProvenanceRecorderGenerated is a recorder fact: lifecycle, blocks, integrity.
	ProvenanceRecorderGenerated Provenance = "recorder_generated"
)

// EventType is the discriminator in the envelope.
type EventType string

const (
	EventRecorderStarted  EventType = "recorder_started"
	EventRecorderStopped  EventType = "recorder_stopped"
	EventRecorderDegraded EventType = "recorder_degraded"

	EventOperationPrescribed EventType = "operation_prescribed"
	EventOperationReplaced   EventType = "operation_replaced"
	EventOperationReported   EventType = "operation_reported"

	EventExecutionStarted  EventType = "execution_started"
	EventExecutionFinished EventType = "execution_finished"

	EventProtocolViolation EventType = "protocol_violation"
)

// Substantive reports whether the event is evidence rather than lifecycle noise.
// A recorder that exits having written only lifecycle events removes its
// directory instead of leaving clutter behind (§20).
func (t EventType) Substantive() bool {
	switch t {
	case EventRecorderStarted, EventRecorderStopped:
		return false
	default:
		return true
	}
}

// ExecutionStatus is the terminal state of one proxied call. Unknown is
// representable on purpose: a call whose response never arrived must not be
// recorded as either success or failure.
type ExecutionStatus string

const (
	ExecutionSuccess   ExecutionStatus = "success"
	ExecutionError     ExecutionStatus = "error"
	ExecutionCancelled ExecutionStatus = "cancelled"
	ExecutionUnknown   ExecutionStatus = "unknown"
)

// ActorRef identifies who is accountable for an event. Kept minimal per §14: an
// id and the client version, no domain verification in the MVP. The obvious name
// is taken by the v1 entry type, which this package keeps until §59 step 9 prunes
// it; the JSON field is unaffected.
type ActorRef struct {
	ID      string `json:"id"`
	Version string `json:"version,omitempty"`
}

// Event is one record in the chain.
type Event struct {
	SchemaVersion string    `json:"schema_version"`
	Seq           uint64    `json:"seq"`
	EventID       string    `json:"event_id"`
	EventType     EventType `json:"event_type"`
	RecordedAt    time.Time `json:"recorded_at"`

	RecorderInstanceID string `json:"recorder_instance_id"`
	SessionID          string `json:"session_id,omitempty"`
	OperationID        string `json:"operation_id,omitempty"`

	UpstreamID string `json:"upstream_id,omitempty"`
	ServerName string `json:"server_name,omitempty"`

	Actor      ActorRef   `json:"actor,omitempty"`
	Provenance Provenance `json:"provenance"`

	Payload json.RawMessage `json:"payload,omitempty"`

	PreviousHash string `json:"previous_hash,omitempty"`
	Hash         string `json:"hash"`
	Signature    string `json:"signature"`
}

// PrescribedPayload is the agent's declaration when it opens an operation.
//
// Objective and expected outcome stay readable as text: the product's whole
// purpose is comparing stated intent with observed execution, and a fingerprint
// of the intent cannot be reconciled with anything (§22).
type PrescribedPayload struct {
	Objective       string   `json:"objective"`
	ExpectedOutcome string   `json:"expected_outcome,omitempty"`
	ToolHints       []string `json:"tool_hints,omitempty"`
}

// ReplacedPayload records an agent-authorized replacement of an open operation.
type ReplacedPayload struct {
	ReplacedOperationID string `json:"replaced_operation_id"`
	Reason              string `json:"reason,omitempty"`
	Objective           string `json:"objective,omitempty"`
}

// ReportedPayload is the agent's account of how the operation ended.
type ReportedPayload struct {
	Status        string `json:"status"`
	Outcome       string `json:"outcome"`
	Summary       string `json:"summary,omitempty"`
	OperationID   string `json:"operation_id"`
	OperationOpen bool   `json:"-"`
}

// ExecutionStartedPayload is written before the call is forwarded upstream, so an
// execution that never finished still leaves a trace (§17).
type ExecutionStartedPayload struct {
	ExecutionID   string          `json:"execution_id"`
	Tool          string          `json:"tool"`
	ArgumentsHMAC string          `json:"arguments_hmac"`
	Annotations   json.RawMessage `json:"annotations,omitempty"`
	StartedAt     time.Time       `json:"started_at"`
}

// ExecutionFinishedPayload carries the bounded fingerprints of the result.
type ExecutionFinishedPayload struct {
	ExecutionID             string          `json:"execution_id"`
	Tool                    string          `json:"tool"`
	Status                  ExecutionStatus `json:"status"`
	ResultHMAC              *string         `json:"result_hmac"`
	ResultFingerprintStatus string          `json:"result_fingerprint_status"`
	ResultFingerprintFormat string          `json:"result_fingerprint_format,omitempty"`
	ErrorCode               string          `json:"error_code,omitempty"`
	ErrorMessageHMAC        string          `json:"error_message_hmac,omitempty"`
	DurationMS              int64           `json:"duration_ms"`
	FinishedAt              time.Time       `json:"finished_at"`
}

// ViolationPayload records an enforcement decision the recorder made, such as
// refusing an unprescribed call. This is recorder-generated, never a claim.
type ViolationPayload struct {
	Kind        string `json:"kind"`
	Tool        string `json:"tool,omitempty"`
	EnforceMode string `json:"enforce_mode,omitempty"`
	Detail      string `json:"detail,omitempty"`
}

// DegradedPayload marks a window in which enforcement kept deciding while
// evidence could not be persisted. Chain validity and coverage are separate
// statements: a valid chain does not mean every decision was recorded (§18).
type DegradedPayload struct {
	WindowStart time.Time `json:"window_start"`
	WindowEnd   time.Time `json:"window_end"`
	Reason      string    `json:"reason"`
}

// LifecyclePayload is used by recorder_started and recorder_stopped.
type LifecyclePayload struct {
	Reason          string `json:"reason,omitempty"`
	EvidraVersion   string `json:"evidra_version,omitempty"`
	EnforceMode     string `json:"enforce_mode,omitempty"`
	UpstreamCommand string `json:"upstream_command,omitempty"`
}

// FingerprintStatus values for result fingerprints (§24).
const (
	FingerprintPresent         = "present"
	FingerprintOmittedOversize = "omitted_oversize"
	FingerprintUnavailable     = "unavailable"
)

// SetPayload encodes p into the event payload, rejecting nil so a silently
// missing payload cannot be mistaken for an event with nothing to say.
func (e *Event) SetPayload(p any) error {
	if p == nil {
		return fmt.Errorf("event %s: nil payload", e.EventType)
	}
	raw, err := json.Marshal(p)
	if err != nil {
		return fmt.Errorf("event %s: encode payload: %w", e.EventType, err)
	}
	e.Payload = raw
	return nil
}

// DecodePayload reads the payload into the typed struct for its event type.
func (e Event) DecodePayload() (any, error) {
	var target any
	switch e.EventType {
	case EventOperationPrescribed:
		target = &PrescribedPayload{}
	case EventOperationReplaced:
		target = &ReplacedPayload{}
	case EventOperationReported:
		target = &ReportedPayload{}
	case EventExecutionStarted:
		target = &ExecutionStartedPayload{}
	case EventExecutionFinished:
		target = &ExecutionFinishedPayload{}
	case EventProtocolViolation:
		target = &ViolationPayload{}
	case EventRecorderDegraded:
		target = &DegradedPayload{}
	case EventRecorderStarted, EventRecorderStopped:
		target = &LifecyclePayload{}
	default:
		return nil, fmt.Errorf("event %d: unknown type %q", e.Seq, e.EventType)
	}
	if len(e.Payload) == 0 {
		return target, nil
	}
	if err := json.Unmarshal(e.Payload, target); err != nil {
		return nil, fmt.Errorf("event %d (%s): decode payload: %w", e.Seq, e.EventType, err)
	}
	return target, nil
}
