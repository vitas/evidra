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

func TestRunReport_DeclinedRequiresReason(t *testing.T) {
	t.Parallel()

	signingKey := testutil.TestSigningKeyBase64(t)
	tmp := t.TempDir()
	evidenceDir := filepath.Join(tmp, "evidence")
	prescriptionID := prescribeForReportTest(t, signingKey, evidenceDir)

	var out, errBuf bytes.Buffer
	code := run([]string{
		"report",
		"--prescription", prescriptionID,
		"--verdict", "declined",
		"--decline-trigger", "risk_threshold_exceeded",
		"--evidence-dir", evidenceDir,
		"--signing-key", signingKey,
	}, &out, &errBuf)
	if code == 0 {
		t.Fatal("expected non-zero exit code")
	}
	if !strings.Contains(errBuf.String(), "declined report requires --decline-reason") {
		t.Fatalf("stderr missing decline reason validation: %s", errBuf.String())
	}
}

func TestRunReport_DeclinedRejectsExitCode(t *testing.T) {
	t.Parallel()

	signingKey := testutil.TestSigningKeyBase64(t)
	tmp := t.TempDir()
	evidenceDir := filepath.Join(tmp, "evidence")
	prescriptionID := prescribeForReportTest(t, signingKey, evidenceDir)

	var out, errBuf bytes.Buffer
	code := run([]string{
		"report",
		"--prescription", prescriptionID,
		"--verdict", "declined",
		"--exit-code", "0",
		"--decline-trigger", "risk_threshold_exceeded",
		"--decline-reason", "risk_level=critical and blast_radius covers production namespace",
		"--evidence-dir", evidenceDir,
		"--signing-key", signingKey,
	}, &out, &errBuf)
	if code == 0 {
		t.Fatal("expected non-zero exit code")
	}
	if !strings.Contains(errBuf.String(), "declined report must not include --exit-code") {
		t.Fatalf("stderr missing exit-code validation: %s", errBuf.String())
	}
}

func TestRunReport_DeclinedEchoesDecisionContext(t *testing.T) {
	t.Parallel()

	signingKey := testutil.TestSigningKeyBase64(t)
	tmp := t.TempDir()
	evidenceDir := filepath.Join(tmp, "evidence")
	prescriptionID := prescribeForReportTest(t, signingKey, evidenceDir)
	reason := "risk_level=critical and blast_radius covers production namespace"

	var out, errBuf bytes.Buffer
	code := run([]string{
		"report",
		"--prescription", prescriptionID,
		"--verdict", "declined",
		"--decline-trigger", "risk_threshold_exceeded",
		"--decline-reason", reason,
		"--evidence-dir", evidenceDir,
		"--signing-key", signingKey,
	}, &out, &errBuf)
	if code != 0 {
		t.Fatalf("report exit %d: %s", code, errBuf.String())
	}

	var result map[string]any
	if err := json.Unmarshal(out.Bytes(), &result); err != nil {
		t.Fatalf("decode report output: %v", err)
	}
	if result["verdict"] != string(evidence.VerdictDeclined) {
		t.Fatalf("verdict = %#v, want %q", result["verdict"], evidence.VerdictDeclined)
	}
	ctx, ok := result["decision_context"].(map[string]any)
	if !ok {
		t.Fatalf("decision_context missing: %#v", result["decision_context"])
	}
	if ctx["trigger"] != "risk_threshold_exceeded" {
		t.Fatalf("trigger = %#v", ctx["trigger"])
	}
	if ctx["reason"] != reason {
		t.Fatalf("reason = %#v", ctx["reason"])
	}

	entries, err := evidence.ReadAllEntriesAtPath(evidenceDir)
	if err != nil {
		t.Fatalf("ReadAllEntriesAtPath: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("entry count = %d, want 2", len(entries))
	}

	var payload evidence.ReportPayload
	if err := json.Unmarshal(entries[1].Payload, &payload); err != nil {
		t.Fatalf("unmarshal report payload: %v", err)
	}
	if payload.Verdict != evidence.VerdictDeclined {
		t.Fatalf("payload verdict = %q, want %q", payload.Verdict, evidence.VerdictDeclined)
	}
	if payload.DecisionContext == nil {
		t.Fatal("payload decision_context missing")
	}
	if payload.DecisionContext.Trigger != "risk_threshold_exceeded" {
		t.Fatalf("payload trigger = %q", payload.DecisionContext.Trigger)
	}
	if payload.DecisionContext.Reason != reason {
		t.Fatalf("payload reason = %q", payload.DecisionContext.Reason)
	}
}

func TestRunReport_SuccessRequiresExitCode(t *testing.T) {
	t.Parallel()

	signingKey := testutil.TestSigningKeyBase64(t)
	tmp := t.TempDir()
	evidenceDir := filepath.Join(tmp, "evidence")
	prescriptionID := prescribeForReportTest(t, signingKey, evidenceDir)

	var out, errBuf bytes.Buffer
	code := run([]string{
		"report",
		"--prescription", prescriptionID,
		"--verdict", "success",
		"--evidence-dir", evidenceDir,
		"--signing-key", signingKey,
	}, &out, &errBuf)
	if code == 0 {
		t.Fatal("expected non-zero exit code")
	}
	if !strings.Contains(errBuf.String(), "report verdict success requires --exit-code") {
		t.Fatalf("stderr missing exit-code requirement: %s", errBuf.String())
	}
}

func prescribeForReportTest(t *testing.T, signingKey, evidenceDir string) string {
	t.Helper()

	artifact := filepath.Join(t.TempDir(), "artifact.json")
	if err := os.WriteFile(artifact, []byte(`{"noop":true}`), 0o644); err != nil {
		t.Fatalf("write artifact: %v", err)
	}

	var out, errBuf bytes.Buffer
	code := run([]string{
		"prescribe",
		"--tool", "terraform",
		"--artifact", artifact,
		"--canonical-action", testCanonicalAction,
		"--evidence-dir", evidenceDir,
		"--signing-key", signingKey,
	}, &out, &errBuf)
	if code != 0 {
		t.Fatalf("prescribe exit %d: %s", code, errBuf.String())
	}

	var result map[string]any
	if err := json.Unmarshal(out.Bytes(), &result); err != nil {
		t.Fatalf("decode prescribe output: %v", err)
	}

	prescriptionID, ok := result["prescription_id"].(string)
	if !ok || prescriptionID == "" {
		t.Fatalf("invalid prescription_id: %#v", result["prescription_id"])
	}
	return prescriptionID
}
