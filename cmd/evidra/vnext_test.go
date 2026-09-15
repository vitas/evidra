package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/vitas/evidra/pkg/evidence"
	"github.com/vitas/evidra/pkg/report"
)

// writeSessionRecorder stores one session that claims success without any
// observed execution, then blocks a stray attempt: the smallest evidence set that
// exercises every part of the two commands.
func writeSessionRecorder(t *testing.T, mode string) string {
	t.Helper()
	root := filepath.Join(t.TempDir(), "evidence")
	st, err := evidence.OpenStore(evidence.Options{
		Root: root, UpstreamID: "up-fixture", ServerName: "fixture", EnforceMode: mode,
		EvidraVersion: "test", Stderr: os.Stderr,
	})
	if err != nil {
		t.Fatal(err)
	}
	op := st.NewEvent(evidence.EventOperationPrescribed, evidence.ProvenanceAgentDeclared, "SES-A", "")
	op.OperationID = "EV-OP-1"
	if err := op.SetPayload(evidence.PrescribedPayload{Objective: "delete the stale bucket"}); err != nil {
		t.Fatal(err)
	}
	if _, err := st.AppendEvent(op); err != nil {
		t.Fatal(err)
	}
	blocked := st.NewEvent(evidence.EventProtocolViolation, evidence.ProvenanceRecorderGenerated, "SES-A", "")
	if err := blocked.SetPayload(evidence.ViolationPayload{Kind: "unprescribed_execution_attempt", Tool: "delete_bucket"}); err != nil {
		t.Fatal(err)
	}
	if _, err := st.AppendEvent(blocked); err != nil {
		t.Fatal(err)
	}
	rep := st.NewEvent(evidence.EventOperationReported, evidence.ProvenanceAgentDeclared, "SES-A", "EV-OP-1")
	rep.OperationID = "EV-OP-1"
	if err := rep.SetPayload(evidence.ReportedPayload{
		Status: "completed", Outcome: "achieved", Summary: "bucket is gone", OperationID: "EV-OP-1",
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := st.AppendEvent(rep); err != nil {
		t.Fatal(err)
	}
	if err := st.Close("stopped"); err != nil {
		t.Fatal(err)
	}
	return root
}

func TestSummarizeWritesJSONAndTerminalFindings(t *testing.T) {
	t.Setenv("EVIDRA_EVIDENCE_DIR", "")
	root := writeSessionRecorder(t, "all")
	var out, errOut bytes.Buffer
	if code := cmdSummarize([]string{"--dir", root}, &out, &errOut); code != 0 {
		t.Fatalf("summarize exit %d: %s", code, errOut.String())
	}
	text := out.String()
	if !strings.Contains(text, report.AnomalyClaimedAchievedWithoutExecution) {
		t.Errorf("terminal summary lost the anomaly:\n%s", text)
	}
	if !strings.Contains(text, "blocked_attempts") && !strings.Contains(text, "blocked attempts") {
		t.Errorf("terminal summary must show blocked attempts beside the anomaly:\n%s", text)
	}
	raw, err := os.ReadFile(filepath.Join(root, "summary.json"))
	if err != nil {
		t.Fatal(err)
	}
	var sum report.Summary
	if err := json.Unmarshal(raw, &sum); err != nil {
		t.Fatalf("summary.json unparseable: %v", err)
	}
	if sum.Schema != report.Schema {
		t.Errorf("schema = %q", sum.Schema)
	}
	if len(sum.Cells) != 1 || sum.Cells[0].EnforceMode != "all" {
		t.Fatalf("cells = %+v", sum.Cells)
	}
	if sum.Cells[0].BlockedAttempts != 1 {
		t.Errorf("blocked attempts = %d", sum.Cells[0].BlockedAttempts)
	}
	for _, n := range sum.Notes {
		if strings.Contains(n, "never") {
			return
		}
	}
	t.Error("summary.json lost the notes that keep a reader from averaging across modes")
}

func TestVerifySeparatesChainValidityFromCoverage(t *testing.T) {
	t.Setenv("EVIDRA_EVIDENCE_DIR", "")
	root := writeSessionRecorder(t, "off")
	var out, errOut bytes.Buffer
	if code := cmdVerifyChain([]string{"--dir", root}, &out, &errOut); code != 0 {
		t.Fatalf("verify exit %d: %s", code, errOut.String())
	}
	if !strings.Contains(out.String(), "chain=valid") || !strings.Contains(out.String(), "coverage=complete") {
		t.Errorf("verify output = %s", out.String())
	}
	// A mode label must appear: without it a reader cannot tell which comparison
	// domain a number belongs to.
	if !strings.Contains(out.String(), "mode=off") {
		t.Errorf("verify lost the enforcement mode: %s", out.String())
	}
}

func TestVerifyDetectsTampering(t *testing.T) {
	t.Setenv("EVIDRA_EVIDENCE_DIR", "")
	root := writeSessionRecorder(t, "all")
	dirs, err := filepath.Glob(filepath.Join(root, "recorder-*"))
	if err != nil || len(dirs) != 1 {
		t.Fatalf("recorder dirs = %v (%v)", dirs, err)
	}
	path := filepath.Join(dirs[0], "events.jsonl")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	tampered := strings.Replace(string(raw), "delete the stale bucket", "keep the stale bucket  ", 1)
	if tampered == string(raw) {
		t.Fatal("tamper had no effect")
	}
	if err := os.WriteFile(path, []byte(tampered), 0o600); err != nil {
		t.Fatal(err)
	}
	var out, errOut bytes.Buffer
	if code := cmdVerifyChain([]string{"--dir", root}, &out, &errOut); code != 1 {
		t.Fatalf("verify accepted a rewritten record, exit %d: %s", code, out.String())
	}
	if !strings.Contains(out.String(), "chain=INVALID") {
		t.Errorf("verify output = %s", out.String())
	}
}

func TestEvidenceCommandsNeedADirectory(t *testing.T) {
	t.Setenv("EVIDRA_EVIDENCE_DIR", "")
	for name, fn := range map[string]commandHandler{"summarize": cmdSummarize, "verify": cmdVerifyChain} {
		var out, errOut bytes.Buffer
		if code := fn(nil, &out, &errOut); code != 2 {
			t.Errorf("%s without --dir exited %d, want 2", name, code)
		}
		if !strings.Contains(errOut.String(), "--dir") {
			t.Errorf("%s did not say what it needed: %s", name, errOut.String())
		}
	}
}

func TestSinceFilterIsAcceptedByBothCommands(t *testing.T) {
	t.Setenv("EVIDRA_EVIDENCE_DIR", "")
	root := writeSessionRecorder(t, "all")
	var out, errOut bytes.Buffer
	if code := cmdSummarize([]string{"--dir", root, "--since", "7d"}, &out, &errOut); code != 0 {
		t.Fatalf("summarize --since: %d %s", code, errOut.String())
	}
	// A window that opens after everything was recorded must not report a clean
	// store: there is nothing inside it, and silence would read as verification.
	future := time.Now().UTC().Add(24 * time.Hour).Format(time.RFC3339)
	out.Reset()
	errOut.Reset()
	if code := cmdVerifyChain([]string{"--dir", root, "--since", future}, &out, &errOut); code == 0 {
		t.Fatalf("verify claimed success for an empty window: %s", out.String())
	}
}
