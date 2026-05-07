package mcpserver

import (
	"strings"
	"testing"
)

func TestFormatSmartOutput_GetDeployment(t *testing.T) {
	t.Parallel()

	raw := `NAME    READY   UP-TO-DATE   AVAILABLE   AGE
web     0/2     2            0           5m
api     3/3     3            3           10m`

	got := FormatSmartOutput("kubectl get deployments -n demo", raw, 0)

	if !strings.Contains(got, "deployment/web") {
		t.Errorf("expected deployment/web in output, got:\n%s", got)
	}
	if !strings.Contains(got, "0/2 ready") {
		t.Errorf("expected 0/2 ready in output, got:\n%s", got)
	}
	if !strings.Contains(got, "deployment/api") {
		t.Errorf("expected deployment/api in output, got:\n%s", got)
	}
	if !strings.Contains(got, "3/3 ready") {
		t.Errorf("expected 3/3 ready in output, got:\n%s", got)
	}
}

func TestFormatSmartOutput_GetDeployment_SingleDeploy(t *testing.T) {
	t.Parallel()

	raw := `NAME   READY   UP-TO-DATE   AVAILABLE   AGE
web    0/2     2            0           5m`

	got := FormatSmartOutput("kubectl get deploy -n demo", raw, 0)

	if !strings.Contains(got, "deployment/web") {
		t.Errorf("expected deployment/web in output, got:\n%s", got)
	}
}

func TestFormatSmartOutput_GetPods(t *testing.T) {
	t.Parallel()

	raw := `NAME        READY   STATUS         RESTARTS   AGE
web-abc12   1/1     Running        0          5m
web-def34   1/1     Running        0          5m
web-ghi56   0/1     ErrImagePull   0          2m`

	got := FormatSmartOutput("kubectl get pods -n demo", raw, 0)

	if !strings.Contains(got, "3 total") {
		t.Errorf("expected '3 total' in output, got:\n%s", got)
	}
	if !strings.Contains(got, "2 running") {
		t.Errorf("expected '2 running' in output, got:\n%s", got)
	}
	if !strings.Contains(got, "1 errimagepull") {
		t.Errorf("expected '1 errimagepull' in output, got:\n%s", got)
	}
	if !strings.Contains(got, "web-abc12: Running") {
		t.Errorf("expected pod detail 'web-abc12: Running' in output, got:\n%s", got)
	}
}

func TestFormatSmartOutput_Describe(t *testing.T) {
	t.Parallel()

	raw := `Name:                   web
Namespace:              demo
Replicas:               2 desired | 2 updated | 2 total | 0 available | 2 unavailable
Conditions:
  Type           Status  Reason
  ----           ------  ------
  Available      False   MinimumReplicasUnavailable
  Progressing    True    ReplicaSetUpdated
Events:
  Type     Reason            Age   From                   Message
  ----     ------            ----  ----                   -------
  Warning  FailedPull        2m    kubelet                Failed to pull image "nginx:99.99"
  Normal   Pulling           3m    kubelet                Pulling image "nginx:99.99"
  Normal   ScaledUpReplica   5m    deployment-controller  Scaled up replica set web-abc to 2`

	got := FormatSmartOutput("kubectl describe deployment web -n demo", raw, 0)

	if !strings.Contains(got, "Name:") {
		t.Errorf("expected Name: in output, got:\n%s", got)
	}
	if !strings.Contains(got, "Replicas:") {
		t.Errorf("expected Replicas: in output, got:\n%s", got)
	}
	if !strings.Contains(got, "Available") && !strings.Contains(got, "False") {
		t.Errorf("expected condition Available False in output, got:\n%s", got)
	}
	if !strings.Contains(got, "events (last 5)") {
		t.Errorf("expected 'events (last 5)' in output, got:\n%s", got)
	}
	if !strings.Contains(got, "FailedPull") {
		t.Errorf("expected FailedPull event in output, got:\n%s", got)
	}
}

func TestFormatSmartOutput_Logs(t *testing.T) {
	t.Parallel()

	// Build 60 lines.
	var lines []string
	for i := 0; i < 60; i++ {
		lines = append(lines, "log line "+strings.Repeat("x", 10))
	}
	raw := strings.Join(lines, "\n")

	got := FormatSmartOutput("kubectl logs web-abc12 -n demo", raw, 0)

	if !strings.Contains(got, "logs web-abc12") {
		t.Errorf("expected 'logs web-abc12' in output, got:\n%s", got)
	}
	if !strings.Contains(got, "last 50 lines") {
		t.Errorf("expected 'last 50 lines' in output, got:\n%s", got)
	}
	if !strings.Contains(got, "60 total") {
		t.Errorf("expected '60 total' in output, got:\n%s", got)
	}
	// Verify truncation happened (early lines should be dropped).
	outputLines := strings.Split(strings.TrimSpace(got), "\n")
	// 1 header + 50 log lines = 51
	if len(outputLines) != 51 {
		t.Errorf("expected 51 output lines, got %d", len(outputLines))
	}
}

func TestFormatSmartOutput_JsonPassthrough(t *testing.T) {
	t.Parallel()

	raw := `{
  "apiVersion": "apps/v1",
  "kind": "Deployment",
  "metadata": {
    "name": "web",
    "namespace": "demo",
    "uid": "12345-abcde",
    "resourceVersion": "999",
    "generation": 2,
    "creationTimestamp": "2026-01-01T00:00:00Z",
    "managedFields": [{"manager": "kubectl"}],
    "annotations": {
      "kubectl.kubernetes.io/last-applied-configuration": "{}"
    }
  }
}`

	got := FormatSmartOutput("kubectl get deployment web -n demo -o json", raw, 0)

	if strings.Contains(got, "managedFields") {
		t.Errorf("expected managedFields stripped, got:\n%s", got)
	}
	if strings.Contains(got, "uid") {
		t.Errorf("expected uid stripped, got:\n%s", got)
	}
	if strings.Contains(got, "resourceVersion") {
		t.Errorf("expected resourceVersion stripped, got:\n%s", got)
	}
	if strings.Contains(got, "last-applied-configuration") {
		t.Errorf("expected last-applied-configuration stripped, got:\n%s", got)
	}
	if !strings.Contains(got, `"name": "web"`) {
		t.Errorf("expected name preserved, got:\n%s", got)
	}
}

func TestFormatSmartOutput_Error(t *testing.T) {
	t.Parallel()

	raw := "Error from server (NotFound): deployments.apps \"web\" not found"

	got := FormatSmartOutput("kubectl get deployment web -n demo", raw, 1)

	if !strings.Contains(got, "error (exit code 1)") {
		t.Errorf("expected 'error (exit code 1)' in output, got:\n%s", got)
	}
	if !strings.Contains(got, "NotFound") {
		t.Errorf("expected error message in output, got:\n%s", got)
	}
}

func TestFormatSmartOutput_ErrorTruncation(t *testing.T) {
	t.Parallel()

	raw := strings.Repeat("x", 1000)
	got := FormatSmartOutput("kubectl apply -f bad.yaml", raw, 1)

	if len(got) > 600 {
		t.Errorf("expected error output truncated, got length %d", len(got))
	}
}

func TestFormatSmartOutput_Fallback(t *testing.T) {
	t.Parallel()

	raw := "some random output from an unknown command"
	got := FormatSmartOutput("ls -la /tmp", raw, 0)

	if got != raw {
		t.Errorf("expected raw passthrough for unknown command, got:\n%s", got)
	}
}

func TestFormatSmartOutput_FallbackTruncation(t *testing.T) {
	t.Parallel()

	raw := strings.Repeat("a", 3000)
	got := FormatSmartOutput("cat /etc/hosts", raw, 0)

	if len(got) > maxFallbackLen+10 {
		t.Errorf("expected fallback truncation to ~%d chars, got %d", maxFallbackLen, len(got))
	}
}

func TestFormatSmartOutput_TerraformPlan(t *testing.T) {
	t.Parallel()

	raw := `Terraform will perform the following actions:

Plan: 2 to add, 1 to change, 0 to destroy.`

	got := FormatSmartOutput("terraform plan", raw, 0)

	if !strings.Contains(got, "terraform plan") {
		t.Fatalf("expected terraform plan summary, got:\n%s", got)
	}
	if !strings.Contains(got, "2 to add, 1 to change, 0 to destroy") {
		t.Fatalf("expected plan counts, got:\n%s", got)
	}
}

func TestFormatSmartOutput_TerraformApply(t *testing.T) {
	t.Parallel()

	raw := `Apply complete! Resources: 2 added, 1 changed, 0 destroyed.`

	got := FormatSmartOutput("terraform apply -auto-approve", raw, 0)

	if !strings.Contains(got, "terraform apply complete") {
		t.Fatalf("expected terraform apply summary, got:\n%s", got)
	}
	if !strings.Contains(got, "2 added, 1 changed, 0 destroyed") {
		t.Fatalf("expected apply counts, got:\n%s", got)
	}
}

func TestFormatSmartOutput_HelmStatus(t *testing.T) {
	t.Parallel()

	raw := `NAME: web
LAST DEPLOYED: Tue Mar 24 12:00:00 2026
NAMESPACE: demo
STATUS: deployed
REVISION: 3`

	got := FormatSmartOutput("helm status web -n demo", raw, 0)

	if !strings.Contains(got, "helm release web: deployed") {
		t.Fatalf("expected helm release status summary, got:\n%s", got)
	}
	if !strings.Contains(got, "namespace: demo") {
		t.Fatalf("expected namespace in summary, got:\n%s", got)
	}
	if !strings.Contains(got, "revision: 3") {
		t.Fatalf("expected revision in summary, got:\n%s", got)
	}
}

func TestFormatSmartOutput_HelmList(t *testing.T) {
	t.Parallel()

	raw := `NAME	NAMESPACE	REVISION	UPDATED	STATUS	CHART	APP VERSION
web	demo	3	2026-03-24 12:00:00	deployed	web-1.2.3	1.2.3
api	demo	1	2026-03-24 11:00:00	failed	api-0.1.0	0.1.0`

	got := FormatSmartOutput("helm list -n demo", raw, 0)

	if !strings.Contains(got, "release/web (demo): deployed") {
		t.Fatalf("expected web release summary, got:\n%s", got)
	}
	if !strings.Contains(got, "release/api (demo): failed") {
		t.Fatalf("expected api release summary, got:\n%s", got)
	}
}

func TestFormatSmartOutput_AWSEKSUpdateKubeconfig(t *testing.T) {
	t.Parallel()

	raw := `Added new context arn:aws:eks:us-west-2:123456789012:cluster/demo to /Users/vitas/.kube/config`

	got := FormatSmartOutput("aws eks update-kubeconfig --name demo --region us-west-2", raw, 0)

	if !strings.Contains(got, "kubeconfig updated") {
		t.Fatalf("expected kubeconfig summary, got:\n%s", got)
	}
	if !strings.Contains(got, "cluster/demo") {
		t.Fatalf("expected cluster context in summary, got:\n%s", got)
	}
}
