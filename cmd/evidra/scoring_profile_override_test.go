package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"samebits.com/evidra-benchmark/internal/score"
	"samebits.com/evidra-benchmark/internal/testutil"
)

func TestScorecard_UsesScoringProfileOverride(t *testing.T) {
	t.Parallel()

	signingKey := testutil.TestSigningKeyBase64(t)
	profilePath := writeCustomScoringProfile(t, "custom.scorecard-profile")
	tmp := t.TempDir()
	evidenceDir := filepath.Join(tmp, "evidence")
	artifactPath := filepath.Join(tmp, "artifact.json")
	if err := os.WriteFile(artifactPath, []byte(`{"noop":true}`), 0o644); err != nil {
		t.Fatalf("write artifact: %v", err)
	}

	prescriptionID := writeSuccessfulPrescription(t, signingKey, evidenceDir, artifactPath, "session-scorecard-profile")
	writeSuccessfulReport(t, signingKey, evidenceDir, prescriptionID, "session-scorecard-profile")

	var out, errBuf bytes.Buffer
	code := run([]string{
		"scorecard",
		"--session-id", "session-scorecard-profile",
		"--evidence-dir", evidenceDir,
		"--min-operations", "1",
		"--scoring-profile", profilePath,
	}, &out, &errBuf)
	if code != 0 {
		t.Fatalf("scorecard exit %d: %s", code, errBuf.String())
	}

	var sc map[string]interface{}
	if err := json.Unmarshal(out.Bytes(), &sc); err != nil {
		t.Fatalf("decode scorecard: %v", err)
	}
	if got := sc["scoring_profile_id"]; got != "custom.scorecard-profile" {
		t.Fatalf("scoring_profile_id = %v, want custom.scorecard-profile", got)
	}
}

func TestExplain_UsesScoringProfileOverride(t *testing.T) {
	t.Parallel()

	signingKey := testutil.TestSigningKeyBase64(t)
	profilePath := writeCustomScoringProfile(t, "custom.explain-profile")
	tmp := t.TempDir()
	evidenceDir := filepath.Join(tmp, "evidence")
	artifactPath := filepath.Join(tmp, "artifact.json")
	if err := os.WriteFile(artifactPath, []byte(`{"noop":true}`), 0o644); err != nil {
		t.Fatalf("write artifact: %v", err)
	}

	prescriptionID := writeSuccessfulPrescription(t, signingKey, evidenceDir, artifactPath, "session-explain-profile")
	writeSuccessfulReport(t, signingKey, evidenceDir, prescriptionID, "session-explain-profile")

	var out, errBuf bytes.Buffer
	code := run([]string{
		"explain",
		"--session-id", "session-explain-profile",
		"--evidence-dir", evidenceDir,
		"--min-operations", "1",
		"--scoring-profile", profilePath,
	}, &out, &errBuf)
	if code != 0 {
		t.Fatalf("explain exit %d: %s", code, errBuf.String())
	}

	var explained map[string]interface{}
	if err := json.Unmarshal(out.Bytes(), &explained); err != nil {
		t.Fatalf("decode explain: %v", err)
	}
	if got := explained["scoring_profile_id"]; got != "custom.explain-profile" {
		t.Fatalf("scoring_profile_id = %v, want custom.explain-profile", got)
	}

	signals, ok := explained["signals"].([]interface{})
	if !ok || len(signals) == 0 {
		t.Fatalf("signals = %#v, want populated list", explained["signals"])
	}
	for _, item := range signals {
		row, ok := item.(map[string]interface{})
		if !ok {
			t.Fatalf("signal row = %#v, want object", item)
		}
		if row["signal"] == "protocol_violation" {
			if row["weight"] != 0.42 {
				t.Fatalf("protocol_violation weight = %v, want 0.42", row["weight"])
			}
			return
		}
	}
	t.Fatal("protocol_violation row not found in explain output")
}

func TestReport_UsesScoringProfileOverride(t *testing.T) {
	t.Parallel()

	signingKey := testutil.TestSigningKeyBase64(t)
	profilePath := writeCustomScoringProfile(t, "custom.report-profile")
	tmp := t.TempDir()
	evidenceDir := filepath.Join(tmp, "evidence")
	artifactPath := filepath.Join(tmp, "artifact.json")
	if err := os.WriteFile(artifactPath, []byte(`{"noop":true}`), 0o644); err != nil {
		t.Fatalf("write artifact: %v", err)
	}

	prescriptionID := writeSuccessfulPrescription(t, signingKey, evidenceDir, artifactPath, "session-report-profile")

	var out, errBuf bytes.Buffer
	code := run([]string{
		"report",
		"--prescription", prescriptionID,
		"--exit-code", "0",
		"--session-id", "session-report-profile",
		"--evidence-dir", evidenceDir,
		"--signing-key", signingKey,
		"--scoring-profile", profilePath,
	}, &out, &errBuf)
	if code != 0 {
		t.Fatalf("report exit %d: %s", code, errBuf.String())
	}

	var report map[string]interface{}
	if err := json.Unmarshal(out.Bytes(), &report); err != nil {
		t.Fatalf("decode report: %v", err)
	}
	if got := report["scoring_profile_id"]; got != "custom.report-profile" {
		t.Fatalf("scoring_profile_id = %v, want custom.report-profile", got)
	}
}

func TestRun_UsesScoringProfileOverride(t *testing.T) {
	t.Parallel()

	signingKey := testutil.TestSigningKeyBase64(t)
	profilePath := writeCustomScoringProfile(t, "custom.run-profile")
	tmp := t.TempDir()
	evidenceDir := filepath.Join(tmp, "evidence")
	artifactPath := filepath.Join(tmp, "artifact.yaml")
	if err := os.WriteFile(artifactPath, []byte("apiVersion: v1\nkind: ConfigMap\nmetadata:\n  name: run-profile\n"), 0o644); err != nil {
		t.Fatalf("write artifact: %v", err)
	}

	var out, errBuf bytes.Buffer
	code := run([]string{
		"run",
		"--tool", "kubectl",
		"--operation", "apply",
		"--artifact", artifactPath,
		"--environment", "staging",
		"--evidence-dir", evidenceDir,
		"--signing-key", signingKey,
		"--scoring-profile", profilePath,
		"--", "sh", "-c", "exit 0",
	}, &out, &errBuf)
	if code != 0 {
		t.Fatalf("run exit=%d stderr=%s", code, errBuf.String())
	}

	var result map[string]interface{}
	if err := json.Unmarshal(out.Bytes(), &result); err != nil {
		t.Fatalf("decode run output: %v", err)
	}
	if got := result["scoring_profile_id"]; got != "custom.run-profile" {
		t.Fatalf("scoring_profile_id = %v, want custom.run-profile", got)
	}
}

func TestRecord_UsesScoringProfileOverride(t *testing.T) {
	t.Parallel()

	signingKey := testutil.TestSigningKeyBase64(t)
	profilePath := writeCustomScoringProfile(t, "custom.record-profile")
	tmp := t.TempDir()
	evidenceDir := filepath.Join(tmp, "evidence")
	inputPath := filepath.Join(tmp, "record.json")

	recordInput := map[string]interface{}{
		"contract_version": "v1",
		"session_id":       "session-record-profile",
		"operation_id":     "op-record-profile",
		"tool":             "kubectl",
		"operation":        "apply",
		"environment":      "staging",
		"actor": map[string]interface{}{
			"type":       "ci",
			"id":         "pipeline-1",
			"provenance": "github",
		},
		"exit_code":    0,
		"duration_ms":  1200,
		"raw_artifact": "apiVersion: v1\nkind: ConfigMap\nmetadata:\n  name: rec-profile\n  namespace: default\n",
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
		"--scoring-profile", profilePath,
	}, &out, &errBuf)
	if code != 0 {
		t.Fatalf("record exit=%d stderr=%s", code, errBuf.String())
	}

	var result map[string]interface{}
	if err := json.Unmarshal(out.Bytes(), &result); err != nil {
		t.Fatalf("decode record output: %v", err)
	}
	if got := result["scoring_profile_id"]; got != "custom.record-profile" {
		t.Fatalf("scoring_profile_id = %v, want custom.record-profile", got)
	}
}

func TestCompare_UsesScoringProfileOverride(t *testing.T) {
	t.Parallel()

	signingKey := testutil.TestSigningKeyBase64(t)
	profilePath := writeCustomScoringProfile(t, "custom.compare-profile")
	tmp := t.TempDir()
	evidenceDir := filepath.Join(tmp, "evidence")

	writeComparisonOperation(t, signingKey, evidenceDir, "actor-a", "session-compare-a")
	writeComparisonOperation(t, signingKey, evidenceDir, "actor-b", "session-compare-b")

	var out, errBuf bytes.Buffer
	code := run([]string{
		"compare",
		"--actors", "actor-a,actor-b",
		"--evidence-dir", evidenceDir,
		"--scoring-profile", profilePath,
	}, &out, &errBuf)
	if code != 0 {
		t.Fatalf("compare exit=%d stderr=%s", code, errBuf.String())
	}

	var result map[string]interface{}
	if err := json.Unmarshal(out.Bytes(), &result); err != nil {
		t.Fatalf("decode compare output: %v", err)
	}
	if got := result["scoring_profile_id"]; got != "custom.compare-profile" {
		t.Fatalf("scoring_profile_id = %v, want custom.compare-profile", got)
	}
}

func writeComparisonOperation(t *testing.T, signingKey, evidenceDir, actorID, sessionID string) {
	t.Helper()

	artifactPath := filepath.Join(t.TempDir(), actorID+".json")
	if err := os.WriteFile(artifactPath, []byte(`{"noop":true}`), 0o644); err != nil {
		t.Fatalf("write artifact: %v", err)
	}

	var out, errBuf bytes.Buffer
	code := run([]string{
		"prescribe",
		"--tool", "terraform",
		"--artifact", artifactPath,
		"--canonical-action", testCanonicalAction,
		"--actor", actorID,
		"--session-id", sessionID,
		"--evidence-dir", evidenceDir,
		"--signing-key", signingKey,
	}, &out, &errBuf)
	if code != 0 {
		t.Fatalf("prescribe exit %d: %s", code, errBuf.String())
	}

	var presc map[string]interface{}
	if err := json.Unmarshal(out.Bytes(), &presc); err != nil {
		t.Fatalf("decode prescribe output: %v", err)
	}
	prescriptionID, _ := presc["prescription_id"].(string)
	if prescriptionID == "" {
		t.Fatalf("missing prescription_id in %#v", presc)
	}

	out.Reset()
	errBuf.Reset()
	code = run([]string{
		"report",
		"--prescription", prescriptionID,
		"--exit-code", "0",
		"--actor", actorID,
		"--session-id", sessionID,
		"--evidence-dir", evidenceDir,
		"--signing-key", signingKey,
	}, &out, &errBuf)
	if code != 0 {
		t.Fatalf("report exit %d: %s", code, errBuf.String())
	}
}

func writeSuccessfulPrescription(t *testing.T, signingKey, evidenceDir, artifactPath, sessionID string) string {
	t.Helper()

	var out, errBuf bytes.Buffer
	code := run([]string{
		"prescribe",
		"--tool", "terraform",
		"--artifact", artifactPath,
		"--canonical-action", testCanonicalAction,
		"--session-id", sessionID,
		"--evidence-dir", evidenceDir,
		"--signing-key", signingKey,
	}, &out, &errBuf)
	if code != 0 {
		t.Fatalf("prescribe exit %d: %s", code, errBuf.String())
	}

	var presc map[string]interface{}
	if err := json.Unmarshal(out.Bytes(), &presc); err != nil {
		t.Fatalf("decode prescribe output: %v", err)
	}
	prescriptionID, _ := presc["prescription_id"].(string)
	if prescriptionID == "" {
		t.Fatalf("missing prescription_id in %#v", presc)
	}
	return prescriptionID
}

func writeSuccessfulReport(t *testing.T, signingKey, evidenceDir, prescriptionID, sessionID string) {
	t.Helper()

	var out, errBuf bytes.Buffer
	code := run([]string{
		"report",
		"--prescription", prescriptionID,
		"--exit-code", "0",
		"--session-id", sessionID,
		"--evidence-dir", evidenceDir,
		"--signing-key", signingKey,
	}, &out, &errBuf)
	if code != 0 {
		t.Fatalf("report exit %d: %s", code, errBuf.String())
	}
}

func writeCustomScoringProfile(t *testing.T, id string) string {
	t.Helper()

	profile, err := score.LoadDefaultProfile()
	if err != nil {
		t.Fatalf("LoadDefaultProfile: %v", err)
	}
	profile.ID = id
	profile.Weights["protocol_violation"] = 0.42

	data, err := json.Marshal(profile)
	if err != nil {
		t.Fatalf("marshal profile: %v", err)
	}

	path := filepath.Join(t.TempDir(), id+".json")
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatalf("write profile: %v", err)
	}
	return path
}
