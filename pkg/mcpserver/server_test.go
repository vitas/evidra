package mcpserver

import (
	"testing"
)

func TestPrescribe_SimpleK8s(t *testing.T) {
	t.Parallel()

	svc := &BenchmarkService{}
	output := svc.Prescribe(PrescribeInput{
		Actor:       InputActor{Type: "agent", ID: "test"},
		Tool:        "kubectl",
		Operation:   "apply",
		RawArtifact: k8sDeployment,
	})

	if !output.OK {
		t.Fatalf("prescribe failed: %v", output.Error)
	}
	if output.PrescriptionID == "" {
		t.Fatal("missing prescription_id")
	}
	if output.RiskLevel == "" {
		t.Fatal("missing risk_level")
	}
	if output.ArtifactDigest == "" {
		t.Fatal("missing artifact_digest")
	}
	if output.IntentDigest == "" {
		t.Fatal("missing intent_digest")
	}
	if output.CanonVersion != "k8s/v1" {
		t.Errorf("canon_version = %q, want %q", output.CanonVersion, "k8s/v1")
	}
	if output.ResourceCount != 1 {
		t.Errorf("resource_count = %d, want 1", output.ResourceCount)
	}
	if output.OperationClass != "mutate" {
		t.Errorf("operation_class = %q, want %q", output.OperationClass, "mutate")
	}
}

func TestPrescribe_PrivilegedContainer(t *testing.T) {
	t.Parallel()

	svc := &BenchmarkService{}
	output := svc.Prescribe(PrescribeInput{
		Actor:       InputActor{Type: "agent", ID: "test"},
		Tool:        "kubectl",
		Operation:   "apply",
		RawArtifact: k8sPrivileged,
	})

	if !output.OK {
		t.Fatalf("prescribe failed: %v", output.Error)
	}
	assertTagPresent(t, output.RiskTags, "k8s.privileged_container")
}

func TestPrescribe_ParseError(t *testing.T) {
	t.Parallel()

	svc := &BenchmarkService{}
	output := svc.Prescribe(PrescribeInput{
		Actor:       InputActor{Type: "agent", ID: "test"},
		Tool:        "terraform",
		Operation:   "apply",
		RawArtifact: "not valid json {{{",
	})

	if output.OK {
		t.Fatal("expected parse error")
	}
	if output.Error == nil || output.Error.Code != "parse_error" {
		t.Errorf("expected parse_error, got %v", output.Error)
	}
}

func TestReport_MissingPrescriptionID(t *testing.T) {
	t.Parallel()

	svc := &BenchmarkService{}
	output := svc.Report(ReportInput{ExitCode: 0})

	if output.OK {
		t.Fatal("expected error for missing prescription_id")
	}
	if output.Error == nil || output.Error.Code != "invalid_input" {
		t.Errorf("expected invalid_input error, got %v", output.Error)
	}
}

func TestRetryTracker_CountsRetries(t *testing.T) {
	t.Parallel()

	svc := &BenchmarkService{
		retryTracker: NewRetryTracker(10 * 60 * 1e9), // 10 minutes
	}

	input := PrescribeInput{
		Actor:       InputActor{Type: "agent", ID: "test"},
		Tool:        "kubectl",
		Operation:   "apply",
		RawArtifact: k8sDeployment,
	}

	out1 := svc.Prescribe(input)
	if out1.RetryCount != 1 {
		t.Errorf("first prescribe retry_count = %d, want 1", out1.RetryCount)
	}

	out2 := svc.Prescribe(input)
	if out2.RetryCount != 2 {
		t.Errorf("second prescribe retry_count = %d, want 2", out2.RetryCount)
	}
}

func assertTagPresent(t *testing.T, tags []string, want string) {
	t.Helper()
	for _, tag := range tags {
		if tag == want {
			return
		}
	}
	t.Errorf("tags %v does not contain %q", tags, want)
}

const k8sDeployment = `apiVersion: apps/v1
kind: Deployment
metadata:
  name: nginx
  namespace: default
spec:
  replicas: 1
  template:
    spec:
      containers:
      - name: nginx
        image: nginx:1.25
`

const k8sPrivileged = `apiVersion: v1
kind: Pod
metadata:
  name: priv-pod
spec:
  containers:
  - name: app
    image: nginx
    securityContext:
      privileged: true
`
