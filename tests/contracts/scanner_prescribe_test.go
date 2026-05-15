//go:build e2e

package contracts_test

import (
	"encoding/json"
	"path/filepath"
	"testing"

	"samebits.com/evidra/pkg/evidence"
	testcli "samebits.com/evidra/tests/testutil"
)

func TestE2E_PrescribeWithFindings(t *testing.T) {
	bin := testcli.EvidraBinary(t)
	tmpDir := t.TempDir()
	evidenceDir := filepath.Join(tmpDir, "evidence")
	privPath, _ := testcli.GenerateKeyPair(t, tmpDir)
	artifactPath := filepath.Join("..", "..", "tests", "contracts", "fixtures", "k8s_deployment.yaml")
	trivySarif := filepath.Join("..", "..", "tests", "contracts", "fixtures", "trivy.sarif")

	// Prescribe with --findings bundles findings in one call.
	stdout, stderr, exitCode := testcli.RunEvidra(t, bin,
		"prescribe",
		"--tool", "kubectl",
		"--operation", "apply",
		"--artifact", artifactPath,
		"--environment", "staging",
		"--session-id", "e2e-scanner-prescribe",
		"--evidence-dir", evidenceDir,
		"--signing-key-path", privPath,
		"--findings", trivySarif,
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

	if riskInputs, ok := result["risk_inputs"].([]interface{}); ok && len(riskInputs) != 0 {
		t.Fatalf("risk_inputs len = %d, want 0 without assessment", len(riskInputs))
	}
	if effectiveRisk, ok := result["effective_risk"].(string); ok && effectiveRisk != "" {
		t.Errorf("effective_risk = %q, want empty without assessment", effectiveRisk)
	}

	prescriptionID, ok := result["prescription_id"].(string)
	if !ok || prescriptionID == "" {
		t.Fatalf("prescription_id missing: %v", result)
	}

	// Report to complete the lifecycle
	_, stderr, exitCode = testcli.RunEvidra(t, bin,
		"report",
		"--prescription", prescriptionID,
		"--verdict", "success",
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
