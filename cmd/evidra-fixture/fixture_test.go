package main

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"strings"
	"sync"
	"testing"
	"time"
)

// peer drives the fixture over in-memory pipes and speaks just enough MCP to
// exercise the wire-level behaviors the plan requires.
type peer struct {
	t        *testing.T
	writeMu  sync.Mutex
	w        io.WriteCloser
	lines    chan string
	notes    []rpcMessage
	autoRoot []any
}

func startFixture(t *testing.T, opts fixtureOptions) *peer {
	t.Helper()

	clientToServerR, clientToServerW := io.Pipe()
	serverToClientR, serverToClientW := io.Pipe()

	logger := log.New(testWriter{t}, "fixture ", 0)
	f := newFixture(clientToServerR, serverToClientW, opts, logger)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		defer close(done)
		_ = f.run(ctx)
	}()

	p := &peer{t: t, w: clientToServerW, lines: make(chan string, 1024), autoRoot: []any{
		map[string]any{"uri": "file:///root-a", "name": "a"},
		map[string]any{"uri": "file:///root-b", "name": "b"},
	}}

	go func() {
		br := bufio.NewReader(serverToClientR)
		for {
			line, err := br.ReadString('\n')
			if line != "" {
				select {
				case p.lines <- strings.TrimRight(line, "\r\n"):
				default:
					t.Errorf("output buffer overflow")
				}
			}
			if err != nil {
				close(p.lines)
				return
			}
		}
	}()

	t.Cleanup(func() {
		cancel()
		_ = clientToServerW.Close()
		select {
		case <-done:
		case <-time.After(2 * time.Second):
			t.Errorf("fixture server did not exit")
		}
	})
	return p
}

type testWriter struct{ t *testing.T }

func (w testWriter) Write(b []byte) (int, error) {
	w.t.Logf("%s", strings.TrimRight(string(b), "\n"))
	return len(b), nil
}

func (p *peer) sendRaw(line string) {
	p.writeMu.Lock()
	defer p.writeMu.Unlock()
	if _, err := io.WriteString(p.w, line+"\n"); err != nil {
		p.t.Fatalf("write: %v", err)
	}
}

func (p *peer) send(v any) {
	b, err := json.Marshal(v)
	if err != nil {
		p.t.Fatalf("marshal: %v", err)
	}
	p.sendRaw(string(b))
}

func (p *peer) request(id any, method string, params any) {
	msg := map[string]any{"jsonrpc": "2.0", "id": id, "method": method}
	if params != nil {
		msg["params"] = params
	}
	p.send(msg)
}

func (p *peer) notify(method string, params any) {
	msg := map[string]any{"jsonrpc": "2.0", "method": method}
	if params != nil {
		msg["params"] = params
	}
	p.send(msg)
}

// next reads one server message, auto-answering server->client requests so
// that round-trips stay simple.
func (p *peer) next(timeout time.Duration) (rpcMessage, bool) {
	p.t.Helper()
	deadline := time.After(timeout)
	for {
		select {
		case line, ok := <-p.lines:
			if !ok {
				return rpcMessage{}, false
			}
			var msg rpcMessage
			if err := json.Unmarshal([]byte(line), &msg); err != nil {
				p.t.Fatalf("undecodable server message (%d bytes): %v", len(line), err)
			}
			if msg.Method != "" && len(msg.ID) > 0 {
				p.answerPeerRequest(msg)
				continue
			}
			if msg.Method != "" {
				p.notes = append(p.notes, msg)
				continue
			}
			return msg, true
		case <-deadline:
			return rpcMessage{}, false
		}
	}
}

func (p *peer) answerPeerRequest(req rpcMessage) {
	var result any
	switch req.Method {
	case "roots/list":
		result = map[string]any{"roots": p.autoRoot}
	default:
		p.send(map[string]any{
			"jsonrpc": "2.0", "id": req.ID,
			"error": map[string]any{"code": errMethodNotFound, "message": "test client: unsupported " + req.Method},
		})
		return
	}
	p.send(map[string]any{"jsonrpc": "2.0", "id": req.ID, "result": result})
}

func (p *peer) call(id any, method string, params any) rpcMessage {
	p.t.Helper()
	p.request(id, method, params)
	msg, ok := p.next(5 * time.Second)
	if !ok {
		p.t.Fatalf("no response to %s (id %v) within 5s", method, id)
	}
	return msg
}

func (p *peer) noteCount(method string) int {
	n := 0
	for _, m := range p.notes {
		if m.Method == method {
			n++
		}
	}
	return n
}

func (p *peer) lastNote(method string) (rpcMessage, bool) {
	for i := len(p.notes) - 1; i >= 0; i-- {
		if p.notes[i].Method == method {
			return p.notes[i], true
		}
	}
	return rpcMessage{}, false
}

// initialize performs the handshake; rootsCap toggles the client capability
// that ask_client depends on.
func (p *peer) initialize(rootsCap bool) rpcMessage {
	p.t.Helper()
	caps := map[string]any{}
	if rootsCap {
		caps["roots"] = map[string]any{"listChanged": false}
	}
	res := p.call(1, "initialize", map[string]any{
		"protocolVersion": protocolVersionLatest,
		"capabilities":    caps,
		"clientInfo":      map[string]any{"name": "go-test", "version": "1"},
	})
	p.notify("notifications/initialized", nil)
	return res
}

func toolResultStructured(t *testing.T, msg rpcMessage) map[string]any {
	t.Helper()
	var res struct {
		StructuredContent map[string]any `json:"structuredContent"`
		IsError           bool           `json:"isError"`
		Content           []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"content"`
	}
	if err := json.Unmarshal(msg.Result, &res); err != nil {
		t.Fatalf("decode tool result: %v\nraw: %s", err, msg.Result)
	}
	if len(res.Content) == 0 {
		t.Fatalf("tool result has no content: %s", msg.Result)
	}
	return res.StructuredContent
}

func toolIsError(t *testing.T, msg rpcMessage) (bool, string) {
	t.Helper()
	var res struct {
		IsError bool `json:"isError"`
		Content []struct {
			Text string `json:"text"`
		} `json:"content"`
	}
	if err := json.Unmarshal(msg.Result, &res); err != nil {
		t.Fatalf("decode: %v (%s)", err, msg.Result)
	}
	text := ""
	if len(res.Content) > 0 {
		text = res.Content[0].Text
	}
	return res.IsError, text
}

func TestInitializeEchoesVersionAndAdvertisesTools(t *testing.T) {
	p := startFixture(t, fixtureOptions{})
	res := p.initialize(false)

	var init struct {
		ProtocolVersion string         `json:"protocolVersion"`
		Capabilities    map[string]any `json:"capabilities"`
		ServerInfo      map[string]any `json:"serverInfo"`
		Instructions    string         `json:"instructions"`
	}
	if err := json.Unmarshal(res.Result, &init); err != nil {
		t.Fatalf("decode initialize result: %v", err)
	}
	if init.ProtocolVersion != protocolVersionLatest {
		t.Errorf("protocolVersion = %q, want %q", init.ProtocolVersion, protocolVersionLatest)
	}
	tools, ok := init.Capabilities["tools"].(map[string]any)
	if !ok || tools["listChanged"] != true {
		t.Errorf("capabilities.tools = %v, want listChanged true", init.Capabilities["tools"])
	}
	if init.ServerInfo["name"] != fixtureName {
		t.Errorf("serverInfo.name = %v, want %q", init.ServerInfo["name"], fixtureName)
	}
	if init.Instructions == "" {
		t.Errorf("initialize.instructions must be non-empty for wrapper composition tests")
	}

	// Older requested version is echoed back verbatim.
	p2 := startFixture(t, fixtureOptions{})
	old := p2.call(9, "initialize", map[string]any{"protocolVersion": "2024-11-05", "capabilities": map[string]any{}})
	if !strings.Contains(string(old.Result), "2024-11-05") {
		t.Errorf("expected negotiated 2024-11-05, got %s", old.Result)
	}
}

// TestToolsListPaginationAndAnnotationWireForms is the §26/§30 requirement that
// declared-false, absent, and contradictory annotations are distinguishable on
// the wire, and that pagination preserves cursor semantics.
func TestToolsListPaginationAndAnnotationWireForms(t *testing.T) {
	p := startFixture(t, fixtureOptions{PageSize: 3})
	p.initialize(false)

	var (
		cursor string
		pages  int
		seen   = map[string]map[string]any{}
		names  []string
	)
	for {
		params := map[string]any{}
		if cursor != "" {
			params["cursor"] = cursor
		}
		res := p.call(10+pages, "tools/list", params)
		var list struct {
			Tools      []map[string]any `json:"tools"`
			NextCursor string           `json:"nextCursor"`
		}
		if err := json.Unmarshal(res.Result, &list); err != nil {
			t.Fatalf("decode tools/list: %v", err)
		}
		if len(list.Tools) == 0 {
			t.Fatalf("page %d returned no tools (cursor %q)", pages, cursor)
		}
		for _, tool := range list.Tools {
			name, _ := tool["name"].(string)
			seen[name] = tool
			names = append(names, name)
		}
		pages++
		if list.NextCursor == "" {
			break
		}
		if pages > 10 {
			t.Fatalf("cursor loop: %v", list.NextCursor)
		}
		cursor = list.NextCursor
	}

	if pages != 3 {
		t.Errorf("pages = %d, want 3 (9 tools, page-size 3): %v", pages, names)
	}
	if len(names) != 9 {
		t.Errorf("tool count = %d, want 9: %v", len(names), names)
	}
	if _, ok := seen["hidden_tool"]; ok {
		t.Errorf("hidden_tool must not be listed before toggle_tool")
	}

	annotations := func(name string) (map[string]any, bool) {
		tool, ok := seen[name]
		if !ok {
			t.Fatalf("tool %q not listed", name)
		}
		a, present := tool["annotations"]
		if !present {
			return nil, false
		}
		return a.(map[string]any), true
	}

	if a, present := annotations("get_status"); !present || a["readOnlyHint"] != true {
		t.Errorf(`get_status annotations = %v (present=%v), want {"readOnlyHint":true}`, a, present)
	}

	// restart must carry an explicit `"readOnlyHint": false`, which the go-sdk
	// cannot express. Check raw bytes as well as decoded values.
	a, present := annotations("restart")
	if !present || a["readOnlyHint"] != false {
		t.Errorf(`restart annotations = %v (present=%v), want explicit readOnlyHint false`, a, present)
	}
	if !strings.Contains(`"readOnlyHint":false`, fmt.Sprintf("%v", a["readOnlyHint"])) {
		t.Errorf("restart readOnlyHint decoded as %T %v, want bool false", a["readOnlyHint"], a["readOnlyHint"])
	}
	raw := p.rawLineFor(t, "restart")
	if !strings.Contains(raw, `"readOnlyHint":false`) {
		t.Errorf(`wire form for restart must contain "readOnlyHint":false, got %s`, raw)
	}

	if a, present := annotations("unknown_action"); present {
		t.Errorf(`unknown_action must declare no annotations key at all, got %v`, a)
	}

	a, present = annotations("contradictory_action")
	if !present || a["readOnlyHint"] != true || a["destructiveHint"] != true {
		t.Errorf("contradictory_action annotations = %v (present=%v), want readOnlyHint=true with destructiveHint=true", a, present)
	}
}

// rawLineFor re-requests one page of tools and returns the raw JSON line, so
// tests can assert on exact wire forms rather than decoded values.
func (p *peer) rawLineFor(t *testing.T, toolName string) string {
	t.Helper()
	for page := 0; page < 6; page++ {
		params := map[string]any{}
		if page > 0 {
			params["cursor"] = fmt.Sprintf("page-%d", page*3)
		}
		p.request(200+page, "tools/list", params)
		line, ok := p.rawLine(2 * time.Second)
		if !ok {
			t.Fatalf("no tools/list response line")
		}
		if strings.Contains(line, `"`+toolName+`"`) {
			return line
		}
	}
	t.Fatalf("tool %q never appeared in a raw page", toolName)
	return ""
}

func (p *peer) rawLine(timeout time.Duration) (string, bool) {
	deadline := time.After(timeout)
	for {
		select {
		case line, ok := <-p.lines:
			if !ok {
				return "", false
			}
			var msg rpcMessage
			if err := json.Unmarshal([]byte(line), &msg); err != nil {
				continue
			}
			if msg.Method != "" && len(msg.ID) > 0 {
				p.answerPeerRequest(msg)
				continue
			}
			if msg.Method != "" {
				p.notes = append(p.notes, msg)
				continue
			}
			return line, true
		case <-deadline:
			return "", false
		}
	}
}

func TestFailActionBothFailureShapes(t *testing.T) {
	contentPeer := startFixture(t, fixtureOptions{})
	contentPeer.initialize(false)
	res := contentPeer.call(2, "tools/call", map[string]any{"name": "fail_action", "arguments": map[string]any{}})
	isErr, text := toolIsError(t, res)
	if !isErr || !strings.Contains(text, "fail_action") {
		t.Errorf("content-mode fail_action: isError=%v text=%q", isErr, text)
	}
	if res.Error != nil {
		t.Errorf("content-mode fail_action must not be a JSON-RPC error, got %v", res.Error)
	}

	rpcPeer := startFixture(t, fixtureOptions{FailMode: "rpc"})
	rpcPeer.initialize(false)
	res = rpcPeer.call(2, "tools/call", map[string]any{"name": "fail_action", "arguments": map[string]any{}})
	if res.Error == nil || res.Error.Code != errInternal {
		t.Fatalf("rpc-mode fail_action: want JSON-RPC error %d, got %+v", errInternal, res)
	}
}

func TestUnknownActionOutcomes(t *testing.T) {
	p := startFixture(t, fixtureOptions{})
	p.initialize(false)

	for _, tc := range []struct {
		outcome string
		wantErr bool
		code    int
	}{
		{"success", false, 0},
		{"error", true, 0},
		{"cancelled", true, errRequestCancelled},
	} {
		res := p.call(3, "tools/call", map[string]any{"name": "unknown_action", "arguments": map[string]any{"outcome": tc.outcome}})
		switch {
		case tc.code != 0:
			if res.Error == nil || res.Error.Code != tc.code {
				t.Errorf("outcome %q: want rpc error %d, got %+v", tc.outcome, tc.code, res.Error)
			}
		case tc.wantErr:
			if isErr, _ := toolIsError(t, res); !isErr {
				t.Errorf("outcome %q: want isError true", tc.outcome)
			}
		default:
			if isErr, _ := toolIsError(t, res); isErr {
				t.Errorf("outcome %q: want success", tc.outcome)
			}
		}
	}
}

func TestResultsAreByteStableWithoutStateful(t *testing.T) {
	p := startFixture(t, fixtureOptions{})
	p.initialize(false)
	first := p.call(4, "tools/call", map[string]any{"name": "get_status", "arguments": map[string]any{}})
	second := p.call(5, "tools/call", map[string]any{"name": "get_status", "arguments": map[string]any{}})
	if string(first.Result) != string(second.Result) {
		t.Errorf("get_status not deterministic:\n %s\n %s", first.Result, second.Result)
	}
	if strings.Contains(string(first.Result), "20") {
		t.Logf("note: result contains a 20* substring; check for timestamps manually")
	}
}

func TestBigResultExceedsTenMegabytes(t *testing.T) {
	p := startFixture(t, fixtureOptions{})
	p.initialize(false)
	res := p.call(6, "tools/call", map[string]any{"name": "big", "arguments": map[string]any{"bytes": 11 << 20}})
	if res.Error != nil {
		t.Fatalf("big failed: %v", res.Error)
	}
	if len(res.Result) <= 10<<20 {
		t.Errorf("result bytes = %d, want > 10 MiB", len(res.Result))
	}
	var decoded struct {
		Content []struct {
			Text string `json:"text"`
		} `json:"content"`
	}
	if err := json.Unmarshal(res.Result, &decoded); err != nil {
		t.Fatalf("large result not decodable: %v", err)
	}
	if len(decoded.Content[0].Text) != 11<<20 {
		t.Errorf("text length = %d, want %d", len(decoded.Content[0].Text), 11<<20)
	}
}

// TestOversizeMessageRejectedButStreamAlive is the fixture-side counterpart of
// §29: an over-long line must produce an explicit error, never a dead stream.
func TestOversizeMessageRejectedButStreamAlive(t *testing.T) {
	p := startFixture(t, fixtureOptions{MaxMessage: 256})
	p.initialize(false)

	pad := strings.Repeat("y", 4096)
	p.request(7, "tools/call", map[string]any{"name": "get_status", "arguments": map[string]any{"pad": pad}})

	msg, ok := p.next(3 * time.Second)
	if !ok {
		t.Fatalf("no error response for oversize message")
	}
	if msg.Error == nil || msg.Error.Code != errInvalidRequest {
		t.Fatalf("want JSON-RPC error %d for oversize message, got %+v", errInvalidRequest, msg)
	}
	if ping := p.call(8, "ping", nil); ping.Error != nil {
		t.Errorf("stream died after oversize message: %v", ping.Error)
	}
}

func TestServerToClientRequestWithRoots(t *testing.T) {
	p := startFixture(t, fixtureOptions{})
	p.initialize(true)

	res := p.call(9, "tools/call", map[string]any{"name": "ask_client", "arguments": map[string]any{}})
	if res.Error != nil {
		t.Fatalf("ask_client failed: %v", res.Error)
	}
	got := toolResultStructured(t, res)
	if got["roots"] != float64(len(p.autoRoot)) {
		t.Errorf("roots = %v, want %d", got["roots"], len(p.autoRoot))
	}
}

func TestAskClientWithoutRootsCapability(t *testing.T) {
	p := startFixture(t, fixtureOptions{})
	p.initialize(false)
	res := p.call(10, "tools/call", map[string]any{"name": "ask_client", "arguments": map[string]any{}})
	isErr, text := toolIsError(t, res)
	if !isErr || !strings.Contains(text, "roots") {
		t.Errorf("want tool error mentioning roots, got isError=%v text=%q", isErr, text)
	}
}

// TestRequestIDDirectionCollision is T15: a client request and a
// server->client request carrying the same numeric ID must not cross-talk, and
// the pending client request must survive the answered peer request.
func TestRequestIDDirectionCollision(t *testing.T) {
	p := startFixture(t, fixtureOptions{NumericRequestIDs: true})
	p.initialize(true)

	// Client request with numeric id 1 that the fixture will not answer yet.
	p.request(1, "tools/call", map[string]any{"name": "slow", "arguments": map[string]any{"delay_ms": 5000, "label": "hold"}})

	// ask_client emits a server->client roots/list request; with
	// --numeric-request-ids it uses id 1 as well, and the test client answers
	// it with the same numeric id.
	res := p.call(2, "tools/call", map[string]any{"name": "ask_client", "arguments": map[string]any{}})
	if res.Error != nil {
		t.Fatalf("ask_client: %v", res.Error)
	}
	got := toolResultStructured(t, res)
	if got["roots"] != float64(len(p.autoRoot)) {
		t.Fatalf("roots = %v, want %d", got["roots"], len(p.autoRoot))
	}

	// The client's own id-1 request must still be outstanding: cancelling it
	// must produce no response at all.
	p.notify("notifications/cancelled", map[string]any{"requestId": 1, "reason": "collision test"})
	if _, ok := p.nextWithin(400 * time.Millisecond); ok {
		t.Errorf("fixture answered a cancelled request")
	}
	if ping := p.call(3, "ping", nil); ping.Error != nil {
		t.Errorf("stream unhealthy after collision test: %v", ping.Error)
	}
}

func (p *peer) nextWithin(timeout time.Duration) (rpcMessage, bool) {
	deadline := time.After(timeout)
	for {
		select {
		case line, ok := <-p.lines:
			if !ok {
				return rpcMessage{}, false
			}
			var msg rpcMessage
			if err := json.Unmarshal([]byte(line), &msg); err != nil {
				continue
			}
			if msg.Method != "" && len(msg.ID) > 0 {
				p.answerPeerRequest(msg)
				continue
			}
			if msg.Method != "" {
				p.notes = append(p.notes, msg)
				continue
			}
			return msg, true
		case <-deadline:
			return rpcMessage{}, false
		}
	}
}

func TestCancellationProducesNoResponse(t *testing.T) {
	p := startFixture(t, fixtureOptions{})
	p.initialize(false)

	p.request(42, "tools/call", map[string]any{"name": "slow", "arguments": map[string]any{"delay_ms": 4000}})
	time.Sleep(50 * time.Millisecond)
	p.notify("notifications/cancelled", map[string]any{"requestId": 42, "reason": "test"})

	if msg, ok := p.nextWithin(400 * time.Millisecond); ok {
		t.Errorf("cancelled request produced a response: %+v", msg)
	}
	if ping := p.call(43, "ping", nil); ping.Error != nil {
		t.Errorf("stream unhealthy after cancellation: %v", ping.Error)
	}
}

func TestProgressNotificationWithToken(t *testing.T) {
	p := startFixture(t, fixtureOptions{})
	p.initialize(false)

	res := p.call(44, "tools/call", map[string]any{
		"name":      "slow",
		"arguments": map[string]any{"delay_ms": 20, "label": "prog"},
		"_meta":     map[string]any{"progressToken": "tok-1"},
	})
	if res.Error != nil {
		t.Fatalf("slow failed: %v", res.Error)
	}
	note, ok := p.lastNote("notifications/progress")
	if !ok {
		t.Fatalf("no notifications/progress emitted; notes=%+v", p.notes)
	}
	var prog struct {
		Token    any `json:"progressToken"`
		Progress any `json:"progress"`
		Text     string
		Message  string `json:"message"`
	}
	_ = json.Unmarshal(note.Params, &prog)
	if fmt.Sprint(prog.Token) != "tok-1" {
		t.Errorf("progressToken = %v, want tok-1", prog.Token)
	}
}

func TestToggleToolEmitsListChangedAndRepaginates(t *testing.T) {
	p := startFixture(t, fixtureOptions{PageSize: 4})
	p.initialize(false)

	res := p.call(45, "tools/call", map[string]any{"name": "toggle_tool", "arguments": map[string]any{"visible": true}})
	got := toolResultStructured(t, res)
	if got["hidden_tool_visible"] != true {
		t.Fatalf("toggle_tool result = %v", got)
	}
	if p.noteCount("notifications/tools/list_changed") != 1 {
		t.Errorf("want one list_changed notification, notes=%+v", p.notes)
	}

	found := false
	for page := 0; page < 4; page++ {
		params := map[string]any{}
		if page > 0 {
			params["cursor"] = fmt.Sprintf("page-%d", page*4)
		}
		list := p.call(50+page, "tools/list", params)
		if strings.Contains(string(list.Result), `"hidden_tool"`) {
			found = true
		}
	}
	if !found {
		t.Errorf("hidden_tool not listed after toggle")
	}

	// Hide it again: calling it must be a tool error, not a crash.
	p.call(55, "tools/call", map[string]any{"name": "toggle_tool", "arguments": map[string]any{"visible": false}})
	res = p.call(56, "tools/call", map[string]any{"name": "hidden_tool", "arguments": map[string]any{}})
	if isErr, text := toolIsError(t, res); !isErr || !strings.Contains(text, "not currently listed") {
		t.Errorf("hidden_tool after hide: isError=%v text=%q", isErr, text)
	}
}

func TestUnknownMethodAndMalformedLineKeepStreamAlive(t *testing.T) {
	p := startFixture(t, fixtureOptions{})
	p.initialize(false)

	res := p.call(60, "resources/list", map[string]any{})
	if res.Error == nil || res.Error.Code != errMethodNotFound {
		t.Errorf("resources/list: want %d, got %+v", errMethodNotFound, res.Error)
	}

	p.sendRaw(`{"jsonrpc": "2.0", "id": 61, "method": `)
	msg, ok := p.next(2 * time.Second)
	if !ok {
		t.Fatalf("no parse error response")
	}
	if msg.Error == nil || msg.Error.Code != errParse {
		t.Errorf("want parse error, got %+v", msg)
	}
	if ping := p.call(62, "ping", nil); ping.Error != nil {
		t.Errorf("stream died after malformed line: %v", ping.Error)
	}
}

// TestSerialHandlersPreserveArrivalOrder locks the two fixture modes the plan
// needs: concurrent handling (so parallel-call pairing can be tested at all)
// and --serial (so golden summaries compare byte-stable output).
func TestSerialHandlersPreserveArrivalOrder(t *testing.T) {
	p := startFixture(t, fixtureOptions{SerialHandlers: true})
	p.initialize(false)

	p.request(2, "tools/call", map[string]any{"name": "slow", "arguments": map[string]any{"delay_ms": 250, "label": "first"}})
	p.request(3, "tools/call", map[string]any{"name": "get_status", "arguments": map[string]any{}})

	first, ok := p.next(5 * time.Second)
	if !ok {
		t.Fatalf("no response")
	}
	if idKey(first.ID) != "2" {
		t.Errorf("serial mode answered id %s first, want 2", idKey(first.ID))
	}
	second, ok := p.next(5 * time.Second)
	if !ok {
		t.Fatalf("no second response")
	}
	if idKey(second.ID) != "3" {
		t.Errorf("serial mode answered id %s second, want 3", idKey(second.ID))
	}
}

// TestConcurrentHandlersAllowInterleaving documents that the default mode is
// intentionally not ordered: a slow first request must not starve later ones.
func TestConcurrentHandlersAllowInterleaving(t *testing.T) {
	p := startFixture(t, fixtureOptions{})
	p.initialize(false)

	p.request(2, "tools/call", map[string]any{"name": "slow", "arguments": map[string]any{"delay_ms": 1500}})
	p.request(3, "tools/call", map[string]any{"name": "get_status", "arguments": map[string]any{}})

	msg, ok := p.next(3 * time.Second)
	if !ok {
		t.Fatalf("second request was starved by the first")
	}
	if idKey(msg.ID) != "3" {
		t.Errorf("expected the fast request to be answered first, got id %s", idKey(msg.ID))
	}
}

func TestUnknownToolIsInvalidParams(t *testing.T) {
	p := startFixture(t, fixtureOptions{})
	p.initialize(false)
	res := p.call(70, "tools/call", map[string]any{"name": "nope", "arguments": map[string]any{}})
	if res.Error == nil || res.Error.Code != errInvalidParams {
		t.Errorf("want %d for unknown tool, got %+v", errInvalidParams, res.Error)
	}
}

func TestBadCursorRejected(t *testing.T) {
	p := startFixture(t, fixtureOptions{PageSize: 2})
	p.initialize(false)
	res := p.call(80, "tools/list", map[string]any{"cursor": "nonsense"})
	if res.Error == nil || res.Error.Code != errInvalidParams {
		t.Errorf("want invalid params for bad cursor, got %+v", res.Error)
	}
}

func TestParseBytesAcceptsCommonForms(t *testing.T) {
	for _, tc := range []struct {
		in   string
		want int
	}{{"64MiB", 64 << 20}, {"10MB", 10 << 20}, {"4096", 4096}, {"1 GB", 1 << 30}, {"2kib", 2 << 10}} {
		got, err := parseBytes(tc.in)
		if err != nil {
			t.Errorf("parseBytes(%q): %v", tc.in, err)
			continue
		}
		if got != tc.want {
			t.Errorf("parseBytes(%q) = %d, want %d", tc.in, got, tc.want)
		}
	}
	if _, err := parseBytes("0"); err == nil {
		t.Error("parseBytes(\"0\") should fail")
	}
}
