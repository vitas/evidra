package report

import (
	"strings"
	"testing"
	"time"
)

func reviewSummary() Summary {
	return Summary{Cells: []Cell{{
		EnforceMode: "all", UpstreamID: "fixture", ServerName: "fixture", DigestKeyID: "key-1",
		ChainValid: true, SignatureValid: true, Coverage: "complete",
		Operations: []Operation{{
			OperationID: "EV-1", RecorderInstance: "recorder-1", State: "reported",
			Objective: "Restart checkout", ExpectedOutcome: "Checkout responds",
			Report:    &Report{Status: "completed", Outcome: "achieved", Summary: "Service is ready"},
			Anomalies: []string{AnomalyClaimedAchievedWithoutExecution},
		}},
	}}}
}

func TestOperationReviewSeparatesClaimsObservationsAndNextCheck(t *testing.T) {
	text, err := reviewSummary().OperationReviewMarkdown("EV-1")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"# Operation review: EV-1", "## Declared by agent", "Restart checkout",
		"## Observed at MCP boundary", "No upstream executions were observed",
		"## Reported by agent", "completed", "achieved", "Service is ready",
		"## Review finding", "reported achievement without an observed execution",
		"## Next check", "independent", "chain: valid", "signature: valid", "coverage: complete",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("review missing %q:\n%s", want, text)
		}
	}
}

func TestOperationReviewShowsObservedErrorsWithoutCallingOutcomeFalse(t *testing.T) {
	sum := reviewSummary()
	op := &sum.Cells[0].Operations[0]
	op.Executions = []Execution{{
		ExecutionID: "EX-1", Tool: "restart_deployment", Status: "error", Paired: true,
		StartedAt: time.Date(2026, 9, 24, 8, 12, 0, 0, time.UTC),
	}}
	op.Anomalies = nil
	text, err := sum.OperationReviewMarkdown("EV-1")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"EX-1", "2026-09-24T08:12:00Z", "restart_deployment", "error", "Inspect the upstream error", "independent"} {
		if !strings.Contains(text, want) {
			t.Errorf("review missing %q:\n%s", want, text)
		}
	}
	if strings.Contains(text, "objective failed") {
		t.Errorf("review inferred external failure from MCP error:\n%s", text)
	}
}

func TestOperationReviewSurfacesReadOnlyOnlyFact(t *testing.T) {
	sum := reviewSummary()
	op := &sum.Cells[0].Operations[0]
	op.Executions = []Execution{{Tool: "get_status", Status: "success", Paired: true, DeclaredReadOnly: true}}
	op.Anomalies = nil
	op.Facts = []string{FactAchievedWithDeclaredReadOnlyOnly}
	text, err := sum.OperationReviewMarkdown("EV-1")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(text, "only executions that the upstream declared read-only") || !strings.Contains(text, "unverified") {
		t.Fatalf("read-only fact missing or overstated:\n%s", text)
	}
}

func TestOperationReviewRejectsMissingAndAmbiguousIDs(t *testing.T) {
	sum := reviewSummary()
	if _, err := sum.OperationReviewMarkdown("EV-missing"); err == nil || !strings.Contains(err.Error(), "not found") {
		t.Fatalf("missing ID error = %v", err)
	}
	sum.Cells = append(sum.Cells, sum.Cells[0])
	if _, err := sum.OperationReviewMarkdown("EV-1"); err == nil || !strings.Contains(err.Error(), "more than one recorder") {
		t.Fatalf("ambiguous ID error = %v", err)
	}
}

func TestOperationReviewEscapesAgentText(t *testing.T) {
	sum := reviewSummary()
	sum.Cells[0].Operations[0].Objective = "restart\n## Forged finding\n<script>alert(1)</script>"
	text, err := sum.OperationReviewMarkdown("EV-1")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(text, "\n## Forged finding") || strings.Contains(text, "<script>") {
		t.Fatalf("agent text escaped into report structure:\n%s", text)
	}
}
