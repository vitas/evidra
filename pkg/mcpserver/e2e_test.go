package mcpserver

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"samebits.com/evidra-benchmark/pkg/evidence"
)

func TestE2E_PrescribeReportLifecycle(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	server := NewServer(Options{
		Name:         "test",
		Version:      "0.0.1",
		EvidencePath: dir,
		Environment:  "test",
	})

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	st, ct := mcp.NewInMemoryTransports()
	serverSession, err := server.Connect(ctx, st, nil)
	if err != nil {
		t.Fatalf("server connect: %v", err)
	}
	defer serverSession.Wait()

	client := mcp.NewClient(
		&mcp.Implementation{Name: "test-client", Version: "0.0.1"},
		nil,
	)
	session, err := client.Connect(ctx, ct, nil)
	if err != nil {
		t.Fatalf("client connect: %v", err)
	}
	defer session.Close()

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

	var reportOut ReportOutput
	if err := extractStructuredContent(reportResult, &reportOut); err != nil {
		t.Fatalf("parse report output: %v", err)
	}
	if !reportOut.OK {
		t.Fatalf("report not ok: %+v", reportOut)
	}
	if reportOut.ReportID == "" {
		t.Fatal("report returned empty report_id")
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

	server := NewServer(Options{
		Name:         "test",
		Version:      "0.0.1",
		EvidencePath: dir,
	})

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
	defer session.Close()

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

	server := NewServer(Options{
		Name:         "test",
		Version:      "0.0.1",
		EvidencePath: dir,
	})

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
	defer session.Close()

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
