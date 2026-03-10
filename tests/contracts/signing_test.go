//go:build e2e

package contracts_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"samebits.com/evidra-benchmark/pkg/evidence"
	testcli "samebits.com/evidra-benchmark/tests/testutil"
)

func TestE2E_SigningEndToEnd(t *testing.T) {
	bin := testcli.EvidraBinary(t)
	tmpDir := t.TempDir()
	evidenceDir := filepath.Join(tmpDir, "evidence")
	privPath, pubPath := testcli.GenerateKeyPair(t, tmpDir)
	artifactPath := filepath.Join("..", "..", "tests", "contracts", "fixtures", "k8s_deployment.yaml")

	// Step 1: Prescribe with signing key.
	stdout, stderr, exitCode := testcli.RunEvidra(t, bin,
		"prescribe",
		"--tool", "kubectl",
		"--operation", "apply",
		"--artifact", artifactPath,
		"--session-id", "e2e-signing-001",
		"--signing-key-path", privPath,
		"--evidence-dir", evidenceDir,
	)
	if exitCode != 0 {
		t.Fatalf("prescribe exit code = %d, stderr = %s", exitCode, stderr)
	}

	// Parse JSON output to get prescription_id.
	var prescribeResult map[string]interface{}
	if err := json.Unmarshal([]byte(stdout), &prescribeResult); err != nil {
		t.Fatalf("decode prescribe output: %v\nstdout: %s", err, stdout)
	}
	if prescribeResult["ok"] != true {
		t.Fatalf("prescribe result not ok: %v", prescribeResult)
	}
	prescriptionID, ok := prescribeResult["prescription_id"].(string)
	if !ok || prescriptionID == "" {
		t.Fatalf("prescription_id missing or empty in output: %v", prescribeResult)
	}

	// Step 2: Report with signing key.
	stdout, stderr, exitCode = testcli.RunEvidra(t, bin,
		"report",
		"--prescription", prescriptionID,
		"--verdict", "success",
		"--exit-code", "0",
		"--session-id", "e2e-signing-001",
		"--signing-key-path", privPath,
		"--evidence-dir", evidenceDir,
	)
	if exitCode != 0 {
		t.Fatalf("report exit code = %d, stderr = %s", exitCode, stderr)
	}

	// Step 3: Validate with public key — expect success.
	stdout, stderr, exitCode = testcli.RunEvidra(t, bin,
		"validate",
		"--public-key", pubPath,
		"--evidence-dir", evidenceDir,
	)
	if exitCode != 0 {
		t.Fatalf("validate exit code = %d, stderr = %s", exitCode, stderr)
	}
	if !strings.Contains(stdout, "signatures verified") {
		t.Fatalf("validate output missing 'signatures verified': %s", stdout)
	}

	// Step 4: Verify all entries have non-empty Signature and correct SessionID.
	entries, err := evidence.ReadAllEntriesAtPath(evidenceDir)
	if err != nil {
		t.Fatalf("ReadAllEntriesAtPath: %v", err)
	}
	if len(entries) == 0 {
		t.Fatal("no evidence entries found")
	}
	for i, e := range entries {
		if e.Signature == "" {
			t.Errorf("entry %d (%s) has empty Signature", i, e.Type)
		}
		if e.SessionID != "e2e-signing-001" {
			t.Errorf("entry %d (%s) SessionID = %q, want %q", i, e.Type, e.SessionID, "e2e-signing-001")
		}
	}

	// Step 5: Tamper detection — modify 1 byte in the segment file, then validate should fail.
	segmentFiles, err := filepath.Glob(filepath.Join(evidenceDir, "segments", "*.jsonl"))
	if err != nil {
		t.Fatalf("glob segment files: %v", err)
	}
	if len(segmentFiles) == 0 {
		t.Fatal("no segment files found for tamper test")
	}

	segData, err := os.ReadFile(segmentFiles[0])
	if err != nil {
		t.Fatalf("read segment file: %v", err)
	}
	// Flip one byte in the middle of the file.
	mid := len(segData) / 2
	segData[mid] ^= 0xFF
	if err := os.WriteFile(segmentFiles[0], segData, 0o644); err != nil {
		t.Fatalf("write tampered segment file: %v", err)
	}

	_, stderr, exitCode = testcli.RunEvidra(t, bin,
		"validate",
		"--public-key", pubPath,
		"--evidence-dir", evidenceDir,
	)
	if exitCode == 0 {
		t.Fatal("validate should have failed after tampering, but exit code was 0")
	}
}
