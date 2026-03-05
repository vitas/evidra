//go:build e2e

package e2e_test

import (
	"encoding/json"
	"path/filepath"
	"testing"
)

func TestE2E_SessionFilteredScoring(t *testing.T) {
	bin := evidraBinary(t)
	tmpDir := t.TempDir()
	evidenceDir := filepath.Join(tmpDir, "evidence")
	privPath, _ := generateKeyPair(t, tmpDir)
	artifactPath := filepath.Join("..", "..", "tests", "e2e", "fixtures", "k8s_deployment.yaml")

	// --- Session A: clean run (exit_code=0) ---
	stdout, stderr, exitCode := runEvidra(t, bin,
		"prescribe",
		"--tool", "kubectl",
		"--operation", "apply",
		"--artifact", artifactPath,
		"--session-id", "session-A",
		"--evidence-dir", evidenceDir,
		"--signing-key-path", privPath,
	)
	if exitCode != 0 {
		t.Fatalf("session-A prescribe failed: exit=%d stderr=%s", exitCode, stderr)
	}

	var prescribeA map[string]interface{}
	if err := json.Unmarshal([]byte(stdout), &prescribeA); err != nil {
		t.Fatalf("decode session-A prescribe: %v\nstdout: %s", err, stdout)
	}
	prescriptionIDA, ok := prescribeA["prescription_id"].(string)
	if !ok || prescriptionIDA == "" {
		t.Fatalf("session-A prescription_id missing: %v", prescribeA)
	}

	_, stderr, exitCode = runEvidra(t, bin,
		"report",
		"--prescription", prescriptionIDA,
		"--exit-code", "0",
		"--session-id", "session-A",
		"--evidence-dir", evidenceDir,
		"--signing-key-path", privPath,
	)
	if exitCode != 0 {
		t.Fatalf("session-A report failed: exit=%d stderr=%s", exitCode, stderr)
	}

	// --- Session B: failed run (exit_code=1, artifact drift) ---
	stdout, stderr, exitCode = runEvidra(t, bin,
		"prescribe",
		"--tool", "kubectl",
		"--operation", "apply",
		"--artifact", artifactPath,
		"--session-id", "session-B",
		"--evidence-dir", evidenceDir,
		"--signing-key-path", privPath,
	)
	if exitCode != 0 {
		t.Fatalf("session-B prescribe failed: exit=%d stderr=%s", exitCode, stderr)
	}

	var prescribeB map[string]interface{}
	if err := json.Unmarshal([]byte(stdout), &prescribeB); err != nil {
		t.Fatalf("decode session-B prescribe: %v\nstdout: %s", err, stdout)
	}
	prescriptionIDB, ok := prescribeB["prescription_id"].(string)
	if !ok || prescriptionIDB == "" {
		t.Fatalf("session-B prescription_id missing: %v", prescribeB)
	}

	_, stderr, exitCode = runEvidra(t, bin,
		"report",
		"--prescription", prescriptionIDB,
		"--exit-code", "1",
		"--artifact-digest", "sha256:different_from_prescription",
		"--session-id", "session-B",
		"--evidence-dir", evidenceDir,
		"--signing-key-path", privPath,
	)
	if exitCode != 0 {
		t.Fatalf("session-B report failed: exit=%d stderr=%s", exitCode, stderr)
	}

	// --- Scorecard for session-A ---
	stdout, stderr, exitCode = runEvidra(t, bin,
		"scorecard",
		"--session-id", "session-A",
		"--evidence-dir", evidenceDir,
	)
	if exitCode != 0 {
		t.Fatalf("scorecard session-A failed: exit=%d stderr=%s", exitCode, stderr)
	}

	var scorecardA map[string]interface{}
	if err := json.Unmarshal([]byte(stdout), &scorecardA); err != nil {
		t.Fatalf("decode scorecard-A: %v\nstdout: %s", err, stdout)
	}

	totalOpsA := int(scorecardA["total_operations"].(float64))
	if totalOpsA != 1 {
		t.Errorf("session-A total_operations = %d, want 1", totalOpsA)
	}
	scoreA := scorecardA["score"].(float64)
	t.Logf("session-A: score=%.2f total_operations=%d", scoreA, totalOpsA)

	// --- Scorecard for session-B ---
	stdout, stderr, exitCode = runEvidra(t, bin,
		"scorecard",
		"--session-id", "session-B",
		"--evidence-dir", evidenceDir,
	)
	if exitCode != 0 {
		t.Fatalf("scorecard session-B failed: exit=%d stderr=%s", exitCode, stderr)
	}

	var scorecardB map[string]interface{}
	if err := json.Unmarshal([]byte(stdout), &scorecardB); err != nil {
		t.Fatalf("decode scorecard-B: %v\nstdout: %s", err, stdout)
	}

	totalOpsB := int(scorecardB["total_operations"].(float64))
	if totalOpsB != 1 {
		t.Errorf("session-B total_operations = %d, want 1", totalOpsB)
	}
	scoreB := scorecardB["score"].(float64)
	t.Logf("session-B: score=%.2f total_operations=%d", scoreB, totalOpsB)

	// --- Session A (clean) should score >= Session B (failed + drift) ---
	if scoreA < scoreB {
		t.Errorf("session-A score (%.2f) < session-B score (%.2f); clean run should score higher", scoreA, scoreB)
	}

	// --- Scorecard without --session-id should include both sessions ---
	stdout, stderr, exitCode = runEvidra(t, bin,
		"scorecard",
		"--evidence-dir", evidenceDir,
	)
	if exitCode != 0 {
		t.Fatalf("scorecard (all) failed: exit=%d stderr=%s", exitCode, stderr)
	}

	var scorecardAll map[string]interface{}
	if err := json.Unmarshal([]byte(stdout), &scorecardAll); err != nil {
		t.Fatalf("decode scorecard-all: %v\nstdout: %s", err, stdout)
	}

	totalOpsAll := int(scorecardAll["total_operations"].(float64))
	if totalOpsAll != 2 {
		t.Errorf("all-sessions total_operations = %d, want 2", totalOpsAll)
	}
	t.Logf("all-sessions: score=%.2f total_operations=%d", scorecardAll["score"].(float64), totalOpsAll)
}
