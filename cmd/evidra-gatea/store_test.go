package main

import (
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/vitas/evidra/pkg/evidence"
)

// writeRunStore records a minimal session into the layout the runner produces:
// <runDir>/evidence/recorder-*/.
func writeRunStore(t *testing.T, runDir string, execOperationID string, blocks int) string {
	t.Helper()
	st, err := evidence.OpenStore(evidence.Options{
		Root: filepath.Join(runDir, "evidence"), UpstreamID: "up-fixture",
		ServerName: "fixture", EnforceMode: "all", EvidraVersion: "test", Stderr: discardWriter{},
	})
	if err != nil {
		t.Fatal(err)
	}
	op := "EV-OP-STORE"
	prescribe := st.NewEvent(evidence.EventOperationPrescribed, evidence.ProvenanceAgentDeclared, "SES-1", op)
	prescribe.OperationID = op
	if err := prescribe.SetPayload(evidence.PrescribedPayload{Objective: "do the thing"}); err != nil {
		t.Fatal(err)
	}
	if _, err := st.AppendEvent(prescribe); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < blocks; i++ {
		v := st.NewEvent(evidence.EventProtocolViolation, evidence.ProvenanceRecorderGenerated, "SES-1", "")
		if err := v.SetPayload(evidence.ViolationPayload{Kind: "unprescribed_execution_attempt", Tool: "restart"}); err != nil {
			t.Fatal(err)
		}
		if _, err := st.AppendEvent(v); err != nil {
			t.Fatal(err)
		}
	}
	start := st.NewEvent(evidence.EventExecutionStarted, evidence.ProvenanceProxyObserved, "SES-1", execOperationID)
	start.OperationID = execOperationID
	if err := start.SetPayload(evidence.ExecutionStartedPayload{ExecutionID: "EXE-1", Tool: "restart", ArgumentsHMAC: "sha256:a"}); err != nil {
		t.Fatal(err)
	}
	if _, err := st.AppendEvent(start); err != nil {
		t.Fatal(err)
	}
	finish := st.NewEvent(evidence.EventExecutionFinished, evidence.ProvenanceProxyObserved, "SES-1", execOperationID)
	finish.OperationID = execOperationID
	if err := finish.SetPayload(evidence.ExecutionFinishedPayload{
		ExecutionID: "EXE-1", Tool: "restart", Status: evidence.ExecutionSuccess, FinishedAt: time.Now().UTC(),
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := st.AppendEvent(finish); err != nil {
		t.Fatal(err)
	}
	report := st.NewEvent(evidence.EventOperationReported, evidence.ProvenanceAgentDeclared, "SES-1", op)
	report.OperationID = op
	if err := report.SetPayload(evidence.ReportedPayload{Status: "completed", Outcome: "achieved", OperationID: op}); err != nil {
		t.Fatal(err)
	}
	if _, err := st.AppendEvent(report); err != nil {
		t.Fatal(err)
	}
	if err := st.Close("stopped"); err != nil {
		t.Fatal(err)
	}
	return st.Dir()
}

type discardWriter struct{}

func (discardWriter) Write(p []byte) (int, error) { return len(p), nil }

func TestStoreFactsReplaceTranscriptInference(t *testing.T) {
	runDir := t.TempDir()
	dir := writeRunStore(t, runDir, "EV-OP-STORE", 1)

	facts, err := readStoreFacts(runDir)
	if err != nil {
		t.Fatal(err)
	}
	if facts == nil {
		t.Fatal("no facts read although a store exists")
	}
	if facts.Dir != dir {
		t.Errorf("facts.Dir = %s, want %s", facts.Dir, dir)
	}
	if facts.Blocked != 1 || facts.Executions != 1 || facts.Unprescribed != 0 {
		t.Errorf("counts = blocked %d execs %d unprescribed %d", facts.Blocked, facts.Executions, facts.Unprescribed)
	}
	if facts.Prescribed != 1 || facts.Reported != 1 {
		t.Errorf("operations = prescribed %d reported %d", facts.Prescribed, facts.Reported)
	}
	if !facts.ChainValid || !facts.SignatureValid || facts.Coverage != "complete" {
		t.Errorf("integrity = chain %v signature %v coverage %s", facts.ChainValid, facts.SignatureValid, facts.Coverage)
	}

	res := runResult{Blocked: 9, Unprescribed: 4, CountsFrom: "transcript"}
	applyStoreFacts(&res, facts, "all")
	if res.Blocked != 1 || res.CountsFrom != "store" {
		t.Errorf("store did not override the inferred counts: blocked=%d from=%s", res.Blocked, res.CountsFrom)
	}
	if res.Store != facts {
		t.Error("run result lost the store facts it should publish")
	}
	if len(res.Failures) != 0 {
		t.Errorf("clean session produced failures: %v", res.Failures)
	}
}

func TestStoreFactsExposeEnforcementHoles(t *testing.T) {
	// An execution recorded with no operation id under enforce=all is not an agent
	// mistake: it is the recorder watching a call get through that it existed to
	// refuse.
	runDir := t.TempDir()
	facts, err := readStoreFacts(runDir)
	if err != nil || facts != nil {
		t.Fatalf("empty run dir: facts=%v err=%v", facts, err)
	}
	var res runResult
	applyStoreFacts(&res, nil, "all")
	if res.CountsFrom != "transcript" {
		t.Errorf("counts_from = %s without a store", res.CountsFrom)
	}

	dir := writeRunStore(t, runDir, "", 0)
	_ = dir
	facts, err = readStoreFacts(runDir)
	if err != nil {
		t.Fatal(err)
	}
	if facts.Unprescribed != 1 {
		t.Fatalf("unprescribed = %d, want 1", facts.Unprescribed)
	}
	res = runResult{}
	applyStoreFacts(&res, facts, "all")
	if len(res.Failures) == 0 || !strings.Contains(res.Failures[0], "enforcement hole") {
		t.Fatalf("the hole was recorded as a statistic instead of a failure: %v", res.Failures)
	}
	// The same event in observe-only mode is legitimate traffic (§7), so it must
	// not be reported as a defect.
	res = runResult{}
	applyStoreFacts(&res, facts, "off")
	if len(res.Failures) != 0 {
		t.Errorf("observe-only traffic flagged as an enforcement hole: %v", res.Failures)
	}
}

func TestStoreFactsRejectAmbiguousRunDir(t *testing.T) {
	runDir := t.TempDir()
	writeRunStore(t, runDir, "EV-OP-STORE", 0)
	writeRunStore(t, runDir, "EV-OP-STORE", 0)
	if _, err := readStoreFacts(runDir); err == nil {
		t.Fatal("two recorder stores in one run directory were accepted")
	}
}
