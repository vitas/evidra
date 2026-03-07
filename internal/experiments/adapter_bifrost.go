package experiments

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"
)

type bifrostAgent struct{}

var bifrostHTTPClientFactory = func(timeout time.Duration) *http.Client {
	return &http.Client{Timeout: timeout}
}

func (a *bifrostAgent) Name() string { return "bifrost" }

func (a *bifrostAgent) RunArtifact(ctx context.Context, req ArtifactAgentRequest) (ArtifactAgentResult, error) {
	artifactBytes, err := os.ReadFile(req.Case.ArtifactPath)
	if err != nil {
		return ArtifactAgentResult{}, fmt.Errorf("read artifact: %w", err)
	}

	baseURL := emptyTo(os.Getenv("EVIDRA_BIFROST_BASE_URL"), "http://localhost:8080/openai")
	model := req.ModelID
	userPrompt := buildArtifactUserPrompt(string(artifactBytes), req.Case)
	temperature := 0.0
	if req.Temperature != nil {
		temperature = *req.Temperature
	}

	payload := map[string]any{
		"model":       model,
		"temperature": temperature,
		"max_tokens":  700,
		"messages": []map[string]string{
			{"role": "system", "content": req.Prompt.SystemPrompt},
			{"role": "user", "content": userPrompt},
		},
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return ArtifactAgentResult{}, err
	}

	timeout := 120 * time.Second
	if rawTimeout := strings.TrimSpace(os.Getenv("EVIDRA_BIFROST_TIMEOUT_SECONDS")); rawTimeout != "" {
		if seconds, parseErr := strconv.ParseFloat(rawTimeout, 64); parseErr == nil && seconds > 0 {
			timeout = time.Duration(seconds * float64(time.Second))
		}
	}
	httpClient := bifrostHTTPClientFactory(timeout)

	reqURL := strings.TrimRight(baseURL, "/") + "/chat/completions"
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, reqURL, bytes.NewReader(body))
	if err != nil {
		return ArtifactAgentResult{}, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	applyBifrostHeaders(httpReq.Header)

	resp, err := httpClient.Do(httpReq)
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) || errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return ArtifactAgentResult{}, context.DeadlineExceeded
		}
		return ArtifactAgentResult{}, fmt.Errorf("bifrost request failed: %w", err)
	}
	defer resp.Body.Close()

	respBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return ArtifactAgentResult{}, err
	}
	respText := string(respBytes)
	if resp.StatusCode >= 400 {
		return ArtifactAgentResult{StderrLog: fmt.Sprintf("bifrost-risk-agent: FAIL http %d from bifrost endpoint: %s\n", resp.StatusCode, truncateText(respText, 300))}, fmt.Errorf("bifrost http %d", resp.StatusCode)
	}

	messageContent := extractBifrostMessageContent(respBytes)
	parsed := extractJSONObject(messageContent)
	outJSON := normalizeArtifactPrediction(parsed)
	outJSON["prompt_contract_version"] = req.Prompt.ContractVersion
	outJSON["model_id"] = req.ModelID
	outJSON["bifrost_base_url"] = baseURL

	return ArtifactAgentResult{
		Output:    outJSON,
		StdoutLog: respText,
	}, nil
}

func applyBifrostHeaders(header http.Header) {
	if vk := strings.TrimSpace(os.Getenv("EVIDRA_BIFROST_VK")); vk != "" {
		header.Set("x-bf-vk", vk)
	}
	if bearer := strings.TrimSpace(os.Getenv("EVIDRA_BIFROST_AUTH_BEARER")); bearer != "" {
		header.Set("Authorization", "Bearer "+bearer)
	}
	extraRaw := strings.TrimSpace(os.Getenv("EVIDRA_BIFROST_EXTRA_HEADERS_JSON"))
	if extraRaw == "" {
		return
	}
	var extra map[string]any
	if err := json.Unmarshal([]byte(extraRaw), &extra); err != nil {
		return
	}
	for k, v := range extra {
		header.Set(k, fmt.Sprintf("%v", v))
	}
}

func extractBifrostMessageContent(respBody []byte) string {
	var payload map[string]any
	if err := json.Unmarshal(respBody, &payload); err != nil {
		return ""
	}
	choices, ok := payload["choices"].([]any)
	if !ok || len(choices) == 0 {
		return ""
	}
	first, ok := choices[0].(map[string]any)
	if !ok {
		return ""
	}
	message, ok := first["message"].(map[string]any)
	if !ok {
		return ""
	}
	content := message["content"]
	switch c := content.(type) {
	case string:
		return c
	case []any:
		parts := make([]string, 0, len(c))
		for _, block := range c {
			m, ok := block.(map[string]any)
			if !ok {
				continue
			}
			text := readString(m, "text")
			if text == "" {
				text = readString(m, "content")
			}
			if text != "" {
				parts = append(parts, text)
			}
		}
		return strings.Join(parts, "\n")
	default:
		return ""
	}
}

func truncateText(v string, limit int) string {
	if len(v) <= limit {
		return v
	}
	return v[:limit]
}
