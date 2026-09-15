// Package-internal note: this file is the merged endpoint — one upstream child
// process, Evidra's local protocol tools merged into its tool list, evidence
// written around every forwarded call. The legacy Proxy/EvidenceWriter tap it
// replaced is deleted; the fixture keeps its own independent framing
// implementation on purpose — two implementations of newline framing that
// agree is evidence, one that silently drifts is a bug the endpoint cannot
// detect.
package proxy

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/oklog/ulid/v2"
	"github.com/vitas/evidra/pkg/evidence"
	"io"
	"log"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// epDefaultMaxMessage bounds a single JSON-RPC message on either direction.
// 64 MiB covers the fixture's 20 MiB tool result with headroom while still
// refusing an unbounded stream (§28 oversize behavior: explicit error, stream
// survives).
const epDefaultMaxMessage = 64 << 20

// epIDNamespace is reserved for request ids the endpoint itself sends
// upstream. A client request id in this namespace is rejected rather than
// silently remapped, because remapping would break response correlation for
// the client and id rewriting is explicitly not what this relay does (§28).
const epIDNamespace = "_evidra_"

// epMsg is one decoded JSON-RPC 2.0 envelope. Classification follows §28:
// method+id is a request, method alone is a notification, id alone is a
// response. A packet is never assumed to be a response merely because it
// carries an id.
type epMsg struct {
	JSONRPC string          `json:"jsonrpc"`
	Method  string          `json:"method,omitempty"`
	Params  json.RawMessage `json:"params,omitempty"`
	ID      json.RawMessage `json:"id,omitempty"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   json.RawMessage `json:"error,omitempty"`

	raw json.RawMessage
}

func (m *epMsg) isRequest() bool      { return m.Method != "" && len(m.ID) > 0 }
func (m *epMsg) isNotification() bool { return m.Method != "" && len(m.ID) == 0 }
func (m *epMsg) isResponse() bool     { return m.Method == "" && len(m.ID) > 0 }

// idKey canonicalizes a raw id for use as a map key. Numbers and strings are
// distinct keys: 1 and "1" do not collide.
func (m *epMsg) idKey() string {
	if len(m.ID) == 0 {
		return ""
	}
	var buf bytes.Buffer
	if err := json.Compact(&buf, m.ID); err != nil {
		return string(m.ID)
	}
	return buf.String()
}

// epError is a JSON-RPC error with an application-level reason that agents can
// branch on (§31/§32 keep protocol failures machine-readable).
type epError struct {
	Code    int
	Message string
	Data    any
}

func (e *epError) Error() string { return e.Message }

// epStartupTimeout bounds the upstream handshake plus the initial tool-list
// walk.
const epStartupTimeout = 30 * time.Second

const (
	epCodeParseError     = -32700
	epCodeInvalidRequest = -32600
	epCodeMethodNotFound = -32601
	epCodeInternalError  = -32603
	epCodeServerError    = -32000
)

// epEndpoint is the merged single endpoint: one upstream child process plus
// Evidra's local protocol tools, appearing to the client as one MCP server.
type epEndpoint struct {
	args        []string
	env         []string
	logger      *log.Logger
	maxMsg      int
	passthrough bool
	serverName  string
	enforce     string
	evidence    epEvidence
	sessionID   string
	// obs counts executions per operation so a closed report can be answered with
	// what the proxy saw, without waiting for a store read (§47).
	obs *epObservations

	child    *exec.Cmd
	childIn  io.WriteCloser
	childW   *bufio.Writer
	childOut *bufio.Reader
	clientR  *bufio.Reader
	client   *bufio.Writer
	// childOutMu/clientMu serialize whole-frame writes so an upstream request
	// and a locally generated response can never interleave mid-line.
	childOutMu sync.Mutex
	clientMu   sync.Mutex

	// upWait holds responses to requests the endpoint itself sent upstream.
	upWait map[string]chan *epMsg
	// fwd holds client requests forwarded upstream, keyed by request id.
	fwd map[string]*epFwd
	// toClient holds requests forwarded to the client from upstream.
	toClient map[string]*epFwd

	stateMu sync.Mutex
	state   *epSession

	upInit     map[string]any
	upInstr    string
	upInstrSet bool
	upTools    []epTool
	upAnn      map[string]map[string]any

	closeOnce sync.Once
	done      chan struct{}
	pumping   atomic.Bool
}

type epFwd struct {
	clientID string
	tool     string
	// operationID is the prescription in force when the call was forwarded, or
	// empty in observe-only mode (§14, §34).
	operationID string
	// exec identifies the recorded execution this call belongs to, so a terminal
	// event can pair with its start (§16-§17).
	exec epExecution
	// cancelRequested records that the client asked to cancel while the call was
	// in flight. It changes how the terminal event is worded, not whether one is
	// written: an execution that answered anyway after being cancelled is
	// recorded as answered, because that is what happened.
	cancelRequested bool
	// readOnlyAnn is the upstream's own unverified read-only claim for this call
	// (§27). nil means no annotation was present, which is not the same as false.
	readOnlyAnn *bool
	// compose marks a forwarded request whose response the endpoint rewrites.
	compose bool
	// firstPage records that the tools/list request carried no cursor, the only
	// page that receives the local protocol tools (§27).
	firstPage bool
}

// RunEndpointOptions configures RunEndpoint.
type RunEndpointOptions struct {
	// UpstreamArgs is the command line after `--`, e.g.
	// ["evidra-fixture", "--page-size", "3"].
	UpstreamArgs []string
	// Env is the child environment; empty means os.Environ().
	Env []string
	// Logger receives operational diagnostics; nil uses log.Default().
	Logger *log.Logger
	// MaxMessage bounds one JSON-RPC message; 0 uses 64 MiB.
	MaxMessage int
	// AdvertisePassthrough advertises upstream capability families that are
	// relayed but not covered by the supported profile (§27 advertise-down).
	// Off by default: the client is told only what has conformance tests.
	AdvertisePassthrough bool
	// ServerName labels the upstream in evidence and the client-facing
	// serverInfo. Empty means derived from the command name.
	ServerName string
	// EnforceMode is "all" (default) or "off". There is no third mode: an
	// annotation-based exception would route enforcement through untrusted
	// server metadata (§7).
	EnforceMode string
	// EvidenceDir is the evidence *root*: the endpoint creates its own recorder
	// directory inside it, which is what keeps one writer per store structural
	// (§20). Empty means no store is wanted and nothing is recorded; a root that
	// cannot be opened fails the run, because silently falling back to no-op
	// recording would make "evidence was kept" a claim nobody checked.
	EvidenceDir string
	// ActorID labels who is accountable in every event; empty leaves the actor
	// unset rather than inventing one.
	ActorID string
}

// RunEndpoint serves the merged endpoint over clientIn/clientOut until the
// client closes its stdin, the upstream exits, or ctx is cancelled.
func RunEndpoint(ctx context.Context, clientIn io.Reader, clientOut io.Writer, opts RunEndpointOptions) error {
	if len(opts.UpstreamArgs) == 0 {
		return errors.New("endpoint: no upstream command")
	}
	logger := opts.Logger
	if logger == nil {
		logger = log.Default()
	}
	maxMsg := opts.MaxMessage
	if maxMsg <= 0 {
		maxMsg = epDefaultMaxMessage
	}
	ev := epEvidence(epNopEvidence{})
	if opts.EvidenceDir != "" {
		store, err := evidence.OpenStore(evidence.Options{
			Root:          opts.EvidenceDir,
			UpstreamID:    "up-" + epBaseName(opts.UpstreamArgs[0]),
			ServerName:    opts.ServerName,
			UpstreamCmd:   strings.Join(opts.UpstreamArgs, " "),
			EnforceMode:   opts.EnforceMode,
			EvidraVersion: epBuildVersion,
			Stderr:        os.Stderr,
		})
		if err != nil {
			return fmt.Errorf("endpoint: open evidence store %s: %w", opts.EvidenceDir, err)
		}
		ev = newStoreEvidence(store, epBuildVersion, opts.EnforceMode, opts.ActorID)
	}
	enforce := opts.EnforceMode
	if enforce == "" {
		enforce = epEnforceAll
	}
	if enforce != epEnforceAll && enforce != epEnforceOff {
		return fmt.Errorf("endpoint: unknown enforcement mode %q: use --enforce=%s or --enforce=%s", opts.EnforceMode, epEnforceAll, epEnforceOff)
	}
	e := &epEndpoint{
		args:        opts.UpstreamArgs,
		env:         opts.Env,
		logger:      logger,
		maxMsg:      maxMsg,
		passthrough: opts.AdvertisePassthrough,
		serverName:  opts.ServerName,
		enforce:     enforce,
		evidence:    ev,
		sessionID:   "SES-" + ulid.Make().String(),
		obs:         newObservations(),
		upWait:      map[string]chan *epMsg{},
		fwd:         map[string]*epFwd{},
		toClient:    map[string]*epFwd{},
		state:       &epSession{},
		upAnn:       map[string]map[string]any{},
		done:        make(chan struct{}),
	}
	if e.serverName == "" {
		e.serverName = epBaseName(opts.UpstreamArgs[0])
	}
	e.clientR = bufio.NewReaderSize(clientIn, 64<<10)
	e.client = bufio.NewWriter(clientOut)

	if err := e.startChild(ctx); err != nil {
		return err
	}
	defer e.stopChild()

	// The upstream reader runs from the start: endpoint-origin requests such as
	// the handshake and the tool-list prefetch are answered through the same
	// path as relayed traffic, so nothing has to read the child stream by hand
	// and every wait stays subject to its deadline.
	e.pumping.Store(true)
	go e.pumpUpstream(ctx)

	// A silent upstream must fail at startup with a clear message rather than
	// hang the client forever.
	startCtx, cancelStart := context.WithTimeout(ctx, epStartupTimeout)
	defer cancelStart()

	upResult, err := e.handshakeUpstream(startCtx)
	if err != nil {
		return err
	}
	e.upInit = upResult
	e.upInstr = epStringField(upResult, "instructions")
	e.upInstrSet = e.upInstr != ""

	if err := e.prefetchUpstreamTools(startCtx); err != nil {
		return err
	}

	err = e.pumpClient(ctx)

	e.shutdown()
	return err
}

// startChild launches the upstream process with piped stdio.
func (e *epEndpoint) startChild(ctx context.Context) error {
	cmd := exec.CommandContext(ctx, e.args[0], e.args[1:]...)
	if len(e.env) > 0 {
		cmd.Env = e.env
	}
	in, err := cmd.StdinPipe()
	if err != nil {
		return fmt.Errorf("endpoint: upstream stdin: %w", err)
	}
	out, err := cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("endpoint: upstream stdout: %w", err)
	}
	cmd.Stderr = os.Stderr
	e.child = cmd
	e.childIn = in
	e.childW = bufio.NewWriter(in)
	e.childOut = bufio.NewReaderSize(out, 64<<10)
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("endpoint: start upstream %q: %w", e.args[0], err)
	}
	// No lifecycle event here: the store wrote recorder_started when it opened,
	// and a second one would double-count process starts.
	return nil
}

func (e *epEndpoint) stopChild() {
	if e.child == nil || e.child.Process == nil {
		return
	}
	_ = e.childIn.Close()
	_ = e.child.Process.Kill()
	_ = e.child.Wait()
}

func (e *epEndpoint) shutdown() {
	e.closeOnce.Do(func() {
		close(e.done)
		// Anything still outstanding gets its terminal event here, while there is
		// still a recorder to write to. The endpoint is the only witness of these
		// calls and it is going away; leaving an execution started-without-finish
		// would report a session that never ended (§17).
		e.failOutstanding("endpoint shutting down")
		if err := e.evidence.close("stopped"); err != nil {
			e.logger.Printf("endpoint: evidence store close: %v", err)
		}
	})
}

// handshakeUpstream sends initialize/initialized to the upstream and returns
// its result.
func (e *epEndpoint) handshakeUpstream(ctx context.Context) (map[string]any, error) {
	params := map[string]any{
		"protocolVersion": epLatestProtocolVersion,
		"capabilities":    epClientCapabilities(),
		"clientInfo": map[string]any{
			"name":    "evidra-mcp",
			"title":   "Evidra merged endpoint",
			"version": epVersionOf(),
		},
	}
	raw, err := e.requestUpstream(ctx, "initialize", params, 20*time.Second)
	if err != nil {
		return nil, fmt.Errorf("endpoint: upstream initialize: %w", err)
	}
	var result map[string]any
	if err := json.Unmarshal(raw, &result); err != nil {
		return nil, fmt.Errorf("endpoint: upstream initialize result: %w", err)
	}
	_ = e.notifyUpstream(ctx, "notifications/initialized", nil)
	return result, nil
}

// requestUpstream sends one endpoint-origin request and waits for its response.
func (e *epEndpoint) requestUpstream(ctx context.Context, method string, params any, timeout time.Duration) (json.RawMessage, error) {
	id := epQuoteID(fmt.Sprintf("%sup-%d", epIDNamespace, time.Now().UnixNano()))
	ch := make(chan *epMsg, 1)
	e.stateMu.Lock()
	e.upWait[id] = ch
	e.stateMu.Unlock()

	payload, err := epEncode(&epMsg{
		JSONRPC: "2.0",
		Method:  method,
		Params:  epRawParams(params),
		ID:      json.RawMessage(id),
	})
	if err != nil {
		e.dropWait(id)
		return nil, err
	}
	if err := e.writeUpstream(payload); err != nil {
		e.dropWait(id)
		return nil, err
	}
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	select {
	case msg := <-ch:
		e.dropWait(id)
		if len(msg.Error) > 0 {
			return nil, fmt.Errorf("upstream: %s", string(msg.Error))
		}
		return msg.Result, nil
	case <-timer.C:
		e.dropWait(id)
		return nil, fmt.Errorf("upstream: %s timed out after %s", method, timeout)
	case <-ctx.Done():
		e.dropWait(id)
		return nil, ctx.Err()
	}
}

func (e *epEndpoint) dropWait(id string) {
	e.stateMu.Lock()
	delete(e.upWait, id)
	e.stateMu.Unlock()
}

func (e *epEndpoint) notifyUpstream(ctx context.Context, method string, params any) error {
	payload, err := epEncode(&epMsg{
		JSONRPC: "2.0",
		Method:  method,
		Params:  epRawParams(params),
	})
	if err != nil {
		return err
	}
	return e.writeUpstream(payload)
}

func (e *epEndpoint) writeUpstream(payload []byte) error {
	e.childOutMu.Lock()
	defer e.childOutMu.Unlock()
	if _, err := e.childW.Write(payload); err != nil {
		return fmt.Errorf("endpoint: write upstream: %w", err)
	}
	// A bufio.Writer can swallow the failure inside Write when the payload still fit
	// the buffer and surface it at Flush instead, so the flush needs the same
	// directional wrapping. Without it the caller sees a bare upstream error and
	// cannot tell a write failure from the upstream answering badly.
	if err := e.childW.Flush(); err != nil {
		return fmt.Errorf("endpoint: write upstream: %w", err)
	}
	return nil
}

func (e *epEndpoint) writeClient(payload []byte) error {
	e.clientMu.Lock()
	defer e.clientMu.Unlock()
	if _, err := e.client.Write(payload); err != nil {
		return fmt.Errorf("endpoint: write client: %w", err)
	}
	if err := e.client.Flush(); err != nil {
		return fmt.Errorf("endpoint: write client: %w", err)
	}
	return nil
}

// pumpClient reads the client's stdout-facing stream and routes each message.
func (e *epEndpoint) pumpClient(ctx context.Context) error {
	for {
		line, err := epReadMessage(e.clientR, e.maxMsg)
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			// A frame that is too big is reported and skipped: the framing
			// reader drained to the next newline, so the stream stays
			// synchronized and the session survives (§28, T13).
			e.logger.Printf("endpoint: client frame rejected: %v", err)
			if werr := e.writeClient(epErrorFrame(nil, epCodeParseError, err.Error())); werr != nil {
				return werr
			}
			continue
		}
		msg, ok := epDecode(line)
		if !ok {
			if err := e.writeClient(epErrorFrame(nil, epCodeParseError, "parse error: invalid JSON-RPC message")); err != nil {
				return err
			}
			continue
		}
		switch {
		case msg.isRequest():
			if err := e.handleClientRequest(ctx, msg); err != nil {
				e.logger.Printf("endpoint: client request: %v", err)
				return err
			}
		case msg.isNotification():
			if err := e.handleClientNotification(ctx, msg); err != nil {
				return err
			}
		case msg.isResponse():
			if err := e.handleClientResponse(ctx, msg); err != nil {
				return err
			}
		default:
			if err := e.writeClient(epErrorFrame(msg.ID, epCodeInvalidRequest, "invalid request: method and id required")); err != nil {
				return err
			}
		}
	}
}

func (e *epEndpoint) handleClientRequest(ctx context.Context, msg *epMsg) error {
	key := msg.idKey()
	if strings.HasPrefix(key, `"`+epIDNamespace) {
		return e.writeClient(epErrorFrame(msg.ID, epCodeInvalidRequest,
			"invalid request: id namespace "+epIDNamespace+" is reserved by Evidra"))
	}

	switch msg.Method {
	case "initialize":
		result, protocolErr := e.composeInitializeResult(msg)
		return e.reply(msg, result, protocolErr)
	case "tools/list":
		return e.relayComposable(msg)
	case "ping":
		// The client is talking to this endpoint, so this endpoint answers.
		return e.writeClient(epResultFrame(msg.ID, map[string]any{}))
	case "tools/call":
		return e.dispatchToolCall(msg, key)
	default:
		return e.relayPlain(ctx, msg, key)
	}
}

// dispatchToolCall routes a tools/call either to Evidra's local protocol tools
// or to the upstream. Local calls never reach the upstream, and upstream tool
// names are never rewritten (§6).
func (e *epEndpoint) dispatchToolCall(msg *epMsg, key string) error {
	var params struct {
		Name      string          `json:"name"`
		Arguments json.RawMessage `json:"arguments"`
	}
	_ = json.Unmarshal(msg.Params, &params)
	// A missing arguments object is an empty one: the handler's own required
	// field checks then produce the protocol error rather than a decode error.
	args := params.Arguments
	if len(args) == 0 || string(args) == "null" {
		args = []byte("{}")
	}
	switch params.Name {
	case "evidra_prescribe":
		out, protocolErr := e.handlePrescribe(args)
		return e.replyLocalTool(msg, out, protocolErr)
	case "evidra_report":
		out, protocolErr := e.handleReport(args)
		return e.replyLocalTool(msg, out, protocolErr)
	}

	open := e.openOperationID()
	if e.enforce == epEnforceAll && open == "" {
		return e.blockUnprescribed(msg, params.Name)
	}

	// §18: an operational call whose start cannot be persisted is not forwarded.
	// Letting it through would create the one asymmetry this product cannot
	// afford - an action that happened but was never observed.
	if e.evidence.unhealthy() {
		return e.recorderUnavailable(msg, params.Name, "evidence store is unhealthy")
	}
	ann := e.annotationsFor(params.Name)
	ex, err := e.evidence.startExecution(e.sessionID, open, params.Name, args, ann)
	if err != nil {
		e.logger.Printf("endpoint: refused %s, execution_started was not persisted: %v", params.Name, err)
		return e.recorderUnavailable(msg, params.Name, "could not record execution_started")
	}
	e.stateMu.Lock()
	e.fwd[key] = &epFwd{clientID: key, tool: params.Name, operationID: open, exec: ex, readOnlyAnn: epReadOnlyAnnotation(ann)}
	e.stateMu.Unlock()
	return e.writeUpstream(msg.raw)
}

// openOperationID returns the id of the open operation, or "" when none is
// open.
func (e *epEndpoint) openOperationID() string {
	e.stateMu.Lock()
	defer e.stateMu.Unlock()
	if e.state.open == nil {
		return ""
	}
	return e.state.open.ID
}

// blockUnprescribed implements the single enforcement rule of §7: with no open
// prescription an upstream call is not forwarded, is recorded as a protocol
// violation, and the agent gets an explicit instruction instead. The refusal is
// a tool result rather than a transport error so the instruction lands in the
// model's transcript, where it can change the next action.
func (e *epEndpoint) blockUnprescribed(msg *epMsg, tool string) error {
	e.stateMu.Lock()
	e.state.blocks++
	e.stateMu.Unlock()
	if err := e.evidence.blocked(e.sessionID, "", tool, "unprescribed_execution_attempt",
		"no operation was open in this session"); err != nil {
		e.logger.Printf("endpoint: could not record the blocked call for %s: %v", tool, err)
	}
	body := map[string]any{
		"error":           "no_open_operation",
		"tool":            tool,
		"required_action": "evidra_prescribe",
		"instruction": "Call evidra_prescribe with the objective you intend to achieve, then repeat this tool call. " +
			"Every upstream tool of this server requires an open operation, including tools the server declares read-only.",
	}
	return e.writeClient(epResultFrame(msg.ID, map[string]any{
		"content": []any{map[string]any{"type": "text", "text": epJSONString(body)}},
		"isError": true,
	}))
}

// relayPlain forwards a client request upstream verbatim, keeping the id.
func (e *epEndpoint) relayPlain(ctx context.Context, msg *epMsg, key string) error {
	e.stateMu.Lock()
	e.fwd[key] = &epFwd{clientID: key}
	e.stateMu.Unlock()
	return e.writeUpstream(msg.raw)
}

// relayComposable forwards a request whose response the endpoint must rewrite
// (tools/list), while keeping the wire form otherwise identical.
func (e *epEndpoint) relayComposable(msg *epMsg) error {
	var params struct {
		Cursor string `json:"cursor"`
	}
	_ = json.Unmarshal(msg.Params, &params)
	key := msg.idKey()
	e.stateMu.Lock()
	e.fwd[key] = &epFwd{clientID: key, compose: true, firstPage: params.Cursor == ""}
	e.stateMu.Unlock()
	return e.writeUpstream(msg.raw)
}

// reply answers a client request from data the endpoint holds locally.
func (e *epEndpoint) reply(msg *epMsg, result any, err *epError) error {
	if err != nil {
		return e.writeClient(epErrorFrameData(msg.ID, err))
	}
	return e.writeClient(epResultFrame(msg.ID, result))
}

func (e *epEndpoint) replyLocalTool(msg *epMsg, out any, protocolErr *epError) error {
	// Protocol failures are reported as tool *content* with isError so that a
	// model sees the recovery instruction in the transcript instead of a bare
	// transport error (§31/§32 both specify machine-readable failure shapes).
	if protocolErr != nil {
		return e.writeClient(epResultFrame(msg.ID, map[string]any{
			"content": []any{map[string]any{"type": "text", "text": epJSONString(protocolErr.Data)}},
			"isError": true,
		}))
	}
	return e.writeClient(epResultFrame(msg.ID, map[string]any{
		"content": []any{map[string]any{"type": "text", "text": epJSONString(out)}},
		"isError": false,
	}))
}

func (e *epEndpoint) handleClientNotification(ctx context.Context, msg *epMsg) error {
	switch msg.Method {
	case "notifications/cancelled":
		id := epRequestIDField(msg.Params, "requestId")
		fwd := e.lookupFwd(id)
		if fwd == nil {
			// Nothing outstanding: the cancel raced a response. Forwarding it
			// anyway is harmless and keeps the upstream informed.
			e.logger.Printf("endpoint: cancel for unknown request %s", id)
		} else {
			e.stateMu.Lock()
			fwd.cancelRequested = true
			e.stateMu.Unlock()
		}
		return e.writeUpstream(msg.raw)
	default:
		return e.writeUpstream(msg.raw)
	}
}

func (e *epEndpoint) handleClientResponse(ctx context.Context, msg *epMsg) error {
	key := msg.idKey()
	e.stateMu.Lock()
	fwd := e.toClient[key]
	delete(e.toClient, key)
	e.stateMu.Unlock()
	if fwd == nil {
		e.logger.Printf("endpoint: dropping unmatched response from client: %s", key)
		return nil
	}
	return e.writeUpstream(msg.raw)
}

// pumpUpstream reads the upstream's responses and requests and routes them.
func (e *epEndpoint) pumpUpstream(ctx context.Context) {
	for {
		line, err := epReadMessage(e.childOut, e.maxMsg)
		if err != nil {
			if errors.Is(err, io.EOF) {
				e.failOutstanding("upstream process closed the stream")
				return
			}
			if strings.Contains(err.Error(), "exceeds") {
				// Too big to relay. The reader drained to the next newline, so
				// only this message is lost; fail the request it belongs to
				// rather than hanging the client.
				e.logger.Printf("endpoint: upstream frame rejected: %v", err)
				e.failOutstanding(err.Error())
				continue
			}
			e.logger.Printf("endpoint: upstream read: %v", err)
			e.failOutstanding("upstream process closed the stream")
			return
		}
		msg, ok := epDecode(line)
		if !ok {
			// Malformed upstream output must not be relayed verbatim to the
			// client, which would fail its parser with no explanation.
			e.logger.Printf("endpoint: dropping malformed upstream frame")
			continue
		}
		switch {
		case msg.isResponse():
			e.routeUpstreamResponse(msg)
		case msg.isRequest():
			// Server-to-client request: forward with the id preserved and
			// remember to route the client's answer back (§28, T15).
			key := msg.idKey()
			e.stateMu.Lock()
			e.toClient[key] = &epFwd{clientID: key}
			e.stateMu.Unlock()
			if err := e.writeClient(msg.raw); err != nil {
				return
			}
		case msg.isNotification():
			if msg.Method == "notifications/tools/list_changed" {
				// The refresh issues upstream requests, and pumpUpstream is
				// the only reader of upstream responses, so it cannot run
				// inline here.
				go e.refreshUpstreamTools(context.WithoutCancel(ctx))
			}
			if err := e.writeClient(msg.raw); err != nil {
				return
			}
		default:
			e.logger.Printf("endpoint: dropping undclassifiable upstream frame")
		}
	}
}

func (e *epEndpoint) routeUpstreamResponse(msg *epMsg) {
	key := msg.idKey()
	e.stateMu.Lock()
	wait := e.upWait[key]
	fwd := e.fwd[key]
	if fwd != nil {
		delete(e.fwd, key)
	}
	e.stateMu.Unlock()
	if wait != nil {
		select {
		case wait <- msg:
		default:
		}
		return
	}
	if fwd == nil {
		e.logger.Printf("endpoint: dropping unmatched response from upstream: %s", key)
		return
	}
	if fwd.compose {
		out, err := e.composeToolsListResponse(fwd, msg)
		if err != nil {
			e.logger.Printf("endpoint: tools/list compose: %v", err)
			return
		}
		_ = e.writeClient(out)
		return
	}
	// §17 fixes the order: the terminal event is appended, then the response goes
	// back to the client. Relaying first lets an agent that acts on the result
	// immediately - which is the whole point of an agent - land its
	// operation_reported ahead of the execution it is reporting about, and no
	// amount of downstream reconciliation can unscramble that. A failed terminal
	// append still relays the real result (§18), which is why the error is logged
	// here rather than acted on.
	e.finishExecution(fwd, msg.raw)
	if err := e.writeClient(msg.raw); err != nil {
		e.logger.Printf("endpoint: relay to client: %v", err)
	}
}

// lookupFwd finds an outstanding forwarded request from an id written by the
// client. Ids are matched by value, and the cancel notification carries the id as
// the client chose to spell it, so a numeric 7 and a string "7" both have to find
// the request whose key was recorded in whichever form arrived. Without this the
// cancellation lands nowhere and the execution keeps its provisional start.
func (e *epEndpoint) lookupFwd(key string) *epFwd {
	if key == "" {
		return nil
	}
	e.stateMu.Lock()
	defer e.stateMu.Unlock()
	if f := e.fwd[key]; f != nil {
		return f
	}
	var number json.Number
	if err := json.Unmarshal([]byte(key), &number); err == nil {
		return e.fwd[`"`+key+`"`]
	}
	if bare, err := strconv.Unquote(key); err == nil {
		return e.fwd[bare]
	}
	return nil
}

// epRequestIDField reads a request id out of notification params without deciding
// whether it is a string or a number, then compacts it to the same form idKey uses.
func epRequestIDField(params json.RawMessage, key string) string {
	var in map[string]json.RawMessage
	if err := json.Unmarshal(params, &in); err != nil {
		return ""
	}
	raw, ok := in[key]
	if !ok {
		return ""
	}
	var buf bytes.Buffer
	if err := json.Compact(&buf, raw); err != nil {
		return ""
	}
	return buf.String()
}

// annotationsFor returns the upstream-declared annotations for a tool as they
// stood at the last tools/list. Absence is preserved on purpose: evidence that a
// server declared nothing must not look like evidence that it declared read-only
// (§22, §26).
func (e *epEndpoint) annotationsFor(tool string) json.RawMessage {
	e.stateMu.Lock()
	defer e.stateMu.Unlock()
	ann, ok := e.upAnn[tool]
	if !ok || ann == nil {
		return nil
	}
	raw, err := json.Marshal(ann)
	if err != nil {
		return nil
	}
	return raw
}

// finishExecution writes the terminal event for a forwarded call. Losing it
// degrades coverage without rewriting anything the client sees: the real result is
// relayed either way, because a fabricated operational failure invites an
// automatic retry of work that succeeded (§18).
func (e *epEndpoint) finishExecution(fwd *epFwd, raw json.RawMessage) {
	if fwd == nil {
		return
	}
	status, code, message := epClassifyResponse(raw)
	// Counted before the durable-id check on purpose: an execution the proxy watched
	// happened whether or not it was persisted, and dropping it here would make the
	// in-band feedback silently depend on --evidence-dir (§47, §18).
	e.obs.note(fwd.operationID, fwd.tool, status == evidence.ExecutionSuccess, fwd.readOnlyAnn)
	if fwd.exec.ID == "" {
		return
	}
	if err := e.evidence.finishExecution(fwd.exec, raw, status, code, message); err != nil {
		e.logger.Printf("endpoint: execution terminal evidence lost for %s: %v", fwd.tool, err)
	}
}

// recorderUnavailable answers a refused call the way enforcement answers a block:
// as a tool result carrying the instruction rather than a transport error, so the
// agent can read it and change what it does next.
func (e *epEndpoint) recorderUnavailable(msg *epMsg, tool, detail string) error {
	body := map[string]any{
		"error":           "recorder_unhealthy",
		"tool":            tool,
		"required_action": "retry once evidence storage recovers",
		"instruction": "Evidra could not persist the start of this execution, so the call was not " +
			"forwarded and nothing happened. evidra_report stays available; close the current " +
			"operation and retry the tool call later.",
		"detail": detail,
	}
	return e.writeClient(epResultFrame(msg.ID, map[string]any{
		"content": []any{map[string]any{"type": "text", "text": epJSONString(body)}},
		"isError": true,
	}))
}

// epClassifyResponse decides what an upstream frame proves about an execution.
// Only three outcomes are claimable from the wire, so unknown stays reachable.
func epClassifyResponse(raw json.RawMessage) (evidence.ExecutionStatus, string, string) {
	var frame struct {
		Error *struct {
			Code    int    `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
		Result struct {
			IsError bool `json:"isError"`
		} `json:"result"`
	}
	if len(raw) == 0 {
		return evidence.ExecutionUnknown, "no_response", ""
	}
	if err := json.Unmarshal(raw, &frame); err != nil {
		return evidence.ExecutionUnknown, "undecodable_frame", ""
	}
	if frame.Error != nil {
		code := fmt.Sprintf("jsonrpc_%d", frame.Error.Code)
		if strings.Contains(strings.ToLower(frame.Error.Message), "cancel") {
			return evidence.ExecutionCancelled, code, frame.Error.Message
		}
		return evidence.ExecutionError, code, frame.Error.Message
	}
	if frame.Result.IsError {
		return evidence.ExecutionError, "tool_error", ""
	}
	return evidence.ExecutionSuccess, "", ""
}

// failOutstanding answers every client request still waiting on the upstream so
// a client never hangs when the child dies mid-call.
func (e *epEndpoint) failOutstanding(reason string) {
	e.stateMu.Lock()
	ids := make([]string, 0, len(e.fwd))
	var outstanding []*epFwd
	for id, f := range e.fwd {
		outstanding = append(outstanding, f)
		ids = append(ids, id)
		delete(e.fwd, id)
	}
	e.stateMu.Unlock()
	// The upstream died with these in flight. Closing the pair as unknown keeps
	// the record honest about what was and was not observed (§17).
	for _, f := range outstanding {
		if f.exec.ID == "" {
			continue
		}
		status, code, detail := evidence.ExecutionUnknown, "upstream_unavailable",
			"no response arrived before the upstream exited"
		if f.cancelRequested {
			// The client stopped caring and nothing answered: cancelled is the
			// claim the wire supports. Had a response arrived, the real status
			// would have been recorded instead (§17).
			status, code, detail = evidence.ExecutionCancelled, "cancelled_by_client",
				"client cancelled and no response arrived"
		}
		if err := e.evidence.finishExecution(f.exec, nil, status, code, detail); err != nil {
			e.logger.Printf("endpoint: could not close execution evidence: %v", err)
		}
	}
	for _, id := range ids {
		_ = e.writeClient(epErrorFrame(json.RawMessage(id), epCodeInternalError, "upstream unavailable: "+reason))
	}
}

// epReadMessage reads one newline-delimited frame, refusing anything above max
// bytes without desynchronizing the stream.
func epReadMessage(r *bufio.Reader, max int) ([]byte, error) {
	var buf []byte
	for {
		frag, err := r.ReadSlice('\n')
		// The bound is compared against the accumulated frame, never against one
		// slice: a bufio.Reader reports ErrBufferFull at its own buffer size, so
		// treating that as "too large" capped every message at the reader's 64 KiB
		// and made --max-message decoration.
		if len(buf)+len(frag) > max {
			// Drain to the next newline before refusing. Returning early leaves the
			// rest of the frame in the pipe, and the next read would then parse the
			// middle of a large message as a new one.
			if drainErr := epDrainToNewline(r); drainErr != nil {
				return nil, drainErr
			}
			return nil, fmt.Errorf("message exceeds %d bytes", max)
		}
		buf = append(buf, frag...)
		switch {
		case err == nil:
			return buf, nil
		case errors.Is(err, bufio.ErrBufferFull):
			continue
		default:
			return nil, err
		}
	}
}

func epDrainToNewline(r *bufio.Reader) error {
	for {
		_, err := r.ReadSlice('\n')
		if err == nil {
			return nil
		}
		if errors.Is(err, bufio.ErrBufferFull) {
			continue
		}
		return err
	}
}

func epDecode(line []byte) (*epMsg, bool) {
	msg := &epMsg{}
	if err := json.Unmarshal(line, msg); err != nil {
		return nil, false
	}
	msg.raw = json.RawMessage(append([]byte(nil), line...))
	return msg, true
}

func epEncode(msg *epMsg) ([]byte, error) {
	raw, err := json.Marshal(msg)
	if err != nil {
		return nil, err
	}
	return append(raw, '\n'), nil
}

func epResultFrame(id json.RawMessage, result any) []byte {
	raw, _ := json.Marshal(result)
	line, _ := epEncode(&epMsg{JSONRPC: "2.0", ID: id, Result: raw})
	return line
}

func epErrorFrame(id json.RawMessage, code int, message string) []byte {
	return epErrorFrameData(id, &epError{Code: code, Message: message})
}

func epErrorFrameData(id json.RawMessage, e *epError) []byte {
	d := map[string]any{"code": e.Code, "message": e.Message}
	if e.Data != nil {
		d["data"] = e.Data
	}
	raw, _ := json.Marshal(d)
	line, _ := epEncode(&epMsg{JSONRPC: "2.0", ID: id, Error: raw})
	return line
}

func epRawParams(params any) json.RawMessage {
	if params == nil {
		return nil
	}
	raw, err := json.Marshal(params)
	if err != nil {
		return nil
	}
	return raw
}

func epStringField(result map[string]any, key string) string {
	if result == nil {
		return ""
	}
	if v, ok := result[key].(string); ok {
		return v
	}
	return ""
}

func epJSONString(v any) string {
	if s, ok := v.(string); ok {
		return s
	}
	raw, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return fmt.Sprint(v)
	}
	return string(raw)
}

func epBaseName(cmd string) string {
	if i := bytes.LastIndexByte([]byte(cmd), '/'); i >= 0 {
		return cmd[i+1:]
	}
	return cmd
}

func epQuoteID(id string) string {
	raw, err := json.Marshal(id)
	if err != nil {
		return `""`
	}
	return string(raw)
}
