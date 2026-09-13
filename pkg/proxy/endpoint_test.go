package proxy

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

var (
	fixtureBin string
	mcpBin     string
	buildDir   string
)

// TestMain builds the two binaries the endpoint tests drive as real child
// processes. Spawning the actual fixture and the actual CLI is the point: the
// step-2 exit criterion is "direct vs wrapped tools list sane", which needs the
// same framing and flags a user gets.
func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "evidra-endpoint-tests")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	buildDir = dir
	fixtureBin = filepath.Join(dir, "evidra-fixture")
	mcpBin = filepath.Join(dir, "evidra-mcp")
	for _, b := range []struct{ out, pkg string }{
		{fixtureBin, "samebits.com/evidra/cmd/evidra-fixture"},
		{mcpBin, "samebits.com/evidra/cmd/evidra-mcp"},
	} {
		cmd := exec.Command("go", "build", "-o", b.out, b.pkg)
		if out, err := cmd.CombinedOutput(); err != nil {
			fmt.Fprintf(os.Stderr, "build %s: %v\n%s\n", b.pkg, err, out)
			_ = os.RemoveAll(dir)
			os.Exit(1)
		}
	}
	code := m.Run()
	_ = os.RemoveAll(dir)
	os.Exit(code)
}

// wire is a JSON-RPC envelope for test assertions, classified by shape rather
// than by field presence alone (§28).
type wire struct {
	JSONRPC string          `json:"jsonrpc"`
	Method  string          `json:"method,omitempty"`
	Params  json.RawMessage `json:"params,omitempty"`
	ID      json.RawMessage `json:"id,omitempty"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   json.RawMessage `json:"error,omitempty"`
}

// harness speaks stdio JSON-RPC to a child process.
type harness struct {
	t      *testing.T
	cmd    *exec.Cmd
	stdin  io.WriteCloser
	stdout *bufio.Reader
	stderr *bytes.Buffer
	mu     sync.Mutex
	next   int
	queued []*wire
}

func start(t *testing.T, bin string, args ...string) *harness {
	t.Helper()
	cmd := exec.Command(bin, args...)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		t.Fatalf("stdin pipe: %v", err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatalf("stdout pipe: %v", err)
	}
	errBuf := &bytes.Buffer{}
	cmd.Stderr = errBuf
	if err := cmd.Start(); err != nil {
		t.Fatalf("start %s: %v", bin, err)
	}
	return &harness{
		t:      t,
		cmd:    cmd,
		stdin:  stdin,
		stdout: bufio.NewReaderSize(stdout, 1<<20),
		stderr: errBuf,
		next:   100,
	}
}

func startFixture(t *testing.T, args ...string) *harness {
	t.Helper()
	return start(t, fixtureBin, args...)
}

// startEndpoint launches the merged endpoint wrapping the given upstream args.
func startEndpoint(t *testing.T, upstreamArgs ...string) *harness {
	t.Helper()
	return start(t, mcpBin, append([]string{"--proxy"}, upstreamArgs...)...)
}

func (h *harness) id() json.RawMessage {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.next++
	return json.RawMessage(fmt.Sprintf("%d", h.next))
}

func (h *harness) send(line string) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	_, err := h.stdin.Write([]byte(line + "\n"))
	return err
}

func (h *harness) sendMsg(id json.RawMessage, method string, params any) error {
	raw, err := json.Marshal(params)
	if err != nil {
		return err
	}
	frame := map[string]any{"jsonrpc": "2.0", "method": method}
	if len(id) > 0 {
		frame["id"] = json.RawMessage(id)
	}
	if len(raw) > 0 {
		frame["params"] = json.RawMessage(raw)
	}
	line, err := json.Marshal(frame)
	if err != nil {
		return err
	}
	return h.send(string(line))
}

// read returns the next frame, honoring a deadline so a hang fails the test
// instead of the suite.
func (h *harness) read() (*wire, error) {
	type res struct {
		w   *wire
		err error
	}
	ch := make(chan res, 1)
	go func() {
		line, err := h.stdout.ReadBytes('\n')
		if err != nil {
			ch <- res{err: err}
			return
		}
		var w wire
		if err := json.Unmarshal(bytes.TrimRight(line, "\r\n"), &w); err != nil {
			ch <- res{err: fmt.Errorf("bad frame %q: %w", line, err)}
			return
		}
		ch <- res{w: &w}
	}()
	select {
	case r := <-ch:
		return r.w, r.err
	case <-time.After(30 * time.Second):
		return nil, errors.New("timeout waiting for a frame from the endpoint")
	}
}

// response waits for a response carrying id, queueing anything else it passes.
func (h *harness) response(id string) (*wire, error) {
	for {
		w, err := h.read()
		if err != nil {
			return nil, err
		}
		if w.Method == "" && string(w.ID) == id {
			return w, nil
		}
		h.mu.Lock()
		h.queued = append(h.queued, w)
		h.mu.Unlock()
	}
}

func (h *harness) drainNotification(method string) *wire {
	h.mu.Lock()
	defer h.mu.Unlock()
	for i, w := range h.queued {
		if w.Method == method {
			out := w
			h.queued = append(h.queued[:i], h.queued[i+1:]...)
			return out
		}
	}
	return nil
}

func (h *harness) initializeWith(version string) map[string]any {
	h.t.Helper()
	id := h.id()
	if err := h.sendMsg(id, "initialize", map[string]any{
		"protocolVersion": version,
		"capabilities":    map[string]any{"roots": map[string]any{"listChanged": false}},
		"clientInfo":      map[string]any{"name": "endpoint-test", "version": "0"},
	}); err != nil {
		h.t.Fatalf("initialize send: %v", err)
	}
	resp, err := h.response(string(id))
	if err != nil {
		h.t.Fatalf("initialize: %v", err)
	}
	if len(resp.Error) > 0 {
		h.t.Fatalf("initialize error: %s", resp.Error)
	}
	var result map[string]any
	if err := json.Unmarshal(resp.Result, &result); err != nil {
		h.t.Fatalf("initialize result: %v", err)
	}
	_ = h.sendMsg(nil, "notifications/initialized", map[string]any{})
	return result
}

func (h *harness) initialize() map[string]any {
	h.t.Helper()
	return h.initializeWith("2025-06-18")
}

// call performs one tools/call round trip and returns the parsed result.
func (h *harness) call(tool string, args map[string]any) (map[string]any, bool) {
	h.t.Helper()
	id := h.id()
	if args == nil {
		args = map[string]any{}
	}
	if err := h.sendMsg(id, "tools/call", map[string]any{"name": tool, "arguments": args}); err != nil {
		h.t.Fatalf("tools/call %s: %v", tool, err)
	}
	resp, err := h.response(string(id))
	if err != nil {
		h.t.Fatalf("tools/call %s response: %v", tool, err)
	}
	if len(resp.Error) > 0 {
		return map[string]any{"rpc_error": json.RawMessage(resp.Error)}, true
	}
	var result struct {
		Content []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"content"`
		IsError bool `json:"isError"`
	}
	if err := json.Unmarshal(resp.Result, &result); err != nil {
		h.t.Fatalf("tools/call %s result: %v", tool, err)
	}
	text := ""
	if len(result.Content) > 0 {
		text = result.Content[0].Text
	}
	out := map[string]any{"text": text}
	var parsed map[string]any
	if err := json.Unmarshal([]byte(text), &parsed); err == nil {
		for k, v := range parsed {
			out[k] = v
		}
	}
	return out, result.IsError
}

// listTools walks every page and returns names in order plus the raw first
// page tools array and the cursors seen.
func (h *harness) listTools() (names []string, cursors []string, rawByName map[string]json.RawMessage) {
	h.t.Helper()
	rawByName = map[string]json.RawMessage{}
	cursor := ""
	for page := 0; page < 50; page++ {
		params := map[string]any{}
		if cursor != "" {
			params["cursor"] = cursor
		}
		id := h.id()
		if err := h.sendMsg(id, "tools/list", params); err != nil {
			h.t.Fatalf("tools/list: %v", err)
		}
		resp, err := h.response(string(id))
		if err != nil {
			h.t.Fatalf("tools/list response: %v", err)
		}
		if len(resp.Error) > 0 {
			h.t.Fatalf("tools/list error: %s", resp.Error)
		}
		var result struct {
			Tools      []json.RawMessage `json:"tools"`
			NextCursor string            `json:"nextCursor"`
		}
		if err := json.Unmarshal(resp.Result, &result); err != nil {
			h.t.Fatalf("tools/list result: %v", err)
		}
		for _, raw := range result.Tools {
			var only struct {
				Name string `json:"name"`
			}
			if err := json.Unmarshal(raw, &only); err != nil {
				h.t.Fatalf("tool entry: %v", err)
			}
			names = append(names, only.Name)
			rawByName[only.Name] = raw
		}
		cursors = append(cursors, result.NextCursor)
		if result.NextCursor == "" {
			return names, cursors, rawByName
		}
		cursor = result.NextCursor
	}
	h.t.Fatalf("tools/list did not terminate")
	return
}

func (h *harness) closeIn() { _ = h.stdin.Close() }

func (h *harness) wait() error { return h.cmd.Wait() }

func (h *harness) kill() {
	if h.cmd.Process != nil {
		_ = h.cmd.Process.Kill()
	}
	_ = h.cmd.Wait()
}

func TestMainCleanupGuard(t *testing.T) {
	if fixtureBin == "" || mcpBin == "" {
		t.Fatal("binaries not built")
	}
	if _, err := os.Stat(filepath.Join(buildDir, "evidra-mcp")); err != nil {
		t.Fatalf("endpoint binary missing: %v", err)
	}
}

// TestWrappedToolsListEqualsDirectPlusLocal is the §59 step-2 exit criterion:
// the wrapped list must be the direct list plus Evidra's two tools, each
// upstream name exactly once, with upstream cursors and annotation wire forms
// untouched (§6, §27).
func TestWrappedToolsListEqualsDirectPlusLocal(t *testing.T) {
	fixtureArgs := []string{"--page-size", "3"}

	direct := startFixture(t, fixtureArgs...)
	defer direct.kill()
	direct.initialize()
	directNames, directCursors, directRaw := direct.listTools()
	if len(directNames) < 9 {
		t.Fatalf("fixture listing looks truncated: %v", directNames)
	}

	wrapped := startEndpoint(t, append([]string{fixtureBin}, fixtureArgs...)...)
	defer wrapped.kill()
	wrapped.initialize()
	gotNames, gotCursors, gotRaw := wrapped.listTools()

	want := append([]string{"evidra_prescribe", "evidra_report"}, directNames...)
	if strings.Join(gotNames, ",") != strings.Join(want, ",") {
		t.Fatalf("wrapped list differs\n got: %v\nwant: %v", gotNames, want)
	}
	seen := map[string]int{}
	for _, n := range gotNames {
		seen[n]++
	}
	for n, c := range seen {
		if c != 1 {
			t.Errorf("tool %q listed %d times across pages", n, c)
		}
	}
	if len(gotCursors) != len(directCursors) {
		t.Fatalf("page count differs: wrapped %d direct %d", len(gotCursors), len(directCursors))
	}
	for i := range directCursors {
		if gotCursors[i] != directCursors[i] {
			t.Errorf("page %d cursor changed: wrapped %q direct %q", i, gotCursors[i], directCursors[i])
		}
	}
	for _, name := range directNames {
		if !bytes.Equal(gotRaw[name], directRaw[name]) {
			t.Errorf("upstream tool %q wire form changed:\n wrapped: %s\n  direct: %s", name, gotRaw[name], directRaw[name])
		}
	}
}

// TestReservedToolNameCollisionRefusesToStart covers §6: an upstream that
// advertises a reserved local name must make the endpoint fail clearly instead
// of renaming anything.
func TestReservedToolNameCollisionRefusesToStart(t *testing.T) {
	h := startEndpoint(t, fixtureBin, "--steal-names", "evidra_prescribe")
	// Refusal happens during startup, before any initialize response, so the
	// request is sent raw and EOF on stdout is the expected outcome.
	if err := h.sendMsg(h.id(), "initialize", map[string]any{
		"protocolVersion": "2025-06-18",
		"capabilities":    map[string]any{},
		"clientInfo":      map[string]any{"name": "endpoint-test", "version": "0"},
	}); err != nil {
		h.t.Fatalf("initialize send: %v", err)
	}
	if w, err := h.read(); err == nil {
		t.Fatalf("endpoint answered initialize despite the collision: %+v", w)
	}
	h.closeIn()
	done := make(chan error, 1)
	go func() { done <- h.wait() }()
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("endpoint started despite a reserved tool name collision")
		}
	case <-time.After(20 * time.Second):
		h.kill()
		t.Fatal("endpoint did not exit after a reserved-name collision")
	}
	msg := h.stderr.String()
	if !strings.Contains(msg, "reserved tool") {
		t.Fatalf("failure was not explained; stderr:\n%s", msg)
	}
}

// TestLocalPrescribeReportStateMachine exercises §11's second-prescribe fork and
// §32's deterministic report states through the merged endpoint.
func TestLocalPrescribeReportStateMachine(t *testing.T) {
	h := startEndpoint(t, fixtureBin)
	defer h.kill()
	h.initialize()

	opened, isError := h.call("evidra_prescribe", map[string]any{
		"objective":        "restore checkout availability",
		"expected_outcome": "checkout requests succeed",
	})
	if isError {
		t.Fatalf("first prescribe failed: %v", opened)
	}
	opID, _ := opened["operation_id"].(string)
	if !strings.HasPrefix(opID, "EV-") {
		t.Fatalf("operation_id %q is not an EV-prefixed id", opID)
	}
	if opened["state"] != "open" {
		t.Fatalf("state %v, want open", opened["state"])
	}

	second, isError := h.call("evidra_prescribe", map[string]any{"objective": "another thing"})
	if !isError {
		t.Fatal("second prescribe succeeded while an operation was open")
	}
	text, _ := second["text"].(string)
	if !strings.Contains(text, `"operation_already_open"`) {
		t.Fatalf("second prescribe error shape wrong: %s", text)
	}
	if !strings.Contains(text, `"continue_current"`) || !strings.Contains(text, `"abandon_and_replace"`) {
		t.Fatalf("choices missing from the fork: %s", text)
	}

	same, _ := h.call("evidra_prescribe", map[string]any{"objective": "another thing", "continue_current": true})
	if same["operation_id"] != opID {
		t.Fatalf("continue_current opened a new operation: %v", same["operation_id"])
	}

	replaced, isError := h.call("evidra_prescribe", map[string]any{
		"objective": "new objective", "abandon_and_replace": true,
	})
	if isError {
		t.Fatalf("abandon_and_replace failed: %v", replaced)
	}
	newID, _ := replaced["operation_id"].(string)
	if newID == "" || newID == opID {
		t.Fatalf("replacement reused the old id: %q", newID)
	}

	if mismatch, isErr := h.call("evidra_report", map[string]any{
		"operation_id": opID, "status": "completed", "outcome": "achieved",
	}); !isErr || !strings.Contains(fmt.Sprint(mismatch["text"]), "operation_id_mismatch") {
		t.Fatalf("report of a replaced id was not rejected: %v %v", mismatch, isErr)
	}

	reported, isError := h.call("evidra_report", map[string]any{
		"operation_id": newID, "status": "failed", "outcome": "not_achieved", "summary": "upstream kept erroring",
	})
	if isError || reported["state"] != "reported" {
		t.Fatalf("terminal report rejected: %v", reported)
	}

	dup, isErr := h.call("evidra_report", map[string]any{
		"operation_id": newID, "status": "failed", "outcome": "not_achieved",
	})
	if isErr || dup["state"] != "already_reported" {
		t.Fatalf("duplicate report should be idempotent, got %v (isError=%v)", dup, isErr)
	}

	if none, isErr := h.call("evidra_report", map[string]any{
		"operation_id": "EV-nope", "status": "completed", "outcome": "achieved",
	}); !isErr || !strings.Contains(fmt.Sprint(none["text"]), "no_open_operation") {
		t.Fatalf("report with nothing open should say no_open_operation: %v", none)
	}

	if bad, isErr := h.call("evidra_prescribe", map[string]any{"objective": "   "}); !isErr ||
		!strings.Contains(fmt.Sprint(bad["text"]), "objective_required") {
		t.Fatalf("blank objective accepted: %v", bad)
	}
}

// TestUpstreamServerRequestRelayedToClient proves server-to-client requests
// survive the wrapper: the fixture asks for roots, and the answer must come
// from the real client through Evidra.
func TestUpstreamServerRequestRelayedToClient(t *testing.T) {
	h := startEndpoint(t, "--enforce=off", fixtureBin)
	defer h.kill()
	h.initialize()

	id := h.id()
	if err := h.sendMsg(id, "tools/call", map[string]any{"name": "ask_client", "arguments": map[string]any{}}); err != nil {
		t.Fatalf("ask_client: %v", err)
	}
	var rootsReq *wire
	for i := 0; i < 5; i++ {
		w, err := h.read()
		if err != nil {
			t.Fatalf("waiting for the relayed roots/list request: %v", err)
		}
		if w.Method == "roots/list" {
			rootsReq = w
			break
		}
		if w.Method == "" && string(w.ID) == string(id) {
			t.Fatalf("endpoint answered ask_client itself; upstream server request lost: %s", w.Result)
		}
	}
	if rootsReq == nil {
		t.Fatal("no roots/list request reached the client")
	}
	reply := map[string]any{"jsonrpc": "2.0", "id": json.RawMessage(rootsReq.ID),
		"result": map[string]any{"roots": []any{map[string]any{"uri": "file:///srv/evidra", "name": "work"}}}}
	line, _ := json.Marshal(reply)
	if err := h.send(string(line)); err != nil {
		t.Fatalf("answer roots/list: %v", err)
	}
	resp, err := h.response(string(id))
	if err != nil {
		t.Fatalf("ask_client response: %v", err)
	}
	if len(resp.Error) > 0 {
		t.Fatalf("ask_client errored: %s", resp.Error)
	}
	// The fixture reports how many roots the client returned; one proves the
	// answer came through Evidra rather than being synthesized.
	if !strings.Contains(string(resp.Result), `\"roots\":1`) {
		t.Fatalf("fixture did not see exactly the roots we supplied: %s", resp.Result)
	}
}

// TestDirectionIDCollision is T15 through the wrapper: the upstream uses numeric
// server-to-client ids that collide with the client's own request ids, and
// neither side may receive the other's answer.
func TestDirectionIDCollision(t *testing.T) {
	h := startEndpoint(t, "--enforce=off", fixtureBin, "--numeric-request-ids")
	defer h.kill()
	h.initialize()

	const colliding = "1"
	line := `{"jsonrpc":"2.0","id":` + colliding + `,"method":"tools/call","params":{"name":"ask_client","arguments":{}}}`
	if err := h.send(line); err != nil {
		t.Fatalf("send tools/call id 1: %v", err)
	}
	w, err := h.read()
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if w.Method != "roots/list" || string(w.ID) != colliding {
		t.Fatalf("expected a roots/list request with id 1, got method=%q id=%s", w.Method, w.ID)
	}
	reply := `{"jsonrpc":"2.0","id":` + colliding + `,"result":{"roots":[]}}`
	if err := h.send(reply); err != nil {
		t.Fatalf("answer roots/list: %v", err)
	}
	resp, err := h.response(colliding)
	if err != nil {
		t.Fatalf("response for id 1: %v", err)
	}
	if len(resp.Error) > 0 {
		t.Fatalf("id 1 response was an error: %s", resp.Error)
	}
	if !bytes.Contains(resp.Result, []byte("content")) {
		t.Fatalf("id 1 response is not the tools/call result: %s", resp.Result)
	}
}

// TestClientOversizeFrameRejectedStreamSurvives is T13 on the client side.
func TestClientOversizeFrameRejectedStreamSurvives(t *testing.T) {
	h := startEndpoint(t, "--max-message", "1MiB", fixtureBin)
	defer h.kill()

	big := `{"jsonrpc":"2.0","id":7,"method":"tools/call","params":{"name":"x","arguments":{"pad":"` +
		strings.Repeat("a", 2<<20) + `"}}}`
	if err := h.send(big); err != nil {
		t.Fatalf("send oversize: %v", err)
	}
	w, err := h.read()
	if err != nil {
		t.Fatalf("read oversize error: %v", err)
	}
	if len(w.Error) == 0 || !strings.Contains(string(w.Error), "exceeds") {
		t.Fatalf("oversize frame did not produce an explicit error: %+v", w)
	}

	result := h.initialize()
	if info, ok := result["serverInfo"].(map[string]any); !ok || info["name"] != "evidra-mcp" {
		t.Fatalf("session did not survive the oversize frame: %v", result)
	}
}

// TestToolsListChangedForwardedAndRemerged covers the list_changed half of §27.
func TestToolsListChangedForwardedAndRemerged(t *testing.T) {
	h := startEndpoint(t, "--enforce=off", fixtureBin, "--stateful")
	defer h.kill()
	h.initialize()

	before, _, _ := h.listTools()
	if hasName(before, "hidden_tool") {
		t.Fatal("hidden_tool was listed before the toggle")
	}
	if _, isError := h.call("toggle_tool", map[string]any{"visible": true}); isError {
		t.Fatal("toggle_tool errored")
	}
	deadline := time.Now().Add(10 * time.Second)
	for h.drainNotification("notifications/tools/list_changed") == nil {
		if time.Now().After(deadline) {
			// The notification may still be in flight behind other frames.
			w, err := h.read()
			if err != nil {
				t.Fatalf("waiting for list_changed: %v", err)
			}
			if w.Method != "" {
				h.mu.Lock()
				h.queued = append(h.queued, w)
				h.mu.Unlock()
			}
		}
		if time.Now().After(deadline.Add(5 * time.Second)) {
			t.Fatal("no notifications/tools/list_changed reached the client")
		}
	}
	after, _, _ := h.listTools()
	if !hasName(after, "hidden_tool") {
		t.Fatalf("hidden_tool missing after list_changed: %v", after)
	}
	if count := countName(after, "hidden_tool"); count != 1 {
		t.Fatalf("hidden_tool listed %d times after re-merge", count)
	}
	if !hasName(after, "evidra_prescribe") || !hasName(after, "evidra_report") {
		t.Fatalf("local tools disappeared after re-merge: %v", after)
	}
}

// TestInitializeProfileAndInstructions pins §27: advertise only tested
// capability families, never overwrite upstream instructions, and negotiate the
// protocol version explicitly.
func TestInitializeProfileAndInstructions(t *testing.T) {
	h := startEndpoint(t, fixtureBin, "--advertise", "tools,logging,prompts,resources")
	defer h.kill()
	result := h.initializeWith("2024-11-05")

	if result["protocolVersion"] != "2024-11-05" {
		t.Fatalf("protocol version not negotiated: %v", result["protocolVersion"])
	}
	caps, _ := result["capabilities"].(map[string]any)
	if _, ok := caps["tools"]; !ok {
		t.Fatalf("tools capability missing: %v", caps)
	}
	if _, ok := caps["logging"]; !ok {
		t.Fatalf("logging should be advertised because the upstream has it and notifications are relayed: %v", caps)
	}
	for _, fam := range []string{"prompts", "resources", "completions"} {
		if _, ok := caps[fam]; ok {
			t.Errorf("unsupported capability family %q advertised without passthrough", fam)
		}
	}
	instr, _ := result["instructions"].(string)
	if !strings.Contains(instr, "evidra_prescribe") {
		t.Errorf("Evidra protocol instructions missing")
	}
	if !strings.Contains(instr, "Fixture MCP server") {
		t.Errorf("upstream instructions overwritten: %q", instr)
	}
	if !strings.Contains(instr, "upstream server instructions") {
		t.Errorf("source boundary marker missing: %q", instr)
	}
	if info, ok := result["serverInfo"].(map[string]any); !ok || !strings.Contains(fmt.Sprint(info["title"]), filepath.Base(fixtureBin)) {
		t.Errorf("serverInfo does not name the wrapped upstream: %v", result["serverInfo"])
	}

	// Unknown versions fall back to the newest supported version rather than
	// echoing something this endpoint cannot speak.
	h2 := startEndpoint(t, fixtureBin)
	defer h2.kill()
	if got := h2.initializeWith("9999-99-99")["protocolVersion"]; got != epLatestProtocolVersion {
		t.Fatalf("unknown protocol version echoed: %v", got)
	}

	// Opt-in passthrough advertises the relayed-but-untested families.
	h3 := startEndpoint(t, "--advertise-passthrough", fixtureBin, "--advertise", "tools,prompts,resources")
	defer h3.kill()
	caps3, _ := h3.initialize()["capabilities"].(map[string]any)
	if _, ok := caps3["prompts"]; !ok {
		t.Fatalf("passthrough did not advertise prompts: %v", caps3)
	}
}

// TestUnknownToolPassesThrough checks that the endpoint does not swallow
// upstream errors for tools it knows nothing about.
func TestUnknownToolPassesThrough(t *testing.T) {
	h := startEndpoint(t, "--enforce=off", fixtureBin)
	defer h.kill()
	h.initialize()
	resp, err := func() (*wire, error) {
		id := h.id()
		if err := h.sendMsg(id, "tools/call", map[string]any{"name": "no_such_tool", "arguments": map[string]any{}}); err != nil {
			return nil, err
		}
		return h.response(string(id))
	}()
	if err != nil {
		t.Fatalf("unknown tool call: %v", err)
	}
	if len(resp.Error) == 0 && !strings.Contains(string(resp.Result), "not found") {
		t.Fatalf("unknown tool produced neither an error nor a not-found result: %s", resp.Result)
	}
}

// TestReservedIDNamespaceRejected documents the one deliberate deviation from
// full transparency: Evidra's own upstream request ids live in a reserved
// namespace, and a client using it is refused instead of silently remapped.
func TestReservedIDNamespaceRejected(t *testing.T) {
	h := startEndpoint(t, fixtureBin)
	defer h.kill()
	h.initialize()

	line := `{"jsonrpc":"2.0","id":"` + epIDNamespace + `oops","method":"tools/list","params":{}}`
	if err := h.send(line); err != nil {
		t.Fatalf("send: %v", err)
	}
	w, err := h.read()
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if len(w.Error) == 0 || !strings.Contains(string(w.Error), "reserved by Evidra") {
		t.Fatalf("reserved request id was not refused: %+v", w)
	}
}

// TestEnforceAllBlocksUnprescribedCall is the single rule of §7: no open
// prescription, no upstream call. The fixture's own counter proves the blocked
// call never reached the upstream, and that a read-only declaration exempts
// nothing.
func TestEnforceAllBlocksUnprescribedCall(t *testing.T) {
	h := startEndpoint(t, fixtureBin, "--stateful")
	defer h.kill()
	h.initialize()

	blocked, isError := h.call("restart", map[string]any{"service": "payments"})
	if !isError {
		t.Fatalf("unprescribed restart was not blocked: %v", blocked)
	}
	if text, _ := blocked["text"].(string); !strings.Contains(text, "no_open_operation") || !strings.Contains(text, "evidra_prescribe") {
		t.Fatalf("block did not instruct the agent: %s", text)
	}

	// A tool the server declares read-only is blocked by the same rule:
	// annotations are reporting data, not an enforcement input (§7).
	if ro, isErr := h.call("get_status", nil); !isErr || !strings.Contains(fmt.Sprint(ro["text"]), "no_open_operation") {
		t.Fatalf("declared read-only tool bypassed enforcement: %v", ro)
	}

	opened, isError := h.call("evidra_prescribe", map[string]any{"objective": "restart payments"})
	if isError {
		t.Fatalf("prescribe failed: %v", opened)
	}
	opID, _ := opened["operation_id"].(string)

	if check, _ := h.call("get_status", nil); !strings.Contains(fmt.Sprint(check["text"]), `"restarts":0`) {
		t.Fatalf("blocked attempts reached the upstream: %v", check)
	}

	after, isError := h.call("restart", map[string]any{"service": "payments"})
	if isError {
		t.Fatalf("prescribed restart was blocked: %v", after)
	}
	if !strings.Contains(fmt.Sprint(after["text"]), `"restarted":true`) {
		t.Fatalf("prescribed restart did not reach the upstream: %v", after)
	}
	if check, _ := h.call("get_status", nil); !strings.Contains(fmt.Sprint(check["text"]), `"restarts":1`) {
		t.Fatalf("upstream state wrong after one prescribed restart: %v", check)
	}

	if rep, isErr := h.call("evidra_report", map[string]any{
		"operation_id": opID, "status": "completed", "outcome": "achieved",
	}); isErr {
		t.Fatalf("terminal report rejected: %v", rep)
	}
	// Enforcement resumes the moment the operation closes.
	if again, isErr := h.call("get_status", nil); !isErr || !strings.Contains(fmt.Sprint(again["text"]), "no_open_operation") {
		t.Fatalf("upstream call succeeded after the operation closed: %v", again)
	}
}

// TestEnforceOffForwardsUnprescribedCall is the other half of the A/B: nothing
// is blocked, so coverage becomes a measurement of voluntary behavior.
func TestEnforceOffForwardsUnprescribedCall(t *testing.T) {
	h := startEndpoint(t, "--enforce=off", fixtureBin, "--stateful")
	defer h.kill()
	h.initialize()

	out, isError := h.call("restart", map[string]any{"service": "payments"})
	if isError {
		t.Fatalf("observe-only mode blocked an unprescribed call: %v", out)
	}
	if !strings.Contains(fmt.Sprint(out["text"]), `"restarted":true`) {
		t.Fatalf("observe-only call did not reach the upstream: %v", out)
	}
	// The protocol tools still work in this mode; only blocking is removed.
	if _, isErr := h.call("evidra_prescribe", map[string]any{"objective": "check"}); isErr {
		t.Fatal("prescribe failed in observe-only mode")
	}
}

// TestUnknownEnforceModeFailsAtStartup keeps the mode surface at two values.
func TestUnknownEnforceModeFailsAtStartup(t *testing.T) {
	h := startEndpoint(t, "--enforce=mutations", fixtureBin)
	h.closeIn()
	done := make(chan error, 1)
	go func() { done <- h.wait() }()
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("endpoint accepted an unsupported enforcement mode")
		}
	case <-time.After(15 * time.Second):
		h.kill()
		t.Fatal("endpoint did not exit on an unsupported enforcement mode")
	}
	msg := h.stderr.String()
	if !strings.Contains(msg, "unknown enforcement mode") || !strings.Contains(msg, "--enforce=all") {
		t.Fatalf("startup failure was not explained: %s", msg)
	}
}

func hasName(names []string, want string) bool {
	for _, n := range names {
		if n == want {
			return true
		}
	}
	return false
}

func countName(names []string, want string) int {
	c := 0
	for _, n := range names {
		if n == want {
			c++
		}
	}
	return c
}
