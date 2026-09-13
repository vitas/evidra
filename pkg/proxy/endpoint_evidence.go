package proxy

// Durable evidence for the merged endpoint (§14-§21 of the vNext plan).
//
// The interface exists so the endpoint can state what happened without knowing how
// it is stored, and so a store failure can change what the endpoint is allowed to
// do: an execution the recorder could not note must not be forwarded, while an
// agent trying to close its record must always be heard. Those are opposite
// policies and both belong on this boundary.

import (
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/oklog/ulid/v2"

	"samebits.com/evidra/pkg/evidence"
)

// epExecution identifies one forwarded call for its terminal event.
type epExecution struct {
	ID          string
	Tool        string
	SessionID   string
	OperationID string
	StartedAt   time.Time
}

// epEvidence is what the endpoint needs from a recorder. Every method's error is
// a durability statement, not a protocol error, so callers surface it to the agent
// only where the plan says to (§18).
type epEvidence interface {
	startExecution(sessionID, operationID, tool string, args json.RawMessage, annotations json.RawMessage) (epExecution, error)
	finishExecution(ex epExecution, result json.RawMessage, status evidence.ExecutionStatus, errorCode, errorMessage string) error

	prescribed(sessionID string, op epOperation, hints []string) error
	replaced(sessionID string, replacedID, reason string, op epOperation) error
	reported(sessionID string, op epOperation, status, outcome, summary string) error
	blocked(sessionID, operationID, tool, kind, detail string) error

	unhealthy() bool
	digestKey() []byte
	noteLifecycle(eventType evidence.EventType, reason string)
	close(reason string) error
}

// epNopEvidence is used when no --evidence-dir was given: observe and forward,
// record nothing. It is not a fallback that silently pretends to be durable,
// because the caller who asked for no store should not get one, and the caller who
// asked for a store must not get this.
type epNopEvidence struct{}

func (epNopEvidence) startExecution(string, string, string, json.RawMessage, json.RawMessage) (epExecution, error) {
	return epExecution{StartedAt: time.Now()}, nil
}
func (epNopEvidence) finishExecution(epExecution, json.RawMessage, evidence.ExecutionStatus, string, string) error {
	return nil
}
func (epNopEvidence) prescribed(string, epOperation, []string) error             { return nil }
func (epNopEvidence) replaced(string, string, string, epOperation) error         { return nil }
func (epNopEvidence) reported(string, epOperation, string, string, string) error { return nil }
func (epNopEvidence) blocked(string, string, string, string, string) error       { return nil }
func (epNopEvidence) unhealthy() bool                                            { return false }
func (epNopEvidence) digestKey() []byte                                          { return nil }
func (epNopEvidence) noteLifecycle(evidence.EventType, string)                   {}
func (epNopEvidence) close(string) error                                         { return nil }

// epStoreEvidence writes the v2 chain. One store belongs to one recorder process,
// which is also one MCP session family, so seq and the hash tail need no
// coordination beyond the store's own serialization boundary.
type epStoreEvidence struct {
	store   *evidence.Store
	version string
	enforce string
	actor   evidence.ActorRef
	maxRes  int
}

// newStoreEvidence opens (or resumes) a recorder store for this endpoint.
func newStoreEvidence(store *evidence.Store, version, enforce, actorID string) *epStoreEvidence {
	e := &epStoreEvidence{store: store, version: version, enforce: enforce, maxRes: store.MaxResultBytes()}
	if actorID != "" {
		e.actor = evidence.ActorRef{ID: actorID, Version: version}
	}
	return e
}

func (s *epStoreEvidence) unhealthy() bool { return s.store.Unhealthy() }

func (s *epStoreEvidence) digestKey() []byte { return s.store.DigestKey() }

func (s *epStoreEvidence) newEvent(t evidence.EventType, p evidence.Provenance, sessionID, operationID string) evidence.Event {
	ev := s.store.NewEvent(t, p, sessionID, operationID)
	ev.Actor = s.actor
	return ev
}

func (s *epStoreEvidence) append(ev evidence.Event) error {
	if s.store == nil {
		return fmt.Errorf("evidence: closed")
	}
	_, err := s.store.AppendEvent(ev)
	return err
}

// startExecution writes execution_started before anything is forwarded. A failure
// here means the call does not go out: an unrecorded operational action is exactly
// the thing this product exists to prevent (§18).
func (s *epStoreEvidence) startExecution(sessionID, operationID, tool string, args json.RawMessage, annotations json.RawMessage) (epExecution, error) {
	key := s.store.DigestKey()
	fingerprint, err := evidence.ArgumentsHMAC(key, json.RawMessage(args))
	if err != nil {
		return epExecution{}, err
	}
	ex := epExecution{
		ID:          "EXE-" + ulid.Make().String(),
		Tool:        tool,
		SessionID:   sessionID,
		OperationID: operationID,
		StartedAt:   time.Now().UTC(),
	}
	ev := s.newEvent(evidence.EventExecutionStarted, evidence.ProvenanceProxyObserved, sessionID, operationID)
	if err := ev.SetPayload(evidence.ExecutionStartedPayload{
		ExecutionID: ex.ID, Tool: tool, ArgumentsHMAC: fingerprint,
		Annotations: annotations, StartedAt: ex.StartedAt,
	}); err != nil {
		return epExecution{}, err
	}
	if err := s.append(ev); err != nil {
		return epExecution{}, err
	}
	return ex, nil
}

// finishExecution records the outcome after the real response was already relayed.
// Losing this write must not corrupt the agent's view of its own call, so the
// caller relays first and reports the durability failure to the log and to health
// state (§18).
func (s *epStoreEvidence) finishExecution(ex epExecution, result json.RawMessage, status evidence.ExecutionStatus, errorCode, errorMessage string) error {
	fingerprint, state, format := evidence.ResultFingerprint(s.store.DigestKey(), result, s.maxRes)
	ev := s.newEvent(evidence.EventExecutionFinished, evidence.ProvenanceProxyObserved, ex.SessionID, ex.OperationID)
	payload := evidence.ExecutionFinishedPayload{
		ExecutionID: ex.ID, Tool: ex.Tool, Status: status,
		ResultHMAC: pointerTo(fingerprint), ResultFingerprintStatus: state, ResultFingerprintFormat: format,
		ErrorCode: errorCode, ErrorMessageHMAC: evidence.ErrorFingerprint(s.store.DigestKey(), errorMessage),
		DurationMS: time.Since(ex.StartedAt).Milliseconds(), FinishedAt: time.Now().UTC(),
	}
	if err := ev.SetPayload(payload); err != nil {
		return err
	}
	return s.append(ev)
}

func (s *epStoreEvidence) prescribed(sessionID string, op epOperation, hints []string) error {
	ev := s.newEvent(evidence.EventOperationPrescribed, evidence.ProvenanceAgentDeclared, sessionID, op.ID)
	return s.append(withPayload(ev, evidence.PrescribedPayload{
		Objective: op.Objective, ExpectedOutcome: op.ExpectedOutcome, ToolHints: hints,
	}))
}

func (s *epStoreEvidence) replaced(sessionID, replacedID, reason string, op epOperation) error {
	// The agent asked for the replacement, so this is its declaration even though
	// the recorder wrote the event (§15).
	ev := s.newEvent(evidence.EventOperationReplaced, evidence.ProvenanceAgentDeclared, sessionID, op.ID)
	return s.append(withPayload(ev, evidence.ReplacedPayload{
		ReplacedOperationID: replacedID, Reason: reason, Objective: op.Objective,
	}))
}

func (s *epStoreEvidence) reported(sessionID string, op epOperation, status, outcome, summary string) error {
	ev := s.newEvent(evidence.EventOperationReported, evidence.ProvenanceAgentDeclared, sessionID, op.ID)
	return s.append(withPayload(ev, evidence.ReportedPayload{
		Status: status, Outcome: outcome, Summary: summary, OperationID: op.ID,
	}))
}

func (s *epStoreEvidence) blocked(sessionID, operationID, tool, kind, detail string) error {
	ev := s.newEvent(evidence.EventProtocolViolation, evidence.ProvenanceRecorderGenerated, sessionID, operationID)
	return s.append(withPayload(ev, evidence.ViolationPayload{
		Kind: kind, Tool: tool, EnforceMode: s.enforce, Detail: detail,
	}))
}

func (s *epStoreEvidence) noteLifecycle(eventType evidence.EventType, reason string) {
	ev := s.newEvent(eventType, evidence.ProvenanceRecorderGenerated, "", "")
	_ = s.append(withPayload(ev, evidence.LifecyclePayload{
		Reason: reason, EvidraVersion: s.version, EnforceMode: s.enforce,
	}))
}

// close stops the store and removes it if nothing substantive was ever recorded.
func (s *epStoreEvidence) close(reason string) error {
	if s.store == nil {
		return nil
	}
	err := s.store.Close(reason)
	if err == nil && !s.store.Substantive() {
		return s.dirRemove()
	}
	return err
}

func (s *epStoreEvidence) dirRemove() error {
	// A recorder that saw nothing has no business leaving a directory behind (§20).
	return os.RemoveAll(s.store.Dir())
}

func withPayload(ev evidence.Event, p any) evidence.Event {
	_ = ev.SetPayload(p)
	return ev
}

func pointerTo(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

var (
	_ epEvidence = epNopEvidence{}
	_ epEvidence = (*epStoreEvidence)(nil)
)
