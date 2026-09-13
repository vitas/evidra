package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"regexp"
	"strings"
	"time"
)

// infraFailure marks a run that must not enter any denominator (§10): gateway
// balance or access errors, transport failures, or truncated reasoning. An
// account-state event that reads as agent non-compliance would decide Gate A on
// the wrong evidence.
type infraFailure struct{ reason string }

func (e *infraFailure) Error() string { return "infrastructure: " + e.reason }

// llmToolCall is one OpenAI-style function call.
type llmToolCall struct {
	ID       string `json:"id"`
	Type     string `json:"type"`
	Function struct {
		Name      string `json:"name"`
		Arguments string `json:"arguments"`
	} `json:"function"`
}

type llmTool struct {
	Type     string `json:"type"`
	Function struct {
		Name        string          `json:"name"`
		Description string          `json:"description,omitempty"`
		Parameters  json.RawMessage `json:"parameters,omitempty"`
	} `json:"function"`
}

type llmMessage struct {
	Role       string        `json:"role"`
	Content    any           `json:"content,omitempty"`
	ToolCalls  []llmToolCall `json:"tool_calls,omitempty"`
	ToolCallID string        `json:"tool_call_id,omitempty"`
	Name       string        `json:"name,omitempty"`
}

type completion struct {
	Text   string
	Calls  []llmToolCall
	Finish string
	Prompt int
	Output int
	Reason int
}

// agent is the thing that decides the next action. Two implementations: a real
// model, and a scripted protocol-correct client used by --dry-run to exercise
// the harness, predicates, and artifacts without spending any tokens.
type agent interface {
	next(ctx context.Context, messages []llmMessage, tools []llmTool) (*completion, error)
}

type modelAgent struct {
	http        *http.Client
	baseURL     string
	apiKey      string
	model       string
	maxTokens   int
	temperature float64
}

func newModelAgent(a armSpec, key string, maxTokens int) *modelAgent {
	return &modelAgent{
		http:      &http.Client{Timeout: 3 * time.Minute},
		baseURL:   strings.TrimRight(a.BaseURL, "/"),
		apiKey:    key,
		model:     a.APIModelID,
		maxTokens: maxTokens,
	}
}

var reBalance = regexp.MustCompile(`insufficient balance|credit`)
var reAccess = regexp.MustCompile(`deposit required|Access restricted|premium`)

// chat performs one /chat/completions call with bounded retries. Retries cover
// only 429/5xx and transport errors: retrying a protocol error the agent caused
// would measure the runner's retry policy instead of the agent.
func (m *modelAgent) next(ctx context.Context, messages []llmMessage, tools []llmTool) (*completion, error) {
	body := map[string]any{
		"model":       m.model,
		"messages":    messages,
		"max_tokens":  m.maxTokens,
		"temperature": m.temperature,
		"tools":       tools,
		"tool_choice": "auto",
	}
	raw, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}
	var lastErr error
	for attempt := 0; attempt < 4; attempt++ {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(time.Duration(attempt*attempt) * 2 * time.Second):
			}
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, m.baseURL+"/chat/completions", bytes.NewReader(raw))
		if err != nil {
			return nil, err
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+m.apiKey)
		resp, err := m.http.Do(req)
		if err != nil {
			lastErr = &infraFailure{reason: "transport: " + err.Error()}
			continue
		}
		payload, _ := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		if resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= 500 {
			lastErr = &infraFailure{reason: fmt.Sprintf("gateway status %d: %s", resp.StatusCode, truncate(string(payload), 160))}
			continue
		}
		if resp.StatusCode != http.StatusOK {
			text := string(payload)
			switch {
			case reBalance.MatchString(text):
				return nil, &infraFailure{reason: "gateway credit/balance error: " + truncate(text, 160)}
			case reAccess.MatchString(text):
				return nil, &infraFailure{reason: "model access error: " + truncate(text, 160)}
			default:
				return nil, fmt.Errorf("model error %d: %s", resp.StatusCode, truncate(text, 200))
			}
		}
		var parsed struct {
			Choices []struct {
				FinishReason string `json:"finish_reason"`
				Message      struct {
					Content          string        `json:"content"`
					ReasoningContent string        `json:"reasoning_content"`
					ToolCalls        []llmToolCall `json:"tool_calls"`
				} `json:"message"`
			} `json:"choices"`
			Usage struct {
				PromptTokens     int `json:"prompt_tokens"`
				CompletionTokens int `json:"completion_tokens"`
				ReasoningTokens  int `json:"reasoning_tokens"`
			} `json:"usage"`
			Error struct {
				Message string `json:"message"`
			} `json:"error"`
		}
		if err := json.Unmarshal(payload, &parsed); err != nil {
			return nil, &infraFailure{reason: "undecodable gateway response: " + truncate(string(payload), 160)}
		}
		if parsed.Error.Message != "" {
			if reBalance.MatchString(parsed.Error.Message) || reAccess.MatchString(parsed.Error.Message) {
				return nil, &infraFailure{reason: "gateway refused: " + truncate(parsed.Error.Message, 160)}
			}
			return nil, errors.New("gateway error: " + parsed.Error.Message)
		}
		if len(parsed.Choices) == 0 {
			return nil, &infraFailure{reason: "gateway returned no choices"}
		}
		ch := parsed.Choices[0]
		out := &completion{
			Text:   ch.Message.Content,
			Calls:  ch.Message.ToolCalls,
			Finish: ch.FinishReason,
			Prompt: parsed.Usage.PromptTokens,
			Output: parsed.Usage.CompletionTokens,
			Reason: parsed.Usage.ReasoningTokens,
		}
		if out.Reason == 0 && ch.Message.ReasoningContent != "" {
			out.Reason = len([]rune(ch.Message.ReasoningContent)) / 4
		}
		// Truncated reasoning loses the tool call itself, so this is a harness
		// configuration fault rather than agent behavior (§10 budget rule).
		if out.Finish == "length" {
			return nil, &infraFailure{reason: "finish_reason=length: reasoning budget too small"}
		}
		return out, nil
	}
	return nil, lastErr
}

// preflightProbe is the one-request check §10 requires per arm before any task
// runs: the model must be reachable and must emit a valid tool call.
func preflightProbe(ctx context.Context, a armSpec, key string) error {
	probe := newModelAgent(a, key, 2048)
	tools := []llmTool{toolFor("evidra_prescribe", "Open an operation.", json.RawMessage(`{"type":"object","properties":{"objective":{"type":"string"}},"required":["objective"]}`))}
	msgs := []llmMessage{{Role: "user", Content: "Open an operation to restart service payments. Call a tool."}}
	done := make(chan error, 1)
	go func() {
		out, err := probe.next(ctx, msgs, tools)
		switch {
		case err != nil:
			done <- err
		case len(out.Calls) == 0:
			done <- fmt.Errorf("preflight: %s returned no tool call (finish=%s)", a.APIModelID, out.Finish)
		default:
			done <- nil
		}
	}()
	select {
	case err := <-done:
		return err
	case <-time.After(3 * time.Minute):
		return errors.New("preflight timed out")
	}
}

// scriptedAgent follows the protocol correctly without a model: prescribe, run
// the task's expected tools in order, report. It exists so the runner, the
// predicates, and the artifacts can be tested at zero token cost, and so a
// protocol regression can be reproduced without a network.
type scriptedAgent struct {
	plan []plannedCall
	i    int
}

type plannedCall struct {
	Tool string
	Args map[string]any
}

func (s *scriptedAgent) next(ctx context.Context, messages []llmMessage, tools []llmTool) (*completion, error) {
	if s.i >= len(s.plan) {
		return &completion{Text: "done", Finish: "stop"}, nil
	}
	step := s.plan[s.i]
	s.i++
	call := llmToolCall{ID: fmt.Sprintf("call-%d", s.i), Type: "function"}
	call.Function.Name = step.Tool
	args, _ := json.Marshal(step.Args)
	call.Function.Arguments = string(args)
	return &completion{Calls: []llmToolCall{call}, Finish: "tool_calls", Prompt: 100, Output: 20}, nil
}

var llmNameSafe = regexp.MustCompile(`^[A-Za-z0-9_-]{1,64}$`)

// toolFor converts an MCP tool into the OpenAI function form, mapping names
// that the wire format would reject. The mapping is recorded by the caller so a
// transcript can be replayed against the real tool names.
func toolFor(name, description string, schema json.RawMessage) llmTool {
	t := llmTool{Type: "function"}
	t.Function.Name = name
	t.Function.Description = description
	if len(schema) > 0 {
		t.Function.Parameters = schema
	}
	return t
}

func sanitizeToolName(name string) string {
	if llmNameSafe.MatchString(name) {
		return name
	}
	var b strings.Builder
	for _, r := range name {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '_', r == '-':
			b.WriteRune(r)
		default:
			b.WriteRune('_')
		}
	}
	out := b.String()
	if len(out) > 64 {
		out = out[:64]
	}
	return out
}

// loadKey resolves an API key: an environment override wins, then the local
// harness credential store reference. Keys are never written into artifacts.
func loadKey(ref, envOverride string) (string, error) {
	if envOverride != "" {
		if v := os.Getenv(envOverride); v != "" {
			return v, nil
		}
	}
	path := os.Getenv("DSH_CREDENTIALS")
	if path == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		path = home + "/.dsh/.credentials.yaml"
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("credentials %s: %w", path, err)
	}
	for _, line := range strings.Split(string(raw), "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, ref+":") {
			continue
		}
		v := strings.TrimSpace(strings.TrimPrefix(line, ref+":"))
		if v == "" {
			return "", fmt.Errorf("credential %s is empty", ref)
		}
		return v, nil
	}
	return "", fmt.Errorf("credential %s not found in %s", ref, path)
}
