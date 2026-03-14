package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"samebits.com/evidra/internal/testutil"
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
	if _, ok := result["risk_inputs"]; !ok {
		t.Fatalf("missing risk_inputs: %#v", result)
	}
	if _, ok := result["effective_risk"]; !ok {
		t.Fatalf("missing effective_risk: %#v", result)
	}
	if _, ok := result["risk_level"]; ok {
		t.Fatalf("risk_level must not be present: %#v", result)
	}
	if _, ok := result["risk_tags"]; ok {
		t.Fatalf("risk_tags must not be present: %#v", result)
	}
}
