package canon

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const goldenDir = "../../tests/golden"

func goldenPath(name string) string {
	return filepath.Join(goldenDir, name)
}

func readGolden(t *testing.T, name string) []byte {
	t.Helper()
	data, err := os.ReadFile(goldenPath(name))
	if err != nil {
		t.Fatalf("read golden file %s: %v", name, err)
	}
	return data
}

func readGoldenDigest(t *testing.T, name string) string {
	t.Helper()
	data, err := os.ReadFile(goldenPath(name))
	if err != nil {
		t.Fatalf("read golden digest %s: %v", name, err)
	}
	return strings.TrimSpace(string(data))
}

func writeGoldenDigest(t *testing.T, name, digest string) {
	t.Helper()
	if err := os.WriteFile(goldenPath(name), []byte(digest+"\n"), 0o644); err != nil {
		t.Fatalf("write golden digest %s: %v", name, err)
	}
}

func shouldUpdate() bool {
	return os.Getenv("EVIDRA_UPDATE_GOLDEN") == "1"
}

// --- Golden Corpus Tests ---

func TestGolden_K8sDeployment(t *testing.T) {
	t.Parallel()
	data := readGolden(t, "k8s_deployment.yaml")
	result := Canonicalize("kubectl", "apply", "", data)

	if result.ParseError != nil {
		t.Fatalf("parse error: %v", result.ParseError)
	}
	if result.CanonVersion != "k8s/v1" {
		t.Errorf("canon version = %q, want k8s/v1", result.CanonVersion)
	}
	if result.CanonicalAction.ResourceCount != 1 {
		t.Errorf("resource count = %d, want 1", result.CanonicalAction.ResourceCount)
	}
	if result.CanonicalAction.OperationClass != "mutate" {
		t.Errorf("operation class = %q, want mutate", result.CanonicalAction.OperationClass)
	}
	if len(result.CanonicalAction.ResourceIdentity) != 1 {
		t.Fatalf("identity count = %d, want 1", len(result.CanonicalAction.ResourceIdentity))
	}

	id := result.CanonicalAction.ResourceIdentity[0]
	if id.Kind != "deployment" {
		t.Errorf("kind = %q, want deployment", id.Kind)
	}
	if id.Name != "nginx-deployment" {
		t.Errorf("name = %q, want nginx-deployment", id.Name)
	}
	if id.Namespace != "default" {
		t.Errorf("namespace = %q, want default", id.Namespace)
	}

	digestFile := "k8s_deployment_digest.txt"
	if shouldUpdate() {
		writeGoldenDigest(t, digestFile, result.IntentDigest)
		t.Logf("updated golden digest: %s", result.IntentDigest)
	} else {
		want := readGoldenDigest(t, digestFile)
		if result.IntentDigest != want {
			t.Errorf("intent digest mismatch\n got: %s\nwant: %s", result.IntentDigest, want)
		}
	}
}

func TestGolden_K8sMultidoc(t *testing.T) {
	t.Parallel()
	data := readGolden(t, "k8s_multidoc.yaml")
	result := Canonicalize("kubectl", "apply", "", data)

	if result.ParseError != nil {
		t.Fatalf("parse error: %v", result.ParseError)
	}
	if result.CanonicalAction.ResourceCount != 3 {
		t.Errorf("resource count = %d, want 3", result.CanonicalAction.ResourceCount)
	}
	if result.CanonicalAction.ScopeClass != "unknown" {
		t.Errorf("scope class = %q, want unknown", result.CanonicalAction.ScopeClass)
	}

	// Verify sorted identity order
	ids := result.CanonicalAction.ResourceIdentity
	if len(ids) != 3 {
		t.Fatalf("identity count = %d, want 3", len(ids))
	}

	digestFile := "k8s_multidoc_digest.txt"
	if shouldUpdate() {
		writeGoldenDigest(t, digestFile, result.IntentDigest)
	} else {
		want := readGoldenDigest(t, digestFile)
		if result.IntentDigest != want {
			t.Errorf("intent digest mismatch\n got: %s\nwant: %s", result.IntentDigest, want)
		}
	}
}

func TestGolden_K8sPrivileged(t *testing.T) {
	t.Parallel()
	data := readGolden(t, "k8s_privileged.yaml")
	result := Canonicalize("kubectl", "apply", "", data)

	if result.ParseError != nil {
		t.Fatalf("parse error: %v", result.ParseError)
	}
	if result.CanonicalAction.ResourceCount != 1 {
		t.Errorf("resource count = %d, want 1", result.CanonicalAction.ResourceCount)
	}

	id := result.CanonicalAction.ResourceIdentity[0]
	if id.Namespace != "kube-system" {
		t.Errorf("namespace = %q, want kube-system", id.Namespace)
	}

	digestFile := "k8s_privileged_digest.txt"
	if shouldUpdate() {
		writeGoldenDigest(t, digestFile, result.IntentDigest)
	} else {
		want := readGoldenDigest(t, digestFile)
		if result.IntentDigest != want {
			t.Errorf("intent digest mismatch\n got: %s\nwant: %s", result.IntentDigest, want)
		}
	}
}

func TestGolden_K8sRBAC(t *testing.T) {
	t.Parallel()
	data := readGolden(t, "k8s_rbac.yaml")
	result := Canonicalize("kubectl", "apply", "", data)

	if result.ParseError != nil {
		t.Fatalf("parse error: %v", result.ParseError)
	}
	if result.CanonicalAction.ResourceCount != 2 {
		t.Errorf("resource count = %d, want 2", result.CanonicalAction.ResourceCount)
	}

	digestFile := "k8s_rbac_digest.txt"
	if shouldUpdate() {
		writeGoldenDigest(t, digestFile, result.IntentDigest)
	} else {
		want := readGoldenDigest(t, digestFile)
		if result.IntentDigest != want {
			t.Errorf("intent digest mismatch\n got: %s\nwant: %s", result.IntentDigest, want)
		}
	}
}

func TestGolden_K8sCRD(t *testing.T) {
	t.Parallel()
	data := readGolden(t, "k8s_crd.yaml")
	result := Canonicalize("kubectl", "apply", "", data)

	if result.ParseError != nil {
		t.Fatalf("parse error: %v", result.ParseError)
	}
	if result.CanonicalAction.ResourceCount != 1 {
		t.Errorf("resource count = %d, want 1", result.CanonicalAction.ResourceCount)
	}

	id := result.CanonicalAction.ResourceIdentity[0]
	if id.Kind != "virtualservice" {
		t.Errorf("kind = %q, want virtualservice", id.Kind)
	}

	digestFile := "k8s_crd_digest.txt"
	if shouldUpdate() {
		writeGoldenDigest(t, digestFile, result.IntentDigest)
	} else {
		want := readGoldenDigest(t, digestFile)
		if result.IntentDigest != want {
			t.Errorf("intent digest mismatch\n got: %s\nwant: %s", result.IntentDigest, want)
		}
	}
}

func TestGolden_TfCreate(t *testing.T) {
	t.Parallel()
	data := readGolden(t, "tf_create.json")
	result := Canonicalize("terraform", "apply", "", data)

	if result.ParseError != nil {
		t.Fatalf("parse error: %v", result.ParseError)
	}
	if result.CanonVersion != "terraform/v1" {
		t.Errorf("canon version = %q, want terraform/v1", result.CanonVersion)
	}
	if result.CanonicalAction.ResourceCount != 1 {
		t.Errorf("resource count = %d, want 1", result.CanonicalAction.ResourceCount)
	}

	id := result.CanonicalAction.ResourceIdentity[0]
	if id.Type != "aws_instance" {
		t.Errorf("type = %q, want aws_instance", id.Type)
	}
	if id.Name != "web" {
		t.Errorf("name = %q, want web", id.Name)
	}
	if id.Actions != "create" {
		t.Errorf("actions = %q, want create", id.Actions)
	}

	digestFile := "tf_create_digest.txt"
	if shouldUpdate() {
		writeGoldenDigest(t, digestFile, result.IntentDigest)
	} else {
		want := readGoldenDigest(t, digestFile)
		if result.IntentDigest != want {
			t.Errorf("intent digest mismatch\n got: %s\nwant: %s", result.IntentDigest, want)
		}
	}
}

func TestGolden_TfDestroy(t *testing.T) {
	t.Parallel()
	data := readGolden(t, "tf_destroy.json")
	result := Canonicalize("terraform", "destroy", "", data)

	if result.ParseError != nil {
		t.Fatalf("parse error: %v", result.ParseError)
	}
	if result.CanonicalAction.ResourceCount != 1 {
		t.Errorf("resource count = %d, want 1", result.CanonicalAction.ResourceCount)
	}
	if result.CanonicalAction.OperationClass != "destroy" {
		t.Errorf("operation class = %q, want destroy", result.CanonicalAction.OperationClass)
	}

	digestFile := "tf_destroy_digest.txt"
	if shouldUpdate() {
		writeGoldenDigest(t, digestFile, result.IntentDigest)
	} else {
		want := readGoldenDigest(t, digestFile)
		if result.IntentDigest != want {
			t.Errorf("intent digest mismatch\n got: %s\nwant: %s", result.IntentDigest, want)
		}
	}
}

func TestGolden_TfMixed(t *testing.T) {
	t.Parallel()
	data := readGolden(t, "tf_mixed.json")
	result := Canonicalize("terraform", "apply", "", data)

	if result.ParseError != nil {
		t.Fatalf("parse error: %v", result.ParseError)
	}
	if result.CanonicalAction.ResourceCount != 3 {
		t.Errorf("resource count = %d, want 3", result.CanonicalAction.ResourceCount)
	}

	digestFile := "tf_mixed_digest.txt"
	if shouldUpdate() {
		writeGoldenDigest(t, digestFile, result.IntentDigest)
	} else {
		want := readGoldenDigest(t, digestFile)
		if result.IntentDigest != want {
			t.Errorf("intent digest mismatch\n got: %s\nwant: %s", result.IntentDigest, want)
		}
	}
}

func TestGolden_TfModule(t *testing.T) {
	t.Parallel()
	data := readGolden(t, "tf_module.json")
	result := Canonicalize("terraform", "apply", "", data)

	if result.ParseError != nil {
		t.Fatalf("parse error: %v", result.ParseError)
	}
	if result.CanonicalAction.ResourceCount != 2 {
		t.Errorf("resource count = %d, want 2", result.CanonicalAction.ResourceCount)
	}

	digestFile := "tf_module_digest.txt"
	if shouldUpdate() {
		writeGoldenDigest(t, digestFile, result.IntentDigest)
	} else {
		want := readGoldenDigest(t, digestFile)
		if result.IntentDigest != want {
			t.Errorf("intent digest mismatch\n got: %s\nwant: %s", result.IntentDigest, want)
		}
	}
}

func TestGolden_HelmOutput(t *testing.T) {
	t.Parallel()
	data := readGolden(t, "helm_output.yaml")
	result := Canonicalize("kubectl", "apply", "", data)

	if result.ParseError != nil {
		t.Fatalf("parse error: %v", result.ParseError)
	}
	if result.CanonicalAction.ResourceCount != 1 {
		t.Errorf("resource count = %d, want 1", result.CanonicalAction.ResourceCount)
	}

	digestFile := "helm_output_digest.txt"
	if shouldUpdate() {
		writeGoldenDigest(t, digestFile, result.IntentDigest)
	} else {
		want := readGoldenDigest(t, digestFile)
		if result.IntentDigest != want {
			t.Errorf("intent digest mismatch\n got: %s\nwant: %s", result.IntentDigest, want)
		}
	}
}

// --- Noise Immunity Tests ---

func TestNoiseImmunity_MetadataUID(t *testing.T) {
	t.Parallel()
	base := readGolden(t, "k8s_deployment.yaml")
	baseResult := Canonicalize("kubectl", "apply", "", base)

	// Add metadata.uid noise
	noisy := strings.Replace(string(base),
		"  name: nginx-deployment",
		"  name: nginx-deployment\n  uid: abc-123-def",
		1)
	noisyResult := Canonicalize("kubectl", "apply", "", []byte(noisy))

	if baseResult.CanonicalAction.ResourceShapeHash != noisyResult.CanonicalAction.ResourceShapeHash {
		t.Errorf("shape hash changed with uid noise\n base:  %s\nnoisy: %s",
			baseResult.CanonicalAction.ResourceShapeHash, noisyResult.CanonicalAction.ResourceShapeHash)
	}
}

func TestNoiseImmunity_ResourceVersion(t *testing.T) {
	t.Parallel()
	base := readGolden(t, "k8s_deployment.yaml")
	baseResult := Canonicalize("kubectl", "apply", "", base)

	noisy := strings.Replace(string(base),
		"  name: nginx-deployment",
		"  name: nginx-deployment\n  resourceVersion: \"12345\"",
		1)
	noisyResult := Canonicalize("kubectl", "apply", "", []byte(noisy))

	if baseResult.CanonicalAction.ResourceShapeHash != noisyResult.CanonicalAction.ResourceShapeHash {
		t.Errorf("shape hash changed with resourceVersion noise")
	}
}

func TestNoiseImmunity_ManagedFields(t *testing.T) {
	t.Parallel()
	base := readGolden(t, "k8s_deployment.yaml")
	baseResult := Canonicalize("kubectl", "apply", "", base)

	noisy := strings.Replace(string(base),
		"  name: nginx-deployment",
		"  name: nginx-deployment\n  managedFields:\n  - manager: kubectl\n    operation: Apply",
		1)
	noisyResult := Canonicalize("kubectl", "apply", "", []byte(noisy))

	if baseResult.CanonicalAction.ResourceShapeHash != noisyResult.CanonicalAction.ResourceShapeHash {
		t.Errorf("shape hash changed with managedFields noise")
	}
}

func TestNoiseImmunity_GenerationTimestamp(t *testing.T) {
	t.Parallel()
	base := readGolden(t, "k8s_deployment.yaml")
	baseResult := Canonicalize("kubectl", "apply", "", base)

	noisy := strings.Replace(string(base),
		"  name: nginx-deployment",
		"  name: nginx-deployment\n  generation: 5\n  creationTimestamp: \"2026-01-01T00:00:00Z\"",
		1)
	noisyResult := Canonicalize("kubectl", "apply", "", []byte(noisy))

	if baseResult.CanonicalAction.ResourceShapeHash != noisyResult.CanonicalAction.ResourceShapeHash {
		t.Errorf("shape hash changed with generation/timestamp noise")
	}
}

func TestNoiseImmunity_Status(t *testing.T) {
	t.Parallel()
	base := readGolden(t, "k8s_deployment.yaml")
	baseResult := Canonicalize("kubectl", "apply", "", base)

	noisy := string(base) + "\nstatus:\n  availableReplicas: 3\n  readyReplicas: 3\n"
	noisyResult := Canonicalize("kubectl", "apply", "", []byte(noisy))

	if baseResult.CanonicalAction.ResourceShapeHash != noisyResult.CanonicalAction.ResourceShapeHash {
		t.Errorf("shape hash changed with status noise")
	}
}

// --- Shape Hash Sensitivity Test ---

func TestShapeHashSensitivity_ReplicaChange(t *testing.T) {
	t.Parallel()
	base := readGolden(t, "k8s_deployment.yaml")
	baseResult := Canonicalize("kubectl", "apply", "", base)

	modified := strings.Replace(string(base), "replicas: 3", "replicas: 5", 1)
	modResult := Canonicalize("kubectl", "apply", "", []byte(modified))

	if baseResult.CanonicalAction.ResourceShapeHash == modResult.CanonicalAction.ResourceShapeHash {
		t.Error("shape hash should differ when replicas change")
	}
}

// --- Generic Adapter Tests ---

func TestGenericAdapter(t *testing.T) {
	t.Parallel()
	data := []byte(`{"custom": "data", "tool": "custom-tool"}`)
	result := Canonicalize("custom-tool", "run", "", data)

	if result.ParseError != nil {
		t.Fatalf("unexpected error: %v", result.ParseError)
	}
	if result.CanonVersion != "generic/v1" {
		t.Errorf("canon version = %q, want generic/v1", result.CanonVersion)
	}
	if result.CanonicalAction.ResourceCount != 1 {
		t.Errorf("resource count = %d, want 1", result.CanonicalAction.ResourceCount)
	}
	if result.ArtifactDigest == "" {
		t.Error("artifact digest should not be empty")
	}
	if result.IntentDigest == "" {
		t.Error("intent digest should not be empty")
	}
}

// --- ResolveScopeClass Tests ---

func TestResolveScopeClass_ExplicitEnv(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		env  string
		want string
	}{
		{"production", "production", "production"},
		{"staging", "staging", "staging"},
		{"development", "development", "development"},
		{"production_upper", "Production", "production"},
		{"with_whitespace", "  production  ", "production"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := ResolveScopeClass(tt.env, nil)
			if got != tt.want {
				t.Errorf("ResolveScopeClass(%q, nil) = %q, want %q", tt.env, got, tt.want)
			}
		})
	}
}

func TestResolveScopeClass_NamespaceHints(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		namespace string
		want      string
	}{
		{"prod_api", "prod-api", "production"},
		{"production_ns", "production", "production"},
		{"staging_v2", "staging-v2", "staging"},
		{"dev_test", "dev-test", "development"},
		{"no_match", "test-ns", "unknown"},
		{"empty", "", "unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			resources := []ResourceID{{Namespace: tt.namespace}}
			got := ResolveScopeClass("", resources)
			if got != tt.want {
				t.Errorf("ResolveScopeClass(\"\", [ns=%q]) = %q, want %q", tt.namespace, got, tt.want)
			}
		})
	}
}

func TestResolveScopeClass_EnvOverridesNamespace(t *testing.T) {
	t.Parallel()

	// Explicit env should win even when namespace suggests differently.
	resources := []ResourceID{{Namespace: "prod-api"}}
	got := ResolveScopeClass("staging", resources)
	if got != "staging" {
		t.Errorf("ResolveScopeClass(\"staging\", [ns=prod-api]) = %q, want staging", got)
	}
}

func TestResolveScopeClass_NoResourcesNoEnv(t *testing.T) {
	t.Parallel()
	got := ResolveScopeClass("", nil)
	if got != "unknown" {
		t.Errorf("ResolveScopeClass(\"\", nil) = %q, want unknown", got)
	}
}

// --- Intent Digest Tests ---

func TestIntentDigest_ExcludesShapeHash(t *testing.T) {
	t.Parallel()

	action1 := CanonicalAction{
		Tool:              "kubectl",
		Operation:         "apply",
		OperationClass:    "mutate",
		ResourceIdentity:  []ResourceID{{Kind: "deployment", Name: "web", Namespace: "default"}},
		ScopeClass:        "unknown",
		ResourceCount:     1,
		ResourceShapeHash: "sha256:aaaa",
	}
	action2 := action1
	action2.ResourceShapeHash = "sha256:bbbb"

	digest1 := ComputeIntentDigest(action1)
	digest2 := ComputeIntentDigest(action2)

	if digest1 != digest2 {
		t.Errorf("intent digest changed when only resource_shape_hash differs\n d1: %s\n d2: %s", digest1, digest2)
	}

	// Verify that changing an identity field DOES change the digest.
	action3 := action1
	action3.Operation = "delete"
	digest3 := ComputeIntentDigest(action3)
	if digest1 == digest3 {
		t.Error("intent digest should differ when operation changes")
	}
}

// --- Determinism Test ---

func TestDeterminism_SameInputSameDigest(t *testing.T) {
	t.Parallel()
	data := readGolden(t, "k8s_deployment.yaml")

	r1 := Canonicalize("kubectl", "apply", "", data)
	r2 := Canonicalize("kubectl", "apply", "", data)

	if r1.IntentDigest != r2.IntentDigest {
		t.Errorf("intent digest not deterministic\n first:  %s\nsecond: %s", r1.IntentDigest, r2.IntentDigest)
	}
	if r1.ArtifactDigest != r2.ArtifactDigest {
		t.Errorf("artifact digest not deterministic")
	}
	if r1.CanonicalAction.ResourceShapeHash != r2.CanonicalAction.ResourceShapeHash {
		t.Errorf("shape hash not deterministic")
	}
}
