package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"samebits.com/evidra-benchmark/internal/testutil"
	"samebits.com/evidra-benchmark/pkg/evidence"
)

func TestRecordCommandWritesPrescribeAndReport(t *testing.T) {
	t.Parallel()

	signingKey := testutil.TestSigningKeyBase64(t)
	tmp := t.TempDir()
	evidenceDir := filepath.Join(tmp, "evidence")
	inputPath := filepath.Join(tmp, "record.json")

	recordInput := map[string]interface{}{
		"contract_version": "v1",
		"session_id":       "sess-record-1",
		"operation_id":     "op-record-1",
		"tool":             "kubectl",
		"operation":        "apply",
		"environment":      "staging",
		"actor": map[string]interface{}{
			"type":       "ci",
			"id":         "pipeline-1",
			"provenance": "github",
		},
		"exit_code":   0,
		"duration_ms": 1200,
		"raw_artifact": "apiVersion: v1\nkind: ConfigMap\nmetadata:\n" +
			"  name: rec-cm\n  namespace: default\n",
	}

	data, err := json.Marshal(recordInput)
	if err != nil {
		t.Fatalf("marshal input: %v", err)
	}
	if err := os.WriteFile(inputPath, data, 0o644); err != nil {
		t.Fatalf("write input: %v", err)
	}

	var out, errBuf bytes.Buffer
	code := run([]string{
		"record",
		"--input", inputPath,
		"--evidence-dir", evidenceDir,
		"--signing-key", signingKey,
	}, &out, &errBuf)
	if code != 0 {
		t.Fatalf("record exit=%d stderr=%s", code, errBuf.String())
	}

	var result map[string]interface{}
	if err := json.Unmarshal(out.Bytes(), &result); err != nil {
		t.Fatalf("decode output: %v", err)
	}
	if result["ok"] != true {
		t.Fatalf("ok=%v want true", result["ok"])
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

func TestRecordCommandRejectsInvalidPayload(t *testing.T) {
	t.Parallel()

	signingKey := testutil.TestSigningKeyBase64(t)
	tmp := t.TempDir()
	evidenceDir := filepath.Join(tmp, "evidence")
	inputPath := filepath.Join(tmp, "record-invalid.json")

	invalidInput := map[string]interface{}{
		"contract_version": "v1",
		"session_id":       "sess-record-2",
		"operation_id":     "op-record-2",
		// missing tool/operation/environment/actor
		"exit_code":   1,
		"duration_ms": 2200,
		"raw_artifact": "apiVersion: v1\nkind: ConfigMap\nmetadata:\n" +
			"  name: rec-cm-2\n  namespace: default\n",
	}

	data, err := json.Marshal(invalidInput)
	if err != nil {
		t.Fatalf("marshal input: %v", err)
	}
	if err := os.WriteFile(inputPath, data, 0o644); err != nil {
		t.Fatalf("write input: %v", err)
	}

	var out, errBuf bytes.Buffer
	code := run([]string{
		"record",
		"--input", inputPath,
		"--evidence-dir", evidenceDir,
		"--signing-key", signingKey,
	}, &out, &errBuf)
	if code != 2 {
		t.Fatalf("record invalid payload exit=%d want 2 (stderr=%s)", code, errBuf.String())
	}
	if !strings.Contains(errBuf.String(), "validation") {
		t.Fatalf("stderr should mention validation, got: %s", errBuf.String())
	}
}
