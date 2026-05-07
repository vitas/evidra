package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"samebits.com/evidra/internal/testutil"
	"samebits.com/evidra/pkg/evidence"
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

func TestPrescribePersistsExtendedActorMetadata(t *testing.T) {
	t.Parallel()

	signingKey := testutil.TestSigningKeyBase64(t)
	tmp := t.TempDir()
	artifactPath := filepath.Join(tmp, "artifact.yaml")
	evidenceDir := filepath.Join(tmp, "evidence")
	if err := os.WriteFile(artifactPath, []byte("apiVersion: v1\nkind: ConfigMap\nmetadata:\n  name: prescribe-actor-metadata\n"), 0o644); err != nil {
		t.Fatalf("write artifact: %v", err)
	}

	var out, errBuf bytes.Buffer
	code := run([]string{
		"prescribe",
		"--artifact", artifactPath,
		"--tool", "kubectl",
		"--operation", "apply",
		"--actor", "demo-agent",
		"--actor-type", "agent",
		"--actor-origin", "mcp-stdio",
		"--actor-instance-id", "session-123",
		"--actor-version", "claude-sonnet-4.5",
		"--actor-skill-version", "1.0.1",
		"--signing-key", signingKey,
		"--evidence-dir", evidenceDir,
	}, &out, &errBuf)
	if code != 0 {
		t.Fatalf("prescribe exit=%d stderr=%s", code, errBuf.String())
	}

	entries, err := evidence.ReadAllEntriesAtPath(evidenceDir)
	if err != nil {
		t.Fatalf("ReadAllEntriesAtPath: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("entry count = %d, want 1", len(entries))
	}
	if entries[0].Actor.Type != "agent" {
		t.Fatalf("actor.type = %q", entries[0].Actor.Type)
	}
	if entries[0].Actor.ID != "demo-agent" {
		t.Fatalf("actor.id = %q", entries[0].Actor.ID)
	}
	if entries[0].Actor.Provenance != "mcp-stdio" {
		t.Fatalf("actor.provenance = %q", entries[0].Actor.Provenance)
	}
	if entries[0].Actor.InstanceID != "session-123" {
		t.Fatalf("actor.instance_id = %q", entries[0].Actor.InstanceID)
	}
	if entries[0].Actor.Version != "claude-sonnet-4.5" {
		t.Fatalf("actor.version = %q", entries[0].Actor.Version)
	}
	if entries[0].Actor.SkillVersion != "1.0.1" {
		t.Fatalf("actor.skill_version = %q", entries[0].Actor.SkillVersion)
	}
}
