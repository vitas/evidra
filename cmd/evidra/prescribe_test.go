package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"samebits.com/evidra-benchmark/internal/testutil"
)

func TestPrescribeSupportsArtifactShortFlag(t *testing.T) {
	t.Parallel()

	signingKey := testutil.TestSigningKeyBase64(t)
	tmp := t.TempDir()
	artifactPath := filepath.Join(tmp, "artifact.yaml")
	if err := os.WriteFile(artifactPath, []byte("apiVersion: v1\nkind: ConfigMap\nmetadata:\n  name: prescribe-short-flag\n"), 0o644); err != nil {
		t.Fatalf("write artifact: %v", err)
	}

	var out, errBuf bytes.Buffer
	code := run([]string{
		"prescribe",
		"-f", artifactPath,
		"--tool", "kubectl",
		"--operation", "apply",
		"--signing-key", signingKey,
		"--evidence-dir", filepath.Join(tmp, "evidence"),
	}, &out, &errBuf)
	if code != 0 {
		t.Fatalf("prescribe -f exit=%d stderr=%s", code, errBuf.String())
	}

	var result map[string]interface{}
	if err := json.Unmarshal(out.Bytes(), &result); err != nil {
		t.Fatalf("decode prescribe output: %v", err)
	}
	if result["ok"] != true {
		t.Fatalf("prescribe result not ok: %#v", result)
	}
}
