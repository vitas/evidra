package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
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
	if !strings.Contains(string(b), "\"pass\":") {
		t.Fatalf("summary missing pass field: %s", string(b))
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
	if !strings.Contains(string(b), "\"pass\":") {
		t.Fatalf("summary missing pass field: %s", string(b))
	}
}

func TestParseArtifactFlagsDelayBetweenRuns(t *testing.T) {
	var errBuf bytes.Buffer
	opts, code := parseArtifactFlags([]string{
		"--model-id", "test/model",
		"--agent", "dry-run",
		"--delay-between-runs", "250ms",
	}, &errBuf)
	if code != 0 {
		t.Fatalf("code=%d stderr=%s", code, errBuf.String())
	}
	if opts.DelayBetweenRuns != 250*time.Millisecond {
		t.Fatalf("DelayBetweenRuns=%s", opts.DelayBetweenRuns)
	}
}

func TestParseExecutionFlagsDelayBetweenRuns(t *testing.T) {
	var errBuf bytes.Buffer
	opts, code := parseExecutionFlags([]string{
		"--model-id", "test/model",
		"--agent", "dry-run",
		"--delay-between-runs", "1s",
	}, &errBuf)
	if code != 0 {
		t.Fatalf("code=%d stderr=%s", code, errBuf.String())
	}
	if opts.DelayBetweenRuns != time.Second {
		t.Fatalf("DelayBetweenRuns=%s", opts.DelayBetweenRuns)
	}
}

func TestParseArtifactFlagsInvalidDelay(t *testing.T) {
	var errBuf bytes.Buffer
	_, code := parseArtifactFlags([]string{
		"--model-id", "test/model",
		"--agent", "dry-run",
		"--delay-between-runs", "oops",
	}, &errBuf)
	if code != 2 {
		t.Fatalf("code=%d stderr=%s", code, errBuf.String())
	}
}

func TestArtifactHelpIncludesExtendedFlags(t *testing.T) {
	var out, errBuf bytes.Buffer
	code := run([]string{"artifact", "--help"}, &out, &errBuf)
	if code != 0 {
		t.Fatalf("code=%d stderr=%s", code, errBuf.String())
	}
	helpText := out.String()
	for _, needle := range []string{
		"--provider <name>",
		"--prompt-version <label>",
		"--prompt-file <path>",
		"--temperature <float>",
		"--mode <name>",
		"--timeout-seconds <n>",
		"--case-filter <regex>",
		"--max-cases <n>",
	} {
		if !strings.Contains(helpText, needle) {
			t.Fatalf("help missing %q: %s", needle, helpText)
		}
	}
}

func TestExecutionHelpIncludesExtendedFlags(t *testing.T) {
	var out, errBuf bytes.Buffer
	code := run([]string{"execution", "--help"}, &out, &errBuf)
	if code != 0 {
		t.Fatalf("code=%d stderr=%s", code, errBuf.String())
	}
	helpText := out.String()
	for _, needle := range []string{
		"--provider <name>",
		"--prompt-version <label>",
		"--prompt-file <path>",
		"--mode <name>",
		"--timeout-seconds <n>",
		"--scenario-filter <regex>",
		"--max-scenarios <n>",
	} {
		if !strings.Contains(helpText, needle) {
			t.Fatalf("help missing %q: %s", needle, helpText)
		}
	}
}

func TestArtifactBaselineDryRunWritesAggregateSummary(t *testing.T) {
	root := repoRoot(t)
	outDir := filepath.Join(t.TempDir(), "llm-baseline")

	var out, errBuf bytes.Buffer
	code := run([]string{
		"artifact", "baseline",
		"--model-ids", "test/model-a,test/model-b",
		"--provider", "test",
		"--agent", "dry-run",
		"--prompt-file", filepath.Join(root, "prompts/experiments/runtime/system_instructions.txt"),
		"--cases-dir", filepath.Join(root, "tests/benchmark/cases"),
		"--max-cases", "1",
		"--repeats", "1",
		"--out-dir", outDir,
	}, &out, &errBuf)
	if code != 0 {
		t.Fatalf("exit code = %d, stderr=%s", code, errBuf.String())
	}

	summaryPath := filepath.Join(outDir, "summary.json")
	b, err := os.ReadFile(summaryPath)
	if err != nil {
		t.Fatalf("read summary.json: %v", err)
	}

	type baselineSummary struct {
		SchemaVersion string `json:"schema_version"`
		ModelCount    int    `json:"model_count"`
		Models        []struct {
			ModelID      string `json:"model_id"`
			SummaryJSONL string `json:"summary_jsonl"`
		} `json:"models"`
	}
	var got baselineSummary
	if err := json.Unmarshal(b, &got); err != nil {
		t.Fatalf("decode summary.json: %v", err)
	}
	if got.SchemaVersion != "evidra.llm-baseline.v1" {
		t.Fatalf("schema_version=%q", got.SchemaVersion)
	}
	if got.ModelCount != 2 || len(got.Models) != 2 {
		t.Fatalf("model_count=%d len(models)=%d", got.ModelCount, len(got.Models))
	}
	for _, m := range got.Models {
		if strings.TrimSpace(m.ModelID) == "" {
			t.Fatalf("empty model id in summary")
		}
		if _, err := os.Stat(m.SummaryJSONL); err != nil {
			t.Fatalf("model summary jsonl missing: %s (%v)", m.SummaryJSONL, err)
		}
	}
}

func TestArtifactBaselineMissingModelIDs(t *testing.T) {
	root := repoRoot(t)
	var out, errBuf bytes.Buffer
	code := run([]string{
		"artifact", "baseline",
		"--provider", "test",
		"--agent", "dry-run",
		"--prompt-file", filepath.Join(root, "prompts/experiments/runtime/system_instructions.txt"),
		"--cases-dir", filepath.Join(root, "tests/benchmark/cases"),
	}, &out, &errBuf)
	if code != 2 {
		t.Fatalf("exit code=%d stderr=%s", code, errBuf.String())
	}
}

func TestArtifactHelpIncludesBaselineOptions(t *testing.T) {
	var out, errBuf bytes.Buffer
	code := run([]string{"artifact", "--help"}, &out, &errBuf)
	if code != 0 {
		t.Fatalf("code=%d stderr=%s", code, errBuf.String())
	}
	helpText := out.String()
	for _, needle := range []string{
		"artifact <run|baseline>",
		"--model-ids <csv>",
		"experiments/results/llm/<timestamp>",
	} {
		if !strings.Contains(helpText, needle) {
			t.Fatalf("help missing %q: %s", needle, helpText)
		}
	}
}
