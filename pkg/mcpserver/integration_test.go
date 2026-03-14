package mcpserver

import (
	"os"
	"path/filepath"
	"testing"

	"samebits.com/evidra/internal/canon"
	"samebits.com/evidra/internal/testutil"
	"samebits.com/evidra/pkg/evidence"
)

func TestPrescribeReport_Lifecycle(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	svc := &MCPService{
		evidencePath: dir,
		signer:       testutil.TestSigner(t),
	}

	// Prescribe
	prescOutput := svc.Prescribe(PrescribeInput{
		Actor:       InputActor{Type: "ai_agent", ID: "test-agent", Origin: "mcp"},
		Tool:        "kubectl",
		Operation:   "apply",
		RawArtifact: "apiVersion: apps/v1\nkind: Deployment\nmetadata:\n  name: test\n  namespace: staging",
	})

	if !prescOutput.OK {
		t.Fatalf("prescribe failed: %v", prescOutput.Error)
	}
	if prescOutput.PrescriptionID == "" {
		t.Error("prescription_id must not be empty")
	}
	if prescOutput.RiskLevel == "" {
		t.Error("risk_level must not be empty")
	}

	// Report
	reportOutput := svc.Report(ReportInput{
		PrescriptionID: prescOutput.PrescriptionID,
		Verdict:        evidence.VerdictSuccess,
		ExitCode:       intPtr(0),
		ArtifactDigest: prescOutput.ArtifactDigest,
	})

	if !reportOutput.OK {
		t.Fatalf("report failed: %v", reportOutput.Error)
	}
	if reportOutput.ReportID == "" {
		t.Error("report_id must not be empty")
	}

	// Read evidence and verify both entries exist.
	entries, err := evidence.ReadAllEntriesAtPath(dir)
	if err != nil {
		t.Fatalf("ReadAllEntriesAtPath: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("expected 2 evidence entries, got %d", len(entries))
	}
	if entries[0].Type != evidence.EntryTypePrescribe {
		t.Errorf("first entry type: got %q, want prescribe", entries[0].Type)
	}
	if entries[1].Type != evidence.EntryTypeReport {
		t.Errorf("second entry type: got %q, want report", entries[1].Type)
	}
}

func TestReport_ExplicitActor(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	svc := &MCPService{
		evidencePath: dir,
		signer:       testutil.TestSigner(t),
	}

	prescOutput := svc.Prescribe(PrescribeInput{
		Actor:       InputActor{Type: "ai_agent", ID: "agent-1", Origin: "mcp"},
		Tool:        "kubectl",
		Operation:   "apply",
		RawArtifact: "apiVersion: v1\nkind: ConfigMap\nmetadata:\n  name: test\n  namespace: default",
	})

	if !prescOutput.OK {
		t.Fatalf("prescribe failed: %v", prescOutput.Error)
	}

	// Report with explicit different actor
	reportOutput := svc.Report(ReportInput{
		PrescriptionID: prescOutput.PrescriptionID,
		Verdict:        evidence.VerdictSuccess,
		ExitCode:       intPtr(0),
		Actor:          InputActor{Type: "ai_agent", ID: "agent-2", Origin: "mcp"},
	})

	if !reportOutput.OK {
		t.Fatalf("report failed: %v", reportOutput.Error)
	}

	entries, _ := evidence.ReadAllEntriesAtPath(dir)
	if len(entries) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(entries))
	}
	if entries[1].Actor.ID != "agent-2" {
		t.Errorf("report actor: got %q, want agent-2", entries[1].Actor.ID)
	}
}

func TestPrescribeReport_ChainIntegrity(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	svc := &MCPService{
		evidencePath: dir,
		signer:       testutil.TestSigner(t),
	}

	// Prescribe
	svc.Prescribe(PrescribeInput{
		Actor:       InputActor{Type: "ai_agent", ID: "agent-1", Origin: "mcp"},
		Tool:        "kubectl",
		Operation:   "apply",
		RawArtifact: "apiVersion: v1\nkind: ConfigMap\nmetadata:\n  name: test\n  namespace: default",
	})

	// Report
	entries, _ := evidence.ReadAllEntriesAtPath(dir)
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry after prescribe, got %d", len(entries))
	}

	svc.Report(ReportInput{
		PrescriptionID: entries[0].EntryID,
		Verdict:        evidence.VerdictSuccess,
		ExitCode:       intPtr(0),
	})

	// Verify chain.
	allEntries, _ := evidence.ReadAllEntriesAtPath(dir)
	if len(allEntries) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(allEntries))
	}

	// Second entry's PreviousHash should match first entry's Hash.
	if allEntries[1].PreviousHash != allEntries[0].Hash {
		t.Errorf("chain broken: entry[1].PreviousHash=%q, entry[0].Hash=%q",
			allEntries[1].PreviousHash, allEntries[0].Hash)
	}
}

func TestReport_SessionMismatchRejected(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	svc := &MCPService{
		evidencePath: dir,
		signer:       testutil.TestSigner(t),
	}

	presc := svc.Prescribe(PrescribeInput{
		Actor:       InputActor{Type: "ai_agent", ID: "agent-1", Origin: "mcp"},
		Tool:        "kubectl",
		Operation:   "apply",
		RawArtifact: "apiVersion: v1\nkind: ConfigMap\nmetadata:\n  name: test\n  namespace: default",
		SessionID:   "session-a",
	})
	if !presc.OK {
		t.Fatalf("prescribe failed: %v", presc.Error)
	}

	report := svc.Report(ReportInput{
		PrescriptionID: presc.PrescriptionID,
		Verdict:        evidence.VerdictSuccess,
		ExitCode:       intPtr(0),
		SessionID:      "session-b",
	})
	if report.OK {
		t.Fatalf("expected session mismatch error, got success report_id=%s", report.ReportID)
	}
	if report.Error == nil || report.Error.Code != "invalid_input" {
		t.Fatalf("expected invalid_input error, got %+v", report.Error)
	}

	entries, err := evidence.ReadAllEntriesAtPath(dir)
	if err != nil {
		t.Fatalf("ReadAllEntriesAtPath: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("entry count=%d, want 1 (report must not be written)", len(entries))
	}
}

func TestReport_DeclinedRequiresReasonAndEchoesDecisionContext(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	svc := &MCPService{
		evidencePath: dir,
		signer:       testutil.TestSigner(t),
	}

	presc := svc.Prescribe(PrescribeInput{
		Actor:       InputActor{Type: "ai_agent", ID: "agent-1", Origin: "mcp"},
		Tool:        "kubectl",
		Operation:   "apply",
		RawArtifact: "apiVersion: v1\nkind: ConfigMap\nmetadata:\n  name: test\n  namespace: default",
		SessionID:   "session-declined",
	})
	if !presc.OK {
		t.Fatalf("prescribe failed: %v", presc.Error)
	}

	missingReason := svc.Report(ReportInput{
		PrescriptionID: presc.PrescriptionID,
		Verdict:        evidence.VerdictDeclined,
		DecisionContext: &evidence.DecisionContext{
			Trigger: "risk_threshold_exceeded",
		},
	})
	if missingReason.OK {
		t.Fatal("expected invalid_input for missing decline reason")
	}
	if missingReason.Error == nil || missingReason.Error.Code != "invalid_input" {
		t.Fatalf("expected invalid_input error, got %+v", missingReason.Error)
	}

	report := svc.Report(ReportInput{
		PrescriptionID: presc.PrescriptionID,
		Verdict:        evidence.VerdictDeclined,
		DecisionContext: &evidence.DecisionContext{
			Trigger: "risk_threshold_exceeded",
			Reason:  "risk_level=critical and blast_radius covers production namespace",
		},
	})
	if !report.OK {
		t.Fatalf("declined report failed: %+v", report)
	}
	if report.Verdict != evidence.VerdictDeclined {
		t.Fatalf("verdict = %q, want %q", report.Verdict, evidence.VerdictDeclined)
	}
	if report.ExitCode != nil {
		t.Fatalf("exit_code = %v, want nil", report.ExitCode)
	}
	if report.DecisionContext == nil {
		t.Fatal("decision_context missing")
	}
	if report.DecisionContext.Trigger != "risk_threshold_exceeded" {
		t.Fatalf("trigger = %q", report.DecisionContext.Trigger)
	}

	entries, err := evidence.ReadAllEntriesAtPath(dir)
	if err != nil {
		t.Fatalf("ReadAllEntriesAtPath: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("entry count=%d, want 2", len(entries))
	}
}

func TestPrescribe_DefaultTraceIDMatchesSessionIDWhenOmitted(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	svc := &MCPService{
		evidencePath: dir,
		signer:       testutil.TestSigner(t),
	}

	presc := svc.Prescribe(PrescribeInput{
		Actor:       InputActor{Type: "ai_agent", ID: "agent-1", Origin: "mcp"},
		Tool:        "kubectl",
		Operation:   "apply",
		RawArtifact: "apiVersion: v1\nkind: ConfigMap\nmetadata:\n  name: test\n  namespace: default",
		SessionID:   "session-mcp-default-trace",
	})
	if !presc.OK {
		t.Fatalf("prescribe failed: %v", presc.Error)
	}

	entries, err := evidence.ReadAllEntriesAtPath(dir)
	if err != nil {
		t.Fatalf("ReadAllEntriesAtPath: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("entry count=%d, want 1", len(entries))
	}
	if entries[0].SessionID != "session-mcp-default-trace" {
		t.Fatalf("session_id=%q, want session-mcp-default-trace", entries[0].SessionID)
	}
	if entries[0].TraceID != "session-mcp-default-trace" {
		t.Fatalf("trace_id=%q, want session-mcp-default-trace", entries[0].TraceID)
	}
}

func TestPrescribe_InvalidCanonicalScopeClassRejected(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	svc := &MCPService{
		evidencePath: dir,
		signer:       testutil.TestSigner(t),
	}

	presc := svc.Prescribe(PrescribeInput{
		Actor:       InputActor{Type: "ai_agent", ID: "agent-1", Origin: "mcp"},
		Tool:        "terraform",
		Operation:   "apply",
		RawArtifact: `{"noop":true}`,
		CanonicalAction: &canon.CanonicalAction{
			Tool:              "terraform",
			Operation:         "apply",
			OperationClass:    "mutate",
			ScopeClass:        "prod-eu",
			ResourceCount:     1,
			ResourceShapeHash: "sha256:test",
		},
	})
	if presc.OK {
		t.Fatal("expected invalid_input error")
	}
	if presc.Error == nil || presc.Error.Code != "invalid_input" {
		t.Fatalf("expected invalid_input error, got %+v", presc.Error)
	}

	entries, err := evidence.ReadAllEntriesAtPath(dir)
	if err != nil {
		t.Fatalf("ReadAllEntriesAtPath: %v", err)
	}
	if len(entries) != 0 {
		t.Fatalf("entry count=%d, want 0", len(entries))
	}
}

func TestPrescribe_BestEffortWriteModeSuppressesStoreError(t *testing.T) {
	t.Parallel()

	tmp := t.TempDir()
	evidencePath := filepath.Join(tmp, "legacy.log")
	if err := os.WriteFile(evidencePath, []byte("legacy"), 0o644); err != nil {
		t.Fatalf("write evidence file: %v", err)
	}

	svc := &MCPService{
		evidencePath:     evidencePath,
		signer:           testutil.TestSigner(t),
		bestEffortWrites: true,
	}

	presc := svc.Prescribe(PrescribeInput{
		Actor:       InputActor{Type: "ai_agent", ID: "agent-1", Origin: "mcp"},
		Tool:        "kubectl",
		Operation:   "apply",
		RawArtifact: "apiVersion: v1\nkind: ConfigMap\nmetadata:\n  name: test\n  namespace: default",
		SessionID:   "session-mcp-best-effort",
	})
	if !presc.OK {
		t.Fatalf("prescribe failed in best_effort mode: %+v", presc.Error)
	}
}
