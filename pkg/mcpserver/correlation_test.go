package mcpserver

import (
	"encoding/json"
	"reflect"
	"sync"
	"testing"

	"samebits.com/evidra-benchmark/internal/lifecycle"
	"samebits.com/evidra-benchmark/internal/testutil"
	"samebits.com/evidra-benchmark/pkg/evidence"
)

func TestConcurrentReportCorrelation_NoCrossCallContamination(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	svc := &BenchmarkService{
		evidencePath: dir,
		signer:       testutil.TestSigner(t),
	}

	prescA := svc.Prescribe(PrescribeInput{
		Actor:       InputActor{Type: "agent", ID: "actor-a", Origin: "mcp"},
		Tool:        "kubectl",
		Operation:   "apply",
		RawArtifact: "apiVersion: v1\nkind: ConfigMap\nmetadata:\n  name: cm-a\n  namespace: ns-a\n",
		SessionID:   "session-a",
		TraceID:     "trace-a",
	})
	if !prescA.OK {
		t.Fatalf("prescribe A failed: %+v", prescA.Error)
	}

	prescB := svc.Prescribe(PrescribeInput{
		Actor:       InputActor{Type: "agent", ID: "actor-b", Origin: "mcp"},
		Tool:        "kubectl",
		Operation:   "apply",
		RawArtifact: "apiVersion: v1\nkind: ConfigMap\nmetadata:\n  name: cm-b\n  namespace: ns-b\n",
		SessionID:   "session-b",
		TraceID:     "trace-b",
	})
	if !prescB.OK {
		t.Fatalf("prescribe B failed: %+v", prescB.Error)
	}

	var reportA, reportB ReportOutput
	var wg sync.WaitGroup
	wg.Add(2)

	// Intentionally report B first to ensure order does not affect correlation.
	go func() {
		defer wg.Done()
		reportB = svc.Report(ReportInput{
			PrescriptionID: prescB.PrescriptionID,
			Verdict:        evidence.VerdictSuccess,
			ExitCode:       intPtr(0),
		})
	}()
	go func() {
		defer wg.Done()
		reportA = svc.Report(ReportInput{
			PrescriptionID: prescA.PrescriptionID,
			Verdict:        evidence.VerdictFailure,
			ExitCode:       intPtr(1),
		})
	}()

	wg.Wait()

	if !reportA.OK {
		t.Fatalf("report A failed: %+v", reportA.Error)
	}
	if !reportB.OK {
		t.Fatalf("report B failed: %+v", reportB.Error)
	}

	entries, err := evidence.ReadAllEntriesAtPath(dir)
	if err != nil {
		t.Fatalf("ReadAllEntriesAtPath: %v", err)
	}

	indexByID := map[string]evidence.EvidenceEntry{}
	for _, e := range entries {
		indexByID[e.EntryID] = e
	}

	presEntryA := indexByID[prescA.PrescriptionID]
	presEntryB := indexByID[prescB.PrescriptionID]
	repEntryA := indexByID[reportA.ReportID]
	repEntryB := indexByID[reportB.ReportID]

	assertReportCorrelation(t, repEntryA, presEntryA, prescA.PrescriptionID)
	assertReportCorrelation(t, repEntryB, presEntryB, prescB.PrescriptionID)
}

func TestToLifecycleReportInput_MapsActorAndRefs(t *testing.T) {
	t.Parallel()

	in := ReportInput{
		PrescriptionID: "P1",
		Verdict:        evidence.VerdictFailure,
		ExitCode:       intPtr(2),
		ArtifactDigest: "abc",
		Actor: InputActor{
			Type:       "agent",
			ID:         "actor-1",
			Origin:     "mcp",
			InstanceID: "pod-1",
			Version:    "v1",
		},
		ExternalRefs: []evidence.ExternalRef{{Type: "github_run", ID: "123"}},
		SessionID:    "session-1",
		OperationID:  "op-1",
		SpanID:       "span-1",
		ParentSpanID: "span-0",
	}

	got := toLifecycleReportInput(in)
	want := lifecycle.ReportInput{
		PrescriptionID: "P1",
		Verdict:        evidence.VerdictFailure,
		ExitCode:       intPtr(2),
		ArtifactDigest: "abc",
		Actor: evidence.Actor{
			Type:         "agent",
			ID:           "actor-1",
			Provenance:   "mcp",
			InstanceID:   "pod-1",
			Version:      "v1",
			SkillVersion: contractSkillVersion,
		},
		ExternalRefs: []evidence.ExternalRef{{Type: "github_run", ID: "123"}},
		SessionID:    "session-1",
		OperationID:  "op-1",
		SpanID:       "span-1",
		ParentSpanID: "span-0",
	}

	if !reflect.DeepEqual(got, want) {
		t.Fatalf("toLifecycleReportInput() = %#v, want %#v", got, want)
	}
}

func TestToEvidenceActor_UsesProvidedSkillVersion(t *testing.T) {
	t.Parallel()

	in := InputActor{
		Type:         "agent",
		ID:           "actor-1",
		Origin:       "mcp",
		SkillVersion: "1.1.0",
	}

	got := toEvidenceActor(in)
	if got.SkillVersion != "1.1.0" {
		t.Fatalf("actor.skill_version=%q, want %q", got.SkillVersion, "1.1.0")
	}
}

func assertReportCorrelation(t *testing.T, reportEntry, prescriptionEntry evidence.EvidenceEntry, prescriptionID string) {
	t.Helper()

	if reportEntry.EntryID == "" {
		t.Fatal("empty report entry ID")
	}
	if reportEntry.Type != evidence.EntryTypeReport {
		t.Fatalf("report type = %q, want %q", reportEntry.Type, evidence.EntryTypeReport)
	}
	if reportEntry.Actor.ID != prescriptionEntry.Actor.ID {
		t.Fatalf("report actor=%q, want %q", reportEntry.Actor.ID, prescriptionEntry.Actor.ID)
	}
	if reportEntry.TraceID != prescriptionEntry.TraceID {
		t.Fatalf("report trace_id=%q, want %q", reportEntry.TraceID, prescriptionEntry.TraceID)
	}
	if reportEntry.SessionID != prescriptionEntry.SessionID {
		t.Fatalf("report session_id=%q, want %q", reportEntry.SessionID, prescriptionEntry.SessionID)
	}

	var payload evidence.ReportPayload
	if err := json.Unmarshal(reportEntry.Payload, &payload); err != nil {
		t.Fatalf("unmarshal report payload: %v", err)
	}
	if payload.PrescriptionID != prescriptionID {
		t.Fatalf("report payload prescription_id=%q, want %q", payload.PrescriptionID, prescriptionID)
	}
}
