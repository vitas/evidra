// Command evidra-fixture is the deterministic MCP fixture server required by
// the vNext plan (§30, docs/system-design/vnext-mcp-recorder.md).
//
// It is deliberately implemented directly against JSON-RPC over stdio instead
// of the official go-sdk, for two reasons the plan's own tests depend on:
//
//  1. Wire-level control over tool annotations. go-sdk v1.5.0 declares
//     ToolAnnotations.ReadOnlyHint as `bool` with `omitempty`, so "server
//     declared readOnlyHint=false" and "server declared nothing" produce the
//     same bytes. §26 and §37 must distinguish declared-false, absent, and
//     contradictory annotations, and §30 lists exactly those cases as fixture
//     tools. This server can emit all three.
//  2. Hostile-message support. §28/§29 and T13/T15 need a peer that sends
//     server->client requests, oversized messages, contradictory annotation
//     sets, and request IDs that collide with the client's own ID space.
//     A conformance fixture should not share transport code with the
//     implementation it checks.
//
// Determinism: results are byte-stable for a given argument set unless
// --stateful is passed. Timestamps never appear in tool results. Requests are
// handled concurrently by default, so response lines may interleave; pass
// --serial when a test needs byte-stable ordering.
//
// Usage:
//
//	evidra-fixture [--page-size 3] [--advertise tools] [--stateful]
//	               [--fail-mode content|rpc] [--numeric-request-ids]
//	               [--max-message 64MiB] [--client-wait 10s] [--sleep-scale 1]
//	               [--version]
package main

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"
)

const (
	protocolVersionLatest = "2025-06-18"
	fixtureName           = "evidra-fixture"
	fixtureVersion        = "vnext-1"

	errParse            = -32700
	errInvalidRequest   = -32600
	errMethodNotFound   = -32601
	errInvalidParams    = -32602
	errInternal         = -32603
	errRequestCancelled = -32800
)

var supportedProtocolVersions = []string{"2025-06-18", "2025-03-26", "2024-11-05"}

// ---------------------------------------------------------------------------
// JSON-RPC shapes
// ---------------------------------------------------------------------------

type rpcMessage struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method,omitempty"`
	Params  json.RawMessage `json:"params,omitempty"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *rpcError       `json:"error,omitempty"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    any    `json:"data,omitempty"`
}

func (e *rpcError) Error() string { return fmt.Sprintf("%d: %s", e.Code, e.Message) }

// envelope is the minimal classification needed to route a message without ever
// guessing from the ID alone (§28): the presence of `method` decides
// request/notification vs response.
type envelope struct {
	hasID     bool
	idRaw     json.RawMessage
	hasMethod bool
	method    string
}

func classify(raw []byte) (envelope, error) {
	var obj map[string]json.RawMessage
	if err := json.Unmarshal(raw, &obj); err != nil {
		return envelope{}, err
	}
	var e envelope
	if v, ok := obj["id"]; ok && string(v) != "null" {
		e.hasID = true
		e.idRaw = v
	}
	if v, ok := obj["method"]; ok && string(v) != "null" {
		e.hasMethod = true
		if err := json.Unmarshal(v, &e.method); err != nil {
			return envelope{}, err
		}
	}
	return e, nil
}

func idKey(raw json.RawMessage) string { return strings.TrimSpace(string(raw)) }

// ---------------------------------------------------------------------------
// Tools
// ---------------------------------------------------------------------------

type fixtureTool struct {
	Name        string         `json:"name"`
	Description string         `json:"description,omitempty"`
	InputSchema map[string]any `json:"inputSchema"`
	// Annotations is a map so that "no annotations key at all" is
	// representable, which a struct field with omitempty cannot express.
	Annotations map[string]any `json:"annotations,omitempty"`
}

func objectSchema(props map[string]any) map[string]any {
	if props == nil {
		return map[string]any{"type": "object", "properties": map[string]any{}, "additionalProperties": false}
	}
	return map[string]any{"type": "object", "properties": props, "additionalProperties": true}
}

// toolSet builds the advertised tool list. hiddenVisible controls the tool that
// only appears after toggle_tool runs, so that
// notifications/tools/list_changed and re-listing have something real to show.
func toolSet(hiddenVisible bool) []fixtureTool {
	tools := []fixtureTool{
		{
			Name:        "get_status",
			Description: "Return fixture status. Declared read-only.",
			InputSchema: objectSchema(nil),
			Annotations: map[string]any{"readOnlyHint": true},
		},
		{
			Name:        "restart",
			Description: "Restart the fixture service. Declared readOnlyHint=false explicitly.",
			InputSchema: objectSchema(map[string]any{
				"service": map[string]any{"type": "string"},
			}),
			Annotations: map[string]any{
				"readOnlyHint":    false,
				"destructiveHint": false,
				"idempotentHint":  false,
				"openWorldHint":   true,
			},
		},
		{
			Name:        "fail_action",
			Description: "Always fails. Declared readOnlyHint=false.",
			InputSchema: objectSchema(nil),
			Annotations: map[string]any{"readOnlyHint": false},
		},
		{
			Name:        "unknown_action",
			Description: "Succeeds, fails, or reports cancellation based on the outcome argument. Declares no annotations at all.",
			InputSchema: objectSchema(map[string]any{
				"outcome": map[string]any{"type": "string", "enum": []string{"success", "error", "cancelled"}},
			}),
		},
		{
			Name:        "contradictory_action",
			Description: "Advertises an annotation set that contradicts itself (readOnlyHint=true with destructiveHint=true).",
			InputSchema: objectSchema(nil),
			Annotations: map[string]any{"readOnlyHint": true, "destructiveHint": true},
		},
		{
			Name:        "slow",
			Description: "Sleeps for delay_ms, emitting notifications/progress when the request carries a progressToken. Honors notifications/cancelled.",
			InputSchema: objectSchema(map[string]any{
				"delay_ms": map[string]any{"type": "number"},
				"label":    map[string]any{"type": "string"},
			}),
			Annotations: map[string]any{"readOnlyHint": false},
		},
		{
			Name:        "big",
			Description: "Returns a result whose serialized size exceeds the requested byte count (default 20 MiB).",
			InputSchema: objectSchema(map[string]any{
				"bytes": map[string]any{"type": "number"},
			}),
			Annotations: map[string]any{"readOnlyHint": true},
		},
		{
			Name:        "ask_client",
			Description: "Sends a server->client roots/list request and reports what came back.",
			InputSchema: objectSchema(nil),
			Annotations: map[string]any{"readOnlyHint": true},
		},
		{
			Name:        "toggle_tool",
			Description: "Shows or hides hidden_tool and emits notifications/tools/list_changed.",
			InputSchema: objectSchema(map[string]any{
				"visible": map[string]any{"type": "boolean"},
			}),
			Annotations: map[string]any{"readOnlyHint": false},
		},
	}
	if hiddenVisible {
		tools = append(tools, fixtureTool{
			Name:        "hidden_tool",
			Description: "Only listed after toggle_tool makes it visible.",
			InputSchema: objectSchema(nil),
			Annotations: map[string]any{"readOnlyHint": true},
		})
	}
	return tools
}

// ---------------------------------------------------------------------------
// Options
// ---------------------------------------------------------------------------

type fixtureOptions struct {
	// SerialHandlers answers requests in arrival order instead of concurrently.
	// Concurrency is what the proxy pairing tests need (§16/T6); serial mode is
	// what golden-summary tests need, because a concurrent server may interleave
	// responses (legal JSON-RPC, but not byte-stable).
	SerialHandlers bool

	PageSize  int
	Advertise string
	// StealNames renames listed tools to names Evidra reserves locally, to test
	// the reserved-name collision rule (§6).
	StealNames        string
	Stateful          bool
	FailMode          string
	NumericRequestIDs bool
	MaxMessage        int
	ClientWait        time.Duration
	SleepScale        float64
}

func (o *fixtureOptions) applyDefaults() {
	if o.MaxMessage <= 0 {
		o.MaxMessage = 64 << 20
	}
	if o.PageSize == 0 {
		o.PageSize = 3
	}
	if o.ClientWait <= 0 {
		o.ClientWait = 10 * time.Second
	}
	if o.SleepScale == 0 {
		o.SleepScale = 1
	}
	if o.Advertise == "" {
		o.Advertise = "tools"
	}
	if o.FailMode == "" {
		o.FailMode = "content"
	}
}

// parseBytes accepts "64MiB", "10MB", "4096".
func parseBytes(s string) (int, error) {
	up := strings.ToUpper(strings.TrimSpace(s))
	mult := 1
	for _, sufs := range [][2]string{{"MIB", "MB"}, {"GIB", "GB"}, {"KIB", "KB"}} {
		for _, suf := range sufs {
			if strings.HasSuffix(up, suf) {
				size := map[string]int{"KIB": 1 << 10, "KB": 1 << 10, "MIB": 1 << 20, "MB": 1 << 20, "GIB": 1 << 30, "GB": 1 << 30}[suf]
				up = strings.TrimSpace(strings.TrimSuffix(up, suf))
				mult = size
				goto parsed
			}
		}
	}
parsed:
	var n float64
	if _, err := fmt.Sscanf(up, "%g", &n); err != nil {
		return 0, fmt.Errorf("bad byte size %q: %w", s, err)
	}
	if n <= 0 {
		return 0, fmt.Errorf("bad byte size %q: must be positive", s)
	}
	return int(n * float64(mult)), nil
}

// ---------------------------------------------------------------------------
// Server
// ---------------------------------------------------------------------------

type fixture struct {
	opts   fixtureOptions
	in     *bufio.Reader
	logger *log.Logger
	writeC chan any

	mu            sync.Mutex
	clientCaps    map[string]any
	nextStrReqID  int
	nextNumReqID  int
	hiddenVisible bool
	restarts      int
	calls         int

	pending  map[string]chan *rpcMessage
	inflight map[string]context.CancelCauseFunc
}

func newFixture(in io.Reader, out io.Writer, opts fixtureOptions, logger *log.Logger) *fixture {
	opts.applyDefaults()
	f := &fixture{
		opts:     opts,
		in:       bufio.NewReaderSize(in, 64<<10),
		logger:   logger,
		writeC:   make(chan any, 256),
		pending:  map[string]chan *rpcMessage{},
		inflight: map[string]context.CancelCauseFunc{},
	}
	go f.writer(out)
	return f
}

func (f *fixture) writer(out io.Writer) {
	for v := range f.writeC {
		b, err := json.Marshal(v)
		if err != nil {
			f.logger.Printf("marshal error: %v", err)
			continue
		}
		if _, err := out.Write(append(b, '\n')); err != nil {
			f.logger.Printf("write error: %v", err)
			break
		}
	}
	// Closing the output lets a reader on the other side see EOF instead of
	// blocking forever once the server stops producing messages.
	if c, ok := out.(io.Closer); ok {
		_ = c.Close()
	}
}

var errOversize = errors.New("oversize")

// readLine reads one newline-delimited JSON message without ever truncating it
// silently (§29). A line longer than the configured maximum is drained to its
// terminator and reported, so the stream stays usable for the next message.
func (f *fixture) readLine() ([]byte, error) {
	var parts []byte
	for {
		frag, err := f.in.ReadSlice('\n')
		complete := err == nil // ReadSlice consumed the delimiter into frag.
		if len(frag) > 0 {
			if len(parts)+len(frag) > f.opts.MaxMessage {
				if !complete {
					_, _ = f.drainToNewline()
				}
				return nil, errOversize
			}
			parts = append(parts, frag...)
		}
		if complete {
			return trimEOL(parts), nil
		}
		switch {
		case errors.Is(err, bufio.ErrBufferFull):
			continue
		case errors.Is(err, io.EOF):
			if len(parts) > 0 {
				return trimEOL(parts), nil
			}
			return nil, io.EOF
		default:
			return nil, err
		}
	}
}

// drainToNewline consumes and discards the unread remainder of an over-long
// line, so a rejected message never leaves a partial line inside the stream.
func (f *fixture) drainToNewline() (int, error) {
	drained := 0
	for {
		frag, err := f.in.ReadSlice('\n')
		drained += len(frag)
		if err == nil {
			return drained, nil
		}
		if errors.Is(err, bufio.ErrBufferFull) {
			continue
		}
		return drained, err
	}
}

func trimEOL(b []byte) []byte { return []byte(strings.TrimRight(string(b), "\r\n")) }

func (f *fixture) capabilities() map[string]any {
	caps := map[string]any{}
	for _, c := range strings.Split(f.opts.Advertise, ",") {
		switch strings.TrimSpace(c) {
		case "tools":
			caps["tools"] = map[string]any{"listChanged": true}
		case "logging":
			caps["logging"] = map[string]any{}
		case "prompts":
			// Advertised on purpose for §27 tests; calling into it still fails.
			caps["prompts"] = map[string]any{"listChanged": false}
		case "resources":
			caps["resources"] = map[string]any{"subscribe": false, "listChanged": false}
		}
	}
	return caps
}

func (f *fixture) run(ctx context.Context) error {
	// Requests are handled concurrently: the fixture has to be able to sit on a
	// long-running call while it also serves parallel calls (T6), answers
	// server->client requests, and honours cancellation (§16/§28).
	var handlers sync.WaitGroup
	defer func() {
		handlers.Wait()
		close(f.writeC)
	}()

	for {
		raw, err := f.readLine()
		if err != nil {
			if errors.Is(err, errOversize) {
				f.logger.Printf("oversize message rejected (limit %d bytes)", f.opts.MaxMessage)
				f.replyError(nil, errInvalidRequest,
					fmt.Sprintf("message exceeds configured maximum of %d bytes", f.opts.MaxMessage))
				continue
			}
			if errors.Is(err, io.EOF) || errors.Is(err, io.ErrClosedPipe) || errors.Is(err, context.Canceled) {
				return nil
			}
			return err
		}
		if len(strings.TrimSpace(string(raw))) == 0 {
			continue
		}

		env, cerr := classify(raw)
		// Responses to our own server->client requests are answered on the read
		// path so they can never be starved by a busy handler.
		if cerr == nil && env.hasID && !env.hasMethod && f.deliverResponse(raw) {
			continue
		}

		handlers.Add(1)
		if f.opts.SerialHandlers {
			f.serve(&handlers, ctx, env, cerr, raw)
			continue
		}
		go f.serve(&handlers, ctx, env, cerr, raw)
	}
}

func (f *fixture) serve(handlers *sync.WaitGroup, ctx context.Context, env envelope, cerr error, raw []byte) {
	defer handlers.Done()
	if cerr != nil {
		f.replyError(nil, errParse, "parse error: "+cerr.Error())
		return
	}
	f.dispatch(ctx, env, raw)
}

func (f *fixture) dispatch(ctx context.Context, env envelope, raw []byte) {
	switch {
	case env.hasMethod && env.hasID:
		var req rpcMessage
		if err := json.Unmarshal(raw, &req); err != nil {
			f.replyError(env.idRaw, errParse, err.Error())
			return
		}
		f.handleRequest(ctx, &req)
	case env.hasMethod && !env.hasID:
		var n rpcMessage
		if err := json.Unmarshal(raw, &n); err != nil {
			return
		}
		f.handleNotification(&n)
	case !env.hasMethod && env.hasID:
		if !f.deliverResponse(raw) {
			f.logger.Printf("response with unmatched id %s discarded", idKey(env.idRaw))
		}
	default:
		f.logger.Printf("undiagnosable message discarded (%d bytes)", len(raw))
	}
}

func (f *fixture) deliverResponse(raw []byte) bool {
	var msg rpcMessage
	if err := json.Unmarshal(raw, &msg); err != nil || len(msg.ID) == 0 {
		return false
	}
	key := idKey(msg.ID)
	f.mu.Lock()
	ch, ok := f.pending[key]
	if ok {
		delete(f.pending, key)
	}
	f.mu.Unlock()
	if !ok {
		return false
	}
	select {
	case ch <- &msg:
	default:
	}
	return true
}

// ---------------------------------------------------------------------------
// Requests
// ---------------------------------------------------------------------------

func (f *fixture) handleRequest(ctx context.Context, req *rpcMessage) {
	key := idKey(req.ID)
	reqCtx, cancel := context.WithCancelCause(ctx)
	f.mu.Lock()
	f.inflight[key] = cancel
	f.mu.Unlock()
	defer func() {
		cancel(nil)
		f.mu.Lock()
		delete(f.inflight, key)
		f.mu.Unlock()
	}()

	switch req.Method {
	case "initialize":
		f.handleInitialize(req)
	case "ping":
		f.replyResult(req.ID, map[string]any{})
	case "tools/list":
		f.handleToolsList(req)
	case "tools/call":
		f.handleToolsCall(reqCtx, req)
	default:
		f.replyError(req.ID, errMethodNotFound, "method not found: "+req.Method)
	}
}

func (f *fixture) handleInitialize(req *rpcMessage) {
	var p struct {
		ProtocolVersion string         `json:"protocolVersion"`
		Capabilities    map[string]any `json:"capabilities"`
	}
	_ = json.Unmarshal(req.Params, &p)

	version := protocolVersionLatest
	for _, v := range supportedProtocolVersions {
		if p.ProtocolVersion == v {
			version = v
			break
		}
	}

	f.mu.Lock()
	f.clientCaps = p.Capabilities
	f.mu.Unlock()

	f.replyResult(req.ID, map[string]any{
		"protocolVersion": version,
		"capabilities":    f.capabilities(),
		"serverInfo":      map[string]any{"name": fixtureName, "version": fixtureVersion},
		"instructions":    "Fixture MCP server for Evidra vNext conformance tests.",
	})
}

func (f *fixture) handleToolsList(req *rpcMessage) {
	var p struct {
		Cursor string `json:"cursor"`
	}
	_ = json.Unmarshal(req.Params, &p)

	f.mu.Lock()
	tools := toolSet(f.hiddenVisible)
	if f.opts.StealNames != "" {
		for i, name := range strings.Split(f.opts.StealNames, ",") {
			name = strings.TrimSpace(name)
			if name == "" || i >= len(tools) {
				continue
			}
			// Only the listing changes; dispatch still keys off the original
			// name, which is fine because the point of the flag is the wire
			// form of tools/list.
			tools[i].Name = name
		}
	}
	f.mu.Unlock()

	start := 0
	if p.Cursor != "" {
		var n int
		if _, err := fmt.Sscanf(p.Cursor, "page-%d", &n); err != nil || n < 0 || n > len(tools) {
			f.replyError(req.ID, errInvalidParams, "unknown cursor: "+p.Cursor)
			return
		}
		start = n
	}
	size := f.opts.PageSize
	if size <= 0 {
		size = len(tools)
	}
	end := start + size
	nextCursor := ""
	if end < len(tools) {
		nextCursor = fmt.Sprintf("page-%d", end)
	} else {
		end = len(tools)
	}
	if start > end {
		start = end
	}

	res := map[string]any{"tools": tools[start:end]}
	if nextCursor != "" {
		res["nextCursor"] = nextCursor
	}
	f.replyResult(req.ID, res)
}

type toolsCallParams struct {
	Name string         `json:"name"`
	Args map[string]any `json:"arguments"`
	Meta map[string]any `json:"_meta"`
}

func (f *fixture) handleToolsCall(ctx context.Context, req *rpcMessage) {
	var p toolsCallParams
	if err := json.Unmarshal(req.Params, &p); err != nil {
		f.replyError(req.ID, errInvalidParams, err.Error())
		return
	}
	args := p.Args
	if args == nil {
		args = map[string]any{}
	}

	f.mu.Lock()
	f.calls++
	hidden := f.hiddenVisible
	restarts := f.restarts
	f.mu.Unlock()

	switch p.Name {
	case "get_status":
		state := map[string]any{"service": fixtureName, "status": "ok"}
		if f.opts.Stateful {
			state["restarts"] = restarts
		}
		f.replyToolResult(req.ID, state, false)

	case "restart":
		f.mu.Lock()
		f.restarts++
		f.mu.Unlock()
		service, _ := args["service"].(string)
		if service == "" {
			service = fixtureName
		}
		out := map[string]any{"service": service, "restarted": true}
		if f.opts.Stateful {
			out["restart_count"] = f.restarts
		}
		f.replyToolResult(req.ID, out, false)

	case "fail_action":
		msg := "fixture failure: fail_action always fails"
		if f.opts.FailMode == "rpc" {
			f.replyError(req.ID, errInternal, msg)
			return
		}
		f.replyToolError(req.ID, msg)

	case "unknown_action":
		outcome, _ := args["outcome"].(string)
		switch outcome {
		case "", "success":
			f.replyToolResult(req.ID, map[string]any{"outcome": "success"}, false)
		case "cancelled":
			f.replyError(req.ID, errRequestCancelled, "fixture reports cancellation")
		default:
			f.replyToolError(req.ID, "fixture failure: outcome="+outcome)
		}

	case "contradictory_action":
		f.replyToolResult(req.ID, map[string]any{"ok": true, "note": "annotations contradict"}, false)

	case "slow":
		f.handleSlow(ctx, req, args)

	case "big":
		n := 20 << 20
		if v, ok := args["bytes"].(float64); ok && v > 0 {
			n = int(v)
		}
		f.replyResult(req.ID, map[string]any{
			"content": []any{map[string]any{"type": "text", "text": strings.Repeat("x", n)}},
			"isError": false,
		})

	case "ask_client":
		f.handleAskClient(ctx, req)

	case "toggle_tool":
		visible := true
		if v, ok := args["visible"].(bool); ok {
			visible = v
		}
		f.mu.Lock()
		f.hiddenVisible = visible
		f.mu.Unlock()
		f.send("notifications/tools/list_changed", nil)
		f.replyToolResult(req.ID, map[string]any{"hidden_tool_visible": visible}, false)

	case "hidden_tool":
		if !hidden {
			f.replyToolError(req.ID, "hidden_tool is not currently listed")
			return
		}
		f.replyToolResult(req.ID, map[string]any{"hidden": true}, false)

	default:
		f.replyError(req.ID, errInvalidParams, "unknown tool: "+p.Name)
	}
}

func (f *fixture) handleSlow(ctx context.Context, req *rpcMessage, args map[string]any) {
	delayMS := 1000.0
	if v, ok := args["delay_ms"].(float64); ok {
		delayMS = v
	}
	delay := time.Duration(float64(time.Millisecond) * delayMS * f.opts.SleepScale)
	label, _ := args["label"].(string)

	var token any
	if meta, err := f.requestMeta(req); err == nil && meta != nil {
		token = meta["progressToken"]
	}

	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		// The client cancelled: per spec we must not respond for this request.
		return
	case <-timer.C:
	}

	if token != nil {
		f.send("notifications/progress", map[string]any{
			"progressToken": token,
			"progress":      1,
			"total":         1,
			"message":       "fixture slow complete",
		})
	}
	out := map[string]any{"slept_ms": int64(delay / time.Millisecond)}
	if label != "" {
		out["label"] = label
	}
	f.replyToolResult(req.ID, out, false)
}

// requestMeta re-reads the raw params of a request to reach _meta.
func (f *fixture) requestMeta(req *rpcMessage) (map[string]any, error) {
	var p struct {
		Meta map[string]any `json:"_meta"`
	}
	if err := json.Unmarshal(req.Params, &p); err != nil {
		return nil, err
	}
	return p.Meta, nil
}

func (f *fixture) handleAskClient(ctx context.Context, req *rpcMessage) {
	f.mu.Lock()
	_, hasRoots := f.clientCaps["roots"]
	f.mu.Unlock()
	if !hasRoots {
		f.replyToolError(req.ID, "client did not declare roots capability")
		return
	}

	result, err := f.requestPeer(ctx, "roots/list", map[string]any{})
	if err != nil {
		f.replyToolError(req.ID, "server->client request failed: "+err.Error())
		return
	}
	var roots struct {
		Roots []any `json:"roots"`
	}
	_ = json.Unmarshal(result, &roots)
	f.replyToolResult(req.ID, map[string]any{"roots": len(roots.Roots)}, false)
}

// ---------------------------------------------------------------------------
// Notifications
// ---------------------------------------------------------------------------

func (f *fixture) handleNotification(n *rpcMessage) {
	switch n.Method {
	case "notifications/initialized":
		return
	case "notifications/cancelled":
		var p struct {
			RequestID json.RawMessage `json:"requestId"`
			Reason    string          `json:"reason"`
		}
		if err := json.Unmarshal(n.Params, &p); err != nil {
			return
		}
		key := idKey(p.RequestID)
		f.mu.Lock()
		cancel, ok := f.inflight[key]
		f.mu.Unlock()
		if ok {
			f.logger.Printf("cancelling request %s (%s)", key, p.Reason)
			cancel(errors.New("cancelled by client"))
		}
	default:
		f.logger.Printf("ignoring notification %s", n.Method)
	}
}

// ---------------------------------------------------------------------------
// Outgoing messages
// ---------------------------------------------------------------------------

func (f *fixture) nextRequestID() json.RawMessage {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.opts.NumericRequestIDs {
		f.nextNumReqID++
		return json.RawMessage(fmt.Sprintf("%d", f.nextNumReqID))
	}
	f.nextStrReqID++
	return json.RawMessage(fmt.Sprintf("%q", "fixture-req-"+fmt.Sprint(f.nextStrReqID)))
}

// requestPeer sends a server->client request and waits for its response. IDs in
// this direction live in their own namespace (§27/§28).
func (f *fixture) requestPeer(ctx context.Context, method string, params any) (json.RawMessage, error) {
	id := f.nextRequestID()
	key := idKey(id)
	ch := make(chan *rpcMessage, 1)
	f.mu.Lock()
	f.pending[key] = ch
	f.mu.Unlock()

	msg := rpcMessage{JSONRPC: "2.0", ID: id, Method: method}
	b, err := json.Marshal(params)
	if err != nil {
		return nil, err
	}
	msg.Params = b
	f.writeC <- msg

	timer := time.NewTimer(f.opts.ClientWait)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		f.forgetPending(key)
		return nil, context.Cause(ctx)
	case <-timer.C:
		f.forgetPending(key)
		return nil, fmt.Errorf("timed out waiting for client response to %s", method)
	case resp := <-ch:
		if resp.Error != nil {
			return nil, resp.Error
		}
		return resp.Result, nil
	}
}

func (f *fixture) forgetPending(key string) {
	f.mu.Lock()
	delete(f.pending, key)
	f.mu.Unlock()
}

func (f *fixture) send(method string, params any) {
	msg := rpcMessage{JSONRPC: "2.0", Method: method}
	if params != nil {
		b, err := json.Marshal(params)
		if err != nil {
			f.logger.Printf("marshal notification: %v", err)
			return
		}
		msg.Params = b
	}
	f.writeC <- msg
}

func (f *fixture) replyResult(id json.RawMessage, result any) {
	b, err := json.Marshal(result)
	if err != nil {
		f.replyError(id, errInternal, "marshal result: "+err.Error())
		return
	}
	f.writeC <- rpcMessage{JSONRPC: "2.0", ID: id, Result: b}
}

func (f *fixture) replyError(id json.RawMessage, code int, msg string) {
	e := &rpcError{Code: code, Message: msg}
	if len(id) == 0 {
		// Without a request id we cannot address a response; emit a bare error.
		f.writeC <- rpcMessage{JSONRPC: "2.0", Error: e}
		return
	}
	f.writeC <- rpcMessage{JSONRPC: "2.0", ID: id, Error: e}
}

func (f *fixture) replyToolResult(id json.RawMessage, structured any, isErr bool) {
	b, _ := json.Marshal(structured)
	f.replyResult(id, map[string]any{
		"content":           []any{map[string]any{"type": "text", "text": string(b)}},
		"structuredContent": structured,
		"isError":           isErr,
	})
}

func (f *fixture) replyToolError(id json.RawMessage, msg string) {
	f.replyResult(id, map[string]any{
		"content": []any{map[string]any{"type": "text", "text": msg}},
		"isError": true,
	})
}

// ---------------------------------------------------------------------------
// Entry point
// ---------------------------------------------------------------------------

type byteFlag struct{ n int }

func (b *byteFlag) String() string { return fmt.Sprint(b.n) }
func (b *byteFlag) Set(s string) error {
	n, err := parseBytes(s)
	if err != nil {
		return err
	}
	b.n = n
	return nil
}

func main() {
	var (
		maxMsg     byteFlag
		failMode   = flag.String("fail-mode", "content", "fail_action failure style: content|rpc")
		advertise  = flag.String("advertise", "tools", "comma-separated server capabilities to advertise (tools,logging,prompts,resources)")
		pageSize   = flag.Int("page-size", 3, "tools/list page size (<=0 lists everything on one page)")
		stateful   = flag.Bool("stateful", false, "include counters in results (breaks byte-stability)")
		numericIDs = flag.Bool("numeric-request-ids", false, "use numeric IDs for server->client requests, colliding with client IDs on purpose (T15)")
		clientWait = flag.Duration("client-wait", 10*time.Second, "how long to wait for a client response")
		sleepScale = flag.Float64("sleep-scale", 1, "multiplier applied to slow delay_ms")
		stealNames = flag.String("steal-names", "", "rename listed tools to these names in tools/list (tests the §6 reserved-name rule)")
		serial     = flag.Bool("serial", false, "answer requests strictly in arrival order (byte-stable output for golden fixtures)")
		showVer    = flag.Bool("version", false, "print version and exit")
	)
	maxMsg.n = 64 << 20
	flag.Var(&maxMsg, "max-message", "maximum accepted message size (e.g. 64MiB)")
	flag.Parse()

	if *showVer {
		fmt.Printf("%s %s\n", fixtureName, fixtureVersion)
		return
	}
	if *failMode != "content" && *failMode != "rpc" {
		log.Fatalf("--fail-mode must be content or rpc, got %q", *failMode)
	}
	size := *pageSize
	if size <= 0 {
		size = 1 << 20
	}

	opts := fixtureOptions{
		PageSize:          size,
		Advertise:         *advertise,
		StealNames:        *stealNames,
		Stateful:          *stateful,
		FailMode:          *failMode,
		NumericRequestIDs: *numericIDs,
		MaxMessage:        maxMsg.n,
		SerialHandlers:    *serial,
		ClientWait:        *clientWait,
		SleepScale:        *sleepScale,
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	f := newFixture(os.Stdin, os.Stdout, opts, log.New(os.Stderr, "evidra-fixture ", log.LstdFlags))
	if err := f.run(ctx); err != nil && !errors.Is(err, context.Canceled) {
		log.Fatalf("fixture server: %v", err)
	}
}
