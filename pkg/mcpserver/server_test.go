package mcpserver

import (
	"context"
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"samebits.com/evidra/internal/testutil"
	"samebits.com/evidra/pkg/evidence"
	"samebits.com/evidra/pkg/version"
)

func TestDefaultServerVersion_UsesRuntimeVersion(t *testing.T) {
	t.Parallel()

	got := defaultServerVersion("")
	if got != version.Version {
		t.Fatalf("defaultServerVersion(\"\") = %q, want %q", got, version.Version)
	}
}

func TestInitializeInstructions_IncludeContractVersion(t *testing.T) {
	t.Parallel()

	if !strings.Contains(initializeInstructions, "Evidra — Flight recorder for AI infrastructure agents.") {
		t.Fatalf("initialize instructions missing current product positioning: %q", initializeInstructions)
	}
	if !strings.Contains(initializeInstructions, "Contract version: "+contractVersion) {
		t.Fatalf("initialize instructions missing contract version marker for %q", contractVersion)
	}
}

func TestPrescribe_SimpleK8s(t *testing.T) {
	t.Parallel()

	svc := &BenchmarkService{signer: testutil.TestSigner(t)}
	output := svc.Prescribe(PrescribeInput{
		Actor:       InputActor{Type: "agent", ID: "test", Origin: "mcp"},
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

	svc := &BenchmarkService{signer: testutil.TestSigner(t)}
	output := svc.Prescribe(PrescribeInput{
		Actor:       InputActor{Type: "agent", ID: "test", Origin: "mcp"},
		Tool:        "kubectl",
		Operation:   "apply",
		RawArtifact: k8sPrivileged,
	})

	if !output.OK {
		t.Fatalf("prescribe failed: %v", output.Error)
	}
	assertTagPresent(t, output.RiskTags, "k8s.privileged_container")
}

func TestPrescribeCtx_ForwardsCallerContext(t *testing.T) {
	t.Parallel()

	type ctxKey string

	dir := t.TempDir()
	gotCtxValue := make(chan string, 1)
	svc := &BenchmarkService{
		evidencePath: dir,
		signer:       testutil.TestSigner(t),
		forwardFunc: func(ctx context.Context, _ json.RawMessage) {
			value, _ := ctx.Value(ctxKey("trace_id")).(string)
			gotCtxValue <- value
		},
	}

	ctx := context.WithValue(context.Background(), ctxKey("trace_id"), "trace-123")
	output := svc.PrescribeCtx(ctx, PrescribeInput{
		Actor:       InputActor{Type: "agent", ID: "test", Origin: "mcp"},
		Tool:        "kubectl",
		Operation:   "apply",
		RawArtifact: k8sDeployment,
	})
	if !output.OK {
		t.Fatalf("prescribe failed: %v", output.Error)
	}

	if got := <-gotCtxValue; got != "trace-123" {
		t.Fatalf("forward context value = %q, want %q", got, "trace-123")
	}
}

func TestPrescribe_ParseError(t *testing.T) {
	t.Parallel()

	svc := &BenchmarkService{signer: testutil.TestSigner(t)}
	output := svc.Prescribe(PrescribeInput{
		Actor:       InputActor{Type: "agent", ID: "test", Origin: "mcp"},
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

	svc := &BenchmarkService{signer: testutil.TestSigner(t)}
	output := svc.Report(ReportInput{Verdict: evidence.VerdictSuccess, ExitCode: intPtr(0)})

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
		signer:       testutil.TestSigner(t),
	}

	input := PrescribeInput{
		Actor:       InputActor{Type: "agent", ID: "test", Origin: "mcp"},
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

func TestSchemaStructParity(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name       string
		schemaJSON []byte
		structType reflect.Type
	}{
		{"prescribe", prescribeSchemaBytes, reflect.TypeOf(PrescribeInput{})},
		{"report", reportSchemaBytes, reflect.TypeOf(ReportInput{})},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			// Parse schema to get top-level properties.
			var schema struct {
				Properties map[string]interface{} `json:"properties"`
			}
			if err := json.Unmarshal(tc.schemaJSON, &schema); err != nil {
				t.Fatalf("parse schema: %v", err)
			}

			// Extract json tags from struct.
			structFields := make(map[string]bool)
			for i := 0; i < tc.structType.NumField(); i++ {
				field := tc.structType.Field(i)
				tag := field.Tag.Get("json")
				if tag == "" || tag == "-" {
					continue
				}
				// Strip ",omitempty" etc.
				name := strings.Split(tag, ",")[0]
				structFields[name] = true
			}

			// Every schema property must have a struct field.
			for prop := range schema.Properties {
				if !structFields[prop] {
					t.Errorf("schema property %q has no matching struct field (would be silently dropped)", prop)
				}
			}

			// Every struct field must have a schema property.
			for field := range structFields {
				if _, ok := schema.Properties[field]; !ok {
					t.Errorf("struct field %q has no matching schema property (undocumented in schema)", field)
				}
			}
		})
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
