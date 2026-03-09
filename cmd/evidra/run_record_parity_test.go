package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"samebits.com/evidra-benchmark/internal/testutil"
)

func TestRunAndRecord_ProduceEquivalentSignalsForSameOperation(t *testing.T) {
	t.Parallel()

	signingKey := testutil.TestSigningKeyBase64(t)
	tmp := t.TempDir()
	artifactPath := filepath.Join(tmp, "artifact.yaml")
	if err := os.WriteFile(artifactPath, []byte("apiVersion: v1\nkind: ConfigMap\nmetadata:\n  name: parity-cm\n"), 0o644); err != nil {
		t.Fatalf("write artifact: %v", err)
	}

	runEvidenceDir := filepath.Join(tmp, "run-evidence")
	var runOut, runErr bytes.Buffer
	runCode := run([]string{
		"run",
		"--tool", "kubectl",
		"--operation", "apply",
		"--artifact", artifactPath,
		"--environment", "staging",
		"--session-id", "session-run-parity",
		"--operation-id", "op-parity-1",
		"--evidence-dir", runEvidenceDir,
		"--signing-key", signingKey,
		"--", "sh", "-c", "exit 0",
	}, &runOut, &runErr)
	if runCode != 0 {
		t.Fatalf("run exit=%d stderr=%s", runCode, runErr.String())
	}
	var runResult map[string]interface{}
	if err := json.Unmarshal(runOut.Bytes(), &runResult); err != nil {
		t.Fatalf("decode run output: %v", err)
	}

	recordEvidenceDir := filepath.Join(tmp, "record-evidence")
	recordInputPath := filepath.Join(tmp, "record.json")
	recordInput := map[string]interface{}{
		"contract_version": "v1",
		"session_id":       "session-record-parity",
		"operation_id":     "op-parity-1",
		"tool":             "kubectl",
		"operation":        "apply",
		"environment":      "staging",
		"actor": map[string]interface{}{
			"type": "ci",
			"id":   "pipeline-1",
		},
		"exit_code":    0,
		"duration_ms":  1,
		"raw_artifact": "apiVersion: v1\nkind: ConfigMap\nmetadata:\n  name: parity-cm\n",
	}
	recordBytes, err := json.Marshal(recordInput)
	if err != nil {
		t.Fatalf("marshal record input: %v", err)
	}
	if err := os.WriteFile(recordInputPath, recordBytes, 0o644); err != nil {
		t.Fatalf("write record input: %v", err)
	}

	var recOut, recErr bytes.Buffer
	recCode := run([]string{
		"record",
		"--input", recordInputPath,
		"--evidence-dir", recordEvidenceDir,
		"--signing-key", signingKey,
	}, &recOut, &recErr)
	if recCode != 0 {
		t.Fatalf("record exit=%d stderr=%s", recCode, recErr.String())
	}
	var recResult map[string]interface{}
	if err := json.Unmarshal(recOut.Bytes(), &recResult); err != nil {
		t.Fatalf("decode record output: %v", err)
	}

	runSignals := decodeSignalSummary(t, runResult["signal_summary"])
	recSignals := decodeSignalSummary(t, recResult["signal_summary"])
	if !reflect.DeepEqual(runSignals, recSignals) {
		t.Fatalf("signal_summary mismatch\nrun=%v\nrecord=%v", runSignals, recSignals)
	}
	if runResult["score_band"] != recResult["score_band"] {
		t.Fatalf("score_band mismatch run=%v record=%v", runResult["score_band"], recResult["score_band"])
	}
	if _, ok := runResult["risk_classification"]; ok {
		t.Fatalf("run result must not contain risk_classification: %v", runResult)
	}
	if _, ok := recResult["risk_classification"]; ok {
		t.Fatalf("record result must not contain risk_classification: %v", recResult)
	}
}

func decodeSignalSummary(t *testing.T, raw interface{}) map[string]int {
	t.Helper()

	m, ok := raw.(map[string]interface{})
	if !ok {
		t.Fatalf("signal_summary should be object, got %#v", raw)
	}
	result := make(map[string]int, len(m))
	for k, v := range m {
		n, ok := v.(float64)
		if !ok {
			t.Fatalf("signal_summary[%q] should be number, got %#v", k, v)
		}
		result[k] = int(n)
	}
	return result
}
