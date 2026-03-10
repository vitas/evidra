package lifecycle

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"samebits.com/evidra-benchmark/internal/testutil"
	"samebits.com/evidra-benchmark/pkg/evidence"
)

func TestServiceReport_DeclinedStoresDecisionContext(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	svc := NewService(Options{
		EvidencePath: dir,
		Signer:       testutil.TestSigner(t),
	})

	prescribeOut, err := svc.Prescribe(context.Background(), PrescribeInput{
		Actor:       evidence.Actor{Type: "agent", ID: "agent-1", Provenance: "mcp"},
		Tool:        "kubectl",
		Operation:   "apply",
		RawArtifact: []byte("apiVersion: v1\nkind: ConfigMap\nmetadata:\n  name: cm1\n  namespace: default\n"),
		SessionID:   "session-1",
		TraceID:     "trace-1",
	})
	if err != nil {
		t.Fatalf("Prescribe: %v", err)
	}

	reportOut, err := svc.Report(context.Background(), ReportInput{
		PrescriptionID: prescribeOut.PrescriptionID,
		Verdict:        evidence.VerdictDeclined,
		DecisionContext: &evidence.DecisionContext{
			Trigger: "risk_threshold_exceeded",
			Reason:  "risk_level=critical and blast_radius covers production namespace",
		},
	})
	if err != nil {
		t.Fatalf("Report: %v", err)
	}
	if reportOut.ReportID == "" {
		t.Fatal("expected report ID")
	}

	entries, err := evidence.ReadAllEntriesAtPath(dir)
	if err != nil {
		t.Fatalf("ReadAllEntriesAtPath: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("entry count = %d, want 2", len(entries))
	}

	var payload evidence.ReportPayload
	if err := json.Unmarshal(entries[1].Payload, &payload); err != nil {
		t.Fatalf("unmarshal report payload: %v", err)
	}
	if payload.Verdict != evidence.VerdictDeclined {
		t.Fatalf("verdict = %q, want %q", payload.Verdict, evidence.VerdictDeclined)
	}
	if payload.DecisionContext == nil {
		t.Fatal("decision_context missing")
	}
	if payload.DecisionContext.Trigger != "risk_threshold_exceeded" {
		t.Fatalf("trigger = %q", payload.DecisionContext.Trigger)
	}
}

func TestServiceReport_DeclinedRequiresDecisionReason(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	svc := NewService(Options{
		EvidencePath: dir,
		Signer:       testutil.TestSigner(t),
	})

	prescribeOut, err := svc.Prescribe(context.Background(), PrescribeInput{
		Actor:       evidence.Actor{Type: "agent", ID: "agent-1", Provenance: "mcp"},
		Tool:        "kubectl",
		Operation:   "apply",
		RawArtifact: []byte("apiVersion: v1\nkind: ConfigMap\nmetadata:\n  name: cm1\n  namespace: default\n"),
		SessionID:   "session-1",
		TraceID:     "trace-1",
	})
	if err != nil {
		t.Fatalf("Prescribe: %v", err)
	}

	_, err = svc.Report(context.Background(), ReportInput{
		PrescriptionID: prescribeOut.PrescriptionID,
		Verdict:        evidence.VerdictDeclined,
		DecisionContext: &evidence.DecisionContext{
			Trigger: "risk_threshold_exceeded",
		},
	})
	if err == nil {
		t.Fatal("expected declined validation error")
	}
	if ErrorCode(err) != ErrCodeInvalidInput {
		t.Fatalf("error code = %q, want %q", ErrorCode(err), ErrCodeInvalidInput)
	}
	if !strings.Contains(err.Error(), "decision_context.reason is required") {
		t.Fatalf("error = %q", err.Error())
	}
}

func intPtr(v int) *int {
	return &v
}
