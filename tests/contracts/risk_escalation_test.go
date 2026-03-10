//go:build e2e

package contracts_test

import (
	"encoding/json"
	"path/filepath"
	"testing"

	testcli "samebits.com/evidra-benchmark/tests/testutil"
)

func TestE2E_RiskEscalationSignal(t *testing.T) {
	bin := testcli.EvidraBinary(t)
	tmpDir := t.TempDir()
	evidenceDir := filepath.Join(tmpDir, "evidence")
	privPath, _ := testcli.GenerateKeyPair(t, tmpDir)
	artifactPath := filepath.Join("..", "..", "tests", "contracts", "fixtures", "k8s_deployment.yaml")

	// --- 3 medium-risk prescriptions (kubectl apply, staging) establish baseline ---
	for i := 0; i < 3; i++ {
		stdout, stderr, exitCode := testcli.RunEvidra(t, bin,
			"prescribe",
			"--tool", "kubectl",
			"--operation", "apply",
			"--artifact", artifactPath,
			"--environment", "staging",
			"--actor", "agent-escalation",
			"--evidence-dir", evidenceDir,
			"--signing-key-path", privPath,
		)
		if exitCode != 0 {
			t.Fatalf("prescribe[%d] failed: exit=%d stderr=%s", i, exitCode, stderr)
		}

		var prescribe map[string]interface{}
		if err := json.Unmarshal([]byte(stdout), &prescribe); err != nil {
			t.Fatalf("decode prescribe[%d]: %v\nstdout: %s", i, err, stdout)
		}
		prescriptionID := prescribe["prescription_id"].(string)

		_, stderr, exitCode = testcli.RunEvidra(t, bin,
			"report",
			"--prescription", prescriptionID,
			"--verdict", "success",
			"--exit-code", "0",
			"--evidence-dir", evidenceDir,
			"--signing-key-path", privPath,
		)
		if exitCode != 0 {
			t.Fatalf("report[%d] failed: exit=%d stderr=%s", i, exitCode, stderr)
		}
	}

	// --- 1 high-risk prescription (kubectl apply, production) should trigger escalation ---
	stdout, stderr, exitCode := testcli.RunEvidra(t, bin,
		"prescribe",
		"--tool", "kubectl",
		"--operation", "apply",
		"--artifact", artifactPath,
		"--environment", "production",
		"--actor", "agent-escalation",
		"--evidence-dir", evidenceDir,
		"--signing-key-path", privPath,
	)
	if exitCode != 0 {
		t.Fatalf("prescribe[production] failed: exit=%d stderr=%s", exitCode, stderr)
	}

	var prescribeProd map[string]interface{}
	if err := json.Unmarshal([]byte(stdout), &prescribeProd); err != nil {
		t.Fatalf("decode prescribe[production]: %v\nstdout: %s", err, stdout)
	}
	prescriptionIDProd := prescribeProd["prescription_id"].(string)

	_, stderr, exitCode = testcli.RunEvidra(t, bin,
		"report",
		"--prescription", prescriptionIDProd,
		"--verdict", "success",
		"--exit-code", "0",
		"--evidence-dir", evidenceDir,
		"--signing-key-path", privPath,
	)
	if exitCode != 0 {
		t.Fatalf("report[production] failed: exit=%d stderr=%s", exitCode, stderr)
	}

	// --- Scorecard should show risk_escalation >= 1 ---
	stdout, stderr, exitCode = testcli.RunEvidra(t, bin,
		"scorecard",
		"--evidence-dir", evidenceDir,
	)
	if exitCode != 0 {
		t.Fatalf("scorecard failed: exit=%d stderr=%s", exitCode, stderr)
	}

	var scorecard map[string]interface{}
	if err := json.Unmarshal([]byte(stdout), &scorecard); err != nil {
		t.Fatalf("decode scorecard: %v\nstdout: %s", err, stdout)
	}

	signals, ok := scorecard["signals"].(map[string]interface{})
	if !ok {
		t.Fatalf("scorecard missing 'signals' field: %v", scorecard)
	}

	escalationCount, ok := signals["risk_escalation"].(float64)
	if !ok {
		t.Fatalf("scorecard missing risk_escalation signal: signals=%v", signals)
	}

	if escalationCount < 1 {
		t.Errorf("risk_escalation count = %.0f, want >= 1 (agent moved from staging to production)", escalationCount)
	}

	t.Logf("scorecard: risk_escalation=%.0f total_operations=%.0f score=%.2f",
		escalationCount,
		scorecard["total_operations"].(float64),
		scorecard["score"].(float64),
	)
}
