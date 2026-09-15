package proxy

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/vitas/evidra/pkg/evidence"
)

// TestProgressNotificationsSurviveTheWrapper is Gate B's "progress/cancellation
// used by tool flow" line. Progress notifications carry no id, so a relay that
// tracks only request/response pairs drops them silently, and the visible symptom
// is an agent that stops believing the tool is alive.
func TestProgressNotificationsSurviveTheWrapper(t *testing.T) {
	h := startEndpoint(t, "--enforce=off", fixtureBin)
	h.initialize()
	id := h.id()
	if err := h.sendMsg(id, "tools/call", map[string]any{
		"name":      "slow",
		"arguments": map[string]any{"delay_ms": 400, "label": "progress"},
		"_meta":     map[string]any{"progressToken": "pt-1"},
	}); err != nil {
		t.Fatal(err)
	}
	// Read to the response first: notifications arrive interleaved and the
	// harness parks them while it looks for the id, so draining before the
	// exchange finished would test the reader rather than the relay.
	resp, err := h.response(string(id))
	if err != nil {
		t.Fatalf("slow response: %v", err)
	}
	if resp.Error != nil {
		t.Fatalf("slow call failed: %s", resp.Error)
	}
	prog := h.drainNotification("notifications/progress")
	if prog == nil {
		t.Fatal("no notifications/progress reached the client through the wrapper")
	}
	var params struct {
		ProgressToken any    `json:"progressToken"`
		Message       string `json:"message"`
	}
	if err := json.Unmarshal(prog.Params, &params); err != nil {
		t.Fatalf("progress params: %v", err)
	}
	if fmt.Sprint(params.ProgressToken) != "pt-1" {
		t.Errorf("progressToken = %v, want pt-1: a relayed notification must not be renumbered", params.ProgressToken)
	}
	h.closeIn()
	if err := h.wait(); err != nil {
		t.Fatalf("exit: %v", err)
	}
}

// TestLargeResultPassesThroughAndIsFingerprintedWithinBounds covers two Gate B
// lines at once: a message larger than the plan's 10 MiB conformance case must
// cross the wrapper intact, while the evidence record must still obey the bounded
// fingerprint policy instead of hashing 20 MiB into the chain.
func TestLargeResultPassesThroughAndIsFingerprintedWithinBounds(t *testing.T) {
	root := filepath.Join(t.TempDir(), "evidence")
	h := startEndpoint(t, "--enforce=off", "--evidence-dir", root, fixtureBin)
	h.initialize()
	const want = 20 << 20
	out, isErr := h.call("big", map[string]any{"bytes": want})
	if isErr {
		t.Fatalf("the 20 MiB result was refused: %v\nendpoint stderr:\n%s", out["rpc_error"], h.stderrText())
	}
	h.closeIn()
	if err := h.wait(); err != nil {
		t.Fatalf("exit: %v", err)
	}

	events, _, err := evidence.ReadStore(recorderDir(t, root), time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	var started, finished int
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
			if f.ResultFingerprintStatus != evidence.FingerprintOmittedOversize {
				t.Errorf("fingerprint status = %s, want %s", f.ResultFingerprintStatus, evidence.FingerprintOmittedOversize)
			}
			if f.ResultHMAC != nil {
				t.Error("an oversize result stored a fingerprint anyway")
			}
			if f.Status != evidence.ExecutionSuccess {
				t.Errorf("status = %s: the wrapper must not turn a large success into a failure", f.Status)
			}
		}
	}
	if started != 1 || finished != 1 {
		t.Fatalf("started=%d finished=%d, want 1 and 1", started, finished)
	}
	raw, err := json.Marshal(events)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), "AAAA") {
		t.Error("the large payload body reached the evidence store instead of a bounded fingerprint")
	}
}
