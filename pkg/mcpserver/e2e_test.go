package mcpserver

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"samebits.com/evidra/pkg/evidence"
)

func TestE2E_PrescribeFullReportLifecycle(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	server, serverErr := NewServer(Options{
		Name:         "test",
		Version:      "0.0.1",
		EvidencePath: dir,
		Environment:  "test",
		Signer:       newTestSigner(t),
	})
	if serverErr != nil {
		t.Fatalf("NewServer: %v", serverErr)
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

	// Prescribe full
	prescribeResult, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name: "prescribe_full",
		Arguments: map[string]any{
			"actor": map[string]any{
				"type":   "agent",
				"id":     "test-agent",
				"origin": "e2e-test",
			},
			"tool":         "kubectl",
			"operation":    "apply",
			"raw_artifact": "apiVersion: apps/v1\nkind: Deployment\nmetadata:\n  name: nginx\n  namespace: default\nspec:\n  replicas: 1\n  selector:\n    matchLabels:\n      app: nginx\n  template:\n    metadata:\n      labels:\n        app: nginx\n    spec:\n      containers:\n      - name: nginx\n        image: nginx:1.21\n",
		},
	})
	if err != nil {
		t.Fatalf("prescribe: %v", err)
	}

	var prescribeOut PrescribeOutput
	if err := extractStructuredContent(prescribeResult, &prescribeOut); err != nil {
		t.Fatalf("parse prescribe output: %v", err)
	}
	if !prescribeOut.OK {
		t.Fatalf("prescribe not ok: %+v", prescribeOut)
	}
	if prescribeOut.PrescriptionID == "" {
		t.Fatal("prescribe returned empty prescription_id")
	}
	if prescribeOut.EffectiveRisk != "" {
		t.Fatalf("effective_risk = %q, want empty without assessment", prescribeOut.EffectiveRisk)
	}
	if len(prescribeOut.RiskInputs) != 0 {
		t.Fatalf("risk_inputs = %+v, want none without assessment", prescribeOut.RiskInputs)
	}
	if prescribeOut.ArtifactDigest == "" {
		t.Fatal("prescribe returned empty artifact_digest")
	}

	// Report
	reportResult, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name: "report",
		Arguments: map[string]any{
			"prescription_id": prescribeOut.PrescriptionID,
			"verdict":         "success",
			"exit_code":       0,
		},
	})
	if err != nil {
		t.Fatalf("report: %v", err)
	}

	var reportOut map[string]any
	if err := extractStructuredContent(reportResult, &reportOut); err != nil {
		t.Fatalf("parse report output: %v", err)
	}
	if reportOut["ok"] != true {
		t.Fatalf("report not ok: %+v", reportOut)
	}
	reportID, ok := reportOut["report_id"].(string)
	if !ok || reportID == "" {
		t.Fatal("report returned empty report_id")
	}
	for _, key := range []string{"score", "score_band", "scoring_profile_id", "signal_summary", "basis", "confidence", "prescription_id", "exit_code", "verdict"} {
		if _, ok := reportOut[key]; !ok {
			t.Fatalf("missing report field %q: %+v", key, reportOut)
		}
	}
	if _, ok := reportOut["signals"]; ok {
		t.Fatalf("signals must not be present in report output: %+v", reportOut)
	}

	// Get event — retrieve the report entry via the get_event tool.
	getEventResult, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name: "get_event",
		Arguments: map[string]any{
			"event_id": reportID,
		},
	})
	if err != nil {
		t.Fatalf("get_event: %v", err)
	}

	var getEventOut GetEventOutput
	if err := extractStructuredContent(getEventResult, &getEventOut); err != nil {
		t.Fatalf("parse get_event output: %v", err)
	}
	if !getEventOut.OK {
		t.Fatalf("get_event not ok: %+v", getEventOut)
	}
	if getEventOut.Entry == nil {
		t.Fatal("get_event returned nil entry")
	}
	if getEventOut.Entry.EntryID != reportID {
		t.Errorf("get_event entry_id mismatch: got %q, want %q", getEventOut.Entry.EntryID, reportID)
	}
	if getEventOut.Entry.Type != evidence.EntryTypeReport {
		t.Errorf("get_event type: got %q, want %q", getEventOut.Entry.Type, evidence.EntryTypeReport)
	}
}

func TestE2E_PrescribeSmartReportLifecycle(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	server, serverErr := NewServer(Options{
		Name:         "test",
		Version:      "0.0.1",
		EvidencePath: dir,
		Environment:  "test",
		Signer:       newTestSigner(t),
	})
	if serverErr != nil {
		t.Fatalf("NewServer: %v", serverErr)
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

	prescribeResult, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name: "prescribe_smart",
		Arguments: map[string]any{
			"actor": map[string]any{
				"type":   "agent",
				"id":     "test-agent",
				"origin": "e2e-test",
			},
			"tool":      "kubectl",
			"operation": "apply",
			"resource":  "deployment/nginx",
			"namespace": "default",
		},
	})
	if err != nil {
		t.Fatalf("prescribe_smart: %v", err)
	}

	var prescribeOut PrescribeOutput
	if err := extractStructuredContent(prescribeResult, &prescribeOut); err != nil {
		t.Fatalf("parse prescribe_smart output: %v", err)
	}
	if !prescribeOut.OK {
		t.Fatalf("prescribe_smart not ok: %+v", prescribeOut)
	}
	if prescribeOut.PrescriptionID == "" {
		t.Fatal("prescribe_smart returned empty prescription_id")
	}
	if len(prescribeOut.RiskInputs) != 0 {
		t.Fatalf("risk_inputs = %+v, want none without assessment", prescribeOut.RiskInputs)
	}

	reportResult, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name: "report",
		Arguments: map[string]any{
			"prescription_id": prescribeOut.PrescriptionID,
			"verdict":         "success",
			"exit_code":       0,
		},
	})
	if err != nil {
		t.Fatalf("report: %v", err)
	}

	var reportOut ReportOutput
	if err := extractStructuredContent(reportResult, &reportOut); err != nil {
		t.Fatalf("parse report output: %v", err)
	}
	if !reportOut.OK {
		t.Fatalf("report not ok: %+v", reportOut)
	}
}

func TestE2E_ListTools_UsesSplitPrescribeSurface(t *testing.T) {
	t.Parallel()

	server, err := NewServer(Options{
		Name:         "test",
		Version:      "0.0.1",
		EvidencePath: t.TempDir(),
		Environment:  "test",
		Signer:       newTestSigner(t),
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

	toolDefs := make(map[string]*mcp.Tool)
	for _, tool := range tools.Tools {
		toolDefs[tool.Name] = tool
	}

	for _, name := range []string{"run_command", "collect_diagnostics", "write_file", "prescribe_full", "prescribe_smart", "report", "get_event"} {
		if _, ok := toolDefs[name]; !ok {
			t.Fatalf("missing tool %q in tools/list response", name)
		}
	}
	if _, ok := toolDefs["prescribe"]; ok {
		t.Fatal("legacy prescribe tool should not be registered")
	}
	for _, name := range []string{"prescribe_full", "prescribe_smart"} {
		if toolDefs[name].Annotations == nil {
			t.Fatalf("%s tool missing annotations", name)
		}
		if toolDefs[name].Annotations.ReadOnlyHint {
			t.Fatalf("%s tool must not advertise readOnlyHint=true", name)
		}
	}
}

func TestE2E_UnprescribedReport(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	server, serverErr := NewServer(Options{
		Name:         "test",
		Version:      "0.0.1",
		EvidencePath: dir,
		Signer:       newTestSigner(t),
	})
	if serverErr != nil {
		t.Fatalf("NewServer: %v", serverErr)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	st, ct := mcp.NewInMemoryTransports()
	if _, err := server.Connect(ctx, st, nil); err != nil {
		t.Fatalf("server connect: %v", err)
	}

	client := mcp.NewClient(
		&mcp.Implementation{Name: "test-client", Version: "0.0.1"},
		nil,
	)
	session, err := client.Connect(ctx, ct, nil)
	if err != nil {
		t.Fatalf("client connect: %v", err)
	}
	defer func() { _ = session.Close() }()

	// Report with unknown prescription
	result, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name: "report",
		Arguments: map[string]any{
			"prescription_id": "NONEXISTENT",
			"verdict":         "failure",
			"exit_code":       1,
		},
	})
	if err != nil {
		t.Fatalf("report: %v", err)
	}

	var out ReportOutput
	if err := extractStructuredContent(result, &out); err != nil {
		t.Fatalf("parse output: %v", err)
	}
	if out.OK {
		t.Error("report with unknown prescription should not be ok")
	}
	if out.Error == nil || out.Error.Code != "not_found" {
		t.Errorf("expected error code 'not_found', got %+v", out.Error)
	}
}

func TestE2E_ListResources(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	server, serverErr := NewServer(Options{
		Name:         "test",
		Version:      "0.0.1",
		EvidencePath: dir,
		Signer:       newTestSigner(t),
	})
	if serverErr != nil {
		t.Fatalf("NewServer: %v", serverErr)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	st, ct := mcp.NewInMemoryTransports()
	if _, err := server.Connect(ctx, st, nil); err != nil {
		t.Fatalf("server connect: %v", err)
	}

	client := mcp.NewClient(
		&mcp.Implementation{Name: "test-client", Version: "0.0.1"},
		nil,
	)
	session, err := client.Connect(ctx, ct, nil)
	if err != nil {
		t.Fatalf("client connect: %v", err)
	}
	defer func() { _ = session.Close() }()

	// List resources
	resources, err := session.ListResources(ctx, nil)
	if err != nil {
		t.Fatalf("list resources: %v", err)
	}
	resourceNames := make(map[string]bool, len(resources.Resources))
	for _, r := range resources.Resources {
		resourceNames[r.Name] = true
	}
	for _, want := range []string{"evidra-evidence-manifest", "evidra-scorecard-aggregate"} {
		if !resourceNames[want] {
			t.Errorf("expected %q in resource list, got: %v", want, resources.Resources)
		}
	}
}

func TestE2E_WriteFile(t *testing.T) {
	dir := t.TempDir()

	server, serverErr := NewServer(Options{
		Name:         "test",
		Version:      "0.0.1",
		EvidencePath: dir,
		Signer:       newTestSigner(t),
	})
	if serverErr != nil {
		t.Fatalf("NewServer: %v", serverErr)
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

	targetPath := filepath.Join(dir, "configs", "app.yaml")
	wantContent := "apiVersion: v1\nkind: ConfigMap\nmetadata:\n  name: app\n"

	result, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name: "write_file",
		Arguments: map[string]any{
			"path":    targetPath,
			"content": wantContent,
		},
	})
	if err != nil {
		t.Fatalf("write_file: %v", err)
	}

	var out WriteFileOutput
	if err := extractStructuredContent(result, &out); err != nil {
		t.Fatalf("parse write_file output: %v", err)
	}
	if !out.OK {
		t.Fatalf("write_file not ok: %+v", out)
	}
	if out.Message == "" {
		t.Fatalf("write_file returned empty message: %+v", out)
	}

	gotContent, err := os.ReadFile(targetPath)
	if err != nil {
		t.Fatalf("ReadFile(%s): %v", targetPath, err)
	}
	if string(gotContent) != wantContent {
		t.Fatalf("file content mismatch:\n got: %q\nwant: %q", string(gotContent), wantContent)
	}
}

func TestE2E_CollectDiagnostics(t *testing.T) {
	dir := t.TempDir()

	kubectlPath := filepath.Join(dir, "kubectl")
	kubectlScript := `#!/bin/sh
case "$*" in
  "get pods -n demo")
    cat <<'EOF'
NAME      READY   STATUS             RESTARTS   AGE
web-abc   0/1     CrashLoopBackOff   3          2m
web-def   1/1     Running            0          5m
EOF
    ;;
  "describe deployment/web -n demo")
    cat <<'EOF'
Name:                   web
Namespace:              demo
Replicas:               1 desired | 1 updated | 1 total | 0 available | 1 unavailable
Conditions:
  Type           Status  Reason
  ----           ------  ------
  Available      False   MinimumReplicasUnavailable
Events:
  Type     Reason            Age   From                   Message
  ----     ------            ----  ----                   -------
  Warning  FailedPull        2m    kubelet                Failed to pull image "nginx:99.99"
EOF
    ;;
  "get events -n demo --sort-by=.lastTimestamp")
    cat <<'EOF'
LAST SEEN   TYPE      REASON      OBJECT      MESSAGE
2m          Warning   FailedPull  pod/web-abc Failed to pull image "nginx:99.99"
EOF
    ;;
  "logs web-abc -n demo --tail=50")
    cat <<'EOF'
panic: image pull failed
back-off pulling image
check image tag
EOF
    ;;
  *)
    echo "unexpected kubectl args: $*" >&2
    exit 9
    ;;
esac
`
	if err := os.WriteFile(kubectlPath, []byte(kubectlScript), 0o755); err != nil {
		t.Fatalf("WriteFile(kubectl): %v", err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))

	server, serverErr := NewServer(Options{
		Name:         "test",
		Version:      "0.0.1",
		EvidencePath: dir,
		Signer:       newTestSigner(t),
	})
	if serverErr != nil {
		t.Fatalf("NewServer: %v", serverErr)
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

	result, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name: "collect_diagnostics",
		Arguments: map[string]any{
			"namespace":    "demo",
			"workload":     "deployment/web",
			"include_logs": true,
		},
	})
	if err != nil {
		t.Fatalf("collect_diagnostics: %v", err)
	}

	var out CollectDiagnosticsOutput
	if err := extractStructuredContent(result, &out); err != nil {
		t.Fatalf("parse collect_diagnostics output: %v", err)
	}
	if !out.OK {
		t.Fatalf("collect_diagnostics not ok: %+v", out)
	}
	if len(out.Commands) != 4 {
		t.Fatalf("commands len=%d, want 4 (%v)", len(out.Commands), out.Commands)
	}
	if out.Commands[3] != "kubectl logs web-abc -n demo --tail=50" {
		t.Fatalf("logs command=%q, want kubectl logs web-abc -n demo --tail=50", out.Commands[3])
	}
	if len(out.Findings) == 0 {
		t.Fatalf("collect_diagnostics returned no findings: %+v", out)
	}
	if out.Summary == "" {
		t.Fatalf("collect_diagnostics returned empty summary: %+v", out)
	}
}

func TestE2E_ProtocolV1Fields(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	server, serverErr := NewServer(Options{
		Name:         "test",
		Version:      "0.0.1",
		EvidencePath: dir,
		Environment:  "test",
		Signer:       newTestSigner(t),
	})
	if serverErr != nil {
		t.Fatalf("NewServer: %v", serverErr)
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

	// Prescribe with all protocol v1.0 fields
	prescribeResult, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name: "prescribe_full",
		Arguments: map[string]any{
			"actor": map[string]any{
				"type":        "ai_agent",
				"id":          "test-agent",
				"origin":      "mcp",
				"instance_id": "pod-abc123",
				"version":     "v1.3",
			},
			"tool":         "kubectl",
			"operation":    "apply",
			"raw_artifact": "apiVersion: v1\nkind: ConfigMap\nmetadata:\n  name: test-cm\n  namespace: staging\ndata:\n  key: value\n",
			"session_id":   "session-e2e-001",
			"trace_id":     "trace-e2e-001",
			"span_id":      "span-prescribe-001",
			"scope_dimensions": map[string]any{
				"cluster":   "staging-us-east",
				"namespace": "staging",
			},
		},
	})
	if err != nil {
		t.Fatalf("prescribe: %v", err)
	}

	var prescribeOut PrescribeOutput
	if err := extractStructuredContent(prescribeResult, &prescribeOut); err != nil {
		t.Fatalf("parse prescribe output: %v", err)
	}
	if !prescribeOut.OK {
		t.Fatalf("prescribe not ok: %+v", prescribeOut)
	}

	// Read the prescription entry and verify protocol fields
	readResult, err := session.ReadResource(ctx, &mcp.ReadResourceParams{
		URI: "evidra://event/" + prescribeOut.PrescriptionID,
	})
	if err != nil {
		t.Fatalf("read resource: %v", err)
	}
	if len(readResult.Contents) == 0 {
		t.Fatal("read resource returned no contents")
	}

	var entry evidence.EvidenceEntry
	if err := json.Unmarshal([]byte(readResult.Contents[0].Text), &entry); err != nil {
		t.Fatalf("parse entry: %v", err)
	}

	// Verify session_id
	if entry.SessionID != "session-e2e-001" {
		t.Errorf("session_id: got %q, want %q", entry.SessionID, "session-e2e-001")
	}

	// Verify caller-provided trace_id
	if entry.TraceID != "trace-e2e-001" {
		t.Errorf("trace_id: got %q, want %q", entry.TraceID, "trace-e2e-001")
	}

	// Verify span_id
	if entry.SpanID != "span-prescribe-001" {
		t.Errorf("span_id: got %q, want %q", entry.SpanID, "span-prescribe-001")
	}

	// Verify actor extended fields
	if entry.Actor.InstanceID != "pod-abc123" {
		t.Errorf("actor.instance_id: got %q, want %q", entry.Actor.InstanceID, "pod-abc123")
	}
	if entry.Actor.Version != "v1.3" {
		t.Errorf("actor.version: got %q, want %q", entry.Actor.Version, "v1.3")
	}

	// Verify scope_dimensions
	if entry.ScopeDimensions == nil {
		t.Fatal("scope_dimensions is nil")
	}
	if entry.ScopeDimensions["cluster"] != "staging-us-east" {
		t.Errorf("scope_dimensions.cluster: got %q, want %q",
			entry.ScopeDimensions["cluster"], "staging-us-east")
	}
	if entry.ScopeDimensions["namespace"] != "staging" {
		t.Errorf("scope_dimensions.namespace: got %q, want %q",
			entry.ScopeDimensions["namespace"], "staging")
	}

	// Report with protocol v1.0 fields
	reportResult, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name: "report",
		Arguments: map[string]any{
			"prescription_id": prescribeOut.PrescriptionID,
			"verdict":         "success",
			"exit_code":       0,
			"session_id":      "session-e2e-001",
			"span_id":         "span-report-001",
			"parent_span_id":  "span-prescribe-001",
		},
	})
	if err != nil {
		t.Fatalf("report: %v", err)
	}

	var reportOut ReportOutput
	if err := extractStructuredContent(reportResult, &reportOut); err != nil {
		t.Fatalf("parse report output: %v", err)
	}
	if !reportOut.OK {
		t.Fatalf("report not ok: %+v", reportOut)
	}

	// Read the report entry and verify protocol fields
	readResult, err = session.ReadResource(ctx, &mcp.ReadResourceParams{
		URI: "evidra://event/" + reportOut.ReportID,
	})
	if err != nil {
		t.Fatalf("read report resource: %v", err)
	}
	if len(readResult.Contents) == 0 {
		t.Fatal("read report resource returned no contents")
	}

	var reportEntry evidence.EvidenceEntry
	if err := json.Unmarshal([]byte(readResult.Contents[0].Text), &reportEntry); err != nil {
		t.Fatalf("parse report entry: %v", err)
	}

	if reportEntry.SessionID != "session-e2e-001" {
		t.Errorf("report session_id: got %q, want %q", reportEntry.SessionID, "session-e2e-001")
	}
	if reportEntry.SpanID != "span-report-001" {
		t.Errorf("report span_id: got %q, want %q", reportEntry.SpanID, "span-report-001")
	}
	if reportEntry.ParentSpanID != "span-prescribe-001" {
		t.Errorf("report parent_span_id: got %q, want %q", reportEntry.ParentSpanID, "span-prescribe-001")
	}
}

// testSigner implements the evidence.Signer interface for tests.
type testSigner struct {
	priv ed25519.PrivateKey
	pub  ed25519.PublicKey
}

func newTestSigner(t *testing.T) *testSigner {
	t.Helper()
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	return &testSigner{priv: priv, pub: pub}
}

func (s *testSigner) Sign(payload []byte) []byte      { return ed25519.Sign(s.priv, payload) }
func (s *testSigner) Verify(payload, sig []byte) bool { return ed25519.Verify(s.pub, payload, sig) }
func (s *testSigner) PublicKey() ed25519.PublicKey    { return s.pub }

func TestE2E_SignedEntries(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	signer := newTestSigner(t)

	server, serverErr := NewServer(Options{
		Name:         "test",
		Version:      "0.0.1",
		EvidencePath: dir,
		Environment:  "test",
		Signer:       signer,
	})
	if serverErr != nil {
		t.Fatalf("NewServer: %v", serverErr)
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

	// Prescribe
	prescribeResult, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name: "prescribe_full",
		Arguments: map[string]any{
			"actor": map[string]any{
				"type":   "agent",
				"id":     "test-agent",
				"origin": "e2e-sign-test",
			},
			"tool":         "kubectl",
			"operation":    "apply",
			"raw_artifact": "apiVersion: apps/v1\nkind: Deployment\nmetadata:\n  name: nginx\n  namespace: default\nspec:\n  replicas: 1\n  selector:\n    matchLabels:\n      app: nginx\n  template:\n    metadata:\n      labels:\n        app: nginx\n    spec:\n      containers:\n      - name: nginx\n        image: nginx:1.21\n",
		},
	})
	if err != nil {
		t.Fatalf("prescribe: %v", err)
	}

	var prescribeOut PrescribeOutput
	if err := extractStructuredContent(prescribeResult, &prescribeOut); err != nil {
		t.Fatalf("parse prescribe output: %v", err)
	}
	if !prescribeOut.OK {
		t.Fatalf("prescribe not ok: %+v", prescribeOut)
	}

	// Read back the prescription entry and verify it has a signature
	readResult, err := session.ReadResource(ctx, &mcp.ReadResourceParams{
		URI: "evidra://event/" + prescribeOut.PrescriptionID,
	})
	if err != nil {
		t.Fatalf("read resource event: %v", err)
	}
	if len(readResult.Contents) == 0 {
		t.Fatal("read resource returned no contents")
	}
	var entry evidence.EvidenceEntry
	if err := json.Unmarshal([]byte(readResult.Contents[0].Text), &entry); err != nil {
		t.Fatalf("parse resource event: %v", err)
	}
	if entry.Signature == "" {
		t.Fatal("prescription entry has empty signature; expected signed entry")
	}

	// Report
	reportResult, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name: "report",
		Arguments: map[string]any{
			"prescription_id": prescribeOut.PrescriptionID,
			"verdict":         "success",
			"exit_code":       0,
		},
	})
	if err != nil {
		t.Fatalf("report: %v", err)
	}

	var reportOut ReportOutput
	if err := extractStructuredContent(reportResult, &reportOut); err != nil {
		t.Fatalf("parse report output: %v", err)
	}
	if !reportOut.OK {
		t.Fatalf("report not ok: %+v", reportOut)
	}

	// Read back report entry and verify signature
	readResult, err = session.ReadResource(ctx, &mcp.ReadResourceParams{
		URI: "evidra://event/" + reportOut.ReportID,
	})
	if err != nil {
		t.Fatalf("read report resource: %v", err)
	}
	if len(readResult.Contents) == 0 {
		t.Fatal("read report resource returned no contents")
	}
	var reportEntry evidence.EvidenceEntry
	if err := json.Unmarshal([]byte(readResult.Contents[0].Text), &reportEntry); err != nil {
		t.Fatalf("parse report entry: %v", err)
	}
	if reportEntry.Signature == "" {
		t.Fatal("report entry has empty signature; expected signed entry")
	}

	// Validate the entire chain with signatures
	if err := evidence.ValidateChainWithSignatures(dir, signer.pub); err != nil {
		t.Fatalf("ValidateChainWithSignatures: %v", err)
	}
}

// extractStructuredContent parses the structured content from a CallToolResult.
func extractStructuredContent(result *mcp.CallToolResult, v any) error {
	if result.StructuredContent != nil {
		b, err := json.Marshal(result.StructuredContent)
		if err != nil {
			return err
		}
		return json.Unmarshal(b, v)
	}
	// Fallback: parse from text content
	if len(result.Content) > 0 {
		if tc, ok := result.Content[0].(*mcp.TextContent); ok {
			return json.Unmarshal([]byte(tc.Text), v)
		}
	}
	return json.Unmarshal([]byte("{}"), v)
}
