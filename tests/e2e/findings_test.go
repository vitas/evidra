//go:build e2e

package e2e_test

import (
	"encoding/json"
	"path/filepath"
	"testing"

	"samebits.com/evidra-benchmark/pkg/evidence"
)

func TestE2E_FindingsIngestion(t *testing.T) {
	bin := evidraBinary(t)
	tmpDir := t.TempDir()
	evidenceDir := filepath.Join(tmpDir, "evidence")
	privPath, _ := generateKeyPair(t, tmpDir)

	artifactPath := filepath.Join("..", "..", "tests", "e2e", "fixtures", "k8s_deployment.yaml")
	trivySarif := filepath.Join("..", "..", "tests", "e2e", "fixtures", "trivy.sarif")
	kubescapeSarif := filepath.Join("..", "..", "tests", "e2e", "fixtures", "kubescape.sarif")

	const sessionID = "e2e-findings-001"

	// Step 1: Ingest trivy findings (pre-prescribe).
	stdout, stderr, exitCode := runEvidra(t, bin,
		"ingest-findings",
		"--sarif", trivySarif,
		"--artifact", artifactPath,
		"--evidence-dir", evidenceDir,
		"--session-id", sessionID,
		"--actor", "scanner-trivy",
		"--signing-key-path", privPath,
	)
	if exitCode != 0 {
		t.Fatalf("trivy ingest exit code = %d, stderr = %s", exitCode, stderr)
	}

	var trivyResult map[string]interface{}
	if err := json.Unmarshal([]byte(stdout), &trivyResult); err != nil {
		t.Fatalf("decode trivy ingest output: %v\nstdout: %s", err, stdout)
	}
	if trivyResult["ok"] != true {
		t.Fatalf("trivy ingest result not ok: %v", trivyResult)
	}
	trivyFindingsCount, ok := trivyResult["findings_count"].(float64)
	if !ok || trivyFindingsCount != 2 {
		t.Fatalf("trivy findings_count = %v, want 2", trivyResult["findings_count"])
	}
	trivyArtifactDigest, ok := trivyResult["artifact_digest"].(string)
	if !ok || trivyArtifactDigest == "" {
		t.Fatalf("trivy artifact_digest missing or empty: %v", trivyResult)
	}

	// Step 2: Prescribe.
	stdout, stderr, exitCode = runEvidra(t, bin,
		"prescribe",
		"--tool", "kubectl",
		"--operation", "apply",
		"--artifact", artifactPath,
		"--session-id", sessionID,
		"--evidence-dir", evidenceDir,
		"--signing-key-path", privPath,
	)
	if exitCode != 0 {
		t.Fatalf("prescribe exit code = %d, stderr = %s", exitCode, stderr)
	}

	var prescribeResult map[string]interface{}
	if err := json.Unmarshal([]byte(stdout), &prescribeResult); err != nil {
		t.Fatalf("decode prescribe output: %v\nstdout: %s", err, stdout)
	}
	if prescribeResult["ok"] != true {
		t.Fatalf("prescribe result not ok: %v", prescribeResult)
	}
	prescriptionID, ok := prescribeResult["prescription_id"].(string)
	if !ok || prescriptionID == "" {
		t.Fatalf("prescription_id missing or empty: %v", prescribeResult)
	}
	prescribeArtifactDigest, ok := prescribeResult["artifact_digest"].(string)
	if !ok || prescribeArtifactDigest == "" {
		t.Fatalf("prescribe artifact_digest missing or empty: %v", prescribeResult)
	}

	// Verify artifact_digest from trivy ingest matches prescribe.
	if trivyArtifactDigest != prescribeArtifactDigest {
		t.Fatalf("artifact_digest mismatch: trivy = %s, prescribe = %s", trivyArtifactDigest, prescribeArtifactDigest)
	}

	// Step 3: Report.
	_, stderr, exitCode = runEvidra(t, bin,
		"report",
		"--prescription", prescriptionID,
		"--exit-code", "0",
		"--session-id", sessionID,
		"--evidence-dir", evidenceDir,
		"--signing-key-path", privPath,
	)
	if exitCode != 0 {
		t.Fatalf("report exit code = %d, stderr = %s", exitCode, stderr)
	}

	// Step 4: Ingest kubescape findings (post-report).
	stdout, stderr, exitCode = runEvidra(t, bin,
		"ingest-findings",
		"--sarif", kubescapeSarif,
		"--artifact", artifactPath,
		"--evidence-dir", evidenceDir,
		"--session-id", sessionID,
		"--actor", "scanner-kubescape",
		"--signing-key-path", privPath,
	)
	if exitCode != 0 {
		t.Fatalf("kubescape ingest exit code = %d, stderr = %s", exitCode, stderr)
	}

	var kubescapeResult map[string]interface{}
	if err := json.Unmarshal([]byte(stdout), &kubescapeResult); err != nil {
		t.Fatalf("decode kubescape ingest output: %v\nstdout: %s", err, stdout)
	}
	if kubescapeResult["ok"] != true {
		t.Fatalf("kubescape ingest result not ok: %v", kubescapeResult)
	}
	kubescapeFindingsCount, ok := kubescapeResult["findings_count"].(float64)
	if !ok || kubescapeFindingsCount != 1 {
		t.Fatalf("kubescape findings_count = %v, want 1", kubescapeResult["findings_count"])
	}

	// Step 5: Verify evidence chain.
	entries, err := evidence.ReadAllEntriesAtPath(evidenceDir)
	if err != nil {
		t.Fatalf("ReadAllEntriesAtPath: %v", err)
	}

	// Count entries by type.
	typeCounts := make(map[evidence.EntryType]int)
	sessionCount := 0
	for _, e := range entries {
		typeCounts[e.Type]++
		if e.SessionID == sessionID {
			sessionCount++
		}
	}

	// Expect 5 total: 3 findings (2 trivy + 1 kubescape) + 1 prescribe + 1 report.
	if len(entries) != 5 {
		t.Errorf("total entries = %d, want 5 (got types: %v)", len(entries), typeCounts)
	}
	if typeCounts[evidence.EntryTypeFinding] != 3 {
		t.Errorf("finding entries = %d, want 3", typeCounts[evidence.EntryTypeFinding])
	}
	if typeCounts[evidence.EntryTypePrescribe] != 1 {
		t.Errorf("prescribe entries = %d, want 1", typeCounts[evidence.EntryTypePrescribe])
	}
	if typeCounts[evidence.EntryTypeReport] != 1 {
		t.Errorf("report entries = %d, want 1", typeCounts[evidence.EntryTypeReport])
	}

	// All entries should have the session ID.
	if sessionCount != 5 {
		t.Errorf("entries with session-id %q = %d, want 5", sessionID, sessionCount)
	}

	// Validate chain integrity.
	if err := evidence.ValidateChainAtPath(evidenceDir); err != nil {
		t.Fatalf("ValidateChainAtPath: %v", err)
	}
}
