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
	rep, err := evidence.VerifyStore(dir)
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
