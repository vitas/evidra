package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"samebits.com/evidra-benchmark/internal/testutil"
	"samebits.com/evidra-benchmark/pkg/evidence"
)

func TestRunCommandExecutesCommandAndReportsOutcome(t *testing.T) {
	t.Parallel()

	signingKey := testutil.TestSigningKeyBase64(t)
	tmp := t.TempDir()
	evidenceDir := filepath.Join(tmp, "evidence")
	artifactPath := filepath.Join(tmp, "artifact.yaml")
	if err := os.WriteFile(artifactPath, []byte("apiVersion: v1\nkind: ConfigMap\nmetadata:\n  name: run-cm\n"), 0o644); err != nil {
		t.Fatalf("write artifact: %v", err)
	}

	var out, errBuf bytes.Buffer
	code := run([]string{
		"run",
		"--tool", "kubectl",
		"--operation", "apply",
		"--artifact", artifactPath,
		"--environment", "staging",
		"--evidence-dir", evidenceDir,
		"--signing-key", signingKey,
		"--", "sh", "-c", "exit 0",
	}, &out, &errBuf)
	if code != 0 {
		t.Fatalf("run exit=%d stderr=%s", code, errBuf.String())
	}

	var result map[string]interface{}
	if err := json.Unmarshal(out.Bytes(), &result); err != nil {
		t.Fatalf("decode output: %v", err)
	}
	if got := int(result["exit_code"].(float64)); got != 0 {
		t.Fatalf("result exit_code=%d want 0", got)
	}
	for _, key := range []string{"session_id", "operation_id", "prescription_id", "report_id"} {
		if _, ok := result[key]; !ok {
			t.Fatalf("missing key %q in result: %#v", key, result)
		}
	}

	entries, err := evidence.ReadAllEntriesAtPath(evidenceDir)
	if err != nil {
		t.Fatalf("ReadAllEntriesAtPath: %v", err)
	}
	prescribes := 0
	reports := 0
	for _, e := range entries {
		switch e.Type {
		case evidence.EntryTypePrescribe:
			prescribes++
		case evidence.EntryTypeReport:
			reports++
		}
	}
	if prescribes != 1 || reports != 1 {
		t.Fatalf("entries prescribe=%d report=%d want 1/1", prescribes, reports)
	}
}

func TestRunOutput_ContainsFirstUsefulOutputFields(t *testing.T) {
	t.Parallel()

	signingKey := testutil.TestSigningKeyBase64(t)
	tmp := t.TempDir()
	evidenceDir := filepath.Join(tmp, "evidence")
	artifactPath := filepath.Join(tmp, "artifact.yaml")
	if err := os.WriteFile(artifactPath, []byte("apiVersion: v1\nkind: ConfigMap\nmetadata:\n  name: run-first-use\n"), 0o644); err != nil {
		t.Fatalf("write artifact: %v", err)
	}

	var out, errBuf bytes.Buffer
	code := run([]string{
		"run",
		"--tool", "kubectl",
		"--operation", "apply",
		"--artifact", artifactPath,
		"--environment", "staging",
		"--evidence-dir", evidenceDir,
		"--signing-key", signingKey,
		"--", "sh", "-c", "exit 0",
	}, &out, &errBuf)
	if code != 0 {
		t.Fatalf("run exit=%d stderr=%s", code, errBuf.String())
	}

	var result map[string]interface{}
	if err := json.Unmarshal(out.Bytes(), &result); err != nil {
		t.Fatalf("decode output: %v", err)
	}

	for _, key := range []string{
		"risk_classification",
		"risk_level",
		"score",
		"score_band",
		"signal_summary",
		"basis",
		"confidence",
	} {
		if _, ok := result[key]; !ok {
			t.Fatalf("missing first-use field %q in output: %#v", key, result)
		}
	}

	basis, ok := result["basis"].(map[string]interface{})
	if !ok {
		t.Fatalf("basis should be object, got: %#v", result["basis"])
	}
	if _, ok := basis["assessment_mode"]; !ok {
		t.Fatalf("basis missing assessment_mode: %#v", basis)
	}
}

func TestRunCommandFailOpenOnMetricsExportError(t *testing.T) {
	t.Parallel()

	signingKey := testutil.TestSigningKeyBase64(t)
	tmp := t.TempDir()
	evidenceDir := filepath.Join(tmp, "evidence")
	artifactPath := filepath.Join(tmp, "artifact.yaml")
	if err := os.WriteFile(artifactPath, []byte("apiVersion: v1\nkind: ConfigMap\nmetadata:\n  name: run-cm-2\n"), 0o644); err != nil {
		t.Fatalf("write artifact: %v", err)
	}

	originalHook := emitRunMetricsHook
	emitRunMetricsHook = func(context.Context, runMetricsPayload) error {
		return errors.New("metrics unavailable")
	}
	defer func() {
		emitRunMetricsHook = originalHook
	}()

	var out, errBuf bytes.Buffer
	code := run([]string{
		"run",
		"--tool", "kubectl",
		"--operation", "apply",
		"--artifact", artifactPath,
		"--environment", "staging",
		"--evidence-dir", evidenceDir,
		"--signing-key", signingKey,
		"--", "sh", "-c", "exit 7",
	}, &out, &errBuf)
	if code != 7 {
		t.Fatalf("run exit=%d want 7 (stderr=%s)", code, errBuf.String())
	}
	if !strings.Contains(errBuf.String(), "metrics export failed") {
		t.Fatalf("stderr should contain metrics warning, got: %s", errBuf.String())
	}

	var result map[string]interface{}
	if err := json.Unmarshal(out.Bytes(), &result); err != nil {
		t.Fatalf("decode output: %v", err)
	}
	if got := int(result["exit_code"].(float64)); got != 7 {
		t.Fatalf("result exit_code=%d want 7", got)
	}
}
