package report

import (
	"fmt"
	"html"
	"strconv"
	"strings"
)

// OperationReviewMarkdown renders one operation as a source-aware review. The
// input summary must be built without a time filter so its integrity fields
// describe the full recorder history available to the reader.
func (s Summary) OperationReviewMarkdown(operationID string) (string, error) {
	var selected *Operation
	var domain *Cell
	for i := range s.Cells {
		for j := range s.Cells[i].Operations {
			op := &s.Cells[i].Operations[j]
			if op.OperationID != operationID {
				continue
			}
			if selected != nil {
				return "", fmt.Errorf("operation %q occurs in more than one recorder; select a unique operation ID", operationID)
			}
			selected, domain = op, &s.Cells[i]
		}
	}
	if selected == nil {
		return "", fmt.Errorf("operation %q not found", operationID)
	}

	var b strings.Builder
	fmt.Fprintf(&b, "# Operation review: %s\n\n", html.EscapeString(operationID))
	fmt.Fprintf(&b, "Recorder: %s  \n", codeText(selected.RecorderInstance))
	fmt.Fprintf(&b, "Upstream: %s  \n", codeText(domain.UpstreamID))
	fmt.Fprintf(&b, "Enforcement: %s  \n", codeText(domain.EnforceMode))
	fmt.Fprintf(&b, "Evidence in comparison group: chain: %s; signature: %s; coverage: %s.\n\n",
		validityWord(domain.ChainValid), validityWord(domain.SignatureValid), html.EscapeString(domain.Coverage))

	b.WriteString("## Declared by agent\n\n")
	claimLine(&b, "Objective", selected.Objective)
	claimLine(&b, "Expected outcome", selected.ExpectedOutcome)

	b.WriteString("## Observed at MCP boundary\n\n")
	if len(selected.Executions) == 0 {
		b.WriteString("No upstream executions were observed for this operation.\n\n")
	} else {
		fmt.Fprintf(&b, "%d upstream execution(s) were observed:\n\n", len(selected.Executions))
		for i, ex := range selected.Executions {
			status := ex.Status
			if !ex.Paired {
				status = "no matching finish event"
			}
			fmt.Fprintf(&b, "%d. Tool %s — MCP status %s; duration %d ms", i+1,
				codeText(ex.Tool), codeText(status), ex.DurationMS)
			if ex.ExecutionID != "" {
				fmt.Fprintf(&b, "; execution ID %s", codeText(ex.ExecutionID))
			}
			if !ex.StartedAt.IsZero() {
				fmt.Fprintf(&b, "; started %s", ex.StartedAt.UTC().Format("2006-01-02T15:04:05Z07:00"))
			}
			b.WriteString(".\n")
		}
		b.WriteString("\n")
	}
	b.WriteString("MCP success describes the response at the proxy boundary. Raw arguments and results are not stored in this review.\n\n")

	b.WriteString("## Reported by agent\n\n")
	if selected.Report == nil {
		b.WriteString("No terminal report was recorded.\n\n")
	} else {
		fmt.Fprintf(&b, "Status: %s; outcome: %s.\n\n", codeText(selected.Report.Status), codeText(selected.Report.Outcome))
		claimLine(&b, "Agent summary", selected.Report.Summary)
	}

	b.WriteString("## Review finding\n\n")
	findings := operationFindings(*selected, *domain)
	for _, finding := range findings {
		fmt.Fprintf(&b, "- %s\n", finding)
	}
	b.WriteString("\n## Next check\n\n")
	b.WriteString(nextCheck(*selected, *domain))
	b.WriteString("\n\nEvidra does not determine whether the external objective was achieved. Local signatures do not establish organizational identity or protect against replacement of the entire recorder directory.\n")
	return b.String(), nil
}

func claimLine(b *strings.Builder, label, value string) {
	if value == "" {
		fmt.Fprintf(b, "%s: not recorded.\n\n", label)
		return
	}
	fmt.Fprintf(b, "%s (agent claim):\n\n    %s\n\n", label, html.EscapeString(strconv.Quote(value)))
}

func codeText(value string) string {
	return "<code>" + html.EscapeString(strconv.Quote(value)) + "</code>"
}

func validityWord(valid bool) string {
	if valid {
		return "valid"
	}
	return "INVALID"
}

func operationFindings(op Operation, cell Cell) []string {
	var findings []string
	if !cell.ChainValid || !cell.SignatureValid {
		findings = append(findings, "At least one recorder in this comparison group failed chain or signature verification.")
	}
	if cell.Coverage != "complete" {
		findings = append(findings, "The comparison group contains a known degraded recording window.")
	}
	if op.Report == nil {
		findings = append(findings, "The operation has no terminal agent report.")
	}
	if op.Report != nil && op.Report.Outcome == "achieved" && len(op.Executions) == 0 {
		findings = append(findings, "The agent reported achievement without an observed execution in this operation.")
	}
	for _, fact := range op.Facts {
		if fact == FactAchievedWithDeclaredReadOnlyOnly {
			findings = append(findings, "The agent reported achievement after only executions that the upstream declared read-only; those annotations are unverified.")
		}
	}
	for _, ex := range op.Executions {
		if !ex.Paired {
			findings = append(findings, "An execution start has no matching finish event.")
			break
		}
	}
	for _, ex := range op.Executions {
		if ex.Status == "error" || ex.Status == "cancelled" {
			findings = append(findings, "At least one MCP execution ended with an error or cancellation.")
			break
		}
	}
	if len(findings) == 0 {
		findings = append(findings, "No reconciliation discrepancy was identified in the recorded operation. External outcome remains unverified.")
	}
	return findings
}

func nextCheck(op Operation, cell Cell) string {
	switch {
	case !cell.ChainValid || !cell.SignatureValid:
		return "Investigate the recorder integrity failure before relying on this evidence; verify external state independently."
	case cell.Coverage != "complete":
		return "Investigate the degraded recording window and verify external state independently."
	case op.Report == nil:
		return "Ask why the operation was not closed, then verify external state independently."
	case op.Report.Outcome == "achieved" && len(op.Executions) == 0:
		return "Find evidence of the claimed action outside this recorder and verify external state independently."
	}
	for _, ex := range op.Executions {
		if !ex.Paired {
			return "Determine how the unfinished MCP call ended and verify external state independently."
		}
	}
	for _, ex := range op.Executions {
		if ex.Status == "error" || ex.Status == "cancelled" {
			return "Inspect the upstream error or cancellation and verify external state independently."
		}
	}
	return "Verify external state independently before accepting the agent's reported outcome."
}
