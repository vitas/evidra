//go:build e2e

package contracts_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	testcli "samebits.com/evidra-benchmark/tests/testutil"
)

func TestE2E_RunRecordParity(t *testing.T) {
	bin := testcli.EvidraBinary(t)
	tmpDir := t.TempDir()
	privPath, _ := testcli.GenerateKeyPair(t, tmpDir)
	artifactPath := filepath.Join(tmpDir, "artifact.yaml")
	if err := os.WriteFile(artifactPath, []byte("apiVersion: v1\nkind: ConfigMap\nmetadata:\n  name: parity-e2e\n"), 0o644); err != nil {
		t.Fatalf("write artifact: %v", err)
	}

	runEvidenceDir := filepath.Join(tmpDir, "run-evidence")
	runStdout, runStderr, runCode := testcli.RunEvidra(t, bin,
		"run",
		"--tool", "kubectl",
		"--operation", "apply",
		"--artifact", artifactPath,
		"--environment", "staging",
		"--session-id", "e2e-run-session",
		"--operation-id", "e2e-op-1",
		"--evidence-dir", runEvidenceDir,
		"--signing-key-path", privPath,
		"--", "sh", "-c", "exit 0",
	)
	if runCode != 0 {
		t.Fatalf("run exit=%d stderr=%s", runCode, runStderr)
	}
	var runResult map[string]interface{}
	if err := json.Unmarshal([]byte(runStdout), &runResult); err != nil {
		t.Fatalf("decode run output: %v", err)
	}

	recordEvidenceDir := filepath.Join(tmpDir, "record-evidence")
	recordInputPath := filepath.Join(tmpDir, "record.json")
	recordInput := map[string]interface{}{
		"contract_version": "v1",
		"session_id":       "e2e-record-session",
		"operation_id":     "e2e-op-1",
		"tool":             "kubectl",
		"operation":        "apply",
		"environment":      "staging",
		"actor": map[string]interface{}{
			"type": "ci",
			"id":   "gha",
		},
		"exit_code":    0,
		"duration_ms":  1,
		"raw_artifact": "apiVersion: v1\nkind: ConfigMap\nmetadata:\n  name: parity-e2e\n",
	}
	recordBytes, err := json.Marshal(recordInput)
	if err != nil {
		t.Fatalf("marshal record input: %v", err)
	}
	if err := os.WriteFile(recordInputPath, recordBytes, 0o644); err != nil {
		t.Fatalf("write record input: %v", err)
	}

	recStdout, recStderr, recCode := testcli.RunEvidra(t, bin,
		"record",
		"--input", recordInputPath,
		"--evidence-dir", recordEvidenceDir,
		"--signing-key-path", privPath,
	)
	if recCode != 0 {
		t.Fatalf("record exit=%d stderr=%s", recCode, recStderr)
	}
	var recResult map[string]interface{}
	if err := json.Unmarshal([]byte(recStdout), &recResult); err != nil {
		t.Fatalf("decode record output: %v", err)
	}

	runSignals := toIntMap(t, runResult["signal_summary"])
	recSignals := toIntMap(t, recResult["signal_summary"])
	if !reflect.DeepEqual(runSignals, recSignals) {
		t.Fatalf("signal mismatch\nrun=%v\nrecord=%v", runSignals, recSignals)
	}
}

func toIntMap(t *testing.T, raw interface{}) map[string]int {
	t.Helper()
	m, ok := raw.(map[string]interface{})
	if !ok {
		t.Fatalf("expected object, got %#v", raw)
	}
	out := make(map[string]int, len(m))
	for k, v := range m {
		n, ok := v.(float64)
		if !ok {
			t.Fatalf("expected number for key %q, got %#v", k, v)
		}
		out[k] = int(n)
	}
	return out
}
