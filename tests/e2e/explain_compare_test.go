//go:build e2e

package e2e_test

import (
	"encoding/json"
	"path/filepath"
	"testing"
)

// setupTwoActorEvidence creates evidence for two actors (agent-a and agent-b)
// in the same evidence directory. agent-a has a clean run, agent-b has a
// failed run with artifact drift.
func setupTwoActorEvidence(t *testing.T, bin, evidenceDir, privPath string) {
	t.Helper()
	artifactPath := filepath.Join("..", "..", "tests", "e2e", "fixtures", "k8s_deployment.yaml")

	// Actor A: clean prescribe+report
	stdout, stderr, exitCode := runEvidra(t, bin,
		"prescribe",
		"--tool", "kubectl",
		"--operation", "apply",
		"--artifact", artifactPath,
		"--environment", "staging",
		"--actor", "agent-a",
		"--session-id", "e2e-compare",
		"--evidence-dir", evidenceDir,
		"--signing-key-path", privPath,
	)
	if exitCode != 0 {
		t.Fatalf("agent-a prescribe: exit=%d stderr=%s", exitCode, stderr)
	}
	var prescA map[string]interface{}
	if err := json.Unmarshal([]byte(stdout), &prescA); err != nil {
		t.Fatalf("decode agent-a prescribe: %v", err)
	}
	pidA := prescA["prescription_id"].(string)

	_, stderr, exitCode = runEvidra(t, bin,
		"report",
		"--prescription", pidA,
		"--exit-code", "0",
		"--actor", "agent-a",
		"--session-id", "e2e-compare",
		"--evidence-dir", evidenceDir,
		"--signing-key-path", privPath,
	)
	if exitCode != 0 {
		t.Fatalf("agent-a report: exit=%d stderr=%s", exitCode, stderr)
	}

	// Actor B: prescribe+report with failure and artifact drift
	stdout, stderr, exitCode = runEvidra(t, bin,
		"prescribe",
		"--tool", "kubectl",
		"--operation", "apply",
		"--artifact", artifactPath,
		"--environment", "staging",
		"--actor", "agent-b",
		"--session-id", "e2e-compare",
		"--evidence-dir", evidenceDir,
		"--signing-key-path", privPath,
	)
	if exitCode != 0 {
		t.Fatalf("agent-b prescribe: exit=%d stderr=%s", exitCode, stderr)
	}
	var prescB map[string]interface{}
	if err := json.Unmarshal([]byte(stdout), &prescB); err != nil {
		t.Fatalf("decode agent-b prescribe: %v", err)
	}
	pidB := prescB["prescription_id"].(string)

	_, stderr, exitCode = runEvidra(t, bin,
		"report",
		"--prescription", pidB,
		"--exit-code", "1",
		"--artifact-digest", "sha256:drifted",
		"--actor", "agent-b",
		"--session-id", "e2e-compare",
		"--evidence-dir", evidenceDir,
		"--signing-key-path", privPath,
	)
	if exitCode != 0 {
		t.Fatalf("agent-b report: exit=%d stderr=%s", exitCode, stderr)
	}
}

func TestE2E_Explain(t *testing.T) {
	bin := evidraBinary(t)
	tmpDir := t.TempDir()
	evidenceDir := filepath.Join(tmpDir, "evidence")
	privPath, _ := generateKeyPair(t, tmpDir)

	setupTwoActorEvidence(t, bin, evidenceDir, privPath)

	// Explain for agent-b (has artifact drift)
	stdout, stderr, exitCode := runEvidra(t, bin,
		"explain",
		"--actor", "agent-b",
		"--evidence-dir", evidenceDir,
		"--min-operations", "1",
	)
	if exitCode != 0 {
		t.Fatalf("explain exit=%d stderr=%s", exitCode, stderr)
	}

	var result map[string]interface{}
	if err := json.Unmarshal([]byte(stdout), &result); err != nil {
		t.Fatalf("decode explain: %v\nstdout: %s", err, stdout)
	}

	// Must have score, band, signals array
	if result["score"] == nil {
		t.Error("explain missing score")
	}
	if result["band"] == nil {
		t.Error("explain missing band")
	}

	signals, ok := result["signals"].([]interface{})
	if !ok || len(signals) == 0 {
		t.Fatalf("explain signals missing or empty: %v", result["signals"])
	}

	// Check that artifact_drift signal is present with count > 0
	foundDrift := false
	for _, s := range signals {
		sig := s.(map[string]interface{})
		if sig["signal"] == "artifact_drift" {
			count := int(sig["count"].(float64))
			if count < 1 {
				t.Errorf("artifact_drift count = %d, want >= 1", count)
			}
			if sig["weight"] == nil {
				t.Error("artifact_drift missing weight")
			}
			foundDrift = true
		}
	}
	if !foundDrift {
		t.Error("artifact_drift signal not found in explain output")
	}
}

func TestE2E_Compare(t *testing.T) {
	bin := evidraBinary(t)
	tmpDir := t.TempDir()
	evidenceDir := filepath.Join(tmpDir, "evidence")
	privPath, _ := generateKeyPair(t, tmpDir)

	setupTwoActorEvidence(t, bin, evidenceDir, privPath)

	stdout, stderr, exitCode := runEvidra(t, bin,
		"compare",
		"--actors", "agent-a,agent-b",
		"--evidence-dir", evidenceDir,
	)
	if exitCode != 0 {
		t.Fatalf("compare exit=%d stderr=%s", exitCode, stderr)
	}

	var result map[string]interface{}
	if err := json.Unmarshal([]byte(stdout), &result); err != nil {
		t.Fatalf("decode compare: %v\nstdout: %s", err, stdout)
	}

	actors, ok := result["actors"].([]interface{})
	if !ok || len(actors) != 2 {
		t.Fatalf("compare actors: got %v, want 2 entries", result["actors"])
	}

	actorA := actors[0].(map[string]interface{})
	actorB := actors[1].(map[string]interface{})

	if actorA["actor_id"] != "agent-a" {
		t.Errorf("first actor = %v, want agent-a", actorA["actor_id"])
	}
	if actorB["actor_id"] != "agent-b" {
		t.Errorf("second actor = %v, want agent-b", actorB["actor_id"])
	}

	scoreA := actorA["score"].(float64)
	scoreB := actorB["score"].(float64)
	if scoreA < scoreB {
		t.Errorf("agent-a score (%.2f) < agent-b score (%.2f); clean actor should score higher", scoreA, scoreB)
	}

	// workload_overlap should be present (both use same tool+scope)
	overlap, ok := result["workload_overlap"].(float64)
	if !ok {
		t.Fatal("workload_overlap missing from compare output")
	}
	if overlap <= 0 {
		t.Errorf("workload_overlap = %.2f, want > 0 (both actors use kubectl+staging)", overlap)
	}
}

func TestE2E_CompareRequiresTwoActors(t *testing.T) {
	bin := evidraBinary(t)

	_, _, exitCode := runEvidra(t, bin,
		"compare",
		"--actors", "only-one",
	)
	if exitCode == 0 {
		t.Error("compare with single actor should fail")
	}
}
