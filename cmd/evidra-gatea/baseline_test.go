package main

// Tests for the no-Evidra baseline arm (harness mode `none`).
//
// The questions these answer are structural, not cosmetic: did a baseline run really start no
// endpoint, was the model really shown no protocol tool, and do the numbers that a baseline
// cell cannot support stay out of the rollup? A comparison whose control arm accidentally
// included the thing being controlled for is worse than no comparison, because it looks like
// data. Where a claim can be checked two ways - by inspecting an artifact and by asserting in
// code - both are done, because the artifact is what a later reader trusts.

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

var (
	// testFixtureBin and testMCPBin are the real binaries, built once. A baseline test that
	// stubbed the fixture would not prove the tool list came from a server the endpoint did
	// not touch.
	testFixtureBin string
	testMCPBin     string
	testBuildDir   string
)

func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "evidra-gatea-baseline")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	testBuildDir = dir
	testFixtureBin = filepath.Join(dir, "evidra-fixture")
	testMCPBin = filepath.Join(dir, "evidra-mcp")
	for _, b := range []struct{ out, pkg string }{
		{testFixtureBin, "samebits.com/evidra/cmd/evidra-fixture"},
		{testMCPBin, "samebits.com/evidra/cmd/evidra-mcp"},
	} {
		cmd := exec.Command("go", "build", "-o", b.out, b.pkg)
		if out, err := cmd.CombinedOutput(); err != nil {
			fmt.Fprintf(os.Stderr, "build %s: %v\n%s\n", b.pkg, err, out)
			_ = os.RemoveAll(dir)
			os.Exit(1)
		}
	}
	code := m.Run()
	_ = os.RemoveAll(dir)
	os.Exit(code)
}

func baselineArm() armSpec {
	return armSpec{ID: "baseline-arm", APIModelID: "test-model", Modes: []string{modeNone, modeOff, modeAll}}
}

// baselineCLIArgs is the shared flag set for a baseline run through the real CLI: dry-run so
// no token is spent, an endpoint binary that does not exist, and the freshly built fixture.
func baselineCLIArgs(t *testing.T, extra ...string) []string {
	args := []string{
		"--dry-run", "--modes-only", modeNone, "--runs", "1",
		"--arms-only", "qwen38-flash",
		"--fixture", testFixtureBin,
		"--evidra-mcp", missingEndpoint(t),
	}
	return append(args, extra...)
}

func mustTasks(t *testing.T) []taskSpec {
	t.Helper()
	tasks, err := loadTasks("")
	if err != nil {
		t.Fatalf("load tasks: %v", err)
	}
	return tasks
}

func taskByID(t *testing.T, id string) taskSpec {
	t.Helper()
	for _, ts := range mustTasks(t) {
		if ts.ID == id {
			return ts
		}
	}
	t.Fatalf("task %q missing", id)
	return taskSpec{}
}

// missingEndpoint returns a path that cannot be executed. Used as the endpoint binary in
// baseline runs: if any part of the baseline path tried to start it, the run would fail, which
// is a stronger assertion than inspecting a command line after the fact.
func missingEndpoint(t *testing.T) string {
	return filepath.Join(t.TempDir(), "no-such-evidra-mcp")
}

func baselineOptions(t *testing.T) options {
	return options{
		runs: 1, parallel: 1, maxTokens: 2048, wallClock: 90 * time.Second,
		dryRun: true, script: scriptCompliant,
		mcpBin: missingEndpoint(t), fixtureBin: testFixtureBin,
	}
}

// frames reads the persisted transcript frames of one run.
func frames(t *testing.T, path string) []struct {
	Kind    string
	Payload json.RawMessage
} {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatalf("open transcript: %v", err)
	}
	defer func() { _ = f.Close() }()
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 64*1024*1024)
	var out []struct {
		Kind    string
		Payload json.RawMessage
	}
	for sc.Scan() {
		var rec struct {
			Kind    string          `json:"kind"`
			Payload json.RawMessage `json:"payload"`
		}
		if err := json.Unmarshal(sc.Bytes(), &rec); err != nil {
			t.Fatalf("frame: %v", err)
		}
		out = append(out, struct {
			Kind    string
			Payload json.RawMessage
		}{rec.Kind, rec.Payload})
	}
	if err := sc.Err(); err != nil {
		t.Fatalf("scan: %v", err)
	}
	return out
}

// 1 + 7: the baseline path starts the fixture and nothing else, and records that no endpoint
// binary participated.
func TestBaselineRunStartsNoEndpointAndClaimsNone(t *testing.T) {
	out := t.TempDir()
	o := baselineOptions(t)
	res := runOne(context.Background(), o, baselineArm(), modeNone, taskByID(t, "status-then-report"), 1, out)
	if res.InvalidRun != "" {
		t.Fatalf("baseline run invalid: %s", res.InvalidRun)
	}
	if !res.Success {
		t.Fatalf("baseline run did not pass its operational predicates: %v", res.Failures)
	}
	if res.Provenance == nil {
		t.Fatal("no provenance recorded")
	}
	if res.Provenance.EndpointSHA256 != "" {
		t.Errorf("baseline provenance hashed an endpoint binary: %s", res.Provenance.EndpointSHA256)
	}
	if res.Provenance.ExecutionPath != pathDirectFixture {
		t.Errorf("execution path = %q, want %q", res.Provenance.ExecutionPath, pathDirectFixture)
	}
	if res.Provenance.FixtureSHA256 == "" || res.Provenance.RunnerSHA256 == "" {
		t.Error("baseline provenance must still name the fixture and runner that did participate")
	}
	runDir := filepath.Dir(res.TranscriptPath)
	if _, err := os.Stat(filepath.Join(runDir, "evidence")); !os.IsNotExist(err) {
		t.Errorf("baseline run left an evidence directory in %s: an endpoint wrote it", runDir)
	}
	if res.Store != nil {
		t.Errorf("baseline run reported store facts from %s", res.Store.Dir)
	}

	// The contrast that makes the assertion mean anything: with the same options and a
	// wrapped mode, the missing endpoint must break the run, because that path does exec it.
	wrapped := runOne(context.Background(), o, baselineArm(), modeOff, taskByID(t, "status-then-report"), 1, t.TempDir())
	if wrapped.InvalidRun == "" || !strings.Contains(wrapped.InvalidRun, "endpoint start") {
		t.Errorf("wrapped run with a missing endpoint = %q, want a failure to start it", wrapped.InvalidRun)
	}
	if wrapped.Provenance.ExecutionPath != pathProxy {
		t.Errorf("wrapped execution path = %q, want %q", wrapped.Provenance.ExecutionPath, pathProxy)
	}
}

// 2 + 3: the model is shown only the fixture's own tools, and the instructions it receives are
// the fixture's, verbatim - not Evidra's.
func TestBaselineModelSeesOnlyFixtureToolsAndInstructions(t *testing.T) {
	// What the fixture publishes on its own, read directly: the reference the baseline run
	// must match, rather than a hand-written list that could drift from the fixture.
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	ep, err := startMCP(ctx, testFixtureBin)
	if err != nil {
		t.Fatalf("start fixture: %v", err)
	}
	defer ep.close()
	if _, err := ep.initialize(ctx); err != nil {
		t.Fatalf("initialize fixture: %v", err)
	}
	native, err := ep.listAllTools(ctx)
	if err != nil {
		t.Fatalf("list tools: %v", err)
	}
	fixtureInstructions, _ := ep.header["instructions"].(string)
	if len(native) == 0 {
		t.Fatal("fixture published no tools; the comparison below is vacuous")
	}

	o := baselineOptions(t)
	res := runOne(ctx, o, baselineArm(), modeNone, taskByID(t, "status-then-report"), 1, t.TempDir())
	if res.InvalidRun != "" {
		t.Fatalf("baseline run invalid: %s", res.InvalidRun)
	}
	var listed []mcpTool
	var instructions string
	for _, fr := range frames(t, res.TranscriptPath) {
		switch fr.Kind {
		case "tools":
			if err := json.Unmarshal(fr.Payload, &listed); err != nil {
				t.Fatal(err)
			}
		case "instructions":
			if err := json.Unmarshal(fr.Payload, &instructions); err != nil {
				t.Fatal(err)
			}
		}
	}
	if len(listed) != len(native) {
		t.Errorf("baseline saw %d tools, fixture publishes %d", len(listed), len(native))
	}
	for _, tl := range listed {
		if strings.HasPrefix(tl.Name, "evidra_") {
			t.Errorf("baseline tool list contains the protocol tool %q", tl.Name)
		}
	}
	if instructions != fixtureInstructions {
		t.Errorf("instructions = %q, want the fixture's own %q", truncate(instructions, 80), truncate(fixtureInstructions, 80))
	}
	// What must be absent is Evidra's protocol surface, not the project's name. The fixture
	// describes itself as "Fixture MCP server for Evidra vNext conformance tests", which is
	// the upstream's own sentence, and rewriting it would change what both arms see. So the
	// property under test is: no operation vocabulary, and the text is the fixture's rather
	// than the endpoint's. A reader of the comparison should know the baseline model does see
	// the word Evidra exactly once, in the upstream's self-description.
	for _, word := range []string{"evidra_prescribe", "evidra_report", "prescribe", "operation", "record"} {
		if strings.Contains(strings.ToLower(instructions), word) {
			t.Errorf("baseline instructions carry protocol vocabulary %q: %q", word, truncate(instructions, 120))
		}
	}
}

// A tool list is only evidence of what the model was offered if it reached the model. This
// wrapper records the `tools` argument of every turn, so the assertion is about the agent's
// input rather than about a file the runner wrote about itself.
type recordingAgent struct {
	inner agent
	seen  [][]llmTool
}

func (r *recordingAgent) next(ctx context.Context, messages []llmMessage, tools []llmTool) (*completion, error) {
	r.seen = append(r.seen, tools)
	return r.inner.next(ctx, messages, tools)
}

func TestBaselineDriveAgentOffersNoProtocolTools(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	ep, err := startMCP(ctx, testFixtureBin)
	if err != nil {
		t.Fatalf("start fixture: %v", err)
	}
	defer ep.close()
	if _, err := ep.initialize(ctx); err != nil {
		t.Fatalf("initialize: %v", err)
	}
	tools, err := ep.listAllTools(ctx)
	if err != nil {
		t.Fatalf("list tools: %v", err)
	}
	llmTools := make([]llmTool, 0, len(tools))
	nameMap := map[string]string{}
	for _, tl := range tools {
		safe := sanitizeToolName(tl.Name)
		nameMap[safe] = tl.Name
		llmTools = append(llmTools, toolFor(safe, tl.Description, tl.InputSchema))
	}
	task := taskByID(t, "status-then-report")
	rec := &recordingAgent{inner: &scriptedAgent{plan: scriptedPlan(task, scriptCompliant, true)}}
	tr, invalid := driveAgent(ctx, rec, ep, llmTools, nameMap, task, false, func(string, any) {})
	if invalid != "" {
		t.Fatalf("drive: %s", invalid)
	}
	if len(rec.seen) == 0 {
		t.Fatal("agent was never asked; this check would pass vacuously")
	}
	for i, turn := range rec.seen {
		for _, tl := range turn {
			if strings.Contains(tl.Function.Name, "evidra") {
				t.Errorf("turn %d offered the protocol tool %q", i+1, tl.Function.Name)
			}
		}
	}
	if tr.Prescribes != 0 || len(tr.Reports) != 0 {
		t.Errorf("baseline transcript recorded protocol traffic: prescribes=%d reports=%d", tr.Prescribes, len(tr.Reports))
	}
}

// 4 + 5: the grading split, over all eight tasks and both topologies.
//
// A baseline run must not fail for want of an evidra_report it could not make; a wrapped run
// must still fail for it. Both directions are asserted on every task, because a split that is
// only demonstrated on the easiest task is not a split.
func TestBaselineGradesOperationallyAndWrappedStillGradesProtocol(t *testing.T) {
	for _, task := range mustTasks(t) {
		tr := replay(scriptedPlan(task, scriptCompliant, true), false)
		if len(tr.Events) == 0 {
			t.Errorf("task %s: baseline scripted plan made no calls; the comparison would be vacuous", task.ID)
		}
		for _, ev := range tr.Events {
			if ev.Local {
				t.Errorf("task %s: baseline plan used the protocol tool %q", task.ID, ev.Name)
			}
		}
		if fails := task.operationalFails(tr); len(fails) > 0 {
			t.Errorf("task %s: baseline operational predicates unsatisfied: %v", task.ID, fails)
		}

		var noneRes, offRes runResult
		noneRes.Mode = modeNone
		finalizeRun(&noneRes, tr, &task, nil, nil, modeNone, false)
		if !noneRes.Success {
			t.Errorf("task %s: baseline run graded as failure: %v", task.ID, noneRes.Failures)
		}
		if !noneRes.ProtocolNotApplicable {
			t.Errorf("task %s: baseline row does not say its protocol metrics are undefined", task.ID)
		}
		if noneRes.Unprescribed != 0 || noneRes.ProtocolOnly {
			t.Errorf("task %s: baseline row carries protocol verdicts (unprescribed=%d protocolOnly=%v)",
				task.ID, noneRes.Unprescribed, noneRes.ProtocolOnly)
		}

		offRes.Mode = modeOff
		finalizeRun(&offRes, tr, &task, nil, nil, modeOff, false)
		if !strings.Contains(strings.Join(offRes.Failures, "; "), "no terminal evidra_report") {
			t.Errorf("task %s: wrapped run without a report did not fail on the report clause: %v", task.ID, offRes.Failures)
		}
		if offRes.ProtocolNotApplicable {
			t.Errorf("task %s: wrapped row marked protocol metrics inapplicable", task.ID)
		}
		// Wrapped grading must remain exactly operational + protocol, in that order, so the
		// existing Gate A verdicts cannot shift under the split.
		want := append(task.operationalFails(tr), task.protocolFails(tr)...)
		got := task.evaluate(tr)
		if strings.Join(want, "|") != strings.Join(got, "|") {
			t.Errorf("task %s: wrapped grading is no longer operational+protocol\n got %v\nwant %v", task.ID, got, want)
		}
	}
}

// 6: a baseline cell's protocol metrics are undefined, and the artifact says so rather than
// printing zeros.
func TestBaselineCellRendersProtocolMetricsAsNotApplicable(t *testing.T) {
	rows := []runResult{
		{Arm: "a", Mode: modeNone, Task: "t1", Run: 1, Success: true, Turns: 3, DurationMS: 1200, UpstreamCalls: 2,
			ProtocolNotApplicable: true, Provenance: &binaryProvenance{SourceRevision: "r", ExecutionPath: pathDirectFixture}},
		{Arm: "a", Mode: modeNone, Task: "t2", Run: 1, Turns: 5, DurationMS: 800, UpstreamCalls: 1,
			ProtocolNotApplicable: true, Provenance: &binaryProvenance{SourceRevision: "r", ExecutionPath: pathDirectFixture}},
	}
	cell := rollupCell(rows)
	raw, err := json.Marshal(cell)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var got map[string]any
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	for _, key := range protocolMetricKeys() {
		if got[key] != metricNotApplicable {
			t.Errorf("%s = %v, want %q in a baseline cell", key, got[key], metricNotApplicable)
		}
	}
	if got["protocol_metrics_applicable"] != false {
		t.Error("baseline cell claims its protocol metrics are applicable")
	}
	if got["execution_path"] != pathDirectFixture {
		t.Errorf("execution_path = %v", got["execution_path"])
	}
	for _, key := range operationalMetricKeys() {
		if _, ok := got[key]; !ok {
			t.Errorf("operational column %s missing from the baseline cell", key)
		}
	}
	if got["task_success"] != float64(1) || got["runs"] != float64(2) {
		t.Errorf("baseline operational columns wrong: runs=%v success=%v", got["runs"], got["task_success"])
	}
	if got["mean_turns"] != float64(4) || got["duration_ms_total"] != float64(2000) ||
		got["upstream_calls"] != float64(3) {
		t.Errorf("baseline operational aggregates wrong: %v %v %v", got["mean_turns"], got["duration_ms_total"], got["upstream_calls"])
	}
	if v := cell.tableRow(); !strings.Contains(v, metricNotApplicable) {
		t.Errorf("console row prints numbers where the artifact prints n/a: %s", v)
	}

	// A wrapped cell keeps its numbers, including its zeros: those are measurements.
	wrapped := rollupCell([]runResult{{Arm: "a", Mode: modeOff, Task: "t1", Run: 1, Turns: 2}})
	wraw, _ := json.Marshal(wrapped)
	var wmap map[string]any
	if err := json.Unmarshal(wraw, &wmap); err != nil {
		t.Fatal(err)
	}
	if wmap["protocol_metrics_applicable"] != true {
		t.Error("wrapped cell says its protocol metrics are not applicable")
	}
	if wmap["terminal_report_coverage"] != float64(0) {
		t.Errorf("wrapped cell report coverage = %v, want a measured 0", wmap["terminal_report_coverage"])
	}
}

// 6 (continued): a baseline cell must not be able to certify Gate A.
func TestBaselineCellsCertifyNothing(t *testing.T) {
	cells := []cellMetrics{rollupCell([]runResult{
		{Arm: "a", Mode: modeNone, Task: "t1", Run: 1, Success: true, Turns: 2,
			ProtocolNotApplicable: true, Provenance: &binaryProvenance{SourceRevision: "r", ExecutionPath: pathDirectFixture}},
	})}
	if conds := judgeGate(cells, map[string]string{"a": "strong"}); len(conds) != 0 {
		t.Errorf("baseline cells produced %d gate conditions: %v", len(conds), conds)
	}
	if gatePassed(judgeGate(cells, nil)) {
		t.Error("an empty condition set must not read as a passed gate")
	}
	if msg := gateApplicability(cells); !strings.Contains(msg, "not applicable") {
		t.Errorf("gate applicability = %q, want it to say the baseline certifies nothing", msg)
	}
	// Mixed sets keep their conditions and say which cells carry them.
	mixed := append([]cellMetrics{}, cells...)
	mixed = append(mixed, rollupCell([]runResult{
		{Arm: "a", Mode: modeAll, Task: "t1", Run: 1, Success: true, Provenance: &binaryProvenance{SourceRevision: "r", ExecutionPath: pathProxy}},
	}))
	if len(judgeGate(mixed, map[string]string{"a": "strong"})) == 0 {
		t.Error("adding a wrapped cell did not restore any gate condition")
	}
	if msg := gateApplicability(mixed); !strings.Contains(msg, "partial") || !strings.Contains(msg, "1 enforce=all") {
		t.Errorf("mixed set applicability = %q, want it to count the enforce=all cells", msg)
	}
	// Observe-only cells are wrapped but carry no condition either: §10's thresholds are
	// defined over enforced cells. A message that called this "partial" would credit the set
	// with a certification it does not have - which is the mistake this function exists to
	// prevent, one level up.
	offAndNone := []cellMetrics{
		rollupCell([]runResult{{Arm: "a", Mode: modeOff, Task: "t1", Run: 1,
			Provenance: &binaryProvenance{SourceRevision: "r", ExecutionPath: pathProxy}}}),
		rollupCell([]runResult{{Arm: "a", Mode: modeNone, Task: "t1", Run: 1, ProtocolNotApplicable: true,
			Provenance: &binaryProvenance{SourceRevision: "r", ExecutionPath: pathDirectFixture}}}),
	}
	if msg := gateApplicability(offAndNone); !strings.Contains(msg, "not applicable") || strings.Contains(msg, "partial") {
		t.Errorf("off+none applicability = %q, want no certification claimed", msg)
	}
	if len(judgeGate(offAndNone, map[string]string{"a": "strong"})) != 0 {
		t.Error("an observe-only cell produced a gate condition")
	}
}

// 11 + 6: the analytics invariants must reject a "baseline" cell that is not one, and accept
// one that is.
func TestInvariantsGuardTheBaselineClaims(t *testing.T) {
	honest := rollupCell([]runResult{{Arm: "a", Mode: modeNone, Task: "t1", Run: 1, Success: true, Turns: 2,
		DurationMS: 100, UpstreamCalls: 1, ProtocolNotApplicable: true,
		Provenance: &binaryProvenance{SourceRevision: "r", ExecutionPath: pathDirectFixture}}})
	rows := []runResult{{Arm: "a", Mode: modeNone, Task: "t1", Run: 1, Success: true, Turns: 2,
		DurationMS: 100, UpstreamCalls: 1, ProtocolNotApplicable: true,
		Provenance: &binaryProvenance{SourceRevision: "r", ExecutionPath: pathDirectFixture}}}
	byCell := map[string][]runResult{"a/none": rows}
	if v := checkRunInvariants(rows, []cellMetrics{honest}, byCell); len(v) != 0 {
		t.Fatalf("a consistent baseline rollup was rejected: %v", v)
	}

	// A cell that talks protocol cannot be a no-Evidra measurement.
	leaky := honest
	leaky.TerminalReportCover = 1
	if v := checkRunInvariants(rows, []cellMetrics{leaky}, byCell); len(v) == 0 {
		t.Error("a baseline cell with protocol traffic passed the invariants")
	}

	// A baseline row whose provenance names an endpoint is the same lie from the other side.
	claimed := rows[0]
	claimed.Provenance = &binaryProvenance{SourceRevision: "r", EndpointSHA256: "abc", ExecutionPath: pathProxy}
	if v := checkRunInvariants([]runResult{claimed}, []cellMetrics{honest},
		map[string][]runResult{"a/none": {claimed}}); len(v) == 0 {
		t.Error("a baseline row claiming an endpoint binary passed the invariants")
	}

	// A mean that is not its total over its denominator is caught, whatever the mode.
	drifted := honest
	drifted.MeanTurns = 9
	if v := checkRunInvariants(rows, []cellMetrics{drifted}, byCell); len(v) == 0 {
		t.Error("mean_turns disconnected from turns_total passed the invariants")
	}
	sumWrong := honest
	sumWrong.UpstreamCalls = 7
	if v := checkRunInvariants(rows, []cellMetrics{sumWrong}, byCell); len(v) == 0 {
		t.Error("upstream_calls that no row produced passed the invariants")
	}
}

// The none-vs-off comparison: built from cells, descriptive, and holding no protocol column.
func TestNoneVsOffComparisonReportsOperationalCostOnly(t *testing.T) {
	none := cellMetrics{Arm: "a", Mode: modeNone, Runs: 2, TaskSuccess: 2, TurnsTotal: 6, MeanTurns: 3,
		DurationMS: 2000, MeanDurationMS: 1000, UpstreamCalls: 3, PromptTokens: 900, OutputTokens: 90}
	off := cellMetrics{Arm: "a", Mode: modeOff, Runs: 2, TaskSuccess: 1, TurnsTotal: 12, MeanTurns: 6,
		DurationMS: 9000, MeanDurationMS: 4500, UpstreamCalls: 4, PromptTokens: 3000, OutputTokens: 200,
		TerminalReportCover: 1, BlockedAttempts: 5}
	pairs := baselineComparisons([]cellMetrics{none, off})
	if len(pairs) != 1 {
		t.Fatalf("pairs = %d, want one", len(pairs))
	}
	p := pairs[0]
	if p["arm"] != "a" {
		t.Errorf("arm = %v", p["arm"])
	}
	delta := p["delta"].(map[string]any)
	if delta["task_success"] != -1 {
		t.Errorf("success delta = %v, want -1 (off did worse than none)", delta["task_success"])
	}
	if delta["mean_turns"] != 3.0 || delta["mean_duration_ms"] != 3500.0 {
		t.Errorf("cost deltas wrong: %v", delta)
	}
	if _, ok := delta["direction_convention"]; !ok {
		t.Error("delta has no sign convention: the same numbers read both ways")
	}
	for _, side := range []string{"baseline", "observe_only"} {
		col := p[side].(map[string]any)
		for _, key := range protocolMetricKeys() {
			if _, ok := col[key]; ok {
				t.Errorf("comparison column %s carries the protocol metric %q", side, key)
			}
		}
	}
	if !strings.Contains(fmt.Sprint(p["note"]), "no harm threshold") {
		t.Errorf("comparison lacks the descriptive-only note: %v", p["note"])
	}
	if v := checkComparisonAgreement([]cellMetrics{none, off}); len(v) != 0 {
		t.Errorf("a consistent comparison was rejected: %v", v)
	}
	// No baseline cell means no comparison, not an empty row of claims.
	if got := baselineComparisons([]cellMetrics{off}); len(got) != 0 {
		t.Errorf("comparison built without a none cell: %v", got)
	}
	lines := comparisonLines([]cellMetrics{none, off})
	if len(lines) < 3 || !strings.Contains(strings.Join(lines, "\n"), "(off-none)") {
		t.Errorf("comparison table incomplete:\n%s", strings.Join(lines, "\n"))
	}
}

// 8 + 9: what a baseline run may not be blocked by, and what still blocks it.
func TestFreshnessOfWhatParticipates(t *testing.T) {
	staleEndpoint := missingEndpoint(t)
	checks := freshnessChecks([]string{modeNone}, staleEndpoint, testFixtureBin, os.Args[0])
	for _, c := range checks {
		if c.binary == staleEndpoint {
			t.Error("a baseline-only run set still requires the endpoint binary to be fresh")
		}
	}
	if !needsEndpointBinary([]string{modeNone, modeOff}) || needsEndpointBinary([]string{modeNone}) {
		t.Error("needsEndpointBinary disagrees with the modes it was given")
	}
	if len(freshnessChecks([]string{modeOff, modeAll}, staleEndpoint, testFixtureBin, os.Args[0])) != len(checks)+1 {
		t.Error("a wrapped run set must add the endpoint to the freshness checks")
	}
	if err := checkBinaryFreshness(checks); err != nil {
		t.Errorf("baseline freshness checks failed against fresh inputs: %v", err)
	}
	// The fixture and the runner are the two artifacts a baseline run does use, so their
	// staleness must still stop it.
	dir := t.TempDir()
	staleFixture := filepath.Join(dir, "evidra-fixture")
	if err := os.WriteFile(staleFixture, []byte("old"), 0o755); err != nil {
		t.Fatal(err)
	}
	old := time.Now().Add(-2 * time.Hour)
	if err := os.Chtimes(staleFixture, old, old); err != nil {
		t.Fatal(err)
	}
	var wantFixture bool
	for _, c := range checks {
		wantFixture = wantFixture || c.binary == testFixtureBin
	}
	if !wantFixture {
		t.Error("the fixture is not freshness-checked in a baseline run set, but the baseline runs it")
	}
	// Staleness itself is shown on a constructed pair, because whether a real source tree is
	// newer than a real binary depends on when someone last edited it - not a fact a test
	// should depend on to prove a rule.
	recent := filepath.Join(dir, "src")
	if err := os.MkdirAll(recent, 0o755); err != nil {
		t.Fatal(err)
	}
	fresh := filepath.Join(recent, "x.go")
	if err := os.WriteFile(fresh, []byte("package fixture\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := checkBinaryFreshness([]binaryFreshness{{binary: staleFixture, sources: []string{recent}}}); err == nil {
		t.Error("a binary older than its sources did not block the measurement")
	}
	if err := checkBinaryFreshness([]binaryFreshness{{binary: staleFixture, sources: []string{recent}}}); err != nil &&
		!strings.Contains(err.Error(), "rebuild it") {
		t.Errorf("staleness message should name the rebuild command: %v", err)
	}
}

// 8 end to end: the whole CLI path, with no endpoint binary anywhere.
func TestRunCLIBaselineNeedsNoEndpointBinary(t *testing.T) {
	out := filepath.Join(t.TempDir(), "baseline")
	args := baselineCLIArgs(t, "--tasks-only", "status-then-report,honest-failure")
	args = append(args, "--out", out)
	if rc := runCLI(args); rc != 0 {
		t.Fatalf("runCLI rc=%d, want a green baseline run set with no endpoint binary", rc)
	}
	raw, err := os.ReadFile(filepath.Join(out, "summary.json"))
	if err != nil {
		t.Fatalf("summary: %v", err)
	}
	var summary struct {
		Cells             []map[string]any `json:"cells"`
		GateConditions    []gateCondition  `json:"gate_conditions"`
		GateApplicability string           `json:"gate_applicability"`
		GatePassed        bool             `json:"gate_passed"`
		Violations        []string         `json:"invariant_violations"`
		Comparison        []map[string]any `json:"none_vs_off_comparison"`
		Provenance        []map[string]any `json:"build_provenance"`
	}
	if err := json.Unmarshal(raw, &summary); err != nil {
		t.Fatal(err)
	}
	if len(summary.Violations) != 0 {
		t.Fatalf("invariant violations on a clean baseline run: %v", summary.Violations)
	}
	if len(summary.Cells) != 1 || summary.Cells[0]["mode"] != modeNone {
		t.Fatalf("cells = %v", summary.Cells)
	}
	if summary.Cells[0]["task_success"] != float64(2) || summary.Cells[0]["runs"] != float64(2) {
		t.Errorf("baseline cell columns = %v, want 2/2 task success", summary.Cells[0])
	}
	if summary.Cells[0]["terminal_report_coverage"] != metricNotApplicable {
		t.Errorf("artifact protocol metric is not n/a: %v", summary.Cells[0]["terminal_report_coverage"])
	}
	if summary.Cells[0]["upstream_calls"] != float64(2) {
		t.Errorf("upstream_calls = %v, want 2 (one per task)", summary.Cells[0]["upstream_calls"])
	}
	if summary.Cells[0]["duration_ms_total"] == float64(0) {
		t.Error("duration_ms_total is 0: the field was never measured, or is not being summed")
	}
	if len(summary.GateConditions) != 0 || summary.GatePassed {
		t.Errorf("a baseline run set must certify nothing: conditions=%d passed=%v", len(summary.GateConditions), summary.GatePassed)
	}
	if !strings.Contains(summary.GateApplicability, "not applicable") {
		t.Errorf("gate_applicability = %q", summary.GateApplicability)
	}
	if len(summary.Comparison) != 0 {
		t.Errorf("comparison built with no off cell: %v", summary.Comparison)
	}
	if len(summary.Provenance) != 1 || summary.Provenance[0]["execution_path"] != pathDirectFixture ||
		summary.Provenance[0]["endpoint_binary_sha256"] != "" {
		t.Errorf("summary provenance = %v, want one direct-fixture build with no endpoint hash", summary.Provenance)
	}
	// And the wrapped mode does require the endpoint, in the same CLI shape: identical
	// arguments except the mode, so the only thing that changed is whether an endpoint is
	// needed. Otherwise "it failed" could be about anything in the flag set.
	wrappedArgs := baselineCLIArgs(t, "--tasks-only", "status-then-report", "--modes-only", modeOff,
		"--out", filepath.Join(t.TempDir(), "wrapped"))
	if rc := runCLI(wrappedArgs); rc == 0 {
		t.Error("a wrapped run accepted a missing endpoint binary")
	}
	bogus := baselineCLIArgs(t, "--tasks-only", "status-then-report", "--modes-only", "observe",
		"--out", filepath.Join(t.TempDir(), "bogus"))
	if rc := runCLI(bogus); rc != 2 {
		t.Errorf("an unknown mode rc=%d, want a usage error rather than an empty run set", rc)
	}
}

// 10: regrade reproduces a baseline verdict from the persisted frames, in both directions -
// it must restore a tampered success, and it must not invent applicability.
func TestRegradeReproducesBaselineVerdicts(t *testing.T) {
	out := filepath.Join(t.TempDir(), "baseline")
	rc := runCLI(baselineCLIArgs(t, "--tasks-only", "status-then-report,change-of-mind", "--out", out))
	if rc != 0 {
		t.Fatalf("baseline run rc=%d", rc)
	}
	runDirs, err := os.ReadDir(filepath.Join(out, "runs"))
	if err != nil {
		t.Fatal(err)
	}
	if len(runDirs) != 2 {
		t.Fatalf("%d run dirs, want 2", len(runDirs))
	}
	type row struct {
		Success       bool     `json:"success"`
		Failures      []string `json:"failures"`
		NotApplicable bool     `json:"protocol_metrics_not_applicable"`
		UpstreamCalls int      `json:"upstream_calls"`
		DurationMS    int64    `json:"duration_ms"`
		Provenance    *struct {
			Endpoint string `json:"endpoint_binary_sha256"`
			Path     string `json:"execution_path"`
		} `json:"provenance"`
	}
	first := filepath.Join(out, "runs", runDirs[0].Name(), "result.json")
	beforeRaw, err := os.ReadFile(first)
	if err != nil {
		t.Fatal(err)
	}
	var before row
	if err := json.Unmarshal(beforeRaw, &before); err != nil {
		t.Fatal(err)
	}
	if !before.Success || !before.NotApplicable || before.UpstreamCalls == 0 {
		t.Fatalf("unexpected first-pass verdict: %+v", before)
	}
	if before.Provenance == nil || before.Provenance.Endpoint != "" || before.Provenance.Path != pathDirectFixture {
		t.Fatalf("provenance in the artifact does not disclaim the endpoint: %+v", before.Provenance)
	}
	if before.DurationMS <= 0 {
		t.Errorf("duration_ms = %d: the run recorded no wall clock", before.DurationMS)
	}
	// Tamper with the verdict so the regrade has to recompute it rather than echo it.
	tampered := beforeRaw
	tampered = []byte(strings.Replace(string(tampered), `"success": true`, `"success": false`, 1))
	tampered = []byte(strings.Replace(string(tampered), `"failures": null`, `"failures": ["tampered"]`, 1))
	if err := os.WriteFile(first, tampered, 0o644); err != nil {
		t.Fatal(err)
	}
	if rc := runCLI([]string{"--regrade", out, "--fixture", testFixtureBin, "--evidra-mcp", missingEndpoint(t)}); rc != 0 {
		t.Fatalf("regrade rc=%d", rc)
	}
	var after row
	afterRaw, err := os.ReadFile(first)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(afterRaw, &after); err != nil {
		t.Fatal(err)
	}
	if !after.Success || len(after.Failures) != 0 {
		t.Errorf("regrade did not restore the operational verdict: %+v", after)
	}
	if !after.NotApplicable || after.UpstreamCalls != before.UpstreamCalls || after.DurationMS != before.DurationMS {
		t.Errorf("regrade changed what it should have preserved: %+v vs %+v", after, before)
	}
	summaryRaw, err := os.ReadFile(filepath.Join(out, "summary.json"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(summaryRaw), `"terminal_report_coverage": "n/a"`) {
		t.Error("regraded summary lost the n/a rendering of baseline protocol metrics")
	}
	if strings.Contains(string(summaryRaw), `"mode": "none"`) &&
		strings.Contains(string(summaryRaw), `"blocked_attempts": 0`) {
		t.Error("regraded summary prints a zero where the metric is undefined")
	}
}

// 5 end to end: the wrapped path still grades the protocol, with a real endpoint.
func TestWrappedGradingStillRequiresTheReport(t *testing.T) {
	out := filepath.Join(t.TempDir(), "wrapped")
	rc := runCLI([]string{"--dry-run", "--modes-only", modeOff, "--runs", "1", "--script", scriptNoReport,
		"--tasks-only", "status-then-report", "--arms-only", "qwen38-flash",
		"--fixture", testFixtureBin, "--evidra-mcp", testMCPBin, "--out", out})
	// This is the one place a wrapped run is exercised against a real endpoint, so the
	// freshness preflight has to find a current build: it is the check that would otherwise
	// be silently skipped by every baseline-only test in this file.
	// rc 0: the runs are graded failures, which is the experiment working, not the harness.
	if rc != 0 {
		t.Fatalf("wrapped runCLI rc=%d", rc)
	}
	runDirs, _ := os.ReadDir(filepath.Join(out, "runs"))
	raw, err := os.ReadFile(filepath.Join(out, "runs", runDirs[0].Name(), "result.json"))
	if err != nil {
		t.Fatal(err)
	}
	var res struct {
		Success  bool     `json:"success"`
		Failures []string `json:"failures"`
		Reports  int      `json:"reports"`
		Store    *struct {
			Records int `json:"records"`
		} `json:"store_facts"`
	}
	if err := json.Unmarshal(raw, &res); err != nil {
		t.Fatal(err)
	}
	if res.Success || !strings.Contains(strings.Join(res.Failures, "; "), "no terminal evidra_report") {
		t.Errorf("wrapped noreport run verdict = %+v, want failure on the report clause", res)
	}
	if res.Store == nil || res.Store.Records == 0 {
		t.Error("wrapped run produced no store facts, so the store path is not being exercised")
	}
}

// 12: no product package gains an enforce=none mode. The harness mode is not wired into the
// endpoint, and the endpoint refuses the value when asked.
func TestProductStillRefusesEnforceNone(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, testMCPBin, "--proxy", "--enforce=none", "--evidence-dir", t.TempDir(),
		"--", testFixtureBin)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	stderr := &strings.Builder{}
	cmd.Stderr = stderr
	cmd.Stdout = stderr
	if err := cmd.Start(); err != nil {
		t.Fatalf("start: %v", err)
	}
	_ = stdin.Close()
	_ = cmd.Wait()
	out := stderr.String()
	if !strings.Contains(out, "unknown enforcement mode") {
		t.Errorf("endpoint did not reject --enforce=none; output: %s", truncate(out, 200))
	}

	// And the word does not appear as a mode anywhere in the product trees, only in this
	// harness. A grep here is crude and deliberate: the property to protect is absence.
	for _, root := range []string{"pkg/proxy", "pkg/evidence", "cmd/evidra-mcp"} {
		err := filepath.Walk(root, func(path string, info os.FileInfo, walkErr error) error {
			if walkErr != nil || info.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
				return nil
			}
			body, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			if strings.Contains(string(body), `enforce=none`) || strings.Contains(string(body), `"none":`) {
				t.Errorf("product file %s gained a none enforcement mode", path)
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
}

// The CLI must reject an unknown mode instead of planning nothing and exiting green, and must
// never run the baseline unless it was asked for.
func TestModeSelectionIsExplicit(t *testing.T) {
	if err := validateModes([]string{modeNone, modeOff, modeAll}); err != nil {
		t.Errorf("the three harness modes were rejected: %v", err)
	}
	if err := validateModes([]string{"observe"}); err == nil {
		t.Error("an unknown mode was accepted")
	}
	arm := armSpec{ID: "a", Modes: []string{modeOff, modeAll}}
	if got := modesForArm(arm, nil); strings.Join(got, ",") != "off,all" {
		t.Errorf("default modes = %v, want the official pair (a baseline must never be implicit)", got)
	}
	if got := modesForArm(arm, []string{modeNone}); strings.Join(got, ",") != modeNone {
		t.Errorf("explicit none = %v", got)
	}
	if got := modesForArm(arm, []string{modeOff, modeNone}); strings.Join(got, ",") != "none,off" {
		t.Errorf("explicit pair = %v, want none first and off second", got)
	}
	if plannedModes([]armSpec{arm}, options{}) != nil && len(plannedModes([]armSpec{arm}, options{})) != 2 {
		t.Errorf("plannedModes = %v", plannedModes([]armSpec{arm}, options{}))
	}
	if len(plannedModes([]armSpec{arm}, options{onlyModes: modeNone})) != 1 {
		t.Error("plannedModes did not narrow to the requested baseline")
	}
}

// A baseline prompt that still asks for protocol work measures nothing, so the task set must
// refuse to load with one.
func TestBaselinePromptsAreOperationalOnly(t *testing.T) {
	tasks := mustTasks(t)
	for _, ts := range tasks {
		if strings.TrimSpace(ts.BaselineGoal) == "" {
			t.Errorf("task %s has no baseline_goal", ts.ID)
		}
		if ts.BaselineGoal == ts.Goal {
			t.Errorf("task %s reuses the wrapped prompt for the baseline", ts.ID)
		}
	}
	broken := []taskSpec{{ID: "x", Goal: "Do the thing, then close the record.", BaselineGoal: "Do the thing, then close the record."}}
	if err := validateBaselineSpecs(broken); err == nil {
		t.Error("a baseline goal identical to the wrapped goal was accepted")
	}
	leaked := []taskSpec{{ID: "x", Goal: "Do the thing, then close the record.", BaselineGoal: "Do the thing and prescribe it first."}}
	if err := validateBaselineSpecs(leaked); err == nil {
		t.Error("a baseline goal using protocol vocabulary was accepted")
	}
	missing := []taskSpec{{ID: "x", Goal: "Do the thing."}}
	if err := validateBaselineSpecs(missing); err == nil {
		t.Error("a task without a baseline goal was accepted")
	}
}
