//go:build e2e

package e2e_test

import (
	"encoding/json"
	"path/filepath"
	"testing"
)

func TestE2E_TerraformRunLifecycle(t *testing.T) {
	bin := evidraBinary(t)
	tmpDir := t.TempDir()
	evidenceDir := filepath.Join(tmpDir, "evidence")
	privPath, _ := generateKeyPair(t, tmpDir)
	artifactPath := filepath.Join("..", "..", "tests", "golden", "tf_create.json")

	// evidra run with terraform adapter
	stdout, stderr, exitCode := runEvidra(t, bin,
		"run",
		"--tool", "terraform",
		"--operation", "apply",
		"--artifact", artifactPath,
		"--environment", "staging",
		"--session-id", "e2e-tf-run",
		"--evidence-dir", evidenceDir,
		"--signing-key-path", privPath,
		"--", "sh", "-c", "exit 0",
	)
	if exitCode != 0 {
		t.Fatalf("run exit=%d stderr=%s", exitCode, stderr)
	}

	var result map[string]interface{}
	if err := json.Unmarshal([]byte(stdout), &result); err != nil {
		t.Fatalf("decode run output: %v\nstdout: %s", err, stdout)
	}

	if result["ok"] != true {
		t.Fatalf("run result not ok: %v", result)
	}
	if result["verdict"] != "success" {
		t.Errorf("verdict = %v, want success", result["verdict"])
	}
	if result["risk_classification"] == nil || result["risk_classification"] == "" {
		t.Error("risk_classification missing")
	}

	// Verify score and signal_summary are present (run output)
	if result["score"] == nil {
		t.Error("score missing from terraform run output")
	}
	if result["signal_summary"] == nil {
		t.Error("signal_summary missing from terraform run output")
	}
}

func TestE2E_TerraformDestroyRisk(t *testing.T) {
	bin := evidraBinary(t)
	tmpDir := t.TempDir()
	evidenceDir := filepath.Join(tmpDir, "evidence")
	privPath, _ := generateKeyPair(t, tmpDir)
	artifactPath := filepath.Join("..", "..", "tests", "golden", "tf_destroy.json")

	stdout, stderr, exitCode := runEvidra(t, bin,
		"run",
		"--tool", "terraform",
		"--operation", "destroy",
		"--artifact", artifactPath,
		"--environment", "production",
		"--session-id", "e2e-tf-destroy",
		"--evidence-dir", evidenceDir,
		"--signing-key-path", privPath,
		"--", "sh", "-c", "exit 0",
	)
	if exitCode != 0 {
		t.Fatalf("run exit=%d stderr=%s", exitCode, stderr)
	}

	var result map[string]interface{}
	if err := json.Unmarshal([]byte(stdout), &result); err != nil {
		t.Fatalf("decode run output: %v\nstdout: %s", err, stdout)
	}

	// Destroy in production should be critical risk
	riskLevel, ok := result["risk_level"].(string)
	if !ok {
		t.Fatalf("risk_level missing: %v", result)
	}
	if riskLevel != "critical" {
		t.Errorf("risk_level = %q, want critical for destroy×production", riskLevel)
	}
}

func TestE2E_TerraformPrescribeReport(t *testing.T) {
	bin := evidraBinary(t)
	tmpDir := t.TempDir()
	evidenceDir := filepath.Join(tmpDir, "evidence")
	privPath, _ := generateKeyPair(t, tmpDir)
	artifactPath := filepath.Join("..", "..", "tests", "golden", "tf_mixed.json")

	// Step 1: Prescribe
	stdout, stderr, exitCode := runEvidra(t, bin,
		"prescribe",
		"--tool", "terraform",
		"--operation", "apply",
		"--artifact", artifactPath,
		"--environment", "staging",
		"--session-id", "e2e-tf-prescribe",
		"--evidence-dir", evidenceDir,
		"--signing-key-path", privPath,
	)
	if exitCode != 0 {
		t.Fatalf("prescribe exit=%d stderr=%s", exitCode, stderr)
	}

	var prescResult map[string]interface{}
	if err := json.Unmarshal([]byte(stdout), &prescResult); err != nil {
		t.Fatalf("decode prescribe: %v\nstdout: %s", err, stdout)
	}

	prescriptionID, ok := prescResult["prescription_id"].(string)
	if !ok || prescriptionID == "" {
		t.Fatalf("prescription_id missing: %v", prescResult)
	}
	if prescResult["canon_version"] != "terraform/v1" {
		t.Errorf("canon_version = %v, want terraform/v1", prescResult["canon_version"])
	}

	// Step 2: Report
	stdout, stderr, exitCode = runEvidra(t, bin,
		"report",
		"--prescription", prescriptionID,
		"--exit-code", "0",
		"--session-id", "e2e-tf-prescribe",
		"--evidence-dir", evidenceDir,
		"--signing-key-path", privPath,
	)
	if exitCode != 0 {
		t.Fatalf("report exit=%d stderr=%s", exitCode, stderr)
	}

	var reportResult map[string]interface{}
	if err := json.Unmarshal([]byte(stdout), &reportResult); err != nil {
		t.Fatalf("decode report: %v\nstdout: %s", err, stdout)
	}
	if reportResult["ok"] != true {
		t.Fatalf("report result not ok: %v", reportResult)
	}
}
