package experiments

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestClaudeAgentRunArtifactOK(t *testing.T) {
	t.Setenv("CLAUDE_HEADLESS_MODEL", "")
	artifact := writeTempFile(t, "artifact.yaml", "apiVersion: v1\nkind: Pod\n")
	fakeBinDir := t.TempDir()
	writeExecutable(t, filepath.Join(fakeBinDir, "claude"), `#!/usr/bin/env bash
set -euo pipefail
cat <<'JSON'
{"type":"text","text":"{\"predicted_risk_level\":\"high\",\"predicted_risk_details\":[\"k8s.privileged_container\"]}"}
JSON
`)
	t.Setenv("PATH", fakeBinDir+string(os.PathListSeparator)+os.Getenv("PATH"))

	agent := &claudeAgent{}
	result, err := agent.RunArtifact(context.Background(), ArtifactAgentRequest{
		Case: ArtifactCase{
			CaseID:       "case-1",
			Category:     "kubernetes",
			Difficulty:   "low",
			ArtifactPath: artifact,
		},
		ModelID: "claude/sonnet",
		Prompt: PromptInfo{
			ContractVersion: "v1.0.1",
			SystemPrompt:    "you are assistant",
		},
	})
	if err != nil {
		t.Fatalf("RunArtifact error: %v", err)
	}
	if got := readString(result.Output, "predicted_risk_level"); got != "high" {
		t.Fatalf("predicted_risk_level=%q", got)
	}
	tags := readStringSlice(result.Output, "predicted_risk_details")
	if len(tags) != 1 || tags[0] != "k8s.privileged_container" {
		t.Fatalf("predicted_risk_details=%v", tags)
	}
	if got := readString(result.Output, "prompt_contract_version"); got != "v1.0.1" {
		t.Fatalf("prompt_contract_version=%q", got)
	}
}

func TestClaudeAgentRunArtifactMultiEvent(t *testing.T) {
	artifact := writeTempFile(t, "artifact.yaml", "apiVersion: v1\nkind: Pod\n")
	fakeBinDir := t.TempDir()
	writeExecutable(t, filepath.Join(fakeBinDir, "claude"), `#!/usr/bin/env bash
set -euo pipefail
cat <<'STREAM'
{"type":"assistant","message":{"content":[{"type":"text","text":"{\"predicted_risk_level\":\"high\",\"predicted_risk_details\":[\"k8s.hostpath_mount\"]}"}]}}
{"type":"result","result":"{\"predicted_risk_level\":\"high\",\"predicted_risk_details\":[\"k8s.hostpath_mount\"]}"}
STREAM
`)
	t.Setenv("PATH", fakeBinDir+string(os.PathListSeparator)+os.Getenv("PATH"))

	agent := &claudeAgent{}
	result, err := agent.RunArtifact(context.Background(), ArtifactAgentRequest{
		Case: ArtifactCase{
			CaseID:       "case-1",
			Category:     "kubernetes",
			Difficulty:   "low",
			ArtifactPath: artifact,
		},
		ModelID: "claude/sonnet",
		Prompt: PromptInfo{
			ContractVersion: "v1.0.1",
			SystemPrompt:    "system",
		},
	})
	if err != nil {
		t.Fatalf("RunArtifact error: %v", err)
	}
	if got := readString(result.Output, "predicted_risk_level"); got != "high" {
		t.Fatalf("predicted_risk_level=%q", got)
	}
}

func TestClaudeAgentRunArtifactParseError(t *testing.T) {
	artifact := writeTempFile(t, "artifact.yaml", "apiVersion: v1\nkind: Pod\n")
	fakeBinDir := t.TempDir()
	writeExecutable(t, filepath.Join(fakeBinDir, "claude"), `#!/usr/bin/env bash
set -euo pipefail
echo '{"type":"text","text":"not-a-json"}'
`)
	t.Setenv("PATH", fakeBinDir+string(os.PathListSeparator)+os.Getenv("PATH"))

	agent := &claudeAgent{}
	_, err := agent.RunArtifact(context.Background(), ArtifactAgentRequest{
		Case: ArtifactCase{
			CaseID:       "case-1",
			Category:     "kubernetes",
			Difficulty:   "low",
			ArtifactPath: artifact,
		},
		ModelID: "claude/haiku",
		Prompt: PromptInfo{
			ContractVersion: "v1.0.1",
			SystemPrompt:    "system",
		},
	})
	if err == nil {
		t.Fatalf("expected parse error")
	}
}

func writeTempFile(t *testing.T, name, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}
	return path
}

func writeExecutable(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o755); err != nil {
		t.Fatalf("write executable: %v", err)
	}
}
