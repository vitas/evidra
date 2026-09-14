package proxy

import (
	"encoding/json"
	"strconv"
	"strings"
	"testing"
)

// obsOf digs the observation body out of an evidra_report acknowledgment.
func obsOf(t *testing.T, ack map[string]any) map[string]any {
	t.Helper()
	raw, ok := ack["observations"]
	if !ok {
		t.Fatalf("report ack carries no observations: %v", ack)
	}
	body, ok := raw.(map[string]any)
	if !ok {
		t.Fatalf("observations is %T, want an object", raw)
	}
	return body
}

func countOf(t *testing.T, body map[string]any, field string) int {
	t.Helper()
	value, ok := body[field]
	if !ok {
		t.Fatalf("observations omit %q: %v", field, body)
	}
	switch number := value.(type) {
	case float64: // through JSON
		return int(number)
	case int: // built in-process by the unit tests
		return number
	default:
		t.Fatalf("%s = %T, want a number", field, value)
		return 0
	}
}

// TestReportAckCarriesWhatTheProxyObserved is the §47 in-band item: the counts must
// reach the agent in the tool result, because a fact that only appears in a file after
// the session cannot change the next action.
func TestReportAckCarriesWhatTheProxyObserved(t *testing.T) {
	h := startEndpoint(t, "--enforce=all", fixtureBin)
	h.initialize()
	prescribed, isErr := h.call("evidra_prescribe", map[string]any{"objective": "check status twice"})
	if isErr {
		t.Fatal("prescribe failed")
	}
	opID, _ := prescribed["operation_id"].(string)
	for range 2 {
		if _, isErr := h.call("get_status", map[string]any{"verbose": true}); isErr {
			t.Fatal("get_status failed")
		}
	}
	ack, isErr := h.call("evidra_report", map[string]any{
		"operation_id": opID, "status": "completed", "outcome": "achieved",
	})
	if isErr {
		t.Fatalf("report rejected: %v", ack)
	}
	body := obsOf(t, ack)
	if got := countOf(t, body, "executions_observed"); got != 2 {
		t.Errorf("executions_observed = %d, want 2", got)
	}
	if got := countOf(t, body, "succeeded"); got != 2 {
		t.Errorf("succeeded = %d, want 2", got)
	}
	if got := countOf(t, body, "failed_or_cancelled"); got != 0 {
		t.Errorf("failed_or_cancelled = %d, want 0", got)
	}
	if body["provenance"] != "proxy_observed" {
		t.Errorf("provenance = %v, want proxy_observed", body["provenance"])
	}
	note, _ := body["note"].(string)
	if note == "" {
		t.Error("observations carry no note explaining their scope")
	}
	// The statement must not read as a verdict: no field here says achieved or not.
	for _, forbidden := range []string{"verdict", "score", "success", "risk"} {
		if _, present := body[forbidden]; present {
			t.Errorf("observations carry the verdict-like field %q", forbidden)
		}
	}
}

// TestReportAckStatesAnEmptyWindow is the case the feedback exists for: an agent that
// closes an operation as achieved while nothing was observed has to be told, in the
// same turn, that the record disagrees.
func TestReportAckStatesAnEmptyWindow(t *testing.T) {
	h := startEndpoint(t, "--enforce=all", fixtureBin)
	h.initialize()
	prescribed, isErr := h.call("evidra_prescribe", map[string]any{"objective": "claim without acting"})
	if isErr {
		t.Fatal("prescribe failed")
	}
	opID, _ := prescribed["operation_id"].(string)
	ack, isErr := h.call("evidra_report", map[string]any{
		"operation_id": opID, "status": "completed", "outcome": "achieved",
	})
	if isErr {
		t.Fatalf("report rejected: %v", ack)
	}
	body := obsOf(t, ack)
	if got := countOf(t, body, "executions_observed"); got != 0 {
		t.Fatalf("executions_observed = %d, want 0", got)
	}
	note, _ := body["note"].(string)
	if note == "" || !strings.Contains(note, "No upstream tool execution was observed") {
		t.Errorf("empty window is not stated plainly: %q", note)
	}
}

// TestObservationsExistWithoutAStore keeps the feedback honest about its own source.
// Observation is what the proxy does; persistence is a separate choice (§18), so
// turning off --evidence-dir must not blind the response.
func TestObservationsExistWithoutAStore(t *testing.T) {
	h := startEndpoint(t, "--enforce=off", fixtureBin)
	h.initialize()
	prescribed, isErr := h.call("evidra_prescribe", map[string]any{"objective": "observe only"})
	if isErr {
		t.Fatal("prescribe failed")
	}
	opID, _ := prescribed["operation_id"].(string)
	if _, isErr := h.call("unknown_action", map[string]any{"a": 1}); isErr {
		t.Fatal("observe-only should forward an unprescribed call")
	}
	ack, isErr := h.call("evidra_report", map[string]any{
		"operation_id": opID, "status": "completed", "outcome": "achieved",
	})
	if isErr {
		t.Fatalf("report rejected: %v", ack)
	}
	if got := countOf(t, obsOf(t, ack), "executions_observed"); got != 1 {
		t.Errorf("executions_observed = %d, want 1 without an evidence store", got)
	}
}

func TestObservationCapBoundsTrackedOperations(t *testing.T) {
	obs := newObservations()
	for i := range epObsCap + 50 {
		id := "EV-" + strconv.Itoa(i)
		obs.note(id, "restart", true, nil)
	}
	obs.mu.Lock()
	tracked := len(obs.byOp)
	order := len(obs.order)
	dropped := obs.dropped
	obs.mu.Unlock()
	if tracked > epObsCap || order > epObsCap {
		t.Fatalf("tracked %d operations with map %d, want capped at %d", order, tracked, epObsCap)
	}
	if dropped == 0 {
		t.Error("the cap silently discarded feedback instead of reporting it")
	}
	body, _ := obs.renderFor("EV-a0")
	if count := countOf(t, body, "older_operations_dropped"); count != dropped {
		t.Errorf("older_operations_dropped = %d, want %d", count, dropped)
	}
}

func TestReadOnlyObservationsFormAConjunctionThatCannotReopen(t *testing.T) {
	trueValue, falseValue := true, false
	cases := []struct {
		name  string
		seen  []*bool
		value any
	}{
		{"all declared read-only", []*bool{&trueValue, &trueValue}, true},
		{"one non-read-only call wins", []*bool{&trueValue, &falseValue}, false},
		{"order does not matter", []*bool{&falseValue, &trueValue}, false},
		{"no annotation says nothing", []*bool{nil, nil}, nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			obs := newObservations()
			for _, flag := range tc.seen {
				obs.note("EV-1", "tool", true, flag)
			}
			body, _ := obs.renderFor("EV-1")
			got, present := body["all_observed_declared_read_only"]
			if tc.value == nil {
				if present {
					t.Fatalf("unannotated traffic produced a read-only statement: %v", got)
				}
				return
			}
			if got != tc.value {
				t.Fatalf("all_observed_declared_read_only = %v, want %v", got, tc.value)
			}
			if body["annotations_verified"] != false {
				t.Error("the read-only statement is reported without saying the annotations are unverified")
			}
		})
	}
}

func TestReadOnlyAnnotationDistinguishesAbsentFromFalse(t *testing.T) {
	if got := epReadOnlyAnnotation(nil); got != nil {
		t.Errorf("absent annotations gave %v, want nil", *got)
	}
	raw, err := json.Marshal(map[string]any{"readOnlyHint": false})
	if err != nil {
		t.Fatal(err)
	}
	got := epReadOnlyAnnotation(raw)
	if got == nil || *got {
		t.Fatalf("readOnlyHint=false parsed as %v", got)
	}
	// A tool whose annotations carry no read-only hint has made no claim; treating
	// silence as "not read-only" would invent enforcement semantics (§27).
	other, err := json.Marshal(map[string]any{"title": "status"})
	if err != nil {
		t.Fatal(err)
	}
	if got := epReadOnlyAnnotation(other); got != nil {
		t.Errorf("annotation without a read-only hint gave %v, want nil", *got)
	}
}
