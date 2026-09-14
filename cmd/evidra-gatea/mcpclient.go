package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"strings"
	"sync"
	"time"
)

// mcpClient is the smallest stdio MCP client that can drive the merged endpoint
// honestly: newline framing, shape-based classification, and answers to
// server-initiated requests. It is written independently of cmd/evidra-fixture's
// server-side reader so that a framing bug cannot hide in one implementation
// mirrored in another.
type mcpClient struct {
	cmd    *exec.Cmd
	in     io.WriteCloser
	out    *bufio.Reader
	nextID int
	stderr *bytes.Buffer

	mu     sync.Mutex
	notes  []mcpFrame
	header map[string]any
}

type mcpFrame struct {
	JSONRPC string          `json:"jsonrpc"`
	Method  string          `json:"method,omitempty"`
	Params  json.RawMessage `json:"params,omitempty"`
	ID      json.RawMessage `json:"id,omitempty"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   json.RawMessage `json:"error,omitempty"`
}

func (f *mcpFrame) isRequest() bool  { return f.Method != "" && len(f.ID) > 0 }
func (f *mcpFrame) isResponse() bool { return f.Method == "" && len(f.ID) > 0 }

type mcpTool struct {
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	InputSchema json.RawMessage `json:"inputSchema,omitempty"`
	Annotations map[string]any  `json:"annotations,omitempty"`
}

type toolResult struct {
	Text    string
	IsError bool
}

// declaredReadOnlyTools returns the names of the tools the server itself annotated
// `readOnlyHint: true`. It reads that declaration and nothing else.
//
// Only a literal `true` counts. An explicit `readOnlyHint: false` and an absent
// `annotations` object both land outside the list, which is the distinction the endpoint
// keeps when it says that evidence a server declared nothing must not be rendered as
// evidence that it declared read-only (§22, §26).
//
// Inferring read-only-ness from a tool's name or its arguments is deliberately not done
// here: that is the content-based classification §7 forbids the product from performing,
// and a metric built on a guess would be a verdict wearing a measurement's clothes.
func declaredReadOnlyTools(tools []mcpTool) []string {
	var out []string
	for _, t := range tools {
		if v, ok := t.Annotations["readOnlyHint"].(bool); ok && v {
			out = append(out, t.Name)
		}
	}
	return out
}

// startMCP launches a command as an MCP server over stdio.
func startMCP(ctx context.Context, bin string, args ...string) (*mcpClient, error) {
	cmd := exec.CommandContext(ctx, bin, args...)
	in, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}
	out, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	errBuf := &bytes.Buffer{}
	cmd.Stderr = &lockedWriter{buf: errBuf}
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("start %s: %w", bin, err)
	}
	return &mcpClient{cmd: cmd, in: in, out: bufio.NewReaderSize(out, 8<<20), nextID: 0, stderr: errBuf}, nil
}

// lockedWriter keeps a child's stderr safe to append from the process while the
// test/runner reads it.
type lockedWriter struct {
	mu  sync.Mutex
	buf *bytes.Buffer
}

func (w *lockedWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.buf.Write(p)
}

func (w *lockedWriter) String() string {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.buf.String()
}

func (c *mcpClient) stderrText() string { return c.stderr.String() }

func (c *mcpClient) send(frame any) error {
	raw, err := json.Marshal(frame)
	if err != nil {
		return err
	}
	_, err = c.in.Write(append(raw, '\n'))
	return err
}

// rpc sends a request and returns its response, answering any server-initiated
// request it meets on the way and queueing notifications.
func (c *mcpClient) rpc(ctx context.Context, method string, params any, timeout time.Duration) (*mcpFrame, error) {
	c.nextID++
	id := c.nextID
	if err := c.send(map[string]any{
		"jsonrpc": "2.0", "id": id, "method": method, "params": params,
	}); err != nil {
		return nil, err
	}
	deadline := time.NewTimer(timeout)
	defer deadline.Stop()
	type res struct {
		f   *mcpFrame
		err error
	}
	ch := make(chan res, 1)
	go func() {
		for {
			f, err := c.read()
			if err != nil {
				ch <- res{err: err}
				return
			}
			switch {
			case f.isResponse() && fmt.Sprint(jsonString(f.ID)) == fmt.Sprint(id):
				ch <- res{f: f}
				return
			case f.isResponse():
				// A response for an id this client is not waiting on: keep it so
				// the artifact still shows what arrived.
				c.note(f)
			case f.isRequest():
				if err := c.answerServerRequest(f); err != nil {
					ch <- res{err: err}
					return
				}
			default:
				c.note(f)
			}
		}
	}()
	select {
	case r := <-ch:
		return r.f, r.err
	case <-deadline.C:
		return nil, &infraFailure{reason: fmt.Sprintf("timeout after %s waiting for %s", timeout, method)}
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// answerServerRequest implements what any real agent client has to do when the
// wrapped server asks for roots. Anything else is refused with a JSON-RPC error
// rather than left unanswered, because a silent drop looks like a protocol
// failure to the upstream.
func (c *mcpClient) answerServerRequest(f *mcpFrame) error {
	switch f.Method {
	case "roots/list":
		return c.send(map[string]any{"jsonrpc": "2.0", "id": json.RawMessage(f.ID),
			"result": map[string]any{"roots": []any{
				map[string]any{"uri": "file:///gatea/workspace", "name": "Gate A workspace"},
			}}})
	default:
		return c.send(map[string]any{"jsonrpc": "2.0", "id": json.RawMessage(f.ID),
			"error": map[string]any{"code": -32601, "message": "method not supported by Gate A client: " + f.Method}})
	}
}

func (c *mcpClient) note(f *mcpFrame) {
	c.mu.Lock()
	c.notes = append(c.notes, *f)
	c.mu.Unlock()
}

func (c *mcpClient) read() (*mcpFrame, error) {
	for {
		line, err := c.out.ReadBytes('\n')
		if err != nil {
			return nil, err
		}
		line = bytes.TrimRight(line, "\r\n")
		if len(line) == 0 {
			continue
		}
		var f mcpFrame
		if err := json.Unmarshal(line, &f); err != nil {
			return nil, fmt.Errorf("malformed frame from endpoint %q: %w", truncate(string(line), 120), err)
		}
		return &f, nil
	}
}

func (c *mcpClient) notify(method string, params any) error {
	return c.send(map[string]any{"jsonrpc": "2.0", "method": method, "params": params})
}

func (c *mcpClient) initialize(ctx context.Context) (map[string]any, error) {
	resp, err := c.rpc(ctx, "initialize", map[string]any{
		"protocolVersion": "2025-06-18",
		"capabilities": map[string]any{
			"roots":    map[string]any{"listChanged": false},
			"sampling": map[string]any{},
		},
		"clientInfo": map[string]any{"name": "evidra-gatea", "version": "0"},
	}, 30*time.Second)
	if err != nil {
		return nil, err
	}
	if len(resp.Error) > 0 {
		return nil, fmt.Errorf("initialize failed: %s", resp.Error)
	}
	if err := c.notify("notifications/initialized", map[string]any{}); err != nil {
		return nil, err
	}
	var result map[string]any
	if err := json.Unmarshal(resp.Result, &result); err != nil {
		return nil, err
	}
	c.header = result
	return result, nil
}

func (c *mcpClient) listAllTools(ctx context.Context) ([]mcpTool, error) {
	var out []mcpTool
	cursor := ""
	for page := 0; page < 50; page++ {
		params := map[string]any{}
		if cursor != "" {
			params["cursor"] = cursor
		}
		resp, err := c.rpc(ctx, "tools/list", params, 30*time.Second)
		if err != nil {
			return nil, err
		}
		if len(resp.Error) > 0 {
			return nil, fmt.Errorf("tools/list failed: %s", resp.Error)
		}
		var result struct {
			Tools      []mcpTool `json:"tools"`
			NextCursor string    `json:"nextCursor"`
		}
		if err := json.Unmarshal(resp.Result, &result); err != nil {
			return nil, err
		}
		out = append(out, result.Tools...)
		if result.NextCursor == "" {
			return out, nil
		}
		cursor = result.NextCursor
	}
	return nil, errors.New("tools/list did not terminate")
}

// callTool performs one tools/call and extracts its text content.
func (c *mcpClient) callTool(ctx context.Context, name string, args map[string]any) (toolResult, error) {
	if args == nil {
		args = map[string]any{}
	}
	resp, err := c.rpc(ctx, "tools/call", map[string]any{"name": name, "arguments": args}, 4*time.Minute)
	if err != nil {
		return toolResult{}, err
	}
	if len(resp.Error) > 0 {
		return toolResult{Text: string(resp.Error), IsError: true}, nil
	}
	var result struct {
		Content []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"content"`
		IsError bool `json:"isError"`
	}
	if err := json.Unmarshal(resp.Result, &result); err != nil {
		return toolResult{}, err
	}
	parts := make([]string, 0, len(result.Content))
	for _, ct := range result.Content {
		if ct.Type == "text" || ct.Type == "" {
			parts = append(parts, ct.Text)
		} else {
			parts = append(parts, "["+ct.Type+" content]")
		}
	}
	return toolResult{Text: strings.Join(parts, "\n"), IsError: result.IsError}, nil
}

func (c *mcpClient) close() {
	_ = c.in.Close()
	done := make(chan struct{})
	go func() { _ = c.cmd.Wait(); close(done) }()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		_ = c.cmd.Process.Kill()
		<-done
	}
}

func jsonString(raw json.RawMessage) string { return strings.Trim(string(raw), `"`) }

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
