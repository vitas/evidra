package report

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/vitas/evidra/pkg/evidence"
)

// testExec is one observed call: the tool, its keyed arguments, the upstream's
// annotation blob as written on the wire, and the outcome the wire showed.
type testExec struct {
	tool, argsHMAC, annotations, status string
}

// appendOp writes one prescribed operation with its executions and report.
func appendOp(t *testing.T, st *evidence.Store, session, objective, status, outcome string, execs []testExec) string {
	t.Helper()
	ev := st.NewEvent(evidence.EventOperationPrescribed, evidence.ProvenanceAgentDeclared, session, "")
	op := ev.OperationID
	if op == "" {
		op = "EV-OP-" + ev.EventID
		// NewEvent leaves operation_id to the caller: an operation id is minted by
		// the endpoint in production, so a test must set it explicitly.
		ev.OperationID = op
	}
	if err := ev.SetPayload(evidence.PrescribedPayload{Objective: objective}); err != nil {
		t.Fatal(err)
	}
	if _, err := st.AppendEvent(ev); err != nil {
		t.Fatal(err)
	}
	for i, e := range execs {
		id := fmt.Sprintf("EXE-%s-%d", ev.EventID, i)
		start := st.NewEvent(evidence.EventExecutionStarted, evidence.ProvenanceProxyObserved, session, op)
		start.OperationID = op
		if err := start.SetPayload(evidence.ExecutionStartedPayload{
			ExecutionID: id, Tool: e.tool, ArgumentsHMAC: e.argsHMAC,
			Annotations: json.RawMessage(e.annotations), StartedAt: time.Now().UTC(),
		}); err != nil {
			t.Fatal(err)
		}
		if _, err := st.AppendEvent(start); err != nil {
			t.Fatal(err)
		}
		fin := st.NewEvent(evidence.EventExecutionFinished, evidence.ProvenanceProxyObserved, session, op)
		fin.OperationID = op
		if err := fin.SetPayload(evidence.ExecutionFinishedPayload{
			ExecutionID: id, Tool: e.tool, Status: evidence.ExecutionStatus(e.status),
			DurationMS: int64(10 + i), FinishedAt: time.Now().UTC(),
		}); err != nil {
			t.Fatal(err)
		}
		if _, err := st.AppendEvent(fin); err != nil {
			t.Fatal(err)
		}
	}
	if status == "" {
		// No report at all: the caller is exercising the states that come from a
		// missing declaration, so writing an empty one would fake the fact.
		return op
	}
	rep := st.NewEvent(evidence.EventOperationReported, evidence.ProvenanceAgentDeclared, session, op)
	rep.OperationID = op
	if err := rep.SetPayload(evidence.ReportedPayload{
		Status: status, Outcome: outcome, Summary: "done", OperationID: op,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := st.AppendEvent(rep); err != nil {
		t.Fatal(err)
	}
	return op
}

func openRecorder(t *testing.T, root, mode string) *evidence.Store {
	t.Helper()
	st, err := evidence.OpenStore(evidence.Options{
		Root: root, UpstreamID: "up-fixture", ServerName: "fixture", EnforceMode: mode, EvidraVersion: "test",
		Stderr: os.Stderr,
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close("stopped") })
	return st
}

func findOp(t *testing.T, sum Summary, id string) (Operation, Cell) {
	t.Helper()
	for _, c := range sum.Cells {
		for _, op := range c.Operations {
			if op.OperationID == id {
				return op, c
			}
		}
	}
	t.Fatalf("operation %s not in summary", id)
	return Operation{}, Cell{}
}

// TestReconcileReportsClaimedAchievedWithoutExecutions is §38.A: the anomaly must
// appear with its observation scope, and the blocked-attempt count beside it must
// distinguish "the agent did nothing here" from "the agent was refused five times".
func TestReconcileReportsClaimedAchievedWithoutExecutions(t *testing.T) {
	root := filepath.Join(t.TempDir(), "evidence")
	st := openRecorder(t, root, "all")
	op := appendOp(t, st, "SES-A", "delete the stale bucket", "completed", "achieved", nil)
	// One block in the same session, recorded after the prescription opened.
	blocked := st.NewEvent(evidence.EventProtocolViolation, evidence.ProvenanceRecorderGenerated, "SES-A", "")
	if err := blocked.SetPayload(evidence.ViolationPayload{Kind: "unprescribed_execution_attempt", Tool: "delete_bucket"}); err != nil {
		t.Fatal(err)
	}
	if _, err := st.AppendEvent(blocked); err != nil {
		t.Fatal(err)
	}
	if err := st.Close("stopped"); err != nil {
		t.Fatal(err)
	}

	sum, err := Reconcile(Options{Root: root})
	if err != nil {
		t.Fatal(err)
	}
	got, cell := findOp(t, sum, op)
	if got.State != stateReported {
		t.Errorf("state = %s, want reported", got.State)
	}
	if !strings.Contains(strings.Join(got.Anomalies, ","), AnomalyClaimedAchievedWithoutExecution) {
		t.Errorf("anomaly missing: %v", got.Anomalies)
	}
	if got.BlockedAttemptsInSession != 1 {
		t.Errorf("blocked_attempts_in_session = %d, want 1", got.BlockedAttemptsInSession)
	}
	if cell.BlockedAttempts != 1 {
		t.Errorf("cell blocked attempts = %d", cell.BlockedAttempts)
	}
	if cell.ChainValid != true || cell.Coverage != "complete" {
		t.Errorf("cell integrity = chain %v coverage %s", cell.ChainValid, cell.Coverage)
	}
	if cell.FirstAttemptCompliance != "0/1" {
		t.Errorf("first attempt compliance = %s, want 0/1", cell.FirstAttemptCompliance)
	}
	if cell.VoluntaryPrescriptionCoverage != "" {
		t.Error("voluntary coverage must stay undefined where prescription was required")
	}
}

// TestReconcileKeepsModesApart is §20's rule that one compliance number may not
// span enforce=all and enforce=off.
func TestReconcileKeepsModesApart(t *testing.T) {
	root := filepath.Join(t.TempDir(), "evidence")
	all := openRecorder(t, root, "all")
	appendOp(t, all, "SES-A", "work", "completed", "achieved", []testExec{{tool: "get_status", argsHMAC: "sha256:aaa"}})
	if err := all.Close("stopped"); err != nil {
		t.Fatal(err)
	}
	off := openRecorder(t, root, "off")
	// An unprescribed execution in observe-only mode: a derived class, not a flag.
	orphan := off.NewEvent(evidence.EventExecutionStarted, evidence.ProvenanceProxyObserved, "SES-B", "")
	if err := orphan.SetPayload(evidence.ExecutionStartedPayload{ExecutionID: "EXE-X", Tool: "restart", ArgumentsHMAC: "sha256:x"}); err != nil {
		t.Fatal(err)
	}
	if _, err := off.AppendEvent(orphan); err != nil {
		t.Fatal(err)
	}
	fin := off.NewEvent(evidence.EventExecutionFinished, evidence.ProvenanceProxyObserved, "SES-B", "")
	if err := fin.SetPayload(evidence.ExecutionFinishedPayload{ExecutionID: "EXE-X", Tool: "restart", Status: evidence.ExecutionSuccess}); err != nil {
		t.Fatal(err)
	}
	if _, err := off.AppendEvent(fin); err != nil {
		t.Fatal(err)
	}
	if err := off.Close("stopped"); err != nil {
		t.Fatal(err)
	}

	sum, err := Reconcile(Options{Root: root})
	if err != nil {
		t.Fatal(err)
	}
	if len(sum.Cells) != 2 {
		t.Fatalf("cells = %d, want one per enforcement mode: %v", len(sum.Cells), groupIDs(sum))
	}
	var sawAll, sawOff bool
	for _, c := range sum.Cells {
		switch c.EnforceMode {
		case "all":
			sawAll = true
			if c.UnprescribedExecutions != 0 || c.SessionsWithNoPrescribe != 0 {
				t.Errorf("all-mode cell counts off-mode things: %+v", c)
			}
		case "off":
			sawOff = true
			if c.UnprescribedExecutions != 1 {
				t.Errorf("unprescribed executions = %d, want 1", c.UnprescribedExecutions)
			}
			if c.VoluntaryPrescriptionCoverage != "0/1" {
				t.Errorf("voluntary coverage = %q, want 0/1", c.VoluntaryPrescriptionCoverage)
			}
			if c.SessionsWithNoPrescribe != 1 {
				t.Errorf("sessions with no prescribe = %d, want 1", c.SessionsWithNoPrescribe)
			}
			if c.FirstAttemptCompliance != "" {
				t.Error("first-attempt compliance is undefined in observe-only mode")
			}
		}
	}
	if !sawAll || !sawOff {
		t.Fatalf("missing a mode cell: %v", groupIDs(sum))
	}
}

// TestReconcileDerivedStatesCoversInterruption checks §34's other two states and
// that no synthetic report is ever invented for them.
func TestReconcileDerivedStatesCoversInterruption(t *testing.T) {
	root := filepath.Join(t.TempDir(), "evidence")
	st := openRecorder(t, root, "all")
	interrupted := appendOp(t, st, "SES-A", "never finished", "", "", nil)
	if err := st.Close("stopped"); err != nil {
		t.Fatal(err)
	}
	// A second recorder whose store ends without a stop event: the recorder died
	// mid-write, so the lifecycle cannot be claimed either way.
	hot := openRecorder(t, root, "all")
	unknown := appendOp(t, hot, "SES-B", "recorder vanished", "", "", nil)

	sum, err := Reconcile(Options{Root: root})
	if err != nil {
		t.Fatal(err)
	}
	got, _ := findOp(t, sum, interrupted)
	if got.State != stateInterruptedSession {
		t.Errorf("state = %s, want interrupted_session", got.State)
	}
	if got.Report != nil {
		t.Error("a synthesized report was created for an unreported operation")
	}
	got2, _ := findOp(t, sum, unknown)
	if got2.State != stateLifecycleUnknown {
		t.Errorf("state = %s, want lifecycle_unknown", got2.State)
	}
	_ = hot.Close("stopped")
}

// TestReconcileFingerprintMetrics covers §38.B and §39 without pretending a
// missing fingerprint proves anything.
func TestReconcileFingerprintMetrics(t *testing.T) {
	root := filepath.Join(t.TempDir(), "evidence")
	st := openRecorder(t, root, "all")
	ann := `{"readOnlyHint":true}`
	achieved := appendOp(t, st, "SES-A", "read the config", "completed", "achieved", []testExec{
		{tool: "get_status", argsHMAC: "sha256:same", annotations: ann, status: "success"},
		{tool: "get_status", argsHMAC: "sha256:same", annotations: ann, status: "success"},
	})
	recovered := appendOp(t, st, "SES-A", "restart then recover", "completed", "achieved", []testExec{
		{tool: "restart", argsHMAC: "sha256:dead", status: "error"},
		{tool: "restart", argsHMAC: "sha256:dead", status: "success"},
	})
	uncomparable := appendOp(t, st, "SES-A", "restart without keys", "completed", "achieved", []testExec{
		{tool: "restart", argsHMAC: "sha256:dead", status: "error"},
		{tool: "restart", status: "success"},
	})
	if err := st.Close("stopped"); err != nil {
		t.Fatal(err)
	}
	sum, err := Reconcile(Options{Root: root})
	if err != nil {
		t.Fatal(err)
	}
	op, _ := findOp(t, sum, achieved)
	if op.RepeatedArgumentFingerprints != 1 {
		t.Errorf("repeated fingerprints = %d, want 1", op.RepeatedArgumentFingerprints)
	}
	if !strings.Contains(strings.Join(op.Facts, ","), FactAchievedWithDeclaredReadOnlyOnly) {
		t.Errorf("read-only reconciliation fact missing: %v (annotations=%v)", op.Facts, readOnlyOf(op))
	}
	got2, _ := findOp(t, sum, recovered)
	if got2.SameFingerprintRecovery != recoveryYes {
		t.Errorf("recovery = %s, want %s", got2.SameFingerprintRecovery, recoveryYes)
	}
	// The successful retry had no argument fingerprint, so the comparison could
	// not be made: that is a stated limitation, not "no recovery happened".
	got3, _ := findOp(t, sum, uncomparable)
	if got3.SameFingerprintRecovery != recoveryInsufficient {
		t.Errorf("recovery = %s, want %s", got3.SameFingerprintRecovery, recoveryInsufficient)
	}
	raw, err := sum.JSON()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), `"schema": "`+Schema+`"`) {
		t.Error("summary json lost its schema name")
	}
}

func readOnlyOf(op Operation) []bool {
	out := make([]bool, 0, len(op.Executions))
	for _, e := range op.Executions {
		out = append(out, e.DeclaredReadOnly)
	}
	return out
}

func groupIDs(sum Summary) []string {
	out := make([]string, 0, len(sum.Cells))
	for _, c := range sum.Cells {
		out = append(out, c.Group)
	}
	return out
}

// TestRecorderDirsDoNotCollideWithinAMillisecond guards §20's structural
// invariant. Two recorder directories sharing a name is not a cosmetic problem:
// both processes append to one file, each resuming the other's tail, and the hash
// chain silently becomes a record of interleaved writers. Removing a store that
// recorded nothing is the endpoint wrapper's job (`epStoreEvidence.close`), so a
// bare Close here leaves every directory in place and the name check is the point.
func TestRecorderDirsDoNotCollideWithinAMillisecond(t *testing.T) {
	root := filepath.Join(t.TempDir(), "evidence")
	seen := map[string]bool{}
	for i := 0; i < 40; i++ {
		st := openRecorder(t, root, "all")
		if err := st.Close("stopped"); err != nil {
			t.Fatal(err)
		}
		dir := st.Dir()
		if seen[dir] {
			t.Fatalf("recorder directory reused: %s", dir)
		}
		seen[dir] = true
	}
	dirs, err := filepath.Glob(filepath.Join(root, "recorder-*"))
	if err != nil {
		t.Fatal(err)
	}
	if len(dirs) != len(seen) {
		t.Errorf("dirs on disk = %d, distinct names = %d", len(dirs), len(seen))
	}
}

// TestReconcileHonorsSinceWindow checks that a scope filter narrows the counts a
// reader acts on. Verification still covers the whole chain, but a summary that
// reported records outside the requested window would answer a question nobody
// asked (§20).
func TestReconcileHonorsSinceWindow(t *testing.T) {
	root := filepath.Join(t.TempDir(), "evidence")
	st := openRecorder(t, root, "all")
	appendOp(t, st, "SES-A", "work", "completed", "achieved", []testExec{
		{tool: "get_status", argsHMAC: "sha256:a", status: "success"},
	})
	if err := st.Close("stopped"); err != nil {
		t.Fatal(err)
	}
	full, err := Reconcile(Options{Root: root})
	if err != nil {
		t.Fatal(err)
	}
	windowed, err := Reconcile(Options{Root: root, Since: time.Now().UTC().Add(time.Hour)})
	if err != nil {
		t.Fatal(err)
	}
	if len(full.Cells) != 1 || len(windowed.Cells) != 1 {
		t.Fatalf("cells = %d / %d", len(full.Cells), len(windowed.Cells))
	}
	if full.Cells[0].Records == 0 {
		t.Fatal("unfiltered summary found no records")
	}
	if windowed.Cells[0].Records != 0 {
		t.Errorf("window records = %d, want 0", windowed.Cells[0].Records)
	}
	if len(windowed.Cells[0].Operations) != 0 {
		t.Errorf("window operations = %d, want 0", len(windowed.Cells[0].Operations))
	}
	if !windowed.Cells[0].ChainValid {
		t.Error("an empty window must not invalidate the chain: integrity is not windowed")
	}
}

// The reconciliation view is the reader's arrangement of the same facts §38 already
// computes. These tests aim at the two ways such a view goes wrong: it starts agreeing
// with the agent (or disagreeing), and it loses the partition between what the server
// claimed and what the recorder watched.

func TestReconciliationViewPutsTheThreeLayersInOrder(t *testing.T) {
	op := &Operation{
		Objective: "restore the ingest service",
		Report:    &Report{Status: "completed", Outcome: "achieved", Summary: "service is healthy again"},
		Executions: []Execution{
			{Tool: "get_status", Status: "success", DeclaredReadOnly: true},
			{Tool: "read_logs", Status: "success", DeclaredReadOnly: true},
			{Tool: "get_status", Status: "success", DeclaredReadOnly: true},
		},
	}
	annotate(op, nil)

	view := op.View
	if view.Declared != "restore the ingest service" {
		t.Errorf("declared = %q, want the agent's own objective verbatim", view.Declared)
	}
	if view.Executions.Count != 3 || view.Executions.Succeeded != 3 {
		t.Errorf("observed counts = %+v", view.Executions)
	}
	if view.Executions.ServerDeclaredRO != 3 || view.Executions.NotDeclaredRO != 0 {
		t.Errorf("read-only split = %d/%d, want 3/0",
			view.Executions.ServerDeclaredRO, view.Executions.NotDeclaredRO)
	}
	if view.Executions.AnnotationsVerified {
		t.Error("the view reports the upstream's annotations as verified")
	}
	if !view.Reported.Present || view.Reported.Outcome != "achieved" {
		t.Errorf("reported = %+v, want the agent's claim kept verbatim", view.Reported)
	}
	// The claim and the observation must both survive without being merged into a
	// conclusion: this is the exact case where a verdict field is most tempting.
	if !strings.Contains(view.Stance, "does not decide") {
		t.Errorf("stance = %q", view.Stance)
	}
	if !contains(op.Facts, FactAchievedWithDeclaredReadOnlyOnly) {
		t.Error("the §38.C fact disappeared when the view was added alongside it")
	}
}

func TestReconciliationViewPartitionsEveryExecutionOnce(t *testing.T) {
	op := &Operation{
		Report: &Report{Status: "completed", Outcome: "achieved"},
		Executions: []Execution{
			{Status: "success"},
			{Status: "error"},
			{Status: "cancelled"},
			{Status: "started"},
			{Status: "something-a-future-writer-invented"},
		},
	}
	annotate(op, nil)
	e := op.View.Executions
	if e.Count != 5 {
		t.Fatalf("count = %d", e.Count)
	}
	if sum := e.Succeeded + e.FailedOrCancelled + e.UnpairedStarts + e.Unknown; sum != e.Count {
		t.Errorf("buckets sum to %d but %d executions were observed: the partition leaks", sum, e.Count)
	}
	// An unannotated call is not a state-changing call. Collapsing those two is how a
	// read-only tool without annotations gets silently accused of writing (§27).
	if e.ServerDeclaredRO != 0 || e.NotDeclaredRO != 5 {
		t.Errorf("read-only split = %d/%d, want 0/5 unverified-no-claim", e.ServerDeclaredRO, e.NotDeclaredRO)
	}
}

func TestReconciliationViewNamesAnAbsentReport(t *testing.T) {
	op := &Operation{Objective: "restart the worker", Executions: []Execution{{Status: "success"}}}
	annotate(op, nil)
	if op.View.Reported.Present {
		t.Fatal("an unreported operation rendered as reported")
	}
	lines := op.View.lines("  ")
	joined := strings.Join(lines, "\n")
	if !strings.Contains(joined, "no terminal report was received") {
		t.Errorf("the human view hides that no report exists:\n%s", joined)
	}
	for _, forbidden := range []string{"verdict", "score", "risk", "incorrect", "did not"} {
		if strings.Contains(strings.ToLower(joined), forbidden) {
			t.Errorf("the rendered view contains verdict-like language %q:\n%s", forbidden, joined)
		}
	}
}

func contains(hay []string, needle string) bool {
	for _, h := range hay {
		if h == needle {
			return true
		}
	}
	return false
}
