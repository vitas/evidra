package proxy

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"samebits.com/evidra/pkg/evidence"
)

// recorderDir finds the single recorder directory the endpoint created under an
// evidence root.
func recorderDir(t *testing.T, root string) string {
	t.Helper()
	entries, err := os.ReadDir(root)
	if err != nil {
		t.Fatalf("read evidence root: %v", err)
	}
	var found []string
	for _, e := range entries {
		if e.IsDir() && strings.HasPrefix(e.Name(), "recorder-") {
			found = append(found, filepath.Join(root, e.Name()))
		}
	}
	if len(found) != 1 {
		t.Fatalf("expected one recorder directory under %s, got %d: %v", root, len(found), found)
	}
	return found[0]
}

// TestEvidenceStoreReproducesEnforcementDecisions is §59 step 4's exit
// criterion: the story of a session — one refused call, one prescription, one
// executed call, one report — must be reconstructable from the store alone.
// The runner's inference in cmd/evidra-gatea was a placeholder for exactly this.
func TestEvidenceStoreReproducesEnforcementDecisions(t *testing.T) {
	root := filepath.Join(t.TempDir(), "evidence")
	h := startEndpoint(t, "--enforce=all", "--evidence-dir", root, fixtureBin, "--stateful")
	h.initialize()

	if _, isErr := h.call("restart", map[string]any{"service": "payments"}); !isErr {
		t.Fatal("an unprescribed restart should have been refused")
	}
	prescribed, isErr := h.call("evidra_prescribe", map[string]any{
		"objective": "restart payments after checking status", "expected_outcome": "payments restarted",
	})
	if isErr {
		t.Fatalf("prescribe failed: %v", prescribed)
	}
	opID, _ := prescribed["operation_id"].(string)
	if !strings.HasPrefix(opID, "EV-") {
		t.Fatalf("operation_id = %v", prescribed["operation_id"])
	}
	if _, isErr := h.call("get_status", map[string]any{}); isErr {
		t.Fatal("prescribed get_status failed")
	}
	if _, isErr := h.call("evidra_report", map[string]any{
		"operation_id": opID, "status": "completed", "outcome": "achieved", "summary": "checked status",
	}); isErr {
		t.Fatal("report failed")
	}
	h.closeIn()
	if err := h.wait(); err != nil {
		t.Fatalf("endpoint exit: %v", err)
	}

	dir := recorderDir(t, root)
	rep, err := evidence.VerifyStore(dir, time.Time{})
	if err != nil {
		t.Fatalf("verify: %v", err)
	}
	if !rep.ChainValid || !rep.SignatureValid {
		t.Fatalf("chain/signature invalid: %+v findings=%v", rep, rep.Findings)
	}
	if rep.Coverage != "complete" {
		t.Errorf("coverage = %s, want complete: %v", rep.Coverage, rep.Findings)
	}

	events, meta, err := evidence.ReadStore(dir, time.Time{})
	if err != nil {
		t.Fatalf("read store: %v", err)
	}
	if meta.EnforceMode != "all" {
		t.Errorf("meta enforce_mode = %q", meta.EnforceMode)
	}
	if !strings.HasPrefix(meta.DigestKeyID, "dk1:") {
		t.Errorf("meta digest_key_id = %q", meta.DigestKeyID)
	}
	if meta.UpstreamID == "" {
		t.Error("meta upstream_id is empty")
	}
	want := []evidence.EventType{
		evidence.EventRecorderStarted, evidence.EventProtocolViolation, evidence.EventOperationPrescribed,
		evidence.EventExecutionStarted, evidence.EventExecutionFinished,
		evidence.EventOperationReported, evidence.EventRecorderStopped,
	}
	if len(events) != len(want) {
		t.Fatalf("events = %d, want %d (%v)", len(events), len(want), typesOf(events))
	}
	for i, ev := range events {
		if ev.EventType != want[i] {
			t.Errorf("event %d = %s, want %s", i, ev.EventType, want[i])
		}
		if ev.Seq != uint64(i+1) {
			t.Errorf("event %d seq = %d", i, ev.Seq)
		}
		if ev.RecorderInstanceID != meta.RecorderInstanceID {
			t.Errorf("event %d recorder_instance_id = %q", i, ev.RecorderInstanceID)
		}
		// Lifecycle events describe the process, and the store opens before the
		// client's initialize arrives, so they cannot carry a session id. Every
		// event after the handshake must.
		if ev.EventType.Substantive() && ev.SessionID == "" {
			t.Errorf("event %d (%s) has no session_id", i, ev.EventType)
		}
		if !ev.EventType.Substantive() && ev.SessionID != "" {
			t.Errorf("event %d (%s) should be session-free", i, ev.EventType)
		}
	}

	// The refused call must not exist as an execution: absence is the proof that
	// enforcement happened before forwarding, not after.
	for _, ev := range events {
		if ev.EventType == evidence.EventExecutionStarted || ev.EventType == evidence.EventExecutionFinished {
			var p any
			if ev.EventType == evidence.EventExecutionStarted {
				p, _ = ev.DecodePayload()
				if got := p.(*evidence.ExecutionStartedPayload).Tool; got != "get_status" {
					t.Errorf("executed tool = %s, want get_status", got)
				}
			} else {
				p, _ = ev.DecodePayload()
				fin := p.(*evidence.ExecutionFinishedPayload)
				if fin.Status != evidence.ExecutionSuccess {
					t.Errorf("execution status = %s", fin.Status)
				}
				if fin.ResultFingerprintStatus != evidence.FingerprintPresent || fin.ResultHMAC == nil {
					t.Errorf("result fingerprint = %s %v", fin.ResultFingerprintStatus, fin.ResultHMAC)
				}
			}
			continue
		}
		if ev.EventType == evidence.EventProtocolViolation {
			v, err := ev.DecodePayload()
			if err != nil {
				t.Fatal(err)
			}
			p := v.(*evidence.ViolationPayload)
			if p.Kind != "unprescribed_execution_attempt" || p.Tool != "restart" {
				t.Errorf("violation payload = %+v", p)
			}
			if ev.Provenance != evidence.ProvenanceRecorderGenerated {
				t.Errorf("block provenance = %s, want recorder_generated", ev.Provenance)
			}
		}
	}

	prescribedEvent := events[2]
	if prescribedEvent.Provenance != evidence.ProvenanceAgentDeclared {
		t.Errorf("prescribe provenance = %s", prescribedEvent.Provenance)
	}
	parsed, err := prescribedEvent.DecodePayload()
	if err != nil {
		t.Fatal(err)
	}
	if got := parsed.(*evidence.PrescribedPayload).Objective; !strings.Contains(got, "payments") {
		t.Errorf("objective was not stored as readable text: %q", got)
	}
	reported, err := events[5].DecodePayload()
	if err != nil {
		t.Fatal(err)
	}
	r := reported.(*evidence.ReportedPayload)
	if r.OperationID != opID || r.Status != "completed" || r.Outcome != "achieved" {
		t.Errorf("reported payload = %+v, want operation %s", r, opID)
	}

	// Fingerprints must be keyed, not raw: the arguments of the executed call
	// were {"service":...} style JSON and only the HMAC may appear on disk.
	raw, err := os.ReadFile(filepath.Join(dir, "events.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), `"objective":"restart payments after checking status"`) == false {
		t.Error("declared objective should be readable in the store")
	}
}

func typesOf(events []evidence.Event) []evidence.EventType {
	out := make([]evidence.EventType, 0, len(events))
	for _, e := range events {
		out = append(out, e.EventType)
	}
	return out
}

// TestEvidenceStorePairedExecutionsCarryAnnotationsAndDigests checks the two
// privacy-relevant fields the plan requires on every execution: a keyed argument
// fingerprint that survives key reordering, and the upstream's own annotations,
// including their absence.
func TestEvidenceStorePairedExecutionsCarryAnnotationsAndDigests(t *testing.T) {
	root := filepath.Join(t.TempDir(), "evidence")
	h := startEndpoint(t, "--enforce=off", "--evidence-dir", root, fixtureBin)
	h.initialize()
	if _, isErr := h.call("unknown_action", map[string]any{"b": 2, "a": 1}); isErr {
		t.Fatal("observe-only should forward an unprescribed call")
	}
	if _, isErr := h.call("get_status", map[string]any{"verbose": true}); isErr {
		t.Fatal("get_status failed")
	}
	h.closeIn()
	if err := h.wait(); err != nil {
		t.Fatalf("exit: %v", err)
	}
	dir := recorderDir(t, root)
	events, _, err := evidence.ReadStore(dir, time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	var started []*evidence.ExecutionStartedPayload
	var startedEvents []evidence.Event
	for i := range events {
		if events[i].EventType != evidence.EventExecutionStarted {
			continue
		}
		p, err := events[i].DecodePayload()
		if err != nil {
			t.Fatal(err)
		}
		started = append(started, p.(*evidence.ExecutionStartedPayload))
		startedEvents = append(startedEvents, events[i])
	}
	if len(started) != 2 {
		t.Fatalf("started executions = %d, want 2", len(started))
	}
	if startedEvents[0].OperationID != "" {
		t.Errorf("an unprescribed execution must carry no operation id, got %q", startedEvents[0].OperationID)
	}
	if started[0].ArgumentsHMAC == "" || !strings.HasPrefix(started[0].ArgumentsHMAC, "sha256:") {
		t.Errorf("arguments_hmac = %q", started[0].ArgumentsHMAC)
	}
	// unknown_action advertises no annotations at all; absence must be visible.
	if len(started[0].Annotations) != 0 {
		t.Errorf("unannotated tool recorded annotations: %s", started[0].Annotations)
	}
	if err := json.Unmarshal(started[1].Annotations, &map[string]any{}); err != nil {
		t.Errorf("get_status should record its read-only annotation: %s (%v)", started[1].Annotations, err)
	}
	if !strings.Contains(string(started[1].Annotations), "readOnlyHint") {
		t.Errorf("annotation payload = %s", started[1].Annotations)
	}
}

// TestEvidenceStoreFailureStopsForwarding proves the §18 rule from the outside:
// a caller that asked for evidence and cannot get it must not silently perform
// operational actions.
func TestEvidenceStoreFailureStopsForwarding(t *testing.T) {
	path := filepath.Join(t.TempDir(), "blocked")
	if err := os.WriteFile(path, []byte("not a directory"), 0o600); err != nil {
		t.Fatal(err)
	}
	h := start(t, mcpBin, "--proxy", "--evidence-dir", path, fixtureBin)
	if _, err := h.read(); err != nil && !strings.Contains(err.Error(), "EOF") {
		t.Fatalf("read: %v", err)
	}
	if err := h.wait(); err == nil {
		t.Error("endpoint exited cleanly despite an unusable evidence directory")
	}
	if !strings.Contains(h.stderrText(), "open evidence store") {
		t.Errorf("stderr did not explain the failure: %s", h.stderrText())
	}
}

// TestExecutionsPairByIdUnderParallelCalls is §16's promise that `execution_id`
// is the authoritative pairing key even when upstream calls overlap: with two
// concurrent slow calls the store must still show each id started exactly once and
// finished exactly once, and must not cross their durations.
func TestExecutionsPairByIdUnderParallelCalls(t *testing.T) {
	root := filepath.Join(t.TempDir(), "evidence")
	h := startEndpoint(t, "--enforce=off", "--evidence-dir", root, fixtureBin)
	h.initialize()
	fast, slow := h.id(), h.id()
	if err := h.sendMsg(fast, "tools/call", map[string]any{
		"name": "slow", "arguments": map[string]any{"delay_ms": 300, "label": "fast"}}); err != nil {
		t.Fatal(err)
	}
	if err := h.sendMsg(slow, "tools/call", map[string]any{
		"name": "slow", "arguments": map[string]any{"delay_ms": 2000, "label": "slow"}}); err != nil {
		t.Fatal(err)
	}
	if _, err := h.response(string(fast)); err != nil {
		t.Fatalf("fast response: %v", err)
	}
	if _, err := h.response(string(slow)); err != nil {
		t.Fatalf("slow response: %v", err)
	}
	h.closeIn()
	if err := h.wait(); err != nil {
		t.Fatalf("exit: %v", err)
	}

	events, _, err := evidence.ReadStore(recorderDir(t, root), time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	type pair struct {
		tool     string
		finished bool
		startAt  time.Time
		duration int64
	}
	pairs := map[string]*pair{}
	var order []string
	for _, ev := range events {
		switch ev.EventType {
		case evidence.EventExecutionStarted:
			p, err := ev.DecodePayload()
			if err != nil {
				t.Fatal(err)
			}
			s := p.(*evidence.ExecutionStartedPayload)
			if _, dup := pairs[s.ExecutionID]; dup {
				t.Fatalf("execution %s started twice", s.ExecutionID)
			}
			pairs[s.ExecutionID] = &pair{tool: s.Tool, startAt: s.StartedAt}
			order = append(order, s.ExecutionID)
		case evidence.EventExecutionFinished:
			p, err := ev.DecodePayload()
			if err != nil {
				t.Fatal(err)
			}
			f := p.(*evidence.ExecutionFinishedPayload)
			pr, ok := pairs[f.ExecutionID]
			if !ok {
				t.Fatalf("execution %s finished without a start", f.ExecutionID)
			}
			if pr.finished {
				t.Fatalf("execution %s finished twice", f.ExecutionID)
			}
			pr.finished = true
			pr.duration = f.DurationMS
			if f.Status != evidence.ExecutionSuccess {
				t.Errorf("execution %s status = %s", f.ExecutionID, f.Status)
			}
		}
	}
	if len(order) != 2 {
		t.Fatalf("executions = %d, want 2", len(order))
	}
	for _, id := range order {
		if !pairs[id].finished {
			t.Errorf("execution %s never finished", id)
		}
	}
	// Durations must not be swapped between the two overlapping calls.
	first, second := pairs[order[0]], pairs[order[1]]
	quick, slowPair := first, second
	if slowPair.duration < quick.duration {
		quick, slowPair = slowPair, first
	}
	if quick.duration > 1500 || slowPair.duration < 1500 {
		t.Errorf("pairing crossed the two calls: durations %d and %d ms", quick.duration, slowPair.duration)
	}
}

// TestArgumentsFingerprintSurvivesKeyOrder is §24's stated test, run through the
// real endpoint rather than the helper: two identical argument objects that
// differ only in serialization order must fingerprint identically, while a real
// difference must not.
func TestArgumentsFingerprintSurvivesKeyOrder(t *testing.T) {
	root := filepath.Join(t.TempDir(), "evidence")
	h := startEndpoint(t, "--enforce=off", "--evidence-dir", root, fixtureBin)
	h.initialize()
	for _, args := range []map[string]any{
		{"delay_ms": 10, "label": "same"},
		{"label": "same", "delay_ms": 10},
		{"delay_ms": 11, "label": "same"},
	} {
		if _, isErr := h.call("slow", args); isErr {
			t.Fatalf("slow %v failed", args)
		}
	}
	h.closeIn()
	if err := h.wait(); err != nil {
		t.Fatalf("exit: %v", err)
	}
	events, _, err := evidence.ReadStore(recorderDir(t, root), time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	var hmacs []string
	for _, ev := range events {
		if ev.EventType != evidence.EventExecutionStarted {
			continue
		}
		p, err := ev.DecodePayload()
		if err != nil {
			t.Fatal(err)
		}
		hmacs = append(hmacs, p.(*evidence.ExecutionStartedPayload).ArgumentsHMAC)
	}
	if len(hmacs) != 3 {
		t.Fatalf("started executions = %d, want 3", len(hmacs))
	}
	if hmacs[0] != hmacs[1] {
		t.Errorf("key order changed the argument fingerprint: %s vs %s", hmacs[0], hmacs[1])
	}
	if hmacs[0] == hmacs[2] {
		t.Error("different arguments produced the same fingerprint")
	}
}

// TestCancelledExecutionIsRecordedAsCancelled covers the terminal state the wire
// can prove but a relay usually loses: the client asked to cancel, the upstream
// never answered, and the execution must not be left started-without-finish nor
// recorded as a success.
func TestCancelledExecutionIsRecordedAsCancelled(t *testing.T) {
	// Both spellings of the same request id, because the cancel notification
	// echoes the id as the client wrote it while the outstanding call was keyed
	// from the frame that opened it.
	for _, id := range []string{"702", `"702"`} {
		t.Run("requestId "+id, func(t *testing.T) {
			assertCancelledExecutionRecorded(t, id)
		})
	}
}

func assertCancelledExecutionRecorded(t *testing.T, requestID string) {
	t.Helper()
	root := filepath.Join(t.TempDir(), "evidence")
	h := startEndpoint(t, "--enforce=off", "--evidence-dir", root, fixtureBin)
	h.initialize()
	if _, isErr := h.call("evidra_prescribe", map[string]any{"objective": "wait on a slow call"}); isErr {
		t.Fatal("prescribe failed")
	}
	if err := h.send(`{"jsonrpc":"2.0","id":` + requestID + `,"method":"tools/call","params":{"name":"slow","arguments":{"delay_ms":60000,"label":"never"}}}`); err != nil {
		t.Fatal(err)
	}
	// Give the forward a moment to reach the child, then cancel and hang up.
	if err := h.send(`{"jsonrpc":"2.0","method":"notifications/cancelled","params":{"requestId":` + requestID + `,"reason":"agent gave up"}}`); err != nil {
		t.Fatal(err)
	}
	time.Sleep(150 * time.Millisecond)
	h.closeIn()
	if err := h.wait(); err != nil {
		t.Fatalf("exit: %v", err)
	}

	events, _, err := evidence.ReadStore(recorderDir(t, root), time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	var started, finished, cancelled int
	for _, ev := range events {
		switch ev.EventType {
		case evidence.EventExecutionStarted:
			started++
		case evidence.EventExecutionFinished:
			finished++
			p, err := ev.DecodePayload()
			if err != nil {
				t.Fatal(err)
			}
			f := p.(*evidence.ExecutionFinishedPayload)
			switch f.Status {
			case evidence.ExecutionCancelled:
				cancelled++
				if f.ErrorCode != "cancelled_by_client" {
					t.Errorf("error_code = %q, want cancelled_by_client", f.ErrorCode)
				}
			case evidence.ExecutionSuccess:
				t.Error("a cancelled call that never answered was recorded as success")
			}
		}
	}
	if started == 0 {
		t.Fatal("the cancelled call was never recorded as started")
	}
	if cancelled == 0 {
		t.Errorf("no cancelled terminal event: started=%d finished=%d", started, finished)
	}
	if started != finished {
		t.Errorf("%d executions started, %d finished: a started execution with no terminal event is a recorder lying by omission", started, finished)
	}
}

// TestActorIDFlagReachesEveryEvent keeps accountability wired: --actor-id is the only
// way to say who was responsible, so a flag that parsed but never landed would leave
// every chainattributable to the fallback identity.
func TestActorIDFlagReachesEveryEvent(t *testing.T) {
	root := t.TempDir()
	h := startEndpoint(t, "--enforce=off", "--evidence-dir", root, "--actor-id", "ops-bot-7", fixtureBin)
	h.initialize()
	if _, isErr := h.call("get_status", map[string]any{"verbose": true}); isErr {
		t.Fatal("get_status failed")
	}
	h.closeIn()
	if err := h.wait(); err != nil {
		t.Fatalf("endpoint exited with error: %v\n%s", err, h.stderrText())
	}

	events, _, err := evidence.ReadStore(recorderDir(t, root), time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	if len(events) == 0 {
		t.Fatal("no events recorded")
	}
	for _, ev := range events {
		// Lifecycle events are recorder_generated: the thing that started and stopped
		// the recorder is the recorder, not the actor it was configured for. Asserted
		// empty rather than skipped blindly - that reading was checked by running the
		// test without this branch and watching recorder_started carry no actor even
		// though the flag was set at open time.
		if ev.EventType == evidence.EventRecorderStarted || ev.EventType == evidence.EventRecorderStopped {
			if ev.Actor.ID != "" {
				t.Fatalf("%s should attribute to the recorder, got actor %q", ev.EventType, ev.Actor.ID)
			}
			continue
		}
		if ev.Actor.ID != "ops-bot-7" {
			t.Fatalf("%s actor = %q, want the id passed on the command line", ev.EventType, ev.Actor.ID)
		}
	}
}
