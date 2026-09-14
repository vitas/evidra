package main

// The no-Evidra baseline arm of the Gate A harness.
//
// Every other mode in this harness measures an agent *talking to Evidra*. `off` and `all`
// differ only in whether the endpoint refuses out-of-order calls; both wrap the upstream, so
// both give the model the same tool list, the same `initialize.instructions`, the same two
// protocol tools and the same proxy transport. That design answers "what does enforcement
// cost?" and cannot answer the product question that comes first:
//
//	does putting Evidra in the MCP path make the agent worse at the underlying work?
//
// `none` is the missing comparison: the agent is driven against `evidra-fixture` directly,
// with no endpoint process at all. It is a harness-only mode. There is no `evidra-mcp
// --enforce=none`, no production meaning, and no cell in any Gate A condition: `none` exists
// to make a wrapped number interpretable, and a baseline that could certify a gate would be a
// way of certifying it with a run that never used the thing under test.
//
// Two rules follow from that and are enforced in code below rather than left to the reader:
//
//  1. Protocol metrics are *undefined* for a `none` cell, never zero. A zero in
//     `terminal_report_coverage` reads as "the agent never closed a record", which is a claim
//     about behaviour that was not observable in this cell - the protocol tools did not
//     exist. The rollup renders those keys as "n/a" and the invariants require them to be
//     structurally absent (zero) in the underlying rows.
//  2. The predicates that decide `none` success are the operational ones only: which upstream
//     tools ran, with what arguments, in what order, without strays. Grading a direct fixture
//     run against `require_report` would score the absence of a tool as a failure of the
//     agent, which is exactly the measurement error this whole file exists to prevent.

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

const (
	// modeNone is the baseline: agent → evidra-fixture, no endpoint in between.
	modeNone = "none"
	// modeOff and modeAll are the wrapped modes, identical except for enforcement.
	modeOff = "off"
	modeAll = "all"

	// metricNotApplicable is what a protocol metric prints for a baseline cell. It is a
	// string in a numeric field on purpose: a reader who tries to do arithmetic with it
	// will notice, which is the point.
	metricNotApplicable = "n/a"
)

// harnessModes is the complete set of modes this *runner* can execute. It is deliberately
// not a product list: `pkg/proxy` knows `off` and `all` and nothing here changes that.
func harnessModes() []string { return []string{modeNone, modeOff, modeAll} }

// executionPath names, for provenance, the process topology a run was measured through. A
// baseline run must not carry the endpoint binary's hash: hashing `bin/evidra-mcp` and
// leaving it in the artifact would let a run claim provenance from a program it never
// started.
// The two topologies, named in artifacts rather than inferred from a mode string by whoever
// reads them.
const (
	pathProxy         = "evidra_proxy"
	pathDirectFixture = "direct_fixture"
)

func executionPath(mode string) string {
	if mode == modeNone {
		return pathDirectFixture
	}
	return pathProxy
}

func isBaseline(mode string) bool { return mode == modeNone }

// validateModes rejects anything outside the harness vocabulary, including a production
// enforcement value that this runner does not implement. Silently dropping an unknown mode
// would plan zero runs and report success.
func validateModes(want []string) error {
	for _, m := range want {
		if !inList(m, harnessModes()) {
			return fmt.Errorf("unknown mode %q: the harness knows %s (production enforce modes are %s and %s)",
				m, strings.Join(harnessModes(), ", "), modeOff, modeAll)
		}
	}
	return nil
}

// modesForArm decides which modes one arm actually runs.
//
// `off`/`all` come from the arm definition, so `--modes-only` can only narrow what arms.json
// pinned. `none` is different: it is not a property of a model, it is a property of the
// comparison, so it runs only when it is asked for explicitly. That asymmetry is what keeps
// an ordinary `--modes-only off,all` from quietly doubling the experiment.
func modesForArm(a armSpec, want []string) []string {
	var out []string
	for _, m := range a.Modes {
		if len(want) > 0 && !inList(m, want) {
			continue
		}
		out = append(out, m)
	}
	if inList(modeNone, want) && !inList(modeNone, out) {
		out = append([]string{modeNone}, out...)
	}
	return out
}

// operationalFails grades what a run did to the upstream. These predicates are meaningful with
// or without Evidra, because they are read off calls the fixture itself received.
func (ts *taskSpec) operationalFails(t *transcript) []string {
	var fails []string
	fails = append(fails, checkExpectations(ts.ExpectTools, t)...)
	fails = append(fails, checkOrder(ts.ExpectOrder, t)...)
	fails = append(fails, checkStrayUpstream(ts, t)...)
	return fails
}

// protocolFails grades the Evidra clauses: the terminal report, and for change-of-mind the
// replacement/abandonment state of the dropped operation. Both need a protocol surface to be
// observable at all, which is why a baseline run must not be scored on them.
func (ts *taskSpec) protocolFails(t *transcript) []string {
	var fails []string
	fails = append(fails, checkAbandonPath(ts, t)...)
	fails = append(fails, checkReport(ts.RequireReport, t)...)
	return fails
}

// baselinePrompt returns the words a `none` run gives the model: the same underlying
// operational work, with no record, no prescribe/report, and no Evidra. It is authored per
// task in tasks.json, not produced by deleting phrases at runtime - a prompt that has had
// sentences removed is a different task from the one a reader can check.
func (ts *taskSpec) baselinePrompt() string { return ts.BaselineGoal }

// validateBaselineSpecs is the load-time guarantee that a baseline comparison is possible at
// all. A missing or protocol-flavoured baseline goal would make `none` measure nothing, or
// measure the agent's ability to guess a protocol it was never told about.
func validateBaselineSpecs(tasks []taskSpec) error {
	// Vocabulary that belongs to the Evidra protocol and to no operational goal. Checking
	// words rather than structure is crude, and that is deliberate: an author who writes
	// "then close the record" in a baseline prompt has described a task that cannot run.
	forbidden := []string{"record", "prescri", "report", "evidra", "abandon"}
	for _, ts := range tasks {
		goal := strings.TrimSpace(ts.BaselineGoal)
		if goal == "" {
			return fmt.Errorf("task %q has no baseline_goal: mode %q needs a prompt that states the operational work without Evidra", ts.ID, modeNone)
		}
		if strings.TrimSpace(goal) == strings.TrimSpace(ts.Goal) {
			return fmt.Errorf("task %q: baseline_goal is identical to the wrapped goal, so it still asks for protocol work", ts.ID)
		}
		low := strings.ToLower(goal)
		for _, word := range forbidden {
			if strings.Contains(low, word) {
				return fmt.Errorf("task %q: baseline_goal mentions %q, which is Evidra protocol vocabulary and does not exist in mode %q", ts.ID, word, modeNone)
			}
		}
		if strings.Contains(low, "then close") {
			return fmt.Errorf("task %q: baseline_goal still asks to close something", ts.ID)
		}
	}
	return nil
}

// countUpstream measures the operational traffic a run generated: calls the upstream actually
// received, and how many of those came back as errors. Both are comparable across `none` and
// `off` because neither depends on the protocol surface existing.
func countUpstream(t *transcript) (calls, errs int) {
	for _, ev := range t.Events {
		if ev.Local || ev.Blocked {
			continue
		}
		calls++
		if ev.IsError {
			errs++
		}
	}
	return calls, errs
}

// protocolMetricsApplicable is the single answer to "may this cell be read as protocol
// evidence?". It is derived from the mode rather than stored, so regrading an artifact set
// produced before this field existed cannot turn a wrapped cell into a baseline one by
// omission.
func protocolMetricsApplicable(mode string) bool { return !isBaseline(mode) }

// MarshalJSON renders a baseline cell's protocol metrics as "n/a".
//
// The struct keeps its numeric types, because the gate judge and the invariant checks need
// arithmetic; only the artifact changes shape. The alternative - writing zeros - is what this
// method exists to prevent: `terminal_report_coverage: 0` over 8 baseline runs is not a
// measurement of 0%, it is a statement about a tool the agent was never given.
func (c cellMetrics) MarshalJSON() ([]byte, error) {
	type plainCell cellMetrics
	raw, err := json.Marshal(plainCell(c))
	if err != nil {
		return nil, err
	}
	var out map[string]any
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, err
	}
	applicable := protocolMetricsApplicable(c.Mode)
	out["protocol_metrics_applicable"] = applicable
	out["execution_path"] = executionPath(c.Mode)
	if !applicable {
		for _, key := range protocolMetricKeys() {
			out[key] = metricNotApplicable
		}
	}
	return json.Marshal(out)
}

// protocolMetricKeys are the §10 metrics that describe protocol behaviour and therefore have
// no value in a cell where no protocol existed.
func protocolMetricKeys() []string {
	return []string{
		"terminal_report_coverage",
		"first_attempt_protocol_compliance",
		"protocol_only_success",
		"blocked_attempts",
		"median_blocked_attempts_per_completed_operation",
		"recovery_after_first_block",
		"voluntary_prescription_coverage",
		"late_prescription_rate",
		"unprescribed_executions",
		"sessions_with_no_prescribe",
		"operations_replaced_without_report",
	}
}

// operationalMetricKeys are what a baseline cell does have, and the columns the none↔off
// comparison is allowed to be read from.
func operationalMetricKeys() []string {
	return []string{
		"runs", "invalid_runs", "task_success", "turns_total", "mean_turns",
		"prompt_tokens_total", "completion_tokens_total", "reasoning_tokens_total",
		"duration_ms_total", "mean_duration_ms", "upstream_calls", "upstream_errors",
	}
}

// baselineComparisons pairs each arm's `none` cell with its `off` cell, per task and overall,
// and reports the operational columns side by side with their difference.
//
// It is descriptive on purpose. No harm threshold was declared before the data existed, so
// none is offered: a score invented after seeing a delta would be a way of letting a
// measurement decide a product question it was not designed to answer.
func baselineComparisons(cells []cellMetrics) []map[string]any {
	byCell := map[string]cellMetrics{}
	for _, c := range cells {
		byCell[c.Arm+"/"+c.Mode] = c
	}
	arms := map[string]bool{}
	for _, c := range cells {
		if c.Mode == modeNone {
			arms[c.Arm] = true
		}
	}
	names := make([]string, 0, len(arms))
	for a := range arms {
		names = append(names, a)
	}
	sort.Strings(names)
	var out []map[string]any
	for _, arm := range names {
		baseline, okB := byCell[arm+"/"+modeNone]
		observe, okO := byCell[arm+"/"+modeOff]
		if !okB || !okO {
			continue
		}
		out = append(out, map[string]any{
			"arm":                    arm,
			"modes":                  []string{modeNone, modeOff},
			"note":                   "descriptive comparison only: no harm threshold was predeclared, and neither cell certifies a gate",
			"baseline":               comparisonColumns(baseline),
			"observe_only":           comparisonColumns(observe),
			"delta":                  comparisonDelta(baseline, observe),
			"enforcement_not_tested": "this pair measures the presence of Evidra; off↔all measures enforcement",
		})
	}
	return out
}

func comparisonColumns(c cellMetrics) map[string]any {
	return map[string]any{
		"mode":              c.Mode,
		"runs":              c.Runs,
		"task_success":      c.TaskSuccess,
		"turns_total":       c.TurnsTotal,
		"mean_turns":        c.MeanTurns,
		"prompt_tokens":     c.PromptTokens,
		"completion_tokens": c.OutputTokens,
		"reasoning_tokens":  c.ReasonTokens,
		"duration_ms":       c.DurationMS,
		"mean_duration_ms":  c.MeanDurationMS,
		"upstream_calls":    c.UpstreamCalls,
		"upstream_errors":   c.UpstreamErrors,
	}
}

func comparisonDelta(baseline, observe cellMetrics) map[string]any {
	diff := func(a, b float64) float64 { return b - a }
	return map[string]any{
		"task_success":            observe.TaskSuccess - baseline.TaskSuccess,
		"mean_turns":              diff(baseline.MeanTurns, observe.MeanTurns),
		"prompt_tokens":           observe.PromptTokens - baseline.PromptTokens,
		"completion_tokens":       observe.OutputTokens - baseline.OutputTokens,
		"mean_duration_ms":        diff(baseline.MeanDurationMS, observe.MeanDurationMS),
		"upstream_errors":         observe.UpstreamErrors - baseline.UpstreamErrors,
		"upstream_calls":          observe.UpstreamCalls - baseline.UpstreamCalls,
		"direction_convention":    "positive = the wrapped (off) cell is larger; for task_success that is better, for tokens, turns and duration it is the cost of Evidra's presence",
		"protocol_metrics_absent": true,
	}
}

// checkBaselineInvariants holds the two claims that make a baseline cell interpretable: it
// carried no protocol traffic, and no comparison number was invented.
func checkBaselineInvariants(cells []cellMetrics, byCell map[string][]runResult) []string {
	var violations []string
	add := func(format string, args ...any) {
		violations = append(violations, fmt.Sprintf(format, args...))
	}
	for _, c := range cells {
		if !isBaseline(c.Mode) {
			continue
		}
		label := c.Arm + "/" + c.Mode
		// A baseline run has no endpoint, so it cannot have protocol traffic. Anything
		// non-zero here means the mode leaked: a faked protocol tool, or a run that went
		// through the proxy while being labelled a baseline.
		// Voluntary coverage and late prescription are rendered strings; their underlying
		// counts are the ones a run row can be wrong about.
		if c.TerminalReportCover != 0 || c.FirstAttemptComply != 0 || c.BlockedAttempts != 0 ||
			c.UnprescribedExecs != 0 || c.SessionsNoPrescribe != 0 || c.ReplacedNoReport != 0 ||
			c.ProtocolOnlySuccess != 0 {
			add("%s: baseline cell carries protocol traffic (reports=%d blocked=%d unprescribed=%d), so it is not a no-Evidra run",
				label, c.TerminalReportCover, c.BlockedAttempts, c.UnprescribedExecs)
		}
		for _, r := range byCell[label] {
			if r.Store != nil {
				add("%s %s run%d: baseline run has an evidence store (%s), so an endpoint participated",
					label, r.Task, r.Run, r.Store.Dir)
			}
			if r.Provenance != nil && r.Provenance.EndpointSHA256 != "" {
				add("%s %s run%d: baseline provenance names an endpoint binary (%s), which this run never started",
					label, r.Task, r.Run, fileName(r.Provenance.EndpointSHA256))
			}
			if r.Mode != modeNone {
				add("%s: run row has mode %q inside a baseline cell", label, r.Mode)
			}
		}
	}
	return violations
}

// plannedModes is the mode set a run set will actually cover, derived the same way the queue
// is built. It decides what preflight may require, so it must not drift from buildQueue.
func plannedModes(arms []armSpec, o options) []string {
	want := splitList(o.onlyModes)
	out := []string{}
	seen := map[string]bool{}
	for _, a := range arms {
		for _, m := range modesForArm(a, want) {
			if !seen[m] {
				seen[m] = true
				out = append(out, m)
			}
		}
	}
	return out
}

// needsEndpointBinary is the precondition the endpoint checks hang off: at least one mode
// that starts an endpoint process.
func needsEndpointBinary(modes []string) bool {
	for _, m := range modes {
		if !isBaseline(m) {
			return true
		}
	}
	return false
}

// tableRow renders one cell for the console in the same shape as the artifact: protocol
// columns become "n/a" in a baseline row, so a reader skimming a terminal cannot absorb a
// zero as a result.
func (c cellMetrics) tableRow() string {
	col := func(v int) string {
		if !protocolMetricsApplicable(c.Mode) {
			return metricNotApplicable
		}
		return fmt.Sprintf("%d", v)
	}
	recovery := c.RecoveryRate
	if !protocolMetricsApplicable(c.Mode) {
		recovery = metricNotApplicable
	}
	return fmt.Sprintf("%-26s %-5s %4d %7d %9s %7s   %7s   %-34s %10.2f  %5d(%d)  %8.0fms",
		c.Arm, c.Mode, c.Runs, c.TaskSuccess, col(c.ProtocolOnlySuccess),
		col(c.TerminalReportCover), col(c.BlockedAttempts), recovery,
		c.MeanTurns, c.UpstreamCalls, c.UpstreamErrors, c.MeanDurationMS)
}

// gateApplicability answers "what do the numbers above certify?" in one line, in the
// artifact and on the terminal.
//
// A run set of baseline cells has no gate conditions at all. Left unstated, that silence
// reads as a gate that passed, which is the exact misuse this mode invites: a no-Evidra
// comparison cannot certify a protocol it excluded by construction.
func gateApplicability(cells []cellMetrics) string {
	// The count that matters is of the mode the conditions are actually defined over.
	// Counting "any wrapped cell" would let an enforce=off-only set claim to carry Gate A
	// conditions, which §10 does not do: none of its thresholds are evaluated off-band.
	var enforced, observeOnly, baseline int
	for _, c := range cells {
		switch c.Mode {
		case modeNone:
			baseline++
		case modeAll:
			enforced++
		default:
			observeOnly++
		}
	}
	switch {
	case enforced > 0 && baseline > 0:
		return fmt.Sprintf("partial: %d enforce=all cell(s) carry the Gate A conditions; %d baseline cell(s) (mode %s) are "+
			"comparison-only and contribute no protocol metric to them", enforced, baseline, modeNone)
	case enforced > 0:
		return fmt.Sprintf("Gate A conditions apply to the %d enforce=all cell(s) above; this set contains no baseline cell", enforced)
	case baseline > 0 && observeOnly > 0:
		return fmt.Sprintf("not applicable: no enforce=all cell in this set (%d observe-only, %d baseline), and Gate A's thresholds are "+
			"defined over enforced cells only; the %d baseline cell(s) measure the presence of Evidra, not its protocol",
			observeOnly, baseline, baseline)
	case baseline > 0:
		return "not applicable: every cell here is a no-Evidra baseline (direct fixture, no endpoint process). " +
			"Gate A is defined over wrapped enforce=all cells, so this run set can compare task success and cost, " +
			"and certifies nothing."
	default:
		return fmt.Sprintf("not certified: %d observe-only cell(s) and no enforce=all cell, so no Gate A threshold applies to this set", observeOnly)
	}
}

func gateApplicabilityLines(cells []cellMetrics, conds []gateCondition) []string {
	out := []string{"  [GATE        ] " + gateApplicability(cells)}
	if hasBaseline(cells) && len(conds) > 0 {
		out = append(out, "  [GATE        ] baseline cells were excluded above: mode "+modeNone+" has no protocol surface to judge")
	}
	return out
}

func hasBaseline(cells []cellMetrics) bool {
	for _, c := range cells {
		if isBaseline(c.Mode) {
			return true
		}
	}
	return false
}

// comparisonLines prints each none/off pair with its delta. The convention is spelled out on
// every row: "the wrapped cell is larger by 2 turns" and its negation are otherwise the same
// four characters.
func comparisonLines(cells []cellMetrics) []string {
	pairs := baselineComparisons(cells)
	if len(pairs) == 0 {
		return nil
	}
	out := []string{"\nnone vs off - cost of introducing Evidra's surface (operational columns only, descriptive, no threshold declared):"}
	out = append(out, "cell                     mode   runs  success  mean-turns  mean-duration   upstream(err)   tokens p+c+r")
	for _, pair := range pairs {
		arm := pair["arm"].(string)
		for _, key := range []string{"baseline", "observe_only"} {
			c := pair[key].(map[string]any)
			out = append(out, fmt.Sprintf("%-26s %-6s %4d %4d/%-3d %10.2f %13.0fms %8d(%d) %8d+%d+%d",
				arm, c["mode"], c["runs"], c["task_success"], c["runs"],
				c["mean_turns"], c["mean_duration_ms"], c["upstream_calls"], c["upstream_errors"],
				c["prompt_tokens"], c["completion_tokens"], c["reasoning_tokens"]))
		}
		d := pair["delta"].(map[string]any)
		out = append(out, fmt.Sprintf("%-26s        Δ(off-none):  success %+d   turns %+.2f   duration %+.0fms   upstream errors %+d   tokens %d",
			"  "+arm, d["task_success"], d["mean_turns"], d["mean_duration_ms"], d["upstream_errors"],
			d["prompt_tokens"]))
	}
	out = append(out, "  protocol metrics are absent from this table by construction: undefined where no protocol ran")
	return out
}
