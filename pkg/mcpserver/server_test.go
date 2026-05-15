package mcpserver

import (
	"context"
	"encoding/json"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"samebits.com/evidra/internal/assessment"
	"samebits.com/evidra/internal/lifecycle"
	"samebits.com/evidra/internal/testutil"
	"samebits.com/evidra/pkg/evidence"
	"samebits.com/evidra/pkg/execcontract"
	"samebits.com/evidra/pkg/version"
)

func TestDefaultServerVersion_UsesRuntimeVersion(t *testing.T) {
	t.Parallel()

	got := defaultServerVersion("")
	if got != version.Version {
		t.Fatalf("defaultServerVersion(\"\") = %q, want %q", got, version.Version)
	}
}

func TestMCPServiceForwardUsesWrittenEntryBytes(t *testing.T) {
	t.Parallel()

	signer := testutil.TestSigner(t)
	writeDir := t.TempDir()
	readDir := t.TempDir()
	gotForward := make(chan json.RawMessage, 1)

	svc := &MCPService{
		evidencePath: readDir,
		signer:       signer,
		lifecycle: lifecycle.NewService(lifecycle.Options{
			EvidencePath: writeDir,
			Signer:       signer,
		}),
		forwardFunc: func(_ context.Context, entry json.RawMessage) {
			gotForward <- append(json.RawMessage(nil), entry...)
		},
	}

	output := svc.PrescribeCtx(context.Background(), PrescribeInput{
		Actor:       InputActor{Type: "agent", ID: "test", Origin: "mcp"},
		Tool:        "kubectl",
		Operation:   "apply",
		RawArtifact: k8sDeployment,
	})
	if !output.OK {
		t.Fatalf("prescribe failed: %v", output.Error)
	}

	select {
	case raw := <-gotForward:
		if len(raw) == 0 {
			t.Fatal("forwarded entry is empty")
		}
		var entry evidence.EvidenceEntry
		if err := json.Unmarshal(raw, &entry); err != nil {
			t.Fatalf("json.Unmarshal(forwarded): %v", err)
		}
		if entry.EntryID != output.PrescriptionID {
			t.Fatalf("forwarded entry_id = %q, want %q", entry.EntryID, output.PrescriptionID)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("forward callback was not invoked")
	}
}

func TestMCPServiceClose_StopsRetryTracker(t *testing.T) {
	t.Parallel()

	svc := &MCPService{
		retryTracker: NewRetryTracker(10 * time.Minute),
	}

	if err := svc.Close(); err != nil {
		t.Fatalf("Close(first): %v", err)
	}
	select {
	case <-svc.retryTracker.stop:
	default:
		t.Fatal("retry tracker stop channel is still open")
	}

	if err := svc.Close(); err != nil {
		t.Fatalf("Close(second): %v", err)
	}
}

func TestNewServerWithCleanup_ReturnsIdempotentCleanup(t *testing.T) {
	t.Parallel()

	server, cleanup, err := NewServerWithCleanup(Options{
		Signer:       testutil.TestSigner(t),
		RetryTracker: true,
	})
	if err != nil {
		t.Fatalf("NewServerWithCleanup: %v", err)
	}
	if server == nil {
		t.Fatal("server is nil")
	}
	if cleanup == nil {
		t.Fatal("cleanup is nil")
	}
	if err := cleanup(); err != nil {
		t.Fatalf("cleanup(first): %v", err)
	}
	if err := cleanup(); err != nil {
		t.Fatalf("cleanup(second): %v", err)
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
	if !strings.Contains(defaultInitializeInstructions, "`prescribe_full`") {
		t.Fatalf("default initialize instructions missing prescribe_full guidance: %q", defaultInitializeInstructions)
	}
	if !strings.Contains(defaultInitializeInstructions, "`prescribe_smart`") {
		t.Fatalf("default initialize instructions missing prescribe_smart guidance: %q", defaultInitializeInstructions)
	}
	if strings.Contains(defaultInitializeInstructions, "Call `prescribe` BEFORE") {
		t.Fatalf("default initialize instructions should not mention legacy prescribe flow: %q", defaultInitializeInstructions)
	}
}

func TestDefaultInitializeInstructions_PreferRunCommandWorkflow(t *testing.T) {
	t.Parallel()

	if !strings.Contains(defaultInitializeInstructions, "`run_command`") {
		t.Fatalf("instructions should mention run_command default path: %q", defaultInitializeInstructions)
	}
	if !strings.Contains(defaultInitializeInstructions, "`describe_tool`") {
		t.Fatalf("instructions should mention describe_tool for explicit protocol control: %q", defaultInitializeInstructions)
	}
	if strings.Contains(defaultInitializeInstructions, "auto-expand") {
		t.Fatalf("instructions should not promise dynamic schema auto-expansion: %q", defaultInitializeInstructions)
	}
}

func TestRunCommandTool_HasOutputSchemaAndExamples(t *testing.T) {
	t.Parallel()

	server, err := NewServer(Options{
		Name:         "test",
		Version:      "0.0.1",
		EvidencePath: t.TempDir(),
		Environment:  "test",
		Signer:       testutil.TestSigner(t),
	})
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	st, ct := mcp.NewInMemoryTransports()
	serverSession, err := server.Connect(ctx, st, nil)
	if err != nil {
		t.Fatalf("server connect: %v", err)
	}
	defer func() { _ = serverSession.Wait() }()

	client := mcp.NewClient(
		&mcp.Implementation{Name: "test-client", Version: "0.0.1"},
		nil,
	)
	session, err := client.Connect(ctx, ct, nil)
	if err != nil {
		t.Fatalf("client connect: %v", err)
	}
	defer func() { _ = session.Close() }()

	tools, err := session.ListTools(ctx, nil)
	if err != nil {
		t.Fatalf("list tools: %v", err)
	}

	var runCommand *mcp.Tool
	for _, tool := range tools.Tools {
		if tool.Name == "run_command" {
			runCommand = tool
			break
		}
	}
	if runCommand == nil {
		t.Fatal("run_command tool missing from tools/list response")
		return
	}
	if runCommand.OutputSchema == nil {
		t.Fatal("run_command tool missing output schema")
	}
	if !strings.Contains(runCommand.Description, "Investigate before fixing") {
		t.Fatalf("run_command description missing diagnosis guidance: %q", runCommand.Description)
	}
	if !strings.Contains(runCommand.Description, "kubectl describe pod") {
		t.Fatalf("run_command description missing diagnose example: %q", runCommand.Description)
	}
	if !strings.Contains(runCommand.Description, "kubectl rollout status") {
		t.Fatalf("run_command description missing verify example: %q", runCommand.Description)
	}
}

func TestNewServer_DefaultToolSurfaceUsesDeferredProtocolSchemas(t *testing.T) {
	t.Parallel()

	server, err := NewServer(Options{
		Name:              "test",
		Version:           "0.0.1",
		EvidencePath:      t.TempDir(),
		Environment:       "test",
		Signer:            testutil.TestSigner(t),
		HidePrescribeFull: true,
	})
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	st, ct := mcp.NewInMemoryTransports()
	serverSession, err := server.Connect(ctx, st, nil)
	if err != nil {
		t.Fatalf("server connect: %v", err)
	}
	defer func() { _ = serverSession.Wait() }()

	client := mcp.NewClient(
		&mcp.Implementation{Name: "test-client", Version: "0.0.1"},
		nil,
	)
	session, err := client.Connect(ctx, ct, nil)
	if err != nil {
		t.Fatalf("client connect: %v", err)
	}
	defer func() { _ = session.Close() }()

	tools, err := session.ListTools(ctx, nil)
	if err != nil {
		t.Fatalf("list tools: %v", err)
	}

	toolDefs := make(map[string]*mcp.Tool, len(tools.Tools))
	for _, tool := range tools.Tools {
		toolDefs[tool.Name] = tool
	}

	if _, ok := toolDefs["describe_tool"]; !ok {
		t.Fatal("describe_tool missing from tools/list response")
	}
	if _, ok := toolDefs["prescribe_full"]; ok {
		t.Fatal("prescribe_full should stay hidden from the default tool surface")
	}

	for _, name := range []string{"prescribe_smart", "report"} {
		tool := toolDefs[name]
		if tool == nil {
			t.Fatalf("missing tool %q in tools/list response", name)
		}
		if tool.OutputSchema == nil {
			t.Fatalf("%s should advertise an output schema", name)
		}
		assertMinimalObjectSchema(t, tool.InputSchema)
	}
}

func TestStructuredJSONTools_HaveOutputSchema(t *testing.T) {
	t.Parallel()

	server, err := NewServer(Options{
		Name:         "test",
		Version:      "0.0.1",
		EvidencePath: t.TempDir(),
		Signer:       testutil.TestSigner(t),
	})
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	st, ct := mcp.NewInMemoryTransports()
	serverSession, err := server.Connect(ctx, st, nil)
	if err != nil {
		t.Fatalf("server connect: %v", err)
	}
	defer func() { _ = serverSession.Wait() }()

	client := mcp.NewClient(&mcp.Implementation{Name: "test-client", Version: "0.0.1"}, nil)
	session, err := client.Connect(ctx, ct, nil)
	if err != nil {
		t.Fatalf("client connect: %v", err)
	}
	defer func() { _ = session.Close() }()

	tools, err := session.ListTools(ctx, nil)
	if err != nil {
		t.Fatalf("list tools: %v", err)
	}

	toolDefs := make(map[string]*mcp.Tool, len(tools.Tools))
	for _, tool := range tools.Tools {
		toolDefs[tool.Name] = tool
	}

	for _, name := range []string{
		"prescribe_full",
		"prescribe_smart",
		"report",
		"get_event",
		"run_command",
		"collect_diagnostics",
		"write_file",
	} {
		tool := toolDefs[name]
		if tool == nil {
			t.Fatalf("tool %q missing from tools/list", name)
		}
		if tool.OutputSchema == nil {
			t.Fatalf("structured JSON tool %q is missing OutputSchema", name)
		}
	}

	if tool := toolDefs["describe_tool"]; tool == nil {
		t.Fatal("describe_tool missing from tools/list")
	} else if tool.OutputSchema != nil {
		t.Fatal("describe_tool should remain text-only")
	}
}

func TestPrescribe_SimpleK8s(t *testing.T) {
	t.Parallel()

	svc := &MCPService{signer: testutil.TestSigner(t)}
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
	if output.EffectiveRisk != "" {
		t.Fatalf("effective_risk = %q, want empty without assessment", output.EffectiveRisk)
	}
	if len(output.RiskInputs) != 0 {
		t.Fatalf("risk_inputs = %+v, want none without assessment", output.RiskInputs)
	}
	if output.ArtifactDigest == "" {
		t.Fatal("missing artifact_digest")
	}
	if output.IntentDigest == "" {
		t.Fatal("missing intent_digest")
	}
	if output.CanonVersion != "" {
		t.Errorf("canon_version = %q, want empty", output.CanonVersion)
	}
	if output.ResourceCount != 0 {
		t.Errorf("resource_count = %d, want 0", output.ResourceCount)
	}
	if output.OperationClass != "" {
		t.Errorf("operation_class = %q, want empty", output.OperationClass)
	}
}

func assertMinimalObjectSchema(t *testing.T, schema any) {
	t.Helper()

	var got map[string]any
	raw, err := json.Marshal(schema)
	if err != nil {
		t.Fatalf("marshal schema: %v", err)
	}
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("unmarshal schema: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("schema = %v, want exactly one field", got)
	}
	if got["type"] != "object" {
		t.Fatalf("schema type = %v, want object", got["type"])
	}
}

func TestPrescribe_PrivilegedContainer(t *testing.T) {
	t.Parallel()

	svc := &MCPService{signer: testutil.TestSigner(t)}
	output := svc.Prescribe(PrescribeInput{
		Actor:       InputActor{Type: "agent", ID: "test", Origin: "mcp"},
		Tool:        "kubectl",
		Operation:   "apply",
		RawArtifact: k8sPrivileged,
	})

	if !output.OK {
		t.Fatalf("prescribe failed: %v", output.Error)
	}
	if len(output.RiskInputs) != 0 {
		t.Fatalf("risk_inputs = %+v, want none without assessment", output.RiskInputs)
	}
	if output.EffectiveRisk != "" {
		t.Fatalf("effective_risk = %q, want empty without assessment", output.EffectiveRisk)
	}
}

func TestPrescribe_SmartMode(t *testing.T) {
	t.Parallel()

	svc := &MCPService{signer: testutil.TestSigner(t)}
	output := svc.Prescribe(PrescribeInput{
		Actor:     InputActor{Type: "agent", ID: "test", Origin: "mcp"},
		Tool:      "kubectl",
		Operation: "apply",
		Resource:  "deployment/web",
		Namespace: "staging",
	})

	if !output.OK {
		t.Fatalf("prescribe failed: %v", output.Error)
	}
	if output.PrescriptionID == "" {
		t.Fatal("missing prescription_id")
	}
	if output.OperationClass != "" {
		t.Fatalf("operation_class = %q, want empty", output.OperationClass)
	}
	if output.ScopeClass != "" {
		t.Fatalf("scope_class = %q, want empty", output.ScopeClass)
	}
	if output.ResourceCount != 0 {
		t.Fatalf("resource_count = %d, want 0", output.ResourceCount)
	}
	if len(output.RiskInputs) != 0 {
		t.Fatalf("risk_inputs = %+v, want none without assessment", output.RiskInputs)
	}
}

func TestPrescribe_SmartModeRequiresTarget(t *testing.T) {
	t.Parallel()

	svc := &MCPService{signer: testutil.TestSigner(t)}
	output := svc.Prescribe(PrescribeInput{
		Actor:     InputActor{Type: "agent", ID: "test", Origin: "mcp"},
		Tool:      "kubectl",
		Operation: "apply",
	})

	if output.OK {
		t.Fatal("expected invalid_input error")
	}
	if output.Error == nil || output.Error.Code != "invalid_input" {
		t.Fatalf("error = %+v, want invalid_input", output.Error)
	}
}

func TestPrescribeCtx_ForwardsCallerContext(t *testing.T) {
	t.Parallel()

	type ctxKey string

	dir := t.TempDir()
	gotCtxValue := make(chan string, 1)
	svc := &MCPService{
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

func TestPrescribe_RawArtifactDoesNotParseInCore(t *testing.T) {
	t.Parallel()

	svc := &MCPService{signer: testutil.TestSigner(t)}
	output := svc.Prescribe(PrescribeInput{
		Actor:       InputActor{Type: "agent", ID: "test", Origin: "mcp"},
		Tool:        "terraform",
		Operation:   "apply",
		RawArtifact: "not valid json {{{",
	})

	if !output.OK {
		t.Fatalf("prescribe failed: %v", output.Error)
	}
	if output.CanonVersion != "" {
		t.Fatalf("canon_version = %q, want empty", output.CanonVersion)
	}
}

func TestReport_MissingPrescriptionID(t *testing.T) {
	t.Parallel()

	svc := &MCPService{signer: testutil.TestSigner(t)}
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

	svc := &MCPService{
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
		schema     map[string]any
		structType reflect.Type
	}{
		{name: "prescribe_full", schema: mustToolSchema(t, execcontract.PrescribeFullToolDefinition), structType: reflect.TypeOf(PrescribeFullInput{})},
		{name: "prescribe_smart", schema: mustToolSchema(t, execcontract.PrescribeSmartToolDefinition), structType: reflect.TypeOf(PrescribeSmartInput{})},
		{name: "report", schema: mustToolSchema(t, execcontract.ReportToolDefinition), structType: reflect.TypeOf(ReportInput{})},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

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
			properties, _ := tc.schema["properties"].(map[string]any)
			for prop := range properties {
				if !structFields[prop] {
					t.Errorf("schema property %q has no matching struct field (would be silently dropped)", prop)
				}
			}

			// Every struct field must have a schema property.
			for field := range structFields {
				if _, ok := properties[field]; !ok {
					t.Errorf("struct field %q has no matching schema property (undocumented in schema)", field)
				}
			}
		})
	}
}

func mustToolSchema(t *testing.T, load func() (execcontract.ToolDefinition, error)) map[string]any {
	t.Helper()

	def, err := load()
	if err != nil {
		t.Fatalf("load tool definition: %v", err)
	}
	return def.Parameters
}

func TestReportTool_IncludesResourceLinkOnSuccess(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	svc := &MCPService{
		evidencePath: dir,
		signer:       testutil.TestSigner(t),
	}
	svc.lifecycle = svc.newLifecycleService()

	presc := svc.Prescribe(PrescribeInput{
		Actor:       InputActor{Type: "agent", ID: "test", Origin: "mcp"},
		Tool:        "kubectl",
		Operation:   "apply",
		RawArtifact: "apiVersion: apps/v1\nkind: Deployment\nmetadata:\n  name: web\n  namespace: prod",
	})
	if !presc.OK {
		t.Fatalf("prescribe: %v", presc.Error)
	}

	server, err := NewServer(Options{
		EvidencePath: dir,
		Signer:       testutil.TestSigner(t),
	})
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	st, ct := mcp.NewInMemoryTransports()
	serverSession, err := server.Connect(ctx, st, nil)
	if err != nil {
		t.Fatalf("server connect: %v", err)
	}
	defer func() { _ = serverSession.Wait() }()

	client := mcp.NewClient(&mcp.Implementation{Name: "test-client", Version: "0.0.1"}, nil)
	session, err := client.Connect(ctx, ct, nil)
	if err != nil {
		t.Fatalf("client connect: %v", err)
	}
	defer func() { _ = session.Close() }()

	result, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name: "report",
		Arguments: map[string]any{
			"prescription_id": presc.PrescriptionID,
			"verdict":         string(evidence.VerdictSuccess),
			"exit_code":       0,
			"actor":           map[string]any{"type": "agent", "id": "test", "origin": "mcp"},
		},
	})
	if err != nil {
		t.Fatalf("CallTool report: %v", err)
	}

	var hasResourceLink bool
	for _, c := range result.Content {
		if rl, ok := c.(*mcp.ResourceLink); ok && strings.HasPrefix(rl.URI, "evidra://event/") {
			hasResourceLink = true
		}
	}
	if !hasResourceLink {
		t.Fatalf("report result missing resource_link to evidra://event/: content=%v", result.Content)
	}
}

func TestScorecardAggregateResource_ReturnsSnapshot(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	svc := &MCPService{
		evidencePath: dir,
		signer:       testutil.TestSigner(t),
	}
	svc.lifecycle = svc.newLifecycleService()

	presc := svc.Prescribe(PrescribeInput{
		Actor:       InputActor{Type: "agent", ID: "test", Origin: "mcp"},
		Tool:        "kubectl",
		Operation:   "apply",
		RawArtifact: "apiVersion: apps/v1\nkind: Deployment\nmetadata:\n  name: web\n  namespace: prod",
		SessionID:   "session-a",
	})
	if !presc.OK {
		t.Fatalf("prescribe: %v", presc.Error)
	}
	ec := 0
	report := svc.Report(ReportInput{
		PrescriptionID: presc.PrescriptionID,
		Verdict:        evidence.VerdictSuccess,
		ExitCode:       &ec,
		Actor:          InputActor{Type: "agent", ID: "test", Origin: "mcp"},
		SessionID:      "session-a",
	})
	if !report.OK {
		t.Fatalf("report: %v", report.Error)
	}

	server, err := NewServer(Options{EvidencePath: dir, Signer: testutil.TestSigner(t)})
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	st, ct := mcp.NewInMemoryTransports()
	serverSession, err := server.Connect(ctx, st, nil)
	if err != nil {
		t.Fatalf("server connect: %v", err)
	}
	defer func() { _ = serverSession.Wait() }()

	client := mcp.NewClient(&mcp.Implementation{Name: "test-client", Version: "0.0.1"}, nil)
	session, err := client.Connect(ctx, ct, nil)
	if err != nil {
		t.Fatalf("client connect: %v", err)
	}
	defer func() { _ = session.Close() }()

	result, err := session.ReadResource(ctx, &mcp.ReadResourceParams{URI: "evidra://scorecard/aggregate"})
	if err != nil {
		t.Fatalf("ReadResource: %v", err)
	}
	if len(result.Contents) == 0 || result.Contents[0].Text == "" {
		t.Fatal("scorecard aggregate resource returned empty content")
	}
	var snapshot map[string]any
	if err := json.Unmarshal([]byte(result.Contents[0].Text), &snapshot); err != nil {
		t.Fatalf("unmarshal scorecard: %v", err)
	}
	if _, ok := snapshot["score"]; !ok {
		t.Fatalf("scorecard missing score: %v", snapshot)
	}
}

func TestScorecardSessionResource_FiltersToSession(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	svc := &MCPService{
		evidencePath: dir,
		signer:       testutil.TestSigner(t),
	}
	svc.lifecycle = svc.newLifecycleService()

	makePair := func(sessionID string, verdict evidence.Verdict, exitCode int) {
		presc := svc.Prescribe(PrescribeInput{
			Actor:       InputActor{Type: "agent", ID: sessionID, Origin: "mcp"},
			Tool:        "kubectl",
			Operation:   "apply",
			RawArtifact: "apiVersion: apps/v1\nkind: Deployment\nmetadata:\n  name: web\n  namespace: prod",
			SessionID:   sessionID,
		})
		if !presc.OK {
			t.Fatalf("prescribe(%s): %v", sessionID, presc.Error)
		}
		ec := exitCode
		rep := svc.Report(ReportInput{
			PrescriptionID: presc.PrescriptionID,
			Verdict:        verdict,
			ExitCode:       &ec,
			Actor:          InputActor{Type: "agent", ID: sessionID, Origin: "mcp"},
			SessionID:      sessionID,
		})
		if !rep.OK {
			t.Fatalf("report(%s): %v", sessionID, rep.Error)
		}
	}

	makePair("session-a", evidence.VerdictSuccess, 0)
	makePair("session-b", evidence.VerdictFailure, 1)

	server, err := NewServer(Options{EvidencePath: dir, Signer: testutil.TestSigner(t)})
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	st, ct := mcp.NewInMemoryTransports()
	serverSession, err := server.Connect(ctx, st, nil)
	if err != nil {
		t.Fatalf("server connect: %v", err)
	}
	defer func() { _ = serverSession.Wait() }()

	client := mcp.NewClient(&mcp.Implementation{Name: "test-client", Version: "0.0.1"}, nil)
	session, err := client.Connect(ctx, ct, nil)
	if err != nil {
		t.Fatalf("client connect: %v", err)
	}
	defer func() { _ = session.Close() }()

	result, err := session.ReadResource(ctx, &mcp.ReadResourceParams{URI: "evidra://scorecard/session/session-a"})
	if err != nil {
		t.Fatalf("ReadResource(session-a): %v", err)
	}

	var snapshot assessment.Snapshot
	if err := json.Unmarshal([]byte(result.Contents[0].Text), &snapshot); err != nil {
		t.Fatalf("unmarshal session snapshot: %v", err)
	}
	if snapshot.Basis.TotalOperations != 1 {
		t.Fatalf("session snapshot total_operations=%d, want 1", snapshot.Basis.TotalOperations)
	}
}

func TestNewServer_AdvertisesImplementationDescription(t *testing.T) {
	t.Parallel()

	server, err := NewServer(Options{
		Name:         "test",
		Version:      "0.0.1",
		EvidencePath: t.TempDir(),
		Signer:       testutil.TestSigner(t),
	})
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	st, ct := mcp.NewInMemoryTransports()
	serverSession, err := server.Connect(ctx, st, nil)
	if err != nil {
		t.Fatalf("server connect: %v", err)
	}
	defer func() { _ = serverSession.Wait() }()

	client := mcp.NewClient(&mcp.Implementation{Name: "test-client", Version: "0.0.1"}, nil)
	session, err := client.Connect(ctx, ct, nil)
	if err != nil {
		t.Fatalf("client connect: %v", err)
	}
	defer func() { _ = session.Close() }()

	initResult := session.InitializeResult()
	if initResult == nil || initResult.ServerInfo == nil {
		t.Fatal("InitializeResult.ServerInfo is nil")
	}
	if initResult.ServerInfo.Title != "Flight recorder for AI infrastructure agents" {
		t.Fatalf("ServerInfo.Title=%q", initResult.ServerInfo.Title)
	}
}

func TestScorecardAggregateResource_EmptyEvidenceReturnsZeroSnapshot(t *testing.T) {
	t.Parallel()

	server, err := NewServer(Options{EvidencePath: t.TempDir(), Signer: testutil.TestSigner(t)})
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	st, ct := mcp.NewInMemoryTransports()
	serverSession, err := server.Connect(ctx, st, nil)
	if err != nil {
		t.Fatalf("server connect: %v", err)
	}
	defer func() { _ = serverSession.Wait() }()

	client := mcp.NewClient(&mcp.Implementation{Name: "test-client", Version: "0.0.1"}, nil)
	session, err := client.Connect(ctx, ct, nil)
	if err != nil {
		t.Fatalf("client connect: %v", err)
	}
	defer func() { _ = session.Close() }()

	result, err := session.ReadResource(ctx, &mcp.ReadResourceParams{URI: "evidra://scorecard/aggregate"})
	if err != nil {
		t.Fatalf("ReadResource: %v", err)
	}
	var snapshot assessment.Snapshot
	if err := json.Unmarshal([]byte(result.Contents[0].Text), &snapshot); err != nil {
		t.Fatalf("unmarshal aggregate snapshot: %v", err)
	}
	// Score is -1 when there are no operations (sentinel for "insufficient data").
	if snapshot.Score > 0 {
		t.Fatalf("score=%v, want <=0 for empty evidence", snapshot.Score)
	}
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
