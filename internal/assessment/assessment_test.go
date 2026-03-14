package assessment

import (
	"context"
	"testing"

	"samebits.com/evidra/internal/lifecycle"
	"samebits.com/evidra/internal/score"
	"samebits.com/evidra/internal/testutil"
	"samebits.com/evidra/pkg/evidence"
)

func TestBuildFromResults_PreviewWhenBelowThreshold(t *testing.T) {
	t.Parallel()

	got := BuildFromResults(nil, 1)
	if got.Basis.AssessmentMode != AssessmentModePreview {
		t.Fatalf("mode=%q want %q", got.Basis.AssessmentMode, AssessmentModePreview)
	}
	if got.Basis.Sufficient {
		t.Fatal("basis.sufficient=true want false")
	}
	if got.ScoreBand == "insufficient_data" {
		t.Fatalf("preview mode should return a scored preview band, got %q", got.ScoreBand)
	}
}

func TestBuildFromResults_SufficientAtThreshold(t *testing.T) {
	t.Parallel()

	got := BuildFromResults(nil, score.MinOperations)
	if got.Basis.AssessmentMode != AssessmentModeSufficient {
		t.Fatalf("mode=%q want %q", got.Basis.AssessmentMode, AssessmentModeSufficient)
	}
	if !got.Basis.Sufficient {
		t.Fatal("basis.sufficient=false want true")
	}
}

func TestTrackerIncrementalSnapshotReuse(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	signer := testutil.TestSigner(t)
	svc := lifecycle.NewService(lifecycle.Options{
		EvidencePath: dir,
		Signer:       signer,
	})

	presc, err := svc.Prescribe(context.Background(), lifecycle.PrescribeInput{
		Actor:       evidence.Actor{Type: "agent", ID: "agent-1", Provenance: "cli"},
		Tool:        "kubectl",
		Operation:   "apply",
		RawArtifact: []byte("apiVersion: v1\nkind: ConfigMap\nmetadata:\n  name: test\n  namespace: default\n"),
		SessionID:   "session-1",
	})
	if err != nil {
		t.Fatalf("Prescribe: %v", err)
	}
	if _, err := svc.Report(context.Background(), lifecycle.ReportInput{
		PrescriptionID: presc.PrescriptionID,
		Verdict:        evidence.VerdictSuccess,
		ExitCode:       intPtr(0),
	}); err != nil {
		t.Fatalf("Report: %v", err)
	}

	profile, err := score.ResolveProfile("")
	if err != nil {
		t.Fatalf("ResolveProfile: %v", err)
	}

	tracker := NewTracker(dir)
	first, err := tracker.Snapshot(presc.SessionID, profile)
	if err != nil {
		t.Fatalf("Snapshot(first): %v", err)
	}
	if tracker.scanCount != 1 {
		t.Fatalf("scanCount after cold snapshot = %d, want 1", tracker.scanCount)
	}

	reportOut, err := svc.Report(context.Background(), lifecycle.ReportInput{
		PrescriptionID: presc.PrescriptionID,
		Verdict:        evidence.VerdictFailure,
		ExitCode:       intPtr(1),
	})
	if err != nil {
		t.Fatalf("Report(second): %v", err)
	}
	entry, found, err := evidence.FindEntryByID(dir, reportOut.ReportID)
	if err != nil {
		t.Fatalf("FindEntryByID: %v", err)
	}
	if !found {
		t.Fatalf("report entry %q not found", reportOut.ReportID)
	}

	if err := tracker.Observe(entry); err != nil {
		t.Fatalf("Observe: %v", err)
	}
	second, err := tracker.Snapshot(presc.SessionID, profile)
	if err != nil {
		t.Fatalf("Snapshot(second): %v", err)
	}
	if tracker.scanCount != 1 {
		t.Fatalf("scanCount after observed write = %d, want 1", tracker.scanCount)
	}
	if second.Basis.TotalOperations != first.Basis.TotalOperations {
		t.Fatalf("total operations changed unexpectedly: first=%d second=%d", first.Basis.TotalOperations, second.Basis.TotalOperations)
	}
}

func TestTrackerRebuildsAfterExternalWrite(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	signer := testutil.TestSigner(t)
	svc := lifecycle.NewService(lifecycle.Options{
		EvidencePath: dir,
		Signer:       signer,
	})

	presc, err := svc.Prescribe(context.Background(), lifecycle.PrescribeInput{
		Actor:       evidence.Actor{Type: "agent", ID: "agent-1", Provenance: "cli"},
		Tool:        "kubectl",
		Operation:   "apply",
		RawArtifact: []byte("apiVersion: v1\nkind: ConfigMap\nmetadata:\n  name: test\n  namespace: default\n"),
		SessionID:   "session-2",
	})
	if err != nil {
		t.Fatalf("Prescribe: %v", err)
	}
	if _, err := svc.Report(context.Background(), lifecycle.ReportInput{
		PrescriptionID: presc.PrescriptionID,
		Verdict:        evidence.VerdictSuccess,
		ExitCode:       intPtr(0),
	}); err != nil {
		t.Fatalf("Report: %v", err)
	}

	profile, err := score.ResolveProfile("")
	if err != nil {
		t.Fatalf("ResolveProfile: %v", err)
	}

	tracker := NewTracker(dir)
	if _, err := tracker.Snapshot(presc.SessionID, profile); err != nil {
		t.Fatalf("Snapshot(first): %v", err)
	}
	if tracker.scanCount != 1 {
		t.Fatalf("scanCount after first snapshot = %d, want 1", tracker.scanCount)
	}

	if _, err := svc.Report(context.Background(), lifecycle.ReportInput{
		PrescriptionID: presc.PrescriptionID,
		Verdict:        evidence.VerdictFailure,
		ExitCode:       intPtr(2),
	}); err != nil {
		t.Fatalf("Report(external): %v", err)
	}

	snapshot, err := tracker.Snapshot(presc.SessionID, profile)
	if err != nil {
		t.Fatalf("Snapshot(second): %v", err)
	}
	if tracker.scanCount != 2 {
		t.Fatalf("scanCount after external write = %d, want 2", tracker.scanCount)
	}
	if snapshot.ScoreBand == "" {
		t.Fatal("snapshot score band should not be empty")
	}
}

func intPtr(v int) *int {
	return &v
}
