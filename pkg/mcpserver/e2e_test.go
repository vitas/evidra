package mcpserver

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/json"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"samebits.com/evidra-benchmark/pkg/evidence"
)

func TestE2E_PrescribeReportLifecycle(t *testing.T) {
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

	// List tools — verify all 3 registered
	tools, err := session.ListTools(ctx, nil)
	if err != nil {
		t.Fatalf("list tools: %v", err)
	}
	toolNames := make(map[string]bool)
	for _, tool := range tools.Tools {
		toolNames[tool.Name] = true
	}
	for _, name := range []string{"prescribe", "report", "get_event"} {
		if !toolNames[name] {
			t.Errorf("missing tool %q in tools/list response", name)
		}
	}

	// Prescribe
	prescribeResult, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name: "prescribe",
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
	if prescribeOut.RiskLevel == "" {
		t.Fatal("prescribe returned empty risk_level")
	}
	if prescribeOut.ArtifactDigest == "" {
		t.Fatal("prescribe returned empty artifact_digest")
	}

	// Report
	reportResult, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name: "report",
		Arguments: map[string]any{
			"prescription_id": prescribeOut.PrescriptionID,
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

	// Get event — retrieve the prescription entry via resource template.
	// Note: the get_event tool has a json.RawMessage field (Payload) that
	// the SDK auto-derives as type [null, array], but actual payloads are
	// JSON objects. We verify the entry via the evidra://event/{id} resource
	// which bypasses structured-output schema validation.
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
	if entry.EntryID != prescribeOut.PrescriptionID {
		t.Errorf("resource event entry_id mismatch: got %q, want %q",
			entry.EntryID, prescribeOut.PrescriptionID)
	}
	if entry.Type != evidence.EntryTypePrescribe {
		t.Errorf("resource event type: got %q, want %q", entry.Type, evidence.EntryTypePrescribe)
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
	found := false
	for _, r := range resources.Resources {
		if r.Name == "evidra-evidence-manifest" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected evidra-evidence-manifest in resource list")
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
		Name: "prescribe",
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
		Name: "prescribe",
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
