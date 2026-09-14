package proxy

// In-band observation feedback (§47), returned with the acknowledgment of
// `evidra_report`.
//
// The distinction that makes this allowed at all is §35's: "Do not synchronously
// rebuild a human summary inside evidra_report." So this is not a summary. It is the
// narrowest possible statement about the operation the agent just closed - how many
// calls the endpoint actually observed inside its window, and how they ended - computed
// from what the proxy saw, never from what the agent said. It carries no verdict, no
// score, and no advice about whether the goal was achieved.
//
// The reason to put it in the report response rather than only in summary.json is that
// an agent reads the tool result now. A reconciliation that reaches a human after the
// session is worth something; the same fact arriving before the agent's next action can
// change that action.
//
// The counters live in the endpoint rather than the store, so they are available when
// recording is switched off: observation is what the proxy does, persistence is a
// separate choice (§18). Fingerprints stay on the read side in `pkg/report`, where the
// digest key is available and where retry-pattern analysis belongs.

import (
	"encoding/json"
	"sync"
)

// epObsCap bounds how many closed-but-unreported operations keep counters. A session
// that prescribes thousands of operations without reading their feedback cannot be
// answered in full, and the point of the cap is that the cap itself is visible rather
// than the map growing until the endpoint is the reason a container was OOM-killed.
const epObsCap = 128

// epOpObs is the per-operation observation counter.
type epOpObs struct {
	executions int
	succeeded  int
	failed     int
	// tools names at most epObsToolCap distinct tools called inside the operation,
	// so the feedback can say what was touched without echoing arguments.
	tools []string
	// readOnly is the conjunction of the upstream's own annotations: true only when
	// every observed execution was annotated read-only, false when at least one was
	// not, and nil when no annotation was seen at all. It is upstream-supplied,
	// unverified by construction (§27), and is never used to decide anything.
	readOnly *bool
}

const epObsToolCap = 5

// epObservations holds the counters, keyed by operation id.
type epObservations struct {
	mu      sync.Mutex
	byOp    map[string]*epOpObs
	order   []string
	dropped int
}

func newObservations() *epObservations {
	return &epObservations{byOp: map[string]*epOpObs{}}
}

// note records one finished execution against its operation. An empty operation id is
// still counted: under observe-only mode most calls have no open operation, and the
// totals are the difference between "nothing was prescribed" and "nothing happened".
func (o *epObservations) note(operationID, tool string, ok bool, readOnly *bool) {
	if o == nil {
		return
	}
	o.mu.Lock()
	defer o.mu.Unlock()
	obs := o.byOp[operationID]
	if obs == nil {
		if len(o.order) >= epObsCap {
			// Drop the oldest tracked operation rather than the newest. Its feedback
			// is already stale by definition: the agent has moved on without reading it.
			evict := o.order[0]
			o.order = o.order[1:]
			delete(o.byOp, evict)
			o.dropped++
		}
		obs = &epOpObs{}
		o.byOp[operationID] = obs
		o.order = append(o.order, operationID)
	}
	obs.executions++
	if ok {
		obs.succeeded++
	} else {
		obs.failed++
	}
	if tool != "" && len(obs.tools) < epObsToolCap {
		seen := false
		for _, t := range obs.tools {
			if t == tool {
				seen = true
				break
			}
		}
		if !seen {
			obs.tools = append(obs.tools, tool)
		}
	}
	switch {
	case readOnly == nil:
	case obs.readOnly == nil:
		obs.readOnly = readOnly
	case !*readOnly || !*obs.readOnly:
		// Any non-read-only observation makes the conjunction false; it cannot become
		// true again, because the earlier call really did happen.
		falseValue := false
		obs.readOnly = &falseValue
	}
}

// renderFor returns the compact statement for a closed operation and stops tracking it.
// The bool reports whether anything was observed at all, which the caller needs: an
// absent entry and a zero count are different statements, and conflating them is how a
// recorder starts agreeing with an over-claiming agent.
func (o *epObservations) renderFor(operationID string) (map[string]any, bool) {
	if o == nil {
		return nil, false
	}
	o.mu.Lock()
	defer o.mu.Unlock()
	obs, ok := o.byOp[operationID]
	if !ok {
		return epObservationBody(&epOpObs{}, o.dropped), false
	}
	delete(o.byOp, operationID)
	for i, id := range o.order {
		if id == operationID {
			o.order = append(o.order[:i], o.order[i+1:]...)
			break
		}
	}
	return epObservationBody(obs, o.dropped), true
}

func epObservationBody(obs *epOpObs, dropped int) map[string]any {
	body := map[string]any{
		"executions_observed": obs.executions,
		"succeeded":           obs.succeeded,
		"failed_or_cancelled": obs.failed,
	}
	if len(obs.tools) > 0 {
		body["tools_observed"] = obs.tools
	}
	if obs.readOnly != nil {
		body["all_observed_declared_read_only"] = *obs.readOnly
		// Stated every time the field appears, because the annotation is a claim by
		// the upstream server, not a fact Evidra verified (§27).
		body["annotations_verified"] = false
	}
	if dropped > 0 {
		body["older_operations_dropped"] = dropped
	}
	body["provenance"] = "proxy_observed"
	return body
}

// epReadOnlyAnnotation reads the read-only claim out of an upstream tool annotation.
// Absent is not false: a tool with no annotation has made no claim, and counting it as
// "not read-only" would silently upgrade enforcement semantics into a verdict about the
// operation's safety.
func epReadOnlyAnnotation(raw json.RawMessage) *bool {
	if len(raw) == 0 {
		return nil
	}
	var ann struct {
		ReadOnly    *bool `json:"readOnlyHint"`
		ReadOnlyAlt *bool `json:"readOnly"`
	}
	if err := json.Unmarshal(raw, &ann); err != nil {
		return nil
	}
	if ann.ReadOnly != nil {
		return ann.ReadOnly
	}
	return ann.ReadOnlyAlt
}
