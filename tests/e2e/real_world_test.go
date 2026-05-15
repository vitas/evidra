//go:build e2e

package e2e_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"samebits.com/evidra/pkg/evidence"
	testcli "samebits.com/evidra/tests/testutil"
)

// fixturePath returns the path to a shared vendored fixture.
func fixturePath(family, name string) string {
	return filepath.Join("..", "..", "tests", "artifacts", "fixtures", family, name)
}

// runAndDecode runs evidra with the given args and decodes the JSON output.
func runAndDecode(t *testing.T, bin string, args ...string) map[string]interface{} {
	t.Helper()
	stdout, stderr, exitCode := testcli.RunEvidra(t, bin, args...)
	if exitCode != 0 {
		t.Fatalf("evidra %s exit=%d stderr=%s", args[0], exitCode, stderr)
	}
	var result map[string]interface{}
	if err := json.Unmarshal([]byte(stdout), &result); err != nil {
		t.Fatalf("decode %s output: %v\nstdout: %s", args[0], err, stdout)
	}
	return result
}

type prescribeEvidence struct {
	entry   evidence.EvidenceEntry
	payload evidence.PrescriptionPayload
}

// extractPrescription reads evidence and returns the first prescribe entry.
func extractPrescription(t *testing.T, evidenceDir string) prescribeEvidence {
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
		return prescribeEvidence{entry: entry, payload: payload}
	}
	t.Fatal("no prescribe entry found in evidence")
	return prescribeEvidence{}
}

func runIntentOnlyPrescribe(t *testing.T, bin, evidenceDir, privPath, tool, operation, artifact, environment, sessionID string) (map[string]interface{}, prescribeEvidence) {
	t.Helper()
	output := runAndDecode(t, bin,
		"prescribe",
		"--tool", tool,
		"--operation", operation,
		"--artifact", artifact,
		"--environment", environment,
		"--session-id", sessionID,
		"--evidence-dir", evidenceDir,
		"--signing-key-path", privPath,
	)
	rec := extractPrescription(t, evidenceDir)
	assertIntentOnlyPrescription(t, rec, artifact, tool, operation)
	assertIntentOnlyOutput(t, output, rec)
	return output, rec
}

func assertIntentOnlyPrescription(t *testing.T, rec prescribeEvidence, artifact, tool, operation string) {
	t.Helper()
	payload := rec.payload
	entry := rec.entry
	if payload.PrescriptionID != entry.EntryID {
		t.Fatalf("payload prescription_id = %q, want entry id %q", payload.PrescriptionID, entry.EntryID)
	}
	if payload.Intent == nil {
		t.Fatal("intent is nil")
	}
	if payload.Intent.Tool != tool {
		t.Errorf("intent.tool = %q, want %q", payload.Intent.Tool, tool)
	}
	if payload.Intent.Operation != operation {
		t.Errorf("intent.operation = %q, want %q", payload.Intent.Operation, operation)
	}
	if payload.Intent.Target != artifact {
		t.Errorf("intent.target = %q, want %q", payload.Intent.Target, artifact)
	}

	raw, err := os.ReadFile(artifact)
	if err != nil {
		t.Fatalf("read artifact: %v", err)
	}
	wantArtifactDigest := evidence.SHA256Hex(raw)
	if payload.Intent.ArtifactDigest != wantArtifactDigest {
		t.Errorf("intent.artifact_digest = %q, want %q", payload.Intent.ArtifactDigest, wantArtifactDigest)
	}
	if entry.ArtifactDigest != wantArtifactDigest {
		t.Errorf("entry artifact_digest = %q, want %q", entry.ArtifactDigest, wantArtifactDigest)
	}

	wantIntentDigest := evidence.ComputeDeclaredIntentDigest(*payload.Intent)
	if entry.IntentDigest != wantIntentDigest {
		t.Errorf("entry intent_digest = %q, want %q", entry.IntentDigest, wantIntentDigest)
	}
	if len(payload.CanonicalAction) != 0 {
		t.Fatalf("canonical_action = %s, want absent by default", payload.CanonicalAction)
	}
	if payload.CanonSource != "" {
		t.Errorf("canon_source = %q, want empty without canonical_action", payload.CanonSource)
	}
	if entry.CanonVersion != "" {
		t.Errorf("entry canon_version = %q, want empty without canonical_action", entry.CanonVersion)
	}

	if payload.Assessment == nil {
		t.Fatal("assessment is nil")
	}
	if payload.Assessment.Status != evidence.AssessmentNotProvided {
		t.Errorf("assessment.status = %q, want %q", payload.Assessment.Status, evidence.AssessmentNotProvided)
	}
	if payload.Assessment.EffectiveRisk != "" {
		t.Errorf("assessment.effective_risk = %q, want empty", payload.Assessment.EffectiveRisk)
	}
	if len(payload.Assessment.RiskInputs) != 0 {
		t.Errorf("assessment.risk_inputs = %#v, want empty", payload.Assessment.RiskInputs)
	}
	if payload.EffectiveRisk != "" || payload.RiskLevel != "" || len(payload.RiskInputs) != 0 {
		t.Errorf("legacy risk fields = effective:%q risk_level:%q inputs:%#v; want empty", payload.EffectiveRisk, payload.RiskLevel, payload.RiskInputs)
	}
}

func assertIntentOnlyOutput(t *testing.T, output map[string]interface{}, rec prescribeEvidence) {
	t.Helper()
	assertStringField(t, output, "artifact_digest", rec.entry.ArtifactDigest)
	assertStringField(t, output, "intent_digest", rec.entry.IntentDigest)
	assertStringField(t, output, "effective_risk", "")
	assertStringField(t, output, "operation_class", "")
	assertStringField(t, output, "scope_class", "")
	assertStringField(t, output, "canon_version", "")

	rawRiskInputs, ok := output["risk_inputs"]
	if !ok {
		t.Fatal("missing risk_inputs field")
	}
	if rawRiskInputs == nil {
		return
	}
	inputs, ok := rawRiskInputs.([]interface{})
	if !ok {
		t.Fatalf("risk_inputs = %#v, want null or empty array", rawRiskInputs)
	}
	if len(inputs) != 0 {
		t.Fatalf("risk_inputs = %#v, want empty", inputs)
	}
}

func assertStringField(t *testing.T, output map[string]interface{}, field, want string) {
	t.Helper()
	got, ok := output[field].(string)
	if !ok {
		t.Fatalf("%s = %#v, want string %q", field, output[field], want)
	}
	if got != want {
		t.Fatalf("%s = %q, want %q", field, got, want)
	}
}

// TestE2EReal_K8sCorpusPromotion exercises the CLI against promoted OSS
// corpus fixtures. Core prescribe treats artifacts as opaque intent evidence;
// canonicalization and assessment are optional external enrichments.
func TestE2EReal_K8sCorpusPromotion(t *testing.T) {
	tests := []struct {
		name        string
		artifact    string
		environment string
	}{
		{
			name:        "hostpath fail",
			artifact:    fixturePath("k8s", "kubescape-hostpath-mount-fail.yaml"),
			environment: "staging",
		},
		{
			name:        "non-root pass",
			artifact:    fixturePath("k8s", "kubescape-non-root-deployment-pass.yaml"),
			environment: "staging",
		},
	}

	bin := testcli.EvidraBinary(t)
	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			tmpDir := t.TempDir()
			evidenceDir := filepath.Join(tmpDir, "evidence")
			privPath, _ := testcli.GenerateKeyPair(t, tmpDir)

			_, rec := runIntentOnlyPrescribe(t, bin, evidenceDir, privPath, "kubectl", "apply", tc.artifact, tc.environment, "e2e-real-k8s-corpus")
			t.Logf("K8s corpus fixture %s: intent_digest=%s artifact_digest=%s",
				tc.name, rec.entry.IntentDigest, rec.entry.ArtifactDigest)
		})
	}
}

// TestE2EReal_TerraformCorpusPromotion exercises the CLI against promoted
// Terraform corpus fixtures without forcing in-process canonicalization.
func TestE2EReal_TerraformCorpusPromotion(t *testing.T) {
	tests := []struct {
		name     string
		artifact string
	}{
		{
			name:     "s3 public access fail",
			artifact: fixturePath("terraform", "checkov-s3-public-access-fail.tfplan.json"),
		},
		{
			name:     "iam wildcard fail",
			artifact: fixturePath("terraform", "checkov-iam-wildcard-fail.tfplan.json"),
		},
	}

	bin := testcli.EvidraBinary(t)
	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			tmpDir := t.TempDir()
			evidenceDir := filepath.Join(tmpDir, "evidence")
			privPath, _ := testcli.GenerateKeyPair(t, tmpDir)

			_, rec := runIntentOnlyPrescribe(t, bin, evidenceDir, privPath, "terraform", "apply", tc.artifact, "staging", "e2e-real-tf-corpus")
			t.Logf("Terraform corpus fixture %s: intent_digest=%s artifact_digest=%s",
				tc.name, rec.entry.IntentDigest, rec.entry.ArtifactDigest)
		})
	}
}

// TestE2EReal_HelmRedis exercises intent-only prescribe with rendered Helm YAML.
func TestE2EReal_HelmRedis(t *testing.T) {
	bin := testcli.EvidraBinary(t)
	tmpDir := t.TempDir()
	evidenceDir := filepath.Join(tmpDir, "evidence")
	privPath, _ := testcli.GenerateKeyPair(t, tmpDir)

	artifact := fixturePath("helm", "helm_rendered.yaml")
	_, rec := runIntentOnlyPrescribe(t, bin, evidenceDir, privPath, "helm", "upgrade", artifact, "staging", "e2e-real-helm")

	t.Logf("Helm Redis: intent_digest=%s artifact_digest=%s", rec.entry.IntentDigest, rec.entry.ArtifactDigest)
}

// TestE2EReal_ArgoCDSync exercises intent-only prescribe with ArgoCD-managed
// manifests including tracking annotations and server-side noise.
func TestE2EReal_ArgoCDSync(t *testing.T) {
	bin := testcli.EvidraBinary(t)
	tmpDir := t.TempDir()
	privPath, _ := testcli.GenerateKeyPair(t, tmpDir)
	artifact := fixturePath("argocd", "argocd_app_sync.yaml")

	evidenceDir := filepath.Join(tmpDir, "evidence")
	_, rec := runIntentOnlyPrescribe(t, bin, evidenceDir, privPath, "kubectl", "apply", artifact, "production", "e2e-real-argocd")

	evidenceDir2 := filepath.Join(t.TempDir(), "evidence2")
	_, rec2 := runIntentOnlyPrescribe(t, bin, evidenceDir2, privPath, "kubectl", "apply", artifact, "production", "e2e-real-argocd-2")

	if rec.entry.IntentDigest != rec2.entry.IntentDigest {
		t.Errorf("intent_digest not stable across runs: %s vs %s", rec.entry.IntentDigest, rec2.entry.IntentDigest)
	}
	if rec.entry.ArtifactDigest != rec2.entry.ArtifactDigest {
		t.Errorf("artifact_digest not stable across runs: %s vs %s", rec.entry.ArtifactDigest, rec2.entry.ArtifactDigest)
	}

	t.Logf("ArgoCD sync: intent_digest=%s artifact_digest=%s", rec.entry.IntentDigest, rec.entry.ArtifactDigest)
}

// TestE2EReal_KustomizeMonitoring exercises intent-only prescribe with
// kustomize build output including ClusterRole/ClusterRoleBinding.
func TestE2EReal_KustomizeMonitoring(t *testing.T) {
	bin := testcli.EvidraBinary(t)
	tmpDir := t.TempDir()
	evidenceDir := filepath.Join(tmpDir, "evidence")
	privPath, _ := testcli.GenerateKeyPair(t, tmpDir)

	artifact := fixturePath("kustomize", "kustomize_monitoring.yaml")
	_, rec := runIntentOnlyPrescribe(t, bin, evidenceDir, privPath, "kustomize", "apply", artifact, "staging", "e2e-real-kustomize")

	t.Logf("Kustomize monitoring: intent_digest=%s artifact_digest=%s", rec.entry.IntentDigest, rec.entry.ArtifactDigest)
}

// TestE2EReal_HelmIngressNginx exercises intent-only prescribe with ingress-nginx
// chart output including LoadBalancer and capabilities.
func TestE2EReal_HelmIngressNginx(t *testing.T) {
	bin := testcli.EvidraBinary(t)
	tmpDir := t.TempDir()
	evidenceDir := filepath.Join(tmpDir, "evidence")
	privPath, _ := testcli.GenerateKeyPair(t, tmpDir)

	artifact := fixturePath("helm", "helm_ingress_nginx.yaml")
	_, rec := runIntentOnlyPrescribe(t, bin, evidenceDir, privPath, "helm", "install", artifact, "production", "e2e-real-helm-nginx")

	t.Logf("Helm ingress-nginx: intent_digest=%s artifact_digest=%s", rec.entry.IntentDigest, rec.entry.ArtifactDigest)
}

// TestE2EReal_OpenShiftApp exercises intent-only prescribe via tool=oc with
// OpenShift-specific resources: DeploymentConfig, BuildConfig, ImageStream, Route.
func TestE2EReal_OpenShiftApp(t *testing.T) {
	bin := testcli.EvidraBinary(t)
	tmpDir := t.TempDir()
	evidenceDir := filepath.Join(tmpDir, "evidence")
	privPath, _ := testcli.GenerateKeyPair(t, tmpDir)

	artifact := fixturePath("openshift", "openshift_app.yaml")
	_, rec := runIntentOnlyPrescribe(t, bin, evidenceDir, privPath, "oc", "apply", artifact, "production", "e2e-real-openshift")

	t.Logf("OpenShift app: intent_digest=%s artifact_digest=%s", rec.entry.IntentDigest, rec.entry.ArtifactDigest)
}

// TestE2EReal_NoiseImmunity verifies that repeated prescribe calls for the
// same real-world artifact keep stable declared intent and artifact digests.
func TestE2EReal_NoiseImmunity(t *testing.T) {
	bin := testcli.EvidraBinary(t)
	privPath, _ := testcli.GenerateKeyPair(t, t.TempDir())
	artifact := fixturePath("argocd", "argocd_app_sync.yaml")

	dir1 := filepath.Join(t.TempDir(), "evidence")
	_, rec1 := runIntentOnlyPrescribe(t, bin, dir1, privPath, "kubectl", "apply", artifact, "production", "e2e-noise-1")

	dir2 := filepath.Join(t.TempDir(), "evidence")
	_, rec2 := runIntentOnlyPrescribe(t, bin, dir2, privPath, "kubectl", "apply", artifact, "production", "e2e-noise-2")

	if rec1.entry.IntentDigest != rec2.entry.IntentDigest {
		t.Errorf("intent_digest not stable: %s vs %s", rec1.entry.IntentDigest, rec2.entry.IntentDigest)
	}
	if rec1.entry.ArtifactDigest != rec2.entry.ArtifactDigest {
		t.Errorf("artifact_digest not stable: %s vs %s", rec1.entry.ArtifactDigest, rec2.entry.ArtifactDigest)
	}
	if rec1.payload.Intent.ArtifactDigest != rec2.payload.Intent.ArtifactDigest {
		t.Errorf("payload artifact_digest not stable: %s vs %s", rec1.payload.Intent.ArtifactDigest, rec2.payload.Intent.ArtifactDigest)
	}
}
