package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"samebits.com/evidra-benchmark/internal/canon"
	"samebits.com/evidra-benchmark/internal/testutil"
	"samebits.com/evidra-benchmark/pkg/evidence"
)

func TestRecordCommandExecutesCommandAndReportsOutcome(t *testing.T) {
	t.Parallel()

	signingKey := testutil.TestSigningKeyBase64(t)
	tmp := t.TempDir()
	evidenceDir := filepath.Join(tmp, "evidence")
	artifactPath := filepath.Join(tmp, "artifact.yaml")
	if err := os.WriteFile(artifactPath, []byte("apiVersion: v1\nkind: ConfigMap\nmetadata:\n  name: run-cm\n"), 0o644); err != nil {
		t.Fatalf("write artifact: %v", err)
	}

	var out, errBuf bytes.Buffer
	code := run([]string{
		"record",
		"--tool", "kubectl",
		"--operation", "apply",
		"--artifact", artifactPath,
		"--environment", "staging",
		"--evidence-dir", evidenceDir,
		"--signing-key", signingKey,
		"--", "sh", "-c", "exit 0",
	}, &out, &errBuf)
	if code != 0 {
		t.Fatalf("record exit=%d stderr=%s", code, errBuf.String())
	}

	var result map[string]interface{}
	if err := json.Unmarshal(out.Bytes(), &result); err != nil {
		t.Fatalf("decode output: %v", err)
	}
	if got := int(result["exit_code"].(float64)); got != 0 {
		t.Fatalf("result exit_code=%d want 0", got)
	}
	for _, key := range []string{"session_id", "operation_id", "prescription_id", "report_id"} {
		if _, ok := result[key]; !ok {
			t.Fatalf("missing key %q in result: %#v", key, result)
		}
	}

	entries, err := evidence.ReadAllEntriesAtPath(evidenceDir)
	if err != nil {
		t.Fatalf("ReadAllEntriesAtPath: %v", err)
	}
	prescribes := 0
	reports := 0
	for _, e := range entries {
		switch e.Type {
		case evidence.EntryTypePrescribe:
			prescribes++
		case evidence.EntryTypeReport:
			reports++
		}
	}
	if prescribes != 1 || reports != 1 {
		t.Fatalf("entries prescribe=%d report=%d want 1/1", prescribes, reports)
	}
}

func TestRecordOutput_ContainsFirstUsefulOutputFields(t *testing.T) {
	t.Parallel()

	signingKey := testutil.TestSigningKeyBase64(t)
	tmp := t.TempDir()
	evidenceDir := filepath.Join(tmp, "evidence")
	artifactPath := filepath.Join(tmp, "artifact.yaml")
	if err := os.WriteFile(artifactPath, []byte("apiVersion: v1\nkind: ConfigMap\nmetadata:\n  name: run-first-use\n"), 0o644); err != nil {
		t.Fatalf("write artifact: %v", err)
	}

	var out, errBuf bytes.Buffer
	code := run([]string{
		"record",
		"--tool", "kubectl",
		"--operation", "apply",
		"--artifact", artifactPath,
		"--environment", "staging",
		"--evidence-dir", evidenceDir,
		"--signing-key", signingKey,
		"--", "sh", "-c", "exit 0",
	}, &out, &errBuf)
	if code != 0 {
		t.Fatalf("record exit=%d stderr=%s", code, errBuf.String())
	}

	var result map[string]interface{}
	if err := json.Unmarshal(out.Bytes(), &result); err != nil {
		t.Fatalf("decode output: %v", err)
	}

	for _, key := range []string{
		"risk_level",
		"score",
		"score_band",
		"signal_summary",
		"basis",
		"confidence",
	} {
		if _, ok := result[key]; !ok {
			t.Fatalf("missing first-use field %q in output: %#v", key, result)
		}
	}
	if _, ok := result["risk_classification"]; ok {
		t.Fatalf("risk_classification must not be present: %#v", result)
	}

	basis, ok := result["basis"].(map[string]interface{})
	if !ok {
		t.Fatalf("basis should be object, got: %#v", result["basis"])
	}
	if _, ok := basis["assessment_mode"]; !ok {
		t.Fatalf("basis missing assessment_mode: %#v", basis)
	}
}

func TestRecordCommandFailOpenOnMetricsExportError(t *testing.T) {
	// Not parallel: mutates package-level emitRecordMetricsHook.
	signingKey := testutil.TestSigningKeyBase64(t)
	tmp := t.TempDir()
	evidenceDir := filepath.Join(tmp, "evidence")
	artifactPath := filepath.Join(tmp, "artifact.yaml")
	if err := os.WriteFile(artifactPath, []byte("apiVersion: v1\nkind: ConfigMap\nmetadata:\n  name: run-cm-2\n"), 0o644); err != nil {
		t.Fatalf("write artifact: %v", err)
	}

	originalHook := emitRecordMetricsHook
	emitRecordMetricsHook = func(context.Context, operationMetricsPayload) error {
		return errors.New("metrics unavailable")
	}
	defer func() {
		emitRecordMetricsHook = originalHook
	}()

	var out, errBuf bytes.Buffer
	code := run([]string{
		"record",
		"--tool", "kubectl",
		"--operation", "apply",
		"--artifact", artifactPath,
		"--environment", "staging",
		"--evidence-dir", evidenceDir,
		"--signing-key", signingKey,
		"--", "sh", "-c", "exit 7",
	}, &out, &errBuf)
	if code != 7 {
		t.Fatalf("record exit=%d want 7 (stderr=%s)", code, errBuf.String())
	}
	if !strings.Contains(errBuf.String(), "metrics export failed") {
		t.Fatalf("stderr should contain metrics warning, got: %s", errBuf.String())
	}

	var result map[string]interface{}
	if err := json.Unmarshal(out.Bytes(), &result); err != nil {
		t.Fatalf("decode output: %v", err)
	}
	if got := int(result["exit_code"].(float64)); got != 7 {
		t.Fatalf("result exit_code=%d want 7", got)
	}
}

func TestRecordCommandSupportsCompactKubectlInvocation(t *testing.T) {
	signingKey := testutil.TestSigningKeyBase64(t)
	tmp := t.TempDir()
	evidenceDir := filepath.Join(tmp, "evidence")
	artifactPath := filepath.Join(tmp, "deploy.yaml")
	if err := os.WriteFile(artifactPath, []byte("apiVersion: v1\nkind: ConfigMap\nmetadata:\n  name: compact-kubectl\n"), 0o644); err != nil {
		t.Fatalf("write artifact: %v", err)
	}

	stubDir := filepath.Join(tmp, "bin")
	mustInstallStubCommand(t, stubDir, "kubectl")

	var out, errBuf bytes.Buffer
	code := run([]string{
		"record",
		"-f", artifactPath,
		"--evidence-dir", evidenceDir,
		"--signing-key", signingKey,
		"--", "kubectl", "apply", "-f", artifactPath,
	}, &out, &errBuf)
	if code != 0 {
		t.Fatalf("record compact kubectl exit=%d stderr=%s", code, errBuf.String())
	}

	action := readPrescribedAction(t, evidenceDir)
	if action.Tool != "kubectl" || action.Operation != "apply" {
		t.Fatalf("inferred action = %#v, want kubectl/apply", action)
	}
}

func TestRecordCommandInfersArgoCDOperationWithoutArtifact(t *testing.T) {
	signingKey := testutil.TestSigningKeyBase64(t)
	tmp := t.TempDir()
	evidenceDir := filepath.Join(tmp, "evidence")

	stubDir := filepath.Join(tmp, "bin")
	mustInstallStubCommand(t, stubDir, "argocd")

	var out, errBuf bytes.Buffer
	code := run([]string{
		"record",
		"--evidence-dir", evidenceDir,
		"--signing-key", signingKey,
		"--", "argocd", "app", "sync", "myapp",
	}, &out, &errBuf)
	if code != 0 {
		t.Fatalf("record argocd exit=%d stderr=%s", code, errBuf.String())
	}

	action := readPrescribedAction(t, evidenceDir)
	if action.Tool != "argocd" || action.Operation != "sync" {
		t.Fatalf("inferred action = %#v, want argocd/sync", action)
	}
}

func TestRecordCommandSupportsCompactOCInvocation(t *testing.T) {
	signingKey := testutil.TestSigningKeyBase64(t)
	tmp := t.TempDir()
	evidenceDir := filepath.Join(tmp, "evidence")
	artifactPath := filepath.Join(tmp, "deploy.yaml")
	if err := os.WriteFile(artifactPath, []byte("apiVersion: v1\nkind: ConfigMap\nmetadata:\n  name: compact-oc\n"), 0o644); err != nil {
		t.Fatalf("write artifact: %v", err)
	}

	stubDir := filepath.Join(tmp, "bin")
	mustInstallStubCommand(t, stubDir, "oc")

	var out, errBuf bytes.Buffer
	code := run([]string{
		"record",
		"-f", artifactPath,
		"--evidence-dir", evidenceDir,
		"--signing-key", signingKey,
		"--", "oc", "apply", "-f", artifactPath,
	}, &out, &errBuf)
	if code != 0 {
		t.Fatalf("record compact oc exit=%d stderr=%s", code, errBuf.String())
	}

	action := readPrescribedAction(t, evidenceDir)
	if action.Tool != "oc" || action.Operation != "apply" {
		t.Fatalf("inferred action = %#v, want oc/apply", action)
	}
}

func TestRecordCommandInfersDockerComposeOperation(t *testing.T) {
	signingKey := testutil.TestSigningKeyBase64(t)
	tmp := t.TempDir()
	evidenceDir := filepath.Join(tmp, "evidence")

	stubDir := filepath.Join(tmp, "bin")
	mustInstallStubCommand(t, stubDir, "docker")

	var out, errBuf bytes.Buffer
	code := run([]string{
		"record",
		"--evidence-dir", evidenceDir,
		"--signing-key", signingKey,
		"--", "docker", "compose", "up", "-d",
	}, &out, &errBuf)
	if code != 0 {
		t.Fatalf("record docker compose exit=%d stderr=%s", code, errBuf.String())
	}

	action := readPrescribedAction(t, evidenceDir)
	if action.Tool != "docker" || action.Operation != "up" {
		t.Fatalf("inferred action = %#v, want docker/up", action)
	}
}

func TestRecordCommandRejectsUnknownWrappedTool(t *testing.T) {
	t.Parallel()

	var out, errBuf bytes.Buffer
	code := run([]string{
		"record",
		"--", "sometool", "do-stuff",
	}, &out, &errBuf)
	if code != 2 {
		t.Fatalf("record unknown tool exit=%d want 2 stderr=%s", code, errBuf.String())
	}
	if !strings.Contains(errBuf.String(), "unknown tool 'sometool'") {
		t.Fatalf("stderr should mention unknown tool, got: %s", errBuf.String())
	}
}

func TestRecordCommandRejectsUnsupportedKnownToolPattern(t *testing.T) {
	t.Parallel()

	var out, errBuf bytes.Buffer
	code := run([]string{
		"record",
		"--", "argocd", "version",
	}, &out, &errBuf)
	if code != 2 {
		t.Fatalf("record unsupported pattern exit=%d want 2 stderr=%s", code, errBuf.String())
	}
	if !strings.Contains(errBuf.String(), "please specify --operation explicitly") {
		t.Fatalf("stderr should request explicit operation, got: %s", errBuf.String())
	}
}

func TestRecordCommandRejectsShellWrappedInferenceWithoutExplicitFlags(t *testing.T) {
	t.Parallel()

	var out, errBuf bytes.Buffer
	code := run([]string{
		"record",
		"--", "sh", "-c", "kustomize build . | kubectl apply -f -",
	}, &out, &errBuf)
	if code != 2 {
		t.Fatalf("record shell wrapper exit=%d want 2 stderr=%s", code, errBuf.String())
	}
	if !strings.Contains(errBuf.String(), "please specify --tool and --operation explicitly") {
		t.Fatalf("stderr should request explicit tool/operation, got: %s", errBuf.String())
	}
}

func TestRecordCommandRequiresWrappedCommandSeparator(t *testing.T) {
	t.Parallel()

	var out, errBuf bytes.Buffer
	code := run([]string{
		"record",
		"--tool", "kubectl",
		"--operation", "apply",
		"--artifact", "deploy.yaml",
	}, &out, &errBuf)
	if code != 2 {
		t.Fatalf("record missing separator exit=%d want 2 stderr=%s", code, errBuf.String())
	}
	if !strings.Contains(errBuf.String(), "record requires '--' before wrapped command") {
		t.Fatalf("stderr should mention missing '--', got: %s", errBuf.String())
	}
}

func readPrescribedAction(t *testing.T, evidenceDir string) canon.CanonicalAction {
	t.Helper()

	entries, err := evidence.ReadAllEntriesAtPath(evidenceDir)
	if err != nil {
		t.Fatalf("ReadAllEntriesAtPath: %v", err)
	}

	for _, entry := range entries {
		if entry.Type != evidence.EntryTypePrescribe {
			continue
		}

		var payload evidence.PrescriptionPayload
		if err := json.Unmarshal(entry.Payload, &payload); err != nil {
			t.Fatalf("decode prescription payload: %v", err)
		}

		var action canon.CanonicalAction
		if err := json.Unmarshal(payload.CanonicalAction, &action); err != nil {
			t.Fatalf("decode canonical action: %v", err)
		}
		return action
	}

	t.Fatal("no prescribe entry found")
	return canon.CanonicalAction{}
}

func mustInstallStubCommand(t *testing.T, dir, name string) {
	t.Helper()

	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir stub dir: %v", err)
	}
	scriptPath := filepath.Join(dir, name)
	if err := os.WriteFile(scriptPath, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatalf("write stub command: %v", err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
}
