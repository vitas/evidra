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

func TestE2E_RecordImportParity(t *testing.T) {
	bin := testcli.EvidraBinary(t)
	tmpDir := t.TempDir()
	privPath, _ := testcli.GenerateKeyPair(t, tmpDir)
	artifactPath := filepath.Join(tmpDir, "artifact.yaml")
	if err := os.WriteFile(artifactPath, []byte("apiVersion: v1\nkind: ConfigMap\nmetadata:\n  name: parity-e2e\n"), 0o644); err != nil {
		t.Fatalf("write artifact: %v", err)
	}

	recordEvidenceDir := filepath.Join(tmpDir, "record-evidence")
	recordStdout, recordStderr, recordCode := testcli.RunEvidra(t, bin,
		"record",
		"--tool", "kubectl",
		"--operation", "apply",
		"--artifact", artifactPath,
		"--environment", "staging",
		"--session-id", "e2e-run-session",
		"--operation-id", "e2e-op-1",
		"--evidence-dir", recordEvidenceDir,
		"--signing-key-path", privPath,
		"--", "sh", "-c", "exit 0",
	)
	if recordCode != 0 {
		t.Fatalf("record exit=%d stderr=%s", recordCode, recordStderr)
	}
	var recordResult map[string]interface{}
	if err := json.Unmarshal([]byte(recordStdout), &recordResult); err != nil {
		t.Fatalf("decode record output: %v", err)
	}

	importEvidenceDir := filepath.Join(tmpDir, "import-evidence")
	importInputPath := filepath.Join(tmpDir, "record.json")
	importInput := map[string]interface{}{
		"contract_version": "v1",
		"session_id":       "e2e-import-session",
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
	recordBytes, err := json.Marshal(importInput)
	if err != nil {
		t.Fatalf("marshal record input: %v", err)
	}
	if err := os.WriteFile(importInputPath, recordBytes, 0o644); err != nil {
		t.Fatalf("write record input: %v", err)
	}

	importStdout, importStderr, importCode := testcli.RunEvidra(t, bin,
		"import",
		"--input", importInputPath,
		"--evidence-dir", importEvidenceDir,
		"--signing-key-path", privPath,
	)
	if importCode != 0 {
		t.Fatalf("import exit=%d stderr=%s", importCode, importStderr)
	}
	var importResult map[string]interface{}
	if err := json.Unmarshal([]byte(importStdout), &importResult); err != nil {
		t.Fatalf("decode import output: %v", err)
	}

	recordSignals := toIntMap(t, recordResult["signal_summary"])
	importSignals := toIntMap(t, importResult["signal_summary"])
	if !reflect.DeepEqual(recordSignals, importSignals) {
		t.Fatalf("signal mismatch\nrecord=%v\nimport=%v", recordSignals, importSignals)
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
