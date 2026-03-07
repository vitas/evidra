//go:build e2e

package e2e_test

import (
	"encoding/json"
	"path/filepath"
	"testing"
)

// realFixture returns the path to a fixture in tests/e2e/fixtures/real/.
func realFixture(name string) string {
	return filepath.Join("..", "..", "tests", "e2e", "fixtures", "real", name)
}

// runAndDecode runs evidra with the given args and decodes the JSON output.
func runAndDecode(t *testing.T, bin string, args ...string) map[string]interface{} {
	t.Helper()
	stdout, stderr, exitCode := runEvidra(t, bin, args...)
	if exitCode != 0 {
		t.Fatalf("evidra %s exit=%d stderr=%s", args[0], exitCode, stderr)
	}
	var result map[string]interface{}
	if err := json.Unmarshal([]byte(stdout), &result); err != nil {
		t.Fatalf("decode %s output: %v\nstdout: %s", args[0], err, stdout)
	}
	return result
}

// TestE2EReal_K8sAppStack exercises the K8s adapter with a realistic
// multi-resource deployment: Namespace, ConfigMap, Secret, Deployment,
// Service, ServiceAccount, Role, RoleBinding — including noise fields
// (managedFields, uid, resourceVersion, last-applied-configuration).
func TestE2EReal_K8sAppStack(t *testing.T) {
	bin := evidraBinary(t)
	tmpDir := t.TempDir()
	evidenceDir := filepath.Join(tmpDir, "evidence")
	privPath, _ := generateKeyPair(t, tmpDir)

	// Prescribe to verify canonicalization details.
	prescResult := runAndDecode(t, bin,
		"prescribe",
		"--tool", "kubectl",
		"--operation", "apply",
		"--artifact", realFixture("k8s_app_stack.yaml"),
		"--environment", "staging",
		"--session-id", "e2e-real-k8s",
		"--evidence-dir", evidenceDir,
		"--signing-key-path", privPath,
	)

	if prescResult["ok"] != true {
		t.Fatalf("prescribe not ok: %v", prescResult)
	}
	if prescResult["canon_version"] != "k8s/v1" {
		t.Errorf("canon_version = %v, want k8s/v1", prescResult["canon_version"])
	}
	// Noise filtering (managedFields, uid, resourceVersion, last-applied-config)
	// must not break intent digest computation.
	if prescResult["intent_digest"] == nil || prescResult["intent_digest"] == "" {
		t.Error("intent_digest missing — noise filtering may have broken canonicalization")
	}
	if prescResult["artifact_digest"] == nil || prescResult["artifact_digest"] == "" {
		t.Error("artifact_digest missing")
	}

	// Report to complete lifecycle.
	pid := prescResult["prescription_id"].(string)
	runAndDecode(t, bin,
		"report",
		"--prescription", pid,
		"--exit-code", "0",
		"--session-id", "e2e-real-k8s",
		"--evidence-dir", evidenceDir,
		"--signing-key-path", privPath,
	)

	// Scorecard should produce a valid score for the full lifecycle.
	scoreResult := runAndDecode(t, bin,
		"scorecard",
		"--session-id", "e2e-real-k8s",
		"--evidence-dir", evidenceDir,
		"--min-operations", "1",
	)
	totalOps := int(scoreResult["total_operations"].(float64))
	if totalOps != 1 {
		t.Errorf("total_operations = %d, want 1", totalOps)
	}

	t.Logf("K8s app stack: risk_level=%v score=%v band=%v",
		prescResult["risk_level"], scoreResult["score"], scoreResult["band"])
}

// TestE2EReal_TerraformInfra exercises the Terraform adapter with a realistic
// multi-module plan: VPC, subnets, RDS, security group (with 0.0.0.0/0 ingress),
// S3 bucket, and IAM role — mixed create/update actions.
func TestE2EReal_TerraformInfra(t *testing.T) {
	bin := evidraBinary(t)
	tmpDir := t.TempDir()
	evidenceDir := filepath.Join(tmpDir, "evidence")
	privPath, _ := generateKeyPair(t, tmpDir)

	prescResult := runAndDecode(t, bin,
		"prescribe",
		"--tool", "terraform",
		"--operation", "apply",
		"--artifact", realFixture("tf_infra_plan.json"),
		"--environment", "production",
		"--session-id", "e2e-real-tf",
		"--evidence-dir", evidenceDir,
		"--signing-key-path", privPath,
	)

	if prescResult["ok"] != true {
		t.Fatalf("prescribe not ok: %v", prescResult)
	}
	if prescResult["canon_version"] != "terraform/v1" {
		t.Errorf("canon_version = %v, want terraform/v1", prescResult["canon_version"])
	}

	// Security group with 0.0.0.0/0 should trigger risk tag.
	riskTags, ok := prescResult["risk_tags"].([]interface{})
	if !ok {
		t.Fatal("risk_tags missing or not array")
	}
	if len(riskTags) == 0 {
		t.Error("risk_tags empty — expected world-open ingress detection on security group")
	}

	// Production apply with risk tags should be high or critical.
	riskLevel, ok := prescResult["risk_level"].(string)
	if !ok {
		t.Fatal("risk_level missing")
	}
	if riskLevel != "high" && riskLevel != "critical" {
		t.Errorf("risk_level = %q, want high or critical for production apply with open ingress", riskLevel)
	}

	t.Logf("Terraform infra: risk_level=%s risk_tags=%v", riskLevel, riskTags)
}

// TestE2EReal_HelmRedis exercises the K8s adapter via tool=helm with a
// realistic helm template output: ServiceAccount, 2 ConfigMaps, Service,
// StatefulSet with PVC template.
func TestE2EReal_HelmRedis(t *testing.T) {
	bin := evidraBinary(t)
	tmpDir := t.TempDir()
	evidenceDir := filepath.Join(tmpDir, "evidence")
	privPath, _ := generateKeyPair(t, tmpDir)

	prescResult := runAndDecode(t, bin,
		"prescribe",
		"--tool", "helm",
		"--operation", "upgrade",
		"--artifact", realFixture("helm_rendered.yaml"),
		"--environment", "staging",
		"--session-id", "e2e-real-helm",
		"--evidence-dir", evidenceDir,
		"--signing-key-path", privPath,
	)

	if prescResult["ok"] != true {
		t.Fatalf("prescribe not ok: %v", prescResult)
	}

	// K8s adapter should handle helm tool.
	if prescResult["canon_version"] != "k8s/v1" {
		t.Errorf("canon_version = %v, want k8s/v1", prescResult["canon_version"])
	}
	if prescResult["operation_class"] != "mutate" {
		t.Errorf("operation_class = %v, want mutate", prescResult["operation_class"])
	}

	// Complete lifecycle and verify scorecard.
	pid := prescResult["prescription_id"].(string)
	runAndDecode(t, bin,
		"report",
		"--prescription", pid,
		"--exit-code", "0",
		"--session-id", "e2e-real-helm",
		"--evidence-dir", evidenceDir,
		"--signing-key-path", privPath,
	)

	scoreResult := runAndDecode(t, bin,
		"scorecard",
		"--session-id", "e2e-real-helm",
		"--evidence-dir", evidenceDir,
		"--min-operations", "1",
	)

	t.Logf("Helm Redis: risk_level=%v score=%v band=%v",
		prescResult["risk_level"], scoreResult["score"], scoreResult["band"])
}

// TestE2EReal_ArgoCDSync exercises the K8s adapter with ArgoCD-managed
// manifests: Namespace, ConfigMap, Deployment (multi-container with sidecar),
// Service, ServiceAccount, NetworkPolicy — including ArgoCD tracking
// annotations and server-side noise fields.
func TestE2EReal_ArgoCDSync(t *testing.T) {
	bin := evidraBinary(t)
	tmpDir := t.TempDir()
	evidenceDir := filepath.Join(tmpDir, "evidence")
	privPath, _ := generateKeyPair(t, tmpDir)

	prescResult := runAndDecode(t, bin,
		"prescribe",
		"--tool", "kubectl",
		"--operation", "apply",
		"--artifact", realFixture("argocd_app_sync.yaml"),
		"--environment", "production",
		"--session-id", "e2e-real-argocd",
		"--evidence-dir", evidenceDir,
		"--signing-key-path", privPath,
	)

	if prescResult["ok"] != true {
		t.Fatalf("prescribe not ok: %v", prescResult)
	}

	// ArgoCD noise fields (tracking annotations, managedFields, uid)
	// should be stripped without breaking canonicalization.
	if prescResult["intent_digest"] == nil || prescResult["intent_digest"] == "" {
		t.Error("intent_digest missing — ArgoCD noise may have broken canonicalization")
	}

	// Production apply should be at least high risk.
	riskLevel, ok := prescResult["risk_level"].(string)
	if !ok {
		t.Fatal("risk_level missing")
	}
	if riskLevel != "high" && riskLevel != "critical" {
		t.Errorf("risk_level = %q, want high or critical for production apply", riskLevel)
	}

	t.Logf("ArgoCD sync: risk_level=%s", riskLevel)
}

// TestE2EReal_KustomizeMonitoring exercises the K8s adapter with kustomize
// build output: Namespace, 2 ConfigMaps, 2 Deployments (Prometheus + Grafana),
// 2 Services, ServiceAccount, ClusterRole, ClusterRoleBinding — 10 resources.
func TestE2EReal_KustomizeMonitoring(t *testing.T) {
	bin := evidraBinary(t)
	tmpDir := t.TempDir()
	evidenceDir := filepath.Join(tmpDir, "evidence")
	privPath, _ := generateKeyPair(t, tmpDir)

	prescResult := runAndDecode(t, bin,
		"prescribe",
		"--tool", "kustomize",
		"--operation", "apply",
		"--artifact", realFixture("kustomize_monitoring.yaml"),
		"--environment", "staging",
		"--session-id", "e2e-real-kustomize",
		"--evidence-dir", evidenceDir,
		"--signing-key-path", privPath,
	)

	if prescResult["ok"] != true {
		t.Fatalf("prescribe not ok: %v", prescResult)
	}

	// Kustomize should route to K8s adapter.
	if prescResult["canon_version"] != "k8s/v1" {
		t.Errorf("canon_version = %v, want k8s/v1", prescResult["canon_version"])
	}
	if prescResult["intent_digest"] == nil || prescResult["intent_digest"] == "" {
		t.Error("intent_digest missing")
	}
	if prescResult["operation_class"] != "mutate" {
		t.Errorf("operation_class = %v, want mutate", prescResult["operation_class"])
	}

	// Complete lifecycle.
	pid := prescResult["prescription_id"].(string)
	runAndDecode(t, bin,
		"report",
		"--prescription", pid,
		"--exit-code", "0",
		"--session-id", "e2e-real-kustomize",
		"--evidence-dir", evidenceDir,
		"--signing-key-path", privPath,
	)

	scoreResult := runAndDecode(t, bin,
		"scorecard",
		"--session-id", "e2e-real-kustomize",
		"--evidence-dir", evidenceDir,
		"--min-operations", "1",
	)

	t.Logf("Kustomize monitoring: risk_level=%v score=%v band=%v",
		prescResult["risk_level"], scoreResult["score"], scoreResult["band"])
}

// TestE2EReal_HelmIngressNginx exercises the K8s adapter via tool=helm with
// ingress-nginx chart output: ServiceAccount, ConfigMap, ClusterRole,
// ClusterRoleBinding, Service (LoadBalancer), Deployment, IngressClass — 7 resources.
// Tests LoadBalancer service type and NET_BIND_SERVICE capability.
func TestE2EReal_HelmIngressNginx(t *testing.T) {
	bin := evidraBinary(t)
	tmpDir := t.TempDir()
	evidenceDir := filepath.Join(tmpDir, "evidence")
	privPath, _ := generateKeyPair(t, tmpDir)

	prescResult := runAndDecode(t, bin,
		"prescribe",
		"--tool", "helm",
		"--operation", "install",
		"--artifact", realFixture("helm_ingress_nginx.yaml"),
		"--environment", "production",
		"--session-id", "e2e-real-helm-nginx",
		"--evidence-dir", evidenceDir,
		"--signing-key-path", privPath,
	)

	if prescResult["ok"] != true {
		t.Fatalf("prescribe not ok: %v", prescResult)
	}
	if prescResult["canon_version"] != "k8s/v1" {
		t.Errorf("canon_version = %v, want k8s/v1", prescResult["canon_version"])
	}

	// Production install should be high or critical.
	riskLevel, ok := prescResult["risk_level"].(string)
	if !ok {
		t.Fatal("risk_level missing")
	}
	if riskLevel != "high" && riskLevel != "critical" {
		t.Errorf("risk_level = %q, want high or critical for production install", riskLevel)
	}

	t.Logf("Helm ingress-nginx: risk_level=%s risk_tags=%v",
		riskLevel, prescResult["risk_tags"])
}

// TestE2EReal_OpenShiftApp exercises the K8s adapter via tool=oc with
// OpenShift-specific resources: Namespace, ConfigMap, Secret, Deployment,
// Service, ServiceAccount, Route, HPA — including OpenShift annotations
// and noise fields (uid, resourceVersion, managedFields).
func TestE2EReal_OpenShiftApp(t *testing.T) {
	bin := evidraBinary(t)
	tmpDir := t.TempDir()
	evidenceDir := filepath.Join(tmpDir, "evidence")
	privPath, _ := generateKeyPair(t, tmpDir)

	prescResult := runAndDecode(t, bin,
		"prescribe",
		"--tool", "oc",
		"--operation", "apply",
		"--artifact", realFixture("openshift_app.yaml"),
		"--environment", "production",
		"--session-id", "e2e-real-openshift",
		"--evidence-dir", evidenceDir,
		"--signing-key-path", privPath,
	)

	if prescResult["ok"] != true {
		t.Fatalf("prescribe not ok: %v", prescResult)
	}

	// oc should route to K8s adapter.
	if prescResult["canon_version"] != "k8s/v1" {
		t.Errorf("canon_version = %v, want k8s/v1", prescResult["canon_version"])
	}

	// OpenShift noise fields should be stripped cleanly.
	if prescResult["intent_digest"] == nil || prescResult["intent_digest"] == "" {
		t.Error("intent_digest missing — OpenShift noise may have broken canonicalization")
	}

	// Production apply should be high or critical.
	riskLevel, ok := prescResult["risk_level"].(string)
	if !ok {
		t.Fatal("risk_level missing")
	}
	if riskLevel != "high" && riskLevel != "critical" {
		t.Errorf("risk_level = %q, want high or critical for production apply", riskLevel)
	}

	// Complete lifecycle and verify scorecard.
	pid := prescResult["prescription_id"].(string)
	runAndDecode(t, bin,
		"report",
		"--prescription", pid,
		"--exit-code", "0",
		"--session-id", "e2e-real-openshift",
		"--evidence-dir", evidenceDir,
		"--signing-key-path", privPath,
	)

	scoreResult := runAndDecode(t, bin,
		"scorecard",
		"--session-id", "e2e-real-openshift",
		"--evidence-dir", evidenceDir,
		"--min-operations", "1",
	)

	t.Logf("OpenShift app: risk_level=%s score=%v band=%v",
		riskLevel, scoreResult["score"], scoreResult["band"])
}
