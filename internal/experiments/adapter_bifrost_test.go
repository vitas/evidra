package experiments

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestBifrostAgentRunArtifactOK(t *testing.T) {
	t.Setenv("EVIDRA_BIFROST_BASE_URL", "http://bifrost.test/openai")
	var gotBody map[string]any
	oldFactory := bifrostHTTPClientFactory
	t.Cleanup(func() { bifrostHTTPClientFactory = oldFactory })
	bifrostHTTPClientFactory = func(_ time.Duration) *http.Client {
		return &http.Client{
			Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
				if req.URL.Path != "/openai/chat/completions" {
					t.Fatalf("path=%s", req.URL.Path)
				}
				raw, err := io.ReadAll(req.Body)
				if err != nil {
					t.Fatalf("read request body: %v", err)
				}
				if err := json.Unmarshal(raw, &gotBody); err != nil {
					t.Fatalf("decode request: %v", err)
				}
				body := `{"choices":[{"message":{"content":"{\"predicted_risk_level\":\"critical\",\"predicted_risk_details\":[\"k8s.privileged_container\"]}"}}]}`
				return &http.Response{
					StatusCode: 200,
					Body:       io.NopCloser(strings.NewReader(body)),
					Header:     make(http.Header),
				}, nil
			}),
		}
	}
	artifact := writeTempFile(t, "artifact.yaml", "apiVersion: v1\nkind: Pod\n")
	agent := &bifrostAgent{}

	result, err := agent.RunArtifact(context.Background(), ArtifactAgentRequest{
		Case: ArtifactCase{
			CaseID:       "case-1",
			Category:     "kubernetes",
			Difficulty:   "low",
			ArtifactPath: artifact,
		},
		ModelID: "anthropic/claude-3-5-haiku",
		Prompt: PromptInfo{
			ContractVersion: "v1.0.1",
			SystemPrompt:    "system",
		},
	})
	if err != nil {
		t.Fatalf("RunArtifact error: %v", err)
	}
	if readString(result.Output, "predicted_risk_level") != "critical" {
		t.Fatalf("unexpected risk level: %v", result.Output)
	}
	if len(readStringSlice(result.Output, "predicted_risk_details")) != 1 {
		t.Fatalf("unexpected risk tags: %v", result.Output)
	}
	if gotModel := readString(gotBody, "model"); gotModel != "anthropic/claude-3-5-haiku" {
		t.Fatalf("request model=%q", gotModel)
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}
