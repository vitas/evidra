//go:build e2e

package e2e_test

import (
	"encoding/json"
	"path/filepath"
	"testing"

	"samebits.com/evidra-benchmark/pkg/evidence"
)

func TestE2E_PrescribeWithScannerReport(t *testing.T) {
	bin := evidraBinary(t)
	tmpDir := t.TempDir()
	evidenceDir := filepath.Join(tmpDir, "evidence")
	privPath, _ := generateKeyPair(t, tmpDir)
	artifactPath := filepath.Join("..", "..", "tests", "e2e", "fixtures", "k8s_deployment.yaml")
	trivySarif := filepath.Join("..", "..", "tests", "e2e", "fixtures", "trivy.sarif")

	// Prescribe with --scanner-report bundles findings in one call
	stdout, stderr, exitCode := runEvidra(t, bin,
		"prescribe",
		"--tool", "kubectl",
		"--operation", "apply",
		"--artifact", artifactPath,
		"--environment", "staging",
		"--session-id", "e2e-scanner-prescribe",
		"--evidence-dir", evidenceDir,
		"--signing-key-path", privPath,
		"--scanner-report", trivySarif,
	)
	if exitCode != 0 {
		t.Fatalf("prescribe exit=%d stderr=%s", exitCode, stderr)
	}

	var result map[string]interface{}
	if err := json.Unmarshal([]byte(stdout), &result); err != nil {
		t.Fatalf("decode prescribe: %v\nstdout: %s", err, stdout)
	}

	if result["ok"] != true {
		t.Fatalf("prescribe not ok: %v", result)
	}

	// findings_count should reflect the 2 trivy findings
	findingsCount, ok := result["findings_count"].(float64)
	if !ok {
		t.Fatalf("findings_count missing: %v", result)
	}
	if int(findingsCount) != 2 {
		t.Errorf("findings_count = %v, want 2", findingsCount)
	}

	prescriptionID, ok := result["prescription_id"].(string)
	if !ok || prescriptionID == "" {
		t.Fatalf("prescription_id missing: %v", result)
	}

	// Report to complete the lifecycle
	_, stderr, exitCode = runEvidra(t, bin,
		"report",
		"--prescription", prescriptionID,
		"--exit-code", "0",
		"--session-id", "e2e-scanner-prescribe",
		"--evidence-dir", evidenceDir,
		"--signing-key-path", privPath,
	)
	if exitCode != 0 {
		t.Fatalf("report exit=%d stderr=%s", exitCode, stderr)
	}

	// Verify evidence chain: 1 prescribe + 2 findings + 1 report = 4 entries
	entries, err := evidence.ReadAllEntriesAtPath(evidenceDir)
	if err != nil {
		t.Fatalf("ReadAllEntriesAtPath: %v", err)
	}

	typeCounts := make(map[evidence.EntryType]int)
	for _, e := range entries {
		typeCounts[e.Type]++
	}

	if len(entries) != 4 {
		t.Errorf("total entries = %d, want 4 (got types: %v)", len(entries), typeCounts)
	}
	if typeCounts[evidence.EntryTypeFinding] != 2 {
		t.Errorf("finding entries = %d, want 2", typeCounts[evidence.EntryTypeFinding])
	}
	if typeCounts[evidence.EntryTypePrescribe] != 1 {
		t.Errorf("prescribe entries = %d, want 1", typeCounts[evidence.EntryTypePrescribe])
	}
	if typeCounts[evidence.EntryTypeReport] != 1 {
		t.Errorf("report entries = %d, want 1", typeCounts[evidence.EntryTypeReport])
	}

	// Validate chain integrity
	if err := evidence.ValidateChainAtPath(evidenceDir); err != nil {
		t.Fatalf("ValidateChainAtPath: %v", err)
	}
}
