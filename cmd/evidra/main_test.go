package main

import (
	"bytes"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"samebits.com/evidra-benchmark/internal/testutil"
	"samebits.com/evidra-benchmark/pkg/evidence"
)

const testCanonicalAction = `{"tool":"terraform","operation":"apply","operation_class":"mutate","scope_class":"production","resource_count":1,"resource_shape_hash":"sha256:test"}`

func TestRunPrescribe_ScannerReportParseError(t *testing.T) {
	t.Parallel()
	signingKey := testutil.TestSigningKeyBase64(t)

	tmp := t.TempDir()
	artifact := filepath.Join(tmp, "artifact.json")
	badSarif := filepath.Join(tmp, "bad.sarif")

	if err := os.WriteFile(artifact, []byte(`{"noop":true}`), 0o644); err != nil {
		t.Fatalf("write artifact: %v", err)
	}
	if err := os.WriteFile(badSarif, []byte(`not json`), 0o644); err != nil {
		t.Fatalf("write bad sarif: %v", err)
	}

	args := []string{
		"prescribe",
		"--tool", "terraform",
		"--artifact", artifact,
		"--canonical-action", testCanonicalAction,
		"--scanner-report", badSarif,
		"--evidence-dir", tmp,
		"--signing-key", signingKey,
	}

	var out, errBuf bytes.Buffer
	code := run(args, &out, &errBuf)
	if code == 0 {
		t.Fatal("expected non-zero exit code")
	}
	if !strings.Contains(errBuf.String(), "parse scanner report") {
		t.Fatalf("stderr missing parse scanner report: %s", errBuf.String())
	}
}

func TestRunPrescribe_ScannerReportCountsWrittenFindings(t *testing.T) {
	t.Parallel()
	signingKey := testutil.TestSigningKeyBase64(t)

	tmp := t.TempDir()
	artifact := filepath.Join(tmp, "artifact.json")
	if err := os.WriteFile(artifact, []byte(`{"noop":true}`), 0o644); err != nil {
		t.Fatalf("write artifact: %v", err)
	}

	scannerReport, err := filepath.Abs("../../tests/testdata/sarif_trivy.json")
	if err != nil {
		t.Fatalf("resolve scanner report path: %v", err)
	}

	args := []string{
		"prescribe",
		"--tool", "terraform",
		"--artifact", artifact,
		"--canonical-action", testCanonicalAction,
		"--scanner-report", scannerReport,
		"--evidence-dir", tmp,
		"--signing-key", signingKey,
	}

	var out, errBuf bytes.Buffer
	code := run(args, &out, &errBuf)
	if code != 0 {
		t.Fatalf("expected exit code 0, got %d, stderr=%s", code, errBuf.String())
	}

	var result map[string]interface{}
	if err := json.Unmarshal(out.Bytes(), &result); err != nil {
		t.Fatalf("decode output: %v", err)
	}
	count, ok := result["findings_count"].(float64)
	if !ok {
		t.Fatalf("findings_count missing or non-number: %#v", result["findings_count"])
	}
	if int(count) != 1 {
		t.Fatalf("findings_count = %d, want 1", int(count))
	}

	entries, err := evidence.ReadAllEntriesAtPath(tmp)
	if err != nil {
		t.Fatalf("ReadAllEntriesAtPath: %v", err)
	}

	findingCount := 0
	for _, e := range entries {
		if e.Type != evidence.EntryTypeFinding {
			continue
		}
		findingCount++
		if e.Actor.ID != "cli" {
			t.Fatalf("finding actor id = %q, want cli", e.Actor.ID)
		}
	}

	if findingCount != 1 {
		t.Fatalf("finding entry count = %d, want 1", findingCount)
	}
}

func TestRunIngestFindings_DefaultsTraceIDToSessionID(t *testing.T) {
	t.Parallel()

	signingKey := testutil.TestSigningKeyBase64(t)
	tmp := t.TempDir()
	evidenceDir := filepath.Join(tmp, "evidence")
	scannerReport, err := filepath.Abs("../../tests/testdata/sarif_trivy.json")
	if err != nil {
		t.Fatalf("resolve scanner report path: %v", err)
	}

	var out, errBuf bytes.Buffer
	code := run([]string{
		"ingest-findings",
		"--sarif", scannerReport,
		"--session-id", "session-findings-1",
		"--evidence-dir", evidenceDir,
		"--signing-key", signingKey,
	}, &out, &errBuf)
	if code != 0 {
		t.Fatalf("ingest-findings exit %d: %s", code, errBuf.String())
	}

	entries, err := evidence.ReadAllEntriesAtPath(evidenceDir)
	if err != nil {
		t.Fatalf("ReadAllEntriesAtPath: %v", err)
	}

	findingCount := 0
	for _, e := range entries {
		if e.Type != evidence.EntryTypeFinding {
			continue
		}
		findingCount++
		if e.SessionID != "session-findings-1" {
			t.Fatalf("finding session_id=%q, want session-findings-1", e.SessionID)
		}
		if e.TraceID != "session-findings-1" {
			t.Fatalf("finding trace_id=%q, want session-findings-1", e.TraceID)
		}
	}
	if findingCount == 0 {
		t.Fatal("expected at least one finding entry")
	}
}

func TestRunPrescribe_WithSigningKey(t *testing.T) {
	t.Parallel()

	// Generate Ed25519 key pair.
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}

	tmp := t.TempDir()

	// Write private key PEM.
	der, err := x509.MarshalPKCS8PrivateKey(priv)
	if err != nil {
		t.Fatalf("marshal PKCS8: %v", err)
	}
	privPEM := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der})
	privPath := filepath.Join(tmp, "key.pem")
	if err := os.WriteFile(privPath, privPEM, 0o600); err != nil {
		t.Fatalf("write private key: %v", err)
	}

	// Write public key PEM.
	pubDER, err := x509.MarshalPKIXPublicKey(pub)
	if err != nil {
		t.Fatalf("marshal public key: %v", err)
	}
	pubPEM := pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: pubDER})
	pubPath := filepath.Join(tmp, "pub.pem")
	if err := os.WriteFile(pubPath, pubPEM, 0o644); err != nil {
		t.Fatalf("write public key: %v", err)
	}

	// Write artifact.
	artifact := filepath.Join(tmp, "artifact.json")
	if err := os.WriteFile(artifact, []byte(`{"noop":true}`), 0o644); err != nil {
		t.Fatalf("write artifact: %v", err)
	}

	evidenceDir := filepath.Join(tmp, "evidence")

	// Run prescribe with --signing-key-path.
	args := []string{
		"prescribe",
		"--tool", "terraform",
		"--artifact", artifact,
		"--canonical-action", testCanonicalAction,
		"--signing-key-path", privPath,
		"--evidence-dir", evidenceDir,
	}

	var out, errBuf bytes.Buffer
	code := run(args, &out, &errBuf)
	if code != 0 {
		t.Fatalf("prescribe exit code = %d, stderr = %s", code, errBuf.String())
	}

	var result map[string]interface{}
	if err := json.Unmarshal(out.Bytes(), &result); err != nil {
		t.Fatalf("decode prescribe output: %v", err)
	}
	if result["ok"] != true {
		t.Fatalf("prescribe result not ok: %v", result)
	}

	// Verify the evidence entry has a non-empty Signature.
	entries, err := evidence.ReadAllEntriesAtPath(evidenceDir)
	if err != nil {
		t.Fatalf("ReadAllEntriesAtPath: %v", err)
	}
	if len(entries) == 0 {
		t.Fatal("no evidence entries found")
	}

	prescriptionFound := false
	for _, e := range entries {
		if e.Type == evidence.EntryTypePrescribe {
			prescriptionFound = true
			if e.Signature == "" {
				t.Fatal("prescription entry has empty Signature, expected non-empty when signing key provided")
			}
		}
	}
	if !prescriptionFound {
		t.Fatal("no prescription entry found in evidence")
	}

	// Run validate with --public-key and verify it succeeds.
	var valOut, valErr bytes.Buffer
	valCode := run([]string{
		"validate",
		"--evidence-dir", evidenceDir,
		"--public-key", pubPath,
	}, &valOut, &valErr)
	if valCode != 0 {
		t.Fatalf("validate exit code = %d, stderr = %s", valCode, valErr.String())
	}
	if !strings.Contains(valOut.String(), "signatures verified") {
		t.Fatalf("validate output missing 'signatures verified': %s", valOut.String())
	}
}

func TestScorecard_SessionIDFilter(t *testing.T) {
	t.Parallel()
	signingKey := testutil.TestSigningKeyBase64(t)

	tmp := t.TempDir()
	artifact := filepath.Join(tmp, "artifact.json")
	if err := os.WriteFile(artifact, []byte(`{"noop":true}`), 0o644); err != nil {
		t.Fatalf("write artifact: %v", err)
	}
	evidenceDir := filepath.Join(tmp, "evidence")

	// Prescribe + report in session-A.
	var out, errBuf bytes.Buffer
	code := run([]string{
		"prescribe",
		"--tool", "terraform",
		"--artifact", artifact,
		"--canonical-action", testCanonicalAction,
		"--session-id", "session-A",
		"--evidence-dir", evidenceDir,
		"--signing-key", signingKey,
	}, &out, &errBuf)
	if code != 0 {
		t.Fatalf("prescribe session-A exit %d: %s", code, errBuf.String())
	}
	var prescA map[string]interface{}
	if err := json.Unmarshal(out.Bytes(), &prescA); err != nil {
		t.Fatalf("decode prescribe A: %v", err)
	}
	prescIDA := prescA["prescription_id"].(string)

	out.Reset()
	errBuf.Reset()
	code = run([]string{
		"report",
		"--prescription", prescIDA,
		"--exit-code", "0",
		"--session-id", "session-A",
		"--evidence-dir", evidenceDir,
		"--signing-key", signingKey,
	}, &out, &errBuf)
	if code != 0 {
		t.Fatalf("report session-A exit %d: %s", code, errBuf.String())
	}

	// Prescribe + report in session-B.
	out.Reset()
	errBuf.Reset()
	code = run([]string{
		"prescribe",
		"--tool", "terraform",
		"--artifact", artifact,
		"--canonical-action", testCanonicalAction,
		"--session-id", "session-B",
		"--evidence-dir", evidenceDir,
		"--signing-key", signingKey,
	}, &out, &errBuf)
	if code != 0 {
		t.Fatalf("prescribe session-B exit %d: %s", code, errBuf.String())
	}
	var prescB map[string]interface{}
	if err := json.Unmarshal(out.Bytes(), &prescB); err != nil {
		t.Fatalf("decode prescribe B: %v", err)
	}
	prescIDB := prescB["prescription_id"].(string)

	out.Reset()
	errBuf.Reset()
	code = run([]string{
		"report",
		"--prescription", prescIDB,
		"--exit-code", "0",
		"--session-id", "session-B",
		"--evidence-dir", evidenceDir,
		"--signing-key", signingKey,
	}, &out, &errBuf)
	if code != 0 {
		t.Fatalf("report session-B exit %d: %s", code, errBuf.String())
	}

	// Run scorecard filtered to session-A only.
	out.Reset()
	errBuf.Reset()
	code = run([]string{
		"scorecard",
		"--session-id", "session-A",
		"--evidence-dir", evidenceDir,
	}, &out, &errBuf)
	if code != 0 {
		t.Fatalf("scorecard exit %d: %s", code, errBuf.String())
	}

	var sc map[string]interface{}
	if err := json.Unmarshal(out.Bytes(), &sc); err != nil {
		t.Fatalf("decode scorecard: %v", err)
	}
	totalOps := int(sc["total_operations"].(float64))
	if totalOps != 1 {
		t.Fatalf("total_operations = %d, want 1 (session filter should exclude session-B)", totalOps)
	}
	if sid, ok := sc["session_id"]; !ok || sid != "session-A" {
		t.Fatalf("session_id = %v, want session-A", sid)
	}
}

func TestResolveSigner_OptionalWithoutKey(t *testing.T) {
	t.Setenv("EVIDRA_SIGNING_KEY", "")
	t.Setenv("EVIDRA_SIGNING_KEY_PATH", "")
	s, err := resolveSigner("", "", "optional")
	if err != nil {
		t.Fatalf("resolveSigner(optional): %v", err)
	}
	if s == nil {
		t.Fatal("expected signer in optional mode")
	}
}

func TestResolveSigner_StrictWithoutKeyFails(t *testing.T) {
	t.Setenv("EVIDRA_SIGNING_KEY", "")
	t.Setenv("EVIDRA_SIGNING_KEY_PATH", "")
	if _, err := resolveSigner("", "", "strict"); err == nil {
		t.Fatal("expected strict mode error when no key configured")
	}
}

func TestRunPrescribe_OptionalSigningModeWithoutKey(t *testing.T) {
	t.Setenv("EVIDRA_SIGNING_KEY", "")
	t.Setenv("EVIDRA_SIGNING_KEY_PATH", "")

	tmp := t.TempDir()
	artifact := filepath.Join(tmp, "artifact.json")
	if err := os.WriteFile(artifact, []byte(`{"noop":true}`), 0o644); err != nil {
		t.Fatalf("write artifact: %v", err)
	}

	var out, errBuf bytes.Buffer
	code := run([]string{
		"prescribe",
		"--tool", "terraform",
		"--artifact", artifact,
		"--canonical-action", testCanonicalAction,
		"--signing-mode", "optional",
		"--evidence-dir", filepath.Join(tmp, "evidence"),
	}, &out, &errBuf)
	if code != 0 {
		t.Fatalf("prescribe optional mode failed: code=%d stderr=%s", code, errBuf.String())
	}
}

func TestRunPrescribe_BestEffortWriteModeSuppressesStoreError(t *testing.T) {
	t.Setenv("EVIDRA_EVIDENCE_WRITE_MODE", "best_effort")

	signingKey := testutil.TestSigningKeyBase64(t)
	tmp := t.TempDir()
	artifact := filepath.Join(tmp, "artifact.json")
	if err := os.WriteFile(artifact, []byte(`{"noop":true}`), 0o644); err != nil {
		t.Fatalf("write artifact: %v", err)
	}

	// Existing file path forces legacy-store code path and write failure in strict mode.
	evidencePath := filepath.Join(tmp, "legacy.log")
	if err := os.WriteFile(evidencePath, []byte("legacy"), 0o644); err != nil {
		t.Fatalf("write evidence file: %v", err)
	}

	var out, errBuf bytes.Buffer
	code := run([]string{
		"prescribe",
		"--tool", "terraform",
		"--artifact", artifact,
		"--canonical-action", testCanonicalAction,
		"--evidence-dir", evidencePath,
		"--signing-key", signingKey,
	}, &out, &errBuf)
	if code != 0 {
		t.Fatalf("prescribe in best_effort mode failed: code=%d stderr=%s", code, errBuf.String())
	}

	var result map[string]interface{}
	if err := json.Unmarshal(out.Bytes(), &result); err != nil {
		t.Fatalf("decode output: %v", err)
	}
	if result["ok"] != true {
		t.Fatalf("result not ok: %#v", result)
	}
}

func TestRunPrescribe_InvalidEvidenceWriteModeFails(t *testing.T) {
	t.Setenv("EVIDRA_EVIDENCE_WRITE_MODE", "invalid")

	signingKey := testutil.TestSigningKeyBase64(t)
	tmp := t.TempDir()
	artifact := filepath.Join(tmp, "artifact.json")
	if err := os.WriteFile(artifact, []byte(`{"noop":true}`), 0o644); err != nil {
		t.Fatalf("write artifact: %v", err)
	}

	var out, errBuf bytes.Buffer
	code := run([]string{
		"prescribe",
		"--tool", "terraform",
		"--artifact", artifact,
		"--canonical-action", testCanonicalAction,
		"--evidence-dir", filepath.Join(tmp, "evidence"),
		"--signing-key", signingKey,
	}, &out, &errBuf)
	if code == 0 {
		t.Fatalf("expected non-zero exit; stdout=%s", out.String())
	}
	if !strings.Contains(errBuf.String(), "resolve evidence write mode") {
		t.Fatalf("stderr missing write mode error: %s", errBuf.String())
	}
}

func TestRunReport_DerivesSessionFromPrescriptionWhenOmitted(t *testing.T) {
	t.Parallel()
	signingKey := testutil.TestSigningKeyBase64(t)

	tmp := t.TempDir()
	artifact := filepath.Join(tmp, "artifact.json")
	if err := os.WriteFile(artifact, []byte(`{"noop":true}`), 0o644); err != nil {
		t.Fatalf("write artifact: %v", err)
	}
	evidenceDir := filepath.Join(tmp, "evidence")

	var out, errBuf bytes.Buffer
	code := run([]string{
		"prescribe",
		"--tool", "terraform",
		"--artifact", artifact,
		"--canonical-action", testCanonicalAction,
		"--session-id", "session-presc",
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
	prescriptionID, ok := presc["prescription_id"].(string)
	if !ok || prescriptionID == "" {
		t.Fatalf("invalid prescription_id: %#v", presc["prescription_id"])
	}

	out.Reset()
	errBuf.Reset()
	code = run([]string{
		"report",
		"--prescription", prescriptionID,
		"--exit-code", "0",
		"--evidence-dir", evidenceDir,
		"--signing-key", signingKey,
	}, &out, &errBuf)
	if code != 0 {
		t.Fatalf("report exit %d: %s", code, errBuf.String())
	}

	var report map[string]interface{}
	if err := json.Unmarshal(out.Bytes(), &report); err != nil {
		t.Fatalf("decode report output: %v", err)
	}
	reportID, ok := report["report_id"].(string)
	if !ok || reportID == "" {
		t.Fatalf("invalid report_id: %#v", report["report_id"])
	}

	reportEntry, found, err := evidence.FindEntryByID(evidenceDir, reportID)
	if err != nil {
		t.Fatalf("FindEntryByID report: %v", err)
	}
	if !found {
		t.Fatalf("report entry %s not found", reportID)
	}
	if reportEntry.SessionID != "session-presc" {
		t.Fatalf("report session_id=%q, want session-presc", reportEntry.SessionID)
	}
}

func TestRunReport_SessionMismatchFails(t *testing.T) {
	t.Parallel()
	signingKey := testutil.TestSigningKeyBase64(t)

	tmp := t.TempDir()
	artifact := filepath.Join(tmp, "artifact.json")
	if err := os.WriteFile(artifact, []byte(`{"noop":true}`), 0o644); err != nil {
		t.Fatalf("write artifact: %v", err)
	}
	evidenceDir := filepath.Join(tmp, "evidence")

	var out, errBuf bytes.Buffer
	code := run([]string{
		"prescribe",
		"--tool", "terraform",
		"--artifact", artifact,
		"--canonical-action", testCanonicalAction,
		"--session-id", "session-A",
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
	prescriptionID := presc["prescription_id"].(string)

	out.Reset()
	errBuf.Reset()
	code = run([]string{
		"report",
		"--prescription", prescriptionID,
		"--exit-code", "0",
		"--session-id", "session-B",
		"--evidence-dir", evidenceDir,
		"--signing-key", signingKey,
	}, &out, &errBuf)
	if code == 0 {
		t.Fatalf("expected non-zero exit for session mismatch, got 0; stdout=%s", out.String())
	}
	if !strings.Contains(errBuf.String(), "does not match prescription session_id") {
		t.Fatalf("stderr missing session mismatch message: %s", errBuf.String())
	}

	entries, err := evidence.ReadAllEntriesAtPath(evidenceDir)
	if err != nil {
		t.Fatalf("ReadAllEntriesAtPath: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("entry count=%d, want 1 (report must not be written)", len(entries))
	}
}
