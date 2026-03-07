package experiments

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

type claudeAgent struct{}

func (a *claudeAgent) Name() string { return "claude" }

func (a *claudeAgent) RunArtifact(ctx context.Context, req ArtifactAgentRequest) (ArtifactAgentResult, error) {
	artifactBytes, err := os.ReadFile(req.Case.ArtifactPath)
	if err != nil {
		return ArtifactAgentResult{}, fmt.Errorf("read artifact: %w", err)
	}

	cliModel := resolveClaudeModel(req.ModelID, os.Getenv("CLAUDE_HEADLESS_MODEL"))
	userPrompt := buildArtifactUserPrompt(string(artifactBytes), req.Case)
	cmd := exec.CommandContext(
		ctx,
		"claude",
		"-p", userPrompt,
		"--output-format", "stream-json",
		"--verbose",
		"--model", cliModel,
		"--append-system-prompt", req.Prompt.SystemPrompt,
	)
	cmd.Stdin = nil
	out, err := cmd.CombinedOutput()
	stream := string(out)

	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) || errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return ArtifactAgentResult{RawStream: stream}, context.DeadlineExceeded
		}
		stderr := stream
		return ArtifactAgentResult{RawStream: stream, StderrLog: stderr}, fmt.Errorf("claude command failed: %w", err)
	}

	extracted := extractTextFromClaudeStream(stream)
	if extracted == "" {
		return ArtifactAgentResult{RawStream: stream, StderrLog: "claude-risk-agent: FAIL no parseable text events found in Claude stream output\n"}, errors.New("empty claude text stream")
	}
	parsed := extractJSONObject(extracted)
	if len(parsed) == 0 {
		return ArtifactAgentResult{RawStream: stream, StderrLog: "claude-risk-agent: FAIL could not parse JSON object from Claude stream output\n"}, errors.New("json parse failed")
	}

	outJSON := normalizeArtifactPrediction(parsed)
	outJSON["prompt_contract_version"] = req.Prompt.ContractVersion
	outJSON["model_id"] = req.ModelID
	outJSON["claude_model"] = cliModel
	return ArtifactAgentResult{
		Output:    outJSON,
		StdoutLog: stream,
		RawStream: stream,
	}, nil
}

func resolveClaudeModel(modelID, override string) string {
	if strings.TrimSpace(override) != "" {
		return override
	}
	if strings.HasPrefix(modelID, "claude/") {
		return strings.TrimPrefix(modelID, "claude/")
	}
	lower := strings.ToLower(modelID)
	switch {
	case strings.Contains(lower, "haiku"):
		return "haiku"
	case strings.Contains(lower, "sonnet"):
		return "sonnet"
	case strings.Contains(lower, "opus"):
		return "opus"
	default:
		return modelID
	}
}

func extractTextFromClaudeStream(stream string) string {
	lines := strings.Split(stream, "\n")
	parts := make([]string, 0, len(lines))
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var event map[string]any
		if err := json.Unmarshal([]byte(line), &event); err != nil {
			continue
		}
		parts = append(parts, extractClaudeEventText(event)...)
	}
	return strings.TrimSpace(strings.Join(parts, "\n"))
}

func extractClaudeEventText(event map[string]any) []string {
	eventType := readString(event, "type")
	switch eventType {
	case "text":
		text := readString(event, "text")
		if text != "" {
			return []string{text}
		}
	case "assistant":
		return extractClaudeAssistantBlocks(event)
	case "content_block_start":
		if block, ok := event["content_block"].(map[string]any); ok {
			text := readString(block, "text")
			if text != "" {
				return []string{text}
			}
		}
	case "content_block_delta":
		if delta, ok := event["delta"].(map[string]any); ok {
			text := readString(delta, "text")
			if text != "" {
				return []string{text}
			}
		}
	case "result":
		switch result := event["result"].(type) {
		case string:
			return []string{result}
		case map[string]any:
			b, _ := json.Marshal(result)
			return []string{string(b)}
		}
	}
	return nil
}

func extractClaudeAssistantBlocks(event map[string]any) []string {
	msg, ok := event["message"].(map[string]any)
	if !ok {
		return nil
	}
	content, ok := msg["content"].([]any)
	if !ok {
		return nil
	}
	out := make([]string, 0, len(content))
	for _, block := range content {
		m, ok := block.(map[string]any)
		if !ok {
			continue
		}
		if readString(m, "type") != "text" {
			continue
		}
		text := readString(m, "text")
		if text != "" {
			out = append(out, text)
		}
	}
	return out
}
