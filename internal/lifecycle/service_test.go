package lifecycle

import (
	"context"
	"encoding/json"
	"testing"

	"samebits.com/evidra-benchmark/internal/canon"
	"samebits.com/evidra-benchmark/internal/testutil"
	"samebits.com/evidra-benchmark/pkg/evidence"
)

func TestServicePrescribe_ParseErrorWritesCanonFailure(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	svc := NewService(Options{
		EvidencePath: dir,
		Signer:       testutil.TestSigner(t),
	})

	_, err := svc.Prescribe(context.Background(), PrescribeInput{
		Actor:       evidence.Actor{Type: "agent", ID: "agent-1", Provenance: "mcp"},
		Tool:        "terraform",
		Operation:   "apply",
		RawArtifact: []byte("not valid json {{{"),
		SessionID:   "session-1",
	})
	if err == nil {
		t.Fatal("expected parse error")
	}
	if ErrorCode(err) != ErrCodeParseError {
		t.Fatalf("error code = %q, want %q", ErrorCode(err), ErrCodeParseError)
	}

	entries, readErr := evidence.ReadAllEntriesAtPath(dir)
	if readErr != nil {
		t.Fatalf("ReadAllEntriesAtPath: %v", readErr)
	}
	if len(entries) != 1 {
		t.Fatalf("entry count = %d, want 1", len(entries))
	}
	if entries[0].Type != evidence.EntryTypeCanonFailure {
		t.Fatalf("entry type = %q, want %q", entries[0].Type, evidence.EntryTypeCanonFailure)
	}
}

func TestServicePrescribe_CanonicalActionNormalizesToolOperation(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	svc := NewService(Options{
		EvidencePath: dir,
		Signer:       testutil.TestSigner(t),
	})

	preCanon := &canon.CanonicalAction{
		OperationClass:    "mutate",
		ScopeClass:        "production",
		ResourceCount:     1,
		ResourceShapeHash: "sha256:test-shape",
		ResourceIdentity: []canon.ResourceID{
			{APIVersion: "apps/v1", Kind: "Deployment", Namespace: "default", Name: "demo"},
		},
	}

	out, err := svc.Prescribe(context.Background(), PrescribeInput{
		Actor:           evidence.Actor{Type: "agent", ID: "agent-1", Provenance: "mcp"},
		Tool:            "Kubectl",
		Operation:       "Apply",
		RawArtifact:     []byte(`{"noop":true}`),
		CanonicalAction: preCanon,
		SessionID:       "session-1",
	})
	if err != nil {
		t.Fatalf("Prescribe: %v", err)
	}

	entry, found, findErr := evidence.FindEntryByID(dir, out.PrescriptionID)
	if findErr != nil {
		t.Fatalf("FindEntryByID: %v", findErr)
	}
	if !found {
		t.Fatalf("prescription %s not found", out.PrescriptionID)
	}

	var payload evidence.PrescriptionPayload
	if err := json.Unmarshal(entry.Payload, &payload); err != nil {
		t.Fatalf("unmarshal prescription payload: %v", err)
	}

	var action canon.CanonicalAction
	if err := json.Unmarshal(payload.CanonicalAction, &action); err != nil {
		t.Fatalf("unmarshal canonical action: %v", err)
	}

	if action.Tool != "kubectl" {
		t.Fatalf("canonical_action.tool = %q, want kubectl", action.Tool)
	}
	if action.Operation != "apply" {
		t.Fatalf("canonical_action.operation = %q, want apply", action.Operation)
	}
}

func TestServiceReport_UnknownPrescriptionWritesSignal(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	svc := NewService(Options{
		EvidencePath: dir,
		Signer:       testutil.TestSigner(t),
	})

	_, err := svc.Report(context.Background(), ReportInput{
		PrescriptionID: "NONEXISTENT",
		ExitCode:       1,
		Actor:          evidence.Actor{Type: "agent", ID: "agent-1", Provenance: "mcp"},
	})
	if err == nil {
		t.Fatal("expected not_found error")
	}
	if ErrorCode(err) != ErrCodeNotFound {
		t.Fatalf("error code = %q, want %q", ErrorCode(err), ErrCodeNotFound)
	}

	entries, readErr := evidence.ReadAllEntriesAtPath(dir)
	if readErr != nil {
		t.Fatalf("ReadAllEntriesAtPath: %v", readErr)
	}
	if len(entries) != 1 {
		t.Fatalf("entry count = %d, want 1", len(entries))
	}
	if entries[0].Type != evidence.EntryTypeSignal {
		t.Fatalf("entry type = %q, want %q", entries[0].Type, evidence.EntryTypeSignal)
	}

	var payload evidence.SignalPayload
	if err := json.Unmarshal(entries[0].Payload, &payload); err != nil {
		t.Fatalf("unmarshal signal payload: %v", err)
	}
	if payload.SubSignal != "unprescribed_action" {
		t.Fatalf("sub_signal = %q, want unprescribed_action", payload.SubSignal)
	}
}

func TestServiceReport_KnownPrescriptionUsesPrescriptionCorrelation(t *testing.T) {
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
		ExitCode:       0,
	})
	if err != nil {
		t.Fatalf("Report: %v", err)
	}
	if reportOut.ReportID == "" {
		t.Fatal("expected report ID")
	}

	entries, readErr := evidence.ReadAllEntriesAtPath(dir)
	if readErr != nil {
		t.Fatalf("ReadAllEntriesAtPath: %v", readErr)
	}
	if len(entries) != 2 {
		t.Fatalf("entry count = %d, want 2", len(entries))
	}
	if entries[1].Type != evidence.EntryTypeReport {
		t.Fatalf("entry type = %q, want %q", entries[1].Type, evidence.EntryTypeReport)
	}
	if entries[1].TraceID != entries[0].TraceID {
		t.Fatalf("report trace_id = %q, want %q", entries[1].TraceID, entries[0].TraceID)
	}
	if entries[1].Actor.ID != entries[0].Actor.ID {
		t.Fatalf("report actor = %q, want %q", entries[1].Actor.ID, entries[0].Actor.ID)
	}
}
