package risk

import (
	"encoding/json"
	"testing"
)

// --- Matrix tests ---

func TestRiskLevel_KnownCombinations(t *testing.T) {
	t.Parallel()

	tests := []struct {
		opClass    string
		scopeClass string
		want       string
	}{
		// read: always low
		{"read", "single", "low"},
		{"read", "namespace", "low"},
		{"read", "cluster", "low"},
		{"read", "unknown", "low"},
		// mutate: low for single, medium otherwise
		{"mutate", "single", "low"},
		{"mutate", "namespace", "medium"},
		{"mutate", "cluster", "medium"},
		{"mutate", "unknown", "medium"},
		// destroy: medium for single/namespace, high for cluster/unknown
		{"destroy", "single", "medium"},
		{"destroy", "namespace", "medium"},
		{"destroy", "cluster", "high"},
		{"destroy", "unknown", "high"},
		// plan: always low
		{"plan", "single", "low"},
		{"plan", "namespace", "low"},
		{"plan", "cluster", "low"},
		{"plan", "unknown", "low"},
	}

	for _, tt := range tests {
		t.Run(tt.opClass+"_"+tt.scopeClass, func(t *testing.T) {
			t.Parallel()
			got := RiskLevel(tt.opClass, tt.scopeClass)
			if got != tt.want {
				t.Errorf("RiskLevel(%q, %q) = %q, want %q", tt.opClass, tt.scopeClass, got, tt.want)
			}
		})
	}
}

func TestRiskLevel_UnknownDefaultsHigh(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		opClass    string
		scopeClass string
	}{
		{"unknown_op", "nuke", "single"},
		{"unknown_scope", "mutate", "galaxy"},
		{"both_unknown", "nuke", "galaxy"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := RiskLevel(tt.opClass, tt.scopeClass)
			if got != "high" {
				t.Errorf("RiskLevel(%q, %q) = %q, want %q", tt.opClass, tt.scopeClass, got, "high")
			}
		})
	}
}

// --- K8s detector tests ---

func TestDetectPrivileged_Container(t *testing.T) {
	t.Parallel()

	yaml := []byte(`apiVersion: apps/v1
kind: Deployment
metadata:
  name: test
spec:
  template:
    spec:
      containers:
      - name: app
        image: nginx
        securityContext:
          privileged: true
`)
	tags := DetectPrivileged(yaml)
	assertContains(t, tags, "k8s.privileged_container")
}

func TestDetectPrivileged_InitContainer(t *testing.T) {
	t.Parallel()

	yaml := []byte(`apiVersion: v1
kind: Pod
metadata:
  name: test
spec:
  initContainers:
  - name: init
    image: busybox
    securityContext:
      privileged: true
  containers:
  - name: app
    image: nginx
`)
	tags := DetectPrivileged(yaml)
	assertContains(t, tags, "k8s.privileged_container")
}

func TestDetectPrivileged_Unprivileged(t *testing.T) {
	t.Parallel()

	yaml := []byte(`apiVersion: apps/v1
kind: Deployment
metadata:
  name: test
spec:
  template:
    spec:
      containers:
      - name: app
        image: nginx
        securityContext:
          privileged: false
`)
	tags := DetectPrivileged(yaml)
	assertEmpty(t, tags, "DetectPrivileged")
}

func TestDetectHostNamespace_HostPID(t *testing.T) {
	t.Parallel()

	yaml := []byte(`apiVersion: v1
kind: Pod
metadata:
  name: test
spec:
  hostPID: true
  containers:
  - name: app
    image: nginx
`)
	tags := DetectHostNamespace(yaml)
	assertContains(t, tags, "k8s.host_namespace_escape")
}

func TestDetectHostNamespace_HostNetwork(t *testing.T) {
	t.Parallel()

	yaml := []byte(`apiVersion: v1
kind: Pod
metadata:
  name: test
spec:
  hostNetwork: true
  containers:
  - name: app
    image: nginx
`)
	tags := DetectHostNamespace(yaml)
	assertContains(t, tags, "k8s.host_namespace_escape")
}

func TestDetectHostNamespace_None(t *testing.T) {
	t.Parallel()

	yaml := []byte(`apiVersion: v1
kind: Pod
metadata:
  name: test
spec:
  hostPID: false
  hostIPC: false
  hostNetwork: false
  containers:
  - name: app
    image: nginx
`)
	tags := DetectHostNamespace(yaml)
	assertEmpty(t, tags, "DetectHostNamespace")
}

func TestDetectHostPath_Present(t *testing.T) {
	t.Parallel()

	yaml := []byte(`apiVersion: v1
kind: Pod
metadata:
  name: test
spec:
  volumes:
  - name: data
    hostPath:
      path: /var/data
  containers:
  - name: app
    image: nginx
`)
	tags := DetectHostPath(yaml)
	assertContains(t, tags, "k8s.hostpath_mount")
}

func TestDetectHostPath_PVC(t *testing.T) {
	t.Parallel()

	yaml := []byte(`apiVersion: v1
kind: Pod
metadata:
  name: test
spec:
  volumes:
  - name: data
    persistentVolumeClaim:
      claimName: my-pvc
  containers:
  - name: app
    image: nginx
`)
	tags := DetectHostPath(yaml)
	assertEmpty(t, tags, "DetectHostPath")
}

// --- Terraform detector tests ---

func TestDetectMassDestroy_AboveThreshold(t *testing.T) {
	t.Parallel()

	plan := buildPlanJSON(t, 12, "delete")
	tags := DetectMassDestroy(plan)
	assertContains(t, tags, "ops.mass_delete")
}

func TestDetectMassDestroy_BelowThreshold(t *testing.T) {
	t.Parallel()

	plan := buildPlanJSON(t, 5, "delete")
	tags := DetectMassDestroy(plan)
	assertEmpty(t, tags, "DetectMassDestroy")
}

func TestDetectMassDestroy_CreateNotDelete(t *testing.T) {
	t.Parallel()

	plan := buildPlanJSON(t, 15, "create")
	tags := DetectMassDestroy(plan)
	assertEmpty(t, tags, "DetectMassDestroy")
}

func TestDetectWildcardIAM_BothWildcard(t *testing.T) {
	t.Parallel()

	plan := buildIAMPlanJSON(t, "Allow", "*", "*")
	tags := DetectWildcardIAM(plan)
	assertContains(t, tags, "aws_iam.wildcard_policy")
}

func TestDetectWildcardIAM_ScopedAction(t *testing.T) {
	t.Parallel()

	plan := buildIAMPlanJSON(t, "Allow", "s3:*", "*")
	tags := DetectWildcardIAM(plan)
	assertEmpty(t, tags, "DetectWildcardIAM")
}

func TestDetectTerraformIAMWildcard_WildcardAction(t *testing.T) {
	t.Parallel()

	plan := buildIAMPlanJSON(t, "Allow", "*", "arn:aws:s3:::my-bucket")
	tags := DetectTerraformIAMWildcard(plan)
	assertContains(t, tags, "terraform.iam_wildcard_policy")
}

func TestDetectTerraformIAMWildcard_WildcardResource(t *testing.T) {
	t.Parallel()

	plan := buildIAMPlanJSON(t, "Allow", "s3:GetObject", "*")
	tags := DetectTerraformIAMWildcard(plan)
	assertContains(t, tags, "terraform.iam_wildcard_policy")
}

func TestDetectTerraformIAMWildcard_ScopedPolicy(t *testing.T) {
	t.Parallel()

	plan := buildIAMPlanJSON(t, "Allow", "s3:GetObject", "arn:aws:s3:::my-bucket/*")
	tags := DetectTerraformIAMWildcard(plan)
	assertEmpty(t, tags, "DetectTerraformIAMWildcard")
}

func TestDetectTerraformIAMWildcard_DenyEffect(t *testing.T) {
	t.Parallel()

	plan := buildIAMPlanJSON(t, "Deny", "*", "*")
	tags := DetectTerraformIAMWildcard(plan)
	assertEmpty(t, tags, "DetectTerraformIAMWildcard")
}

func TestDetectS3PublicAccess_MissingBlock(t *testing.T) {
	t.Parallel()

	plan := []byte(`{
		"resource_changes": [
			{
				"type": "aws_s3_bucket",
				"name": "my-bucket",
				"change": {"actions": ["create"], "after": {"bucket": "my-bucket"}}
			}
		]
	}`)
	tags := DetectS3PublicAccess(plan)
	assertContains(t, tags, "terraform.s3_public_access")
}

func TestDetectS3PublicAccess_IncompleteBlock(t *testing.T) {
	t.Parallel()

	plan := []byte(`{
		"resource_changes": [
			{
				"type": "aws_s3_bucket",
				"name": "my-bucket",
				"change": {"actions": ["create"], "after": {"bucket": "my-bucket"}}
			},
			{
				"type": "aws_s3_bucket_public_access_block",
				"name": "my-bucket-block",
				"change": {
					"actions": ["create"],
					"after": {
						"block_public_acls": true,
						"ignore_public_acls": true,
						"block_public_policy": true,
						"restrict_public_buckets": false
					}
				}
			}
		]
	}`)
	tags := DetectS3PublicAccess(plan)
	assertContains(t, tags, "terraform.s3_public_access")
}

func TestDetectS3PublicAccess_CompleteBlock(t *testing.T) {
	t.Parallel()

	plan := []byte(`{
		"resource_changes": [
			{
				"type": "aws_s3_bucket",
				"name": "my-bucket",
				"change": {"actions": ["create"], "after": {"bucket": "my-bucket"}}
			},
			{
				"type": "aws_s3_bucket_public_access_block",
				"name": "my-bucket-block",
				"change": {
					"actions": ["create"],
					"after": {
						"block_public_acls": true,
						"ignore_public_acls": true,
						"block_public_policy": true,
						"restrict_public_buckets": true
					}
				}
			}
		]
	}`)
	tags := DetectS3PublicAccess(plan)
	assertEmpty(t, tags, "DetectS3PublicAccess")
}

func TestRunAll_CombinesTags(t *testing.T) {
	t.Parallel()

	yaml := []byte(`apiVersion: v1
kind: Pod
metadata:
  name: test
spec:
  hostPID: true
  volumes:
  - name: data
    hostPath:
      path: /var/data
  containers:
  - name: app
    image: nginx
    securityContext:
      privileged: true
`)
	tags := RunAll(yaml)
	assertContains(t, tags, "k8s.privileged_container")
	assertContains(t, tags, "k8s.host_namespace_escape")
	assertContains(t, tags, "k8s.hostpath_mount")
}

func TestRunAll_NoTags(t *testing.T) {
	t.Parallel()

	yaml := []byte(`apiVersion: v1
kind: Pod
metadata:
  name: safe-pod
spec:
  containers:
  - name: app
    image: nginx
`)
	tags := RunAll(yaml)
	assertEmpty(t, tags, "RunAll")
}

// --- Test helpers ---

func assertContains(t *testing.T, tags []string, want string) {
	t.Helper()
	for _, tag := range tags {
		if tag == want {
			return
		}
	}
	t.Errorf("tags %v does not contain %q", tags, want)
}

func assertEmpty(t *testing.T, tags []string, name string) {
	t.Helper()
	if len(tags) != 0 {
		t.Errorf("%s returned unexpected tags: %v", name, tags)
	}
}

func buildPlanJSON(t *testing.T, count int, action string) []byte {
	t.Helper()
	changes := make([]map[string]interface{}, count)
	for i := range changes {
		changes[i] = map[string]interface{}{
			"type": "aws_instance",
			"name": "instance",
			"change": map[string]interface{}{
				"actions": []string{action},
				"after":   map[string]interface{}{},
			},
		}
	}
	plan := map[string]interface{}{"resource_changes": changes}
	data, err := json.Marshal(plan)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func buildIAMPlanJSON(t *testing.T, effect, action, resource string) []byte {
	t.Helper()
	policyDoc := map[string]interface{}{
		"Statement": []map[string]interface{}{
			{"Effect": effect, "Action": action, "Resource": resource},
		},
	}
	policyStr, err := json.Marshal(policyDoc)
	if err != nil {
		t.Fatal(err)
	}
	plan := map[string]interface{}{
		"resource_changes": []map[string]interface{}{
			{
				"type": "aws_iam_policy",
				"name": "admin",
				"change": map[string]interface{}{
					"actions": []string{"create"},
					"after": map[string]interface{}{
						"policy": string(policyStr),
					},
				},
			},
		},
	}
	data, err := json.Marshal(plan)
	if err != nil {
		t.Fatal(err)
	}
	return data
}
