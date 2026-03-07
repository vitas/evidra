package main

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func repoRoot(t *testing.T) string {
	t.Helper()
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatalf("resolve repo root: %v", err)
	}
	return root
}

func TestRunHelp(t *testing.T) {
	t.Parallel()

	var out, errBuf bytes.Buffer
	code := run([]string{"--help"}, &out, &errBuf)
	if code != 0 {
		t.Fatalf("exit code = %d, stderr=%s", code, errBuf.String())
	}
	if !strings.Contains(out.String(), "evidra-exp <command>") {
		t.Fatalf("help missing header: %s", out.String())
	}
}

func TestArtifactRunDryRunCleansOutDir(t *testing.T) {
	root := repoRoot(t)
	outDir := filepath.Join(t.TempDir(), "artifact-out")
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		t.Fatalf("mkdir out dir: %v", err)
	}
	stale := filepath.Join(outDir, "stale.txt")
	if err := os.WriteFile(stale, []byte("stale"), 0o644); err != nil {
		t.Fatalf("write stale: %v", err)
	}

	var out, errBuf bytes.Buffer
	code := run([]string{
		"artifact", "run",
		"--model-id", "test/model",
		"--provider", "test",
		"--agent", "dry-run",
		"--prompt-file", filepath.Join(root, "prompts/experiments/runtime/system_instructions.txt"),
		"--cases-dir", filepath.Join(root, "tests/benchmark/cases"),
		"--max-cases", "1",
		"--repeats", "1",
		"--out-dir", outDir,
		"--clean-out-dir",
	}, &out, &errBuf)
	if code != 0 {
		t.Fatalf("exit code = %d, stderr=%s", code, errBuf.String())
	}
	if _, err := os.Stat(stale); err == nil {
		t.Fatalf("stale file still exists: %s", stale)
	}
	summary := filepath.Join(outDir, "summary.jsonl")
	b, err := os.ReadFile(summary)
	if err != nil {
		t.Fatalf("read summary: %v", err)
	}
	if !strings.Contains(string(b), "\"status\":\"dry_run\"") {
		t.Fatalf("summary missing dry_run status: %s", string(b))
	}
}

func TestExecutionRunDryRun(t *testing.T) {
	root := repoRoot(t)
	outDir := filepath.Join(t.TempDir(), "exec-out")
	scenariosDir := filepath.Join(t.TempDir(), "scenarios")
	if err := os.MkdirAll(scenariosDir, 0o755); err != nil {
		t.Fatalf("mkdir scenarios: %v", err)
	}
	scenarioPath := filepath.Join(scenariosDir, "one.json")
	artifactPath := filepath.Join(root, "tests/benchmark/cases/k8s-hostpath-mount-fail/artifacts/input.json")
	scenarioJSON := fmt.Sprintf(`{
  "scenario_id": "local-one",
  "category": "kubernetes",
  "difficulty": "low",
  "tool": "kubectl",
  "operation": "apply",
  "artifact_path": %q,
  "execute_cmd": "true",
  "expected_exit_code": 0,
  "expected_risk_level": "low",
  "expected_risk_tags": []
}`, artifactPath)
	if err := os.WriteFile(scenarioPath, []byte(scenarioJSON), 0o644); err != nil {
		t.Fatalf("write scenario: %v", err)
	}

	var out, errBuf bytes.Buffer
	code := run([]string{
		"execution", "run",
		"--model-id", "test/model",
		"--provider", "test",
		"--agent", "dry-run",
		"--prompt-file", filepath.Join(root, "prompts/experiments/runtime/system_instructions.txt"),
		"--scenarios-dir", scenariosDir,
		"--max-scenarios", "1",
		"--repeats", "1",
		"--out-dir", outDir,
	}, &out, &errBuf)
	if code != 0 {
		t.Fatalf("exit code = %d, stderr=%s", code, errBuf.String())
	}
	summary := filepath.Join(outDir, "summary.jsonl")
	b, err := os.ReadFile(summary)
	if err != nil {
		t.Fatalf("read summary: %v", err)
	}
	if !strings.Contains(string(b), "\"status\":\"dry_run\"") {
		t.Fatalf("summary missing dry_run status: %s", string(b))
	}
}
