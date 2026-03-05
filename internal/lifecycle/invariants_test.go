package lifecycle

import (
	"context"
	"testing"

	"samebits.com/evidra-benchmark/internal/testutil"
	"samebits.com/evidra-benchmark/pkg/evidence"
)

func TestSessionInvariant_ReportDerivesSessionFromPrescription(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	svc := NewService(Options{
		EvidencePath: dir,
		Signer:       testutil.TestSigner(t),
	})

	prescOut, err := svc.Prescribe(context.Background(), PrescribeInput{
		Actor:       evidence.Actor{Type: "agent", ID: "agent-1", Provenance: "mcp"},
		Tool:        "kubectl",
		Operation:   "apply",
		RawArtifact: []byte("apiVersion: v1\nkind: ConfigMap\nmetadata:\n  name: cm1\n  namespace: default\n"),
		SessionID:   "session-derive",
		TraceID:     "trace-derive",
	})
	if err != nil {
		t.Fatalf("Prescribe: %v", err)
	}

	reportOut, err := svc.Report(context.Background(), ReportInput{
		PrescriptionID: prescOut.PrescriptionID,
		ExitCode:       0,
	})
	if err != nil {
		t.Fatalf("Report: %v", err)
	}

	reportEntry, found, findErr := evidence.FindEntryByID(dir, reportOut.ReportID)
	if findErr != nil {
		t.Fatalf("FindEntryByID report: %v", findErr)
	}
	if !found {
		t.Fatalf("report entry %s not found", reportOut.ReportID)
	}
	if reportEntry.SessionID != "session-derive" {
		t.Fatalf("report session_id=%q, want session-derive", reportEntry.SessionID)
	}
}

func TestSessionInvariant_ReportSessionMismatchReturnsValidationError(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	svc := NewService(Options{
		EvidencePath: dir,
		Signer:       testutil.TestSigner(t),
	})

	prescOut, err := svc.Prescribe(context.Background(), PrescribeInput{
		Actor:       evidence.Actor{Type: "agent", ID: "agent-1", Provenance: "mcp"},
		Tool:        "kubectl",
		Operation:   "apply",
		RawArtifact: []byte("apiVersion: v1\nkind: ConfigMap\nmetadata:\n  name: cm1\n  namespace: default\n"),
		SessionID:   "session-a",
		TraceID:     "trace-a",
	})
	if err != nil {
		t.Fatalf("Prescribe: %v", err)
	}

	_, err = svc.Report(context.Background(), ReportInput{
		PrescriptionID: prescOut.PrescriptionID,
		ExitCode:       0,
		SessionID:      "session-b",
	})
	if err == nil {
		t.Fatal("expected session mismatch error")
	}
	if ErrorCode(err) != ErrCodeInvalidInput {
		t.Fatalf("error code=%q, want %q", ErrorCode(err), ErrCodeInvalidInput)
	}

	entries, readErr := evidence.ReadAllEntriesAtPath(dir)
	if readErr != nil {
		t.Fatalf("ReadAllEntriesAtPath: %v", readErr)
	}
	if len(entries) != 1 {
		t.Fatalf("entry count=%d, want 1 (report must not be written on mismatch)", len(entries))
	}
}

func TestSessionInvariant_CanonFailureInheritsSessionAndTrace(t *testing.T) {
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
		SessionID:   "session-fail",
		TraceID:     "trace-fail",
	})
	if err == nil {
		t.Fatal("expected parse error")
	}

	entries, readErr := evidence.ReadAllEntriesAtPath(dir)
	if readErr != nil {
		t.Fatalf("ReadAllEntriesAtPath: %v", readErr)
	}
	if len(entries) != 1 {
		t.Fatalf("entry count=%d, want 1", len(entries))
	}
	if entries[0].Type != evidence.EntryTypeCanonFailure {
		t.Fatalf("entry type=%q, want %q", entries[0].Type, evidence.EntryTypeCanonFailure)
	}
	if entries[0].SessionID != "session-fail" {
		t.Fatalf("canon_failure session_id=%q, want session-fail", entries[0].SessionID)
	}
	if entries[0].TraceID != "trace-fail" {
		t.Fatalf("canon_failure trace_id=%q, want trace-fail", entries[0].TraceID)
	}
}

func TestSessionInvariant_UnknownPrescriptionSignalUsesReportSession(t *testing.T) {
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
		SessionID:      "session-signal",
	})
	if err == nil {
		t.Fatal("expected not found error")
	}
	if ErrorCode(err) != ErrCodeNotFound {
		t.Fatalf("error code=%q, want %q", ErrorCode(err), ErrCodeNotFound)
	}

	entries, readErr := evidence.ReadAllEntriesAtPath(dir)
	if readErr != nil {
		t.Fatalf("ReadAllEntriesAtPath: %v", readErr)
	}
	if len(entries) != 1 {
		t.Fatalf("entry count=%d, want 1", len(entries))
	}
	if entries[0].Type != evidence.EntryTypeSignal {
		t.Fatalf("entry type=%q, want %q", entries[0].Type, evidence.EntryTypeSignal)
	}
	if entries[0].SessionID != "session-signal" {
		t.Fatalf("signal session_id=%q, want session-signal", entries[0].SessionID)
	}
}
