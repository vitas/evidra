package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	_ "embed"
)

//go:embed tasks.json
var embeddedTasks []byte

//go:embed arms.json
var embeddedArms []byte

// defaultTasksJSON / defaultArmsJSON keep spec.go independent of the embed
// directives.
var (
	defaultTasksJSON = embeddedTasks
	defaultArmsJSON  = embeddedArms
)

type options struct {
	outDir     string
	tasksPath  string
	armsPath   string
	onlyArms   string
	onlyTasks  string
	onlyModes  string
	runs       int
	limit      int
	parallel   int
	maxTokens  int
	wallClock  time.Duration
	dryRun     bool
	regrade    string
	script     string
	calibrate  bool
	mcpBin     string
	fixtureBin string
	// allowStaleBuild opts out of the freshness preflight, which is the right call
	// exactly when the point of the run is to compare against an older build.
	allowStaleBuild bool
}

func main() { os.Exit(runCLI(os.Args[1:])) }

func runCLI(args []string) int {
	fs := flag.NewFlagSet("evidra-gatea", flag.ContinueOnError)
	var o options
	fs.StringVar(&o.outDir, "out", "", "artifact directory (default output/gatea/<utc timestamp>)")
	fs.StringVar(&o.tasksPath, "tasks", "", "task spec JSON (default: embedded tasks.json)")
	fs.StringVar(&o.armsPath, "arms", "", "arm spec JSON (default: embedded arms.json)")
	fs.StringVar(&o.onlyArms, "arms-only", "", "comma-separated arm ids to run")
	fs.StringVar(&o.onlyTasks, "tasks-only", "", "comma-separated task ids to run")
	fs.StringVar(&o.onlyModes, "modes-only", "", "comma-separated modes: none,off,all (none is a harness-only no-Evidra baseline, never a product mode)")
	fs.IntVar(&o.runs, "runs", 2, "runs per task/mode/arm cell")
	fs.IntVar(&o.limit, "limit", 0, "stop after N runs (pilots)")
	fs.IntVar(&o.parallel, "parallel", 2, "concurrent runs")
	fs.IntVar(&o.maxTokens, "max-tokens", 4096, "per-turn completion budget (§10: >= 2048)")
	fs.DurationVar(&o.wallClock, "wall-clock", 8*time.Minute, "per-run wall clock")
	fs.BoolVar(&o.dryRun, "dry-run", false, "drive the endpoint with a scripted protocol-correct agent instead of a model")
	fs.StringVar(&o.script, "script", scriptCompliant, "which scripted sequence --dry-run replays: compliant|late|noreport")
	fs.BoolVar(&o.calibrate, "calibrate", false, "measure protocol definition overhead per arm with a differential probe")
	fs.StringVar(&o.mcpBin, "evidra-mcp", "bin/evidra-mcp", "path to the merged endpoint binary")
	fs.StringVar(&o.fixtureBin, "fixture", "bin/evidra-fixture", "path to the fixture binary")
	fs.BoolVar(&o.allowStaleBuild, "allow-stale-build", false, "run against bin/ artifacts newer than nothing, without checking they are up to date (comparison runs only)")
	fs.StringVar(&o.regrade, "regrade", "", "recompute verdicts in an existing artifact directory from its transcripts")
	if err := fs.Parse(args); err != nil {
		return 2
	}

	tasks, err := loadTasks(o.tasksPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	arms, err := loadArms(o.armsPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	arms = filterArms(arms, o.onlyArms, o.dryRun)
	tasks = filterTasks(tasks, o.onlyTasks)
	if len(arms) == 0 || len(tasks) == 0 {
		fmt.Fprintln(os.Stderr, "nothing to run after filtering")
		return 1
	}
	// Validated here rather than by intersecting with arm.Modes: a mode that silently
	// matched nothing would plan zero runs and exit as if the experiment had been run.
	if err := validateModes(splitList(o.onlyModes)); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 2
	}
	if o.regrade != "" {
		if err := regrade(o, tasks); err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 1
		}
		return 0
	}
	if err := prepareOutput(&o); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := preflight(ctx, &o, arms, plannedModes(arms, o)); err != nil {
		fmt.Fprintf(os.Stderr, "preflight: %v\n", err)
		return 1
	}
	overheads := map[string]int{}
	if o.calibrate && !o.dryRun {
		overheads, err = calibrateOverhead(ctx, arms)
		if err != nil {
			fmt.Fprintf(os.Stderr, "calibration: %v\n", err)
			return 1
		}
	}

	queue := buildQueue(arms, tasks, o)
	runs := runQueue(ctx, o, queue)
	violations := writeSummary(o.outDir, arms, tasks, runs, overheads, o)
	return verdict(runs, violations)
}

// runQueue executes the plan and returns the runs worth aggregating. Extracted from runCLI so
// that the flag handling, the plan and the execution each stay readable; a runner whose entry
// function has outgrown its budget tends to hide the one branch that decides a verdict.
func runQueue(ctx context.Context, o options, queue []planRun) []runResult {
	fmt.Printf("gatea: %d runs planned → %s\n", len(queue), o.outDir)
	results := make([]runResult, len(queue))
	sem := make(chan struct{}, max(1, o.parallel))
	var wg sync.WaitGroup
	for i, p := range queue {
		wg.Add(1)
		go func(i int, p planRun) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			res := runOne(ctx, o, p.arm, p.mode, p.task, p.run, o.outDir)
			results[i] = res
			status := "ok  "
			switch {
			case res.InvalidRun != "":
				status = "INVL"
			case !res.Success:
				status = "FAIL"
			}
			printRun(p, res, status)
		}(i, p)
	}
	wg.Wait()
	return collectRuns(results)
}

// collectRuns keeps only runs that produced an artifact or a stated reason for
// having none, so a partially built queue cannot dilute a cell's denominator.
func collectRuns(results []runResult) []runResult {
	out := results[:0]
	for _, r := range results {
		if r.TranscriptPath != "" || r.InvalidRun != "" {
			out = append(out, r)
		}
	}
	return out
}

func costRank(a armSpec) int {
	if a.CostBasis == "free" {
		return 0
	}
	if a.Role == "cheap" || a.Role == "cheap-capable" {
		return 1
	}
	return 2
}

func filterArms(arms []armSpec, only string, dryRun bool) []armSpec {
	var out []armSpec
	want := splitList(only)
	for _, a := range arms {
		if len(want) > 0 && !inList(a.ID, want) {
			continue
		}
		// Optional arms are explicit-only unless a dry run wants every cell.
		if a.Optional && !inList(a.ID, want) && !dryRun {
			continue
		}
		out = append(out, a)
	}
	if len(want) > 0 && len(out) == 0 {
		return nil
	}
	return out
}

func filterTasks(tasks []taskSpec, only string) []taskSpec {
	want := splitList(only)
	if len(want) == 0 {
		return tasks
	}
	var out []taskSpec
	for _, t := range tasks {
		if inList(t.ID, want) {
			out = append(out, t)
		}
	}
	return out
}

// modesFor is the arm-level mode filter. `none` is added only when explicitly requested:
// it is a property of the comparison, not of the model, and an unconditional entry would
// double every experiment that did not ask for a baseline.
func modesFor(a armSpec, only string) []string {
	return modesForArm(a, splitList(only))
}

func splitList(v string) []string {
	if strings.TrimSpace(v) == "" {
		return nil
	}
	parts := strings.Split(v, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

// preflight resolves keys and probes every arm once before any task runs.
//
// The endpoint binary is required only if some mode in this run set starts it. Checking it
// unconditionally would make the no-Evidra baseline depend on the binary it exists to exclude
// - and, worse, a stale bin/evidra-mcp would quietly gate whether a baseline could be
// measured.
func preflight(ctx context.Context, o *options, arms []armSpec, modes []string) error {
	if needsEndpointBinary(modes) {
		if _, err := os.Stat(o.mcpBin); err != nil {
			return fmt.Errorf("endpoint binary %s: %w", o.mcpBin, err)
		}
	}
	if _, err := os.Stat(o.fixtureBin); err != nil {
		return fmt.Errorf("fixture binary %s: %w", o.fixtureBin, err)
	}
	// Measuring a stale build produces a run set that looks like data about the code
	// under review and is actually data about whatever was compiled last. The Go tests
	// build their own endpoint binary, so they can pass while this runner measures an
	// older one; see cmd/evidra-gatea/freshness.go.
	if !o.allowStaleBuild {
		if err := checkBinaryFreshness(freshnessChecks(modes, o.mcpBin, o.fixtureBin, os.Args[0])); err != nil {
			return fmt.Errorf("preflight: %w (or pass --allow-stale-build to measure this build deliberately)", err)
		}
	}
	if o.dryRun {
		return nil
	}
	for _, a := range arms {
		key, err := loadKey(a.KeyRef, "")
		if err != nil {
			return fmt.Errorf("arm %s: %w", a.ID, err)
		}
		if err := preflightProbe(ctx, a, key); err != nil {
			return fmt.Errorf("arm %s: %w", a.ID, err)
		}
		fmt.Printf("preflight: %s (%s @ %s) ok\n", a.ID, a.APIModelID, a.Provider)
	}
	return nil
}

// calibrateOverhead measures the prompt-token cost of Evidra's two protocol tool
// definitions by asking the same arm with and without them. This is a property
// of the arm, not of a run, and it is deliberately not mixed with the per-run
// traffic count.
func calibrateOverhead(ctx context.Context, arms []armSpec) (map[string]int, error) {
	out := map[string]int{}
	upstreamOnly := []llmTool{toolFor("get_status", "Status.", json.RawMessage(`{"type":"object"}`))}
	withProtocol := append([]llmTool{
		toolFor("evidra_prescribe", "Open an operation.", json.RawMessage(`{"type":"object","properties":{"objective":{"type":"string"}},"required":["objective"]}`)),
		toolFor("evidra_report", "Close an operation.", json.RawMessage(`{"type":"object"}`)),
	}, upstreamOnly...)
	for _, a := range arms {
		if a.CostBasis != "free" && a.Role == "strong" {
			continue
		}
		key, err := loadKey(a.KeyRef, "")
		if err != nil {
			return nil, err
		}
		ag := newModelAgent(a, key, 256)
		msgs := []llmMessage{{Role: "user", Content: "Say nothing and call no tool."}}
		with, err := ag.next(ctx, msgs, withProtocol)
		if err != nil {
			return nil, fmt.Errorf("arm %s calibration: %w", a.ID, err)
		}
		without, err := ag.next(ctx, msgs, upstreamOnly)
		if err != nil {
			return nil, fmt.Errorf("arm %s calibration baseline: %w", a.ID, err)
		}
		diff := with.Prompt - without.Prompt
		if diff < 0 {
			diff = 0
		}
		out[a.ID] = diff
		fmt.Printf("protocol definition overhead: %s = %d prompt tokens\n", a.ID, diff)
	}
	return out, nil
}

// runOne executes a single Gate A run and writes its artifacts.
func runOne(ctx context.Context, o options, arm armSpec, mode string, task taskSpec, runNo int, outDir string) runResult {
	runStart := time.Now()
	baseline := isBaseline(mode)
	res := runResult{Arm: arm.ID, ArmModelID: arm.APIModelID, Mode: mode, Task: task.ID, Run: runNo}
	// A baseline run has no protocol surface, so its protocol-shaped fields are undefined
	// rather than zero. Recorded per row because result.json is read on its own.
	res.ProtocolNotApplicable = baseline
	res.Provenance = captureProvenance(o.mcpBin, o.fixtureBin, arm.APIModelID, executionPath(mode))
	dirName := fmt.Sprintf("%s__%s__%s__run%d", arm.ID, mode, task.ID, runNo)
	dir := filepath.Join(outDir, "runs", dirName)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		res.InvalidRun = "artifact dir: " + err.Error()
		return res
	}
	res.TranscriptPath = filepath.Join(dir, "transcript.jsonl")
	f, err := os.Create(res.TranscriptPath)
	if err != nil {
		res.InvalidRun = "artifact create: " + err.Error()
		return res
	}
	defer func() { _ = f.Close() }()
	rec := func(kind string, payload any) {
		raw, _ := json.Marshal(map[string]any{"t": time.Now().UTC().Format(time.RFC3339Nano), "kind": kind, "payload": payload})
		_, _ = f.Write(append(raw, '\n'))
	}

	runCtx, cancel := context.WithTimeout(ctx, o.wallClock)
	defer cancel()

	// The two topologies this harness can build. Wrapped: agent → evidra-mcp --proxy →
	// fixture, with an evidence directory under the run dir. Baseline: agent → fixture, no
	// endpoint process, no evidence directory, and nothing Evidra-shaped in the tool list.
	var (
		childBin string
		epArgs   []string
	)
	if baseline {
		childBin, epArgs = o.fixtureBin, append([]string{}, task.FixtureArgs...)
	} else {
		epArgs = []string{"--proxy", "--enforce=" + mode, "--evidence-dir", filepath.Join(dir, "evidence"), "--", o.fixtureBin}
		epArgs = append(epArgs, task.FixtureArgs...)
		childBin = o.mcpBin
	}
	ep, err := startMCP(runCtx, childBin, epArgs...)
	if err != nil {
		res.InvalidRun = "endpoint start: " + err.Error()
		return res
	}
	defer ep.close()

	if _, err := ep.initialize(runCtx); err != nil {
		res.InvalidRun = classifyEndpointExit(err, ep)
		return res
	}
	tools, err := ep.listAllTools(runCtx)
	if err != nil {
		res.InvalidRun = classifyEndpointExit(err, ep)
		return res
	}
	rec("tools", tools)
	if baseline {
		// Structural guarantee, not a hope: if an endpoint were somehow in this path, the
		// protocol tools would appear in the list, and the "baseline" would be measuring a
		// wrapped run under a name that promises it is not one.
		for _, t := range tools {
			if strings.HasPrefix(t.Name, "evidra_") {
				res.InvalidRun = fmt.Sprintf("baseline run was served the protocol tool %q: no endpoint may participate in mode %q", t.Name, modeNone)
				return res
			}
		}
	}

	llmTools := make([]llmTool, 0, len(tools))
	nameMap := map[string]string{}
	for _, t := range tools {
		safe := sanitizeToolName(t.Name)
		nameMap[safe] = t.Name
		llmTools = append(llmTools, toolFor(safe, t.Description, t.InputSchema))
	}

	// The prompt is the task's own words for the mode. A baseline run is given the
	// operational goal, never the protocol one: "then close the record" cannot be satisfied
	// without a record, and asking for it anyway would score the absence of a tool as the
	// agent's failure.
	promptTask := task
	if baseline {
		promptTask.Goal = task.baselinePrompt()
	}
	rec("goal", map[string]any{"mode": mode, "execution_path": executionPath(mode), "text": promptTask.Goal})

	var ag agent
	if o.dryRun {
		ag = &scriptedAgent{plan: scriptedPlan(promptTask, o.script, baseline)}
	} else {
		key, err := loadKey(arm.KeyRef, "")
		if err != nil {
			res.InvalidRun = "key: " + err.Error()
			return res
		}
		ag = newModelAgent(arm, key, o.maxTokens)
	}

	tr, invalid := driveAgent(runCtx, ag, ep, llmTools, nameMap, promptTask, !baseline, rec)
	if invalid != "" {
		res.InvalidRun = invalid
	}

	res.Turns = tr.Turns
	res.Blocked = tr.BlockedCount
	res.Prescribes = tr.Prescribes
	res.Reports = len(tr.Reports)
	res.Replacements = tr.Replacements
	res.PromptTokens = tr.PromptTokens
	res.OutputTokens = tr.OutputTokens
	res.ReasonTokens = tr.ReasonTokens
	res.UpstreamCalls, res.UpstreamErrors = countUpstream(tr)
	if tr.Prescribes != 0 || len(tr.Reports) != 0 || tr.BlockedCount != 0 {
		if baseline {
			// Structurally impossible: no protocol tool was offered, so protocol traffic means
			// something reached the run that the mode promises is not there.
			res.InvalidRun = "baseline run recorded protocol traffic; the cell cannot be a no-Evidra measurement"
		}
	}
	facts, factsErr := readStoreFacts(dir)
	finalizeRun(&res, tr, &task, facts, factsErr, mode, false)
	res.DurationMS = time.Since(runStart).Milliseconds()
	rec("transcript", tr)
	rec("verdict", res)

	raw, _ := json.MarshalIndent(res, "", "  ")
	_ = os.WriteFile(filepath.Join(dir, "result.json"), raw, 0o644)
	return res
}

// gradeRun turns a transcript into the verdicts for one run. It is shared with --regrade on
// purpose: a verdict that can only be produced inside a live run cannot be challenged later,
// and the two paths drifting apart is how an artifact set ends up with numbers that cannot be
// recomputed from its own frames.
//
// For wrapped modes the predicate set is exactly what it was before the baseline existed:
// operational clauses plus the report and replacement clauses. For `none` only the operational
// clauses apply, because the rest have no observable referent.
func gradeRun(res *runResult, ts *taskSpec, tr *transcript, mode string) {
	if isBaseline(mode) {
		res.Failures = ts.operationalFails(tr)
		res.ProtocolOnlyFails = nil
		res.ProtocolOnly = false
		res.Success = len(res.Failures) == 0 && res.InvalidRun == ""
		return
	}
	res.ProtocolNotApplicable = false
	res.Failures = append(ts.operationalFails(tr), ts.protocolFails(tr)...)
	res.ProtocolOnlyFails = ts.protocolOnlyFails(tr)
	res.ProtocolOnly = len(res.ProtocolOnlyFails) == 0 && res.InvalidRun == ""
	res.Success = len(res.Failures) == 0 && res.InvalidRun == ""
}

// finalizeRun turns one transcript plus the recorder's own account of the run into verdicts.
// Both the live path and --regrade call it.
//
// The ordering here is a correctness rule, not style. Until the baseline mode was added, the
// live path graded the transcript *after* applying the store facts, and grading assigned
// res.Failures rather than appending to it: every enforcement hole and every invalid chain the
// recorder reported was therefore written into a field and immediately overwritten, so the
// "product defect, not agent failure" clause the store exists to assert could not fail a run.
// It never fired on the official Gate A set (no row has an unprescribed execution or an
// invalid chain, checked across all archived run sets), so no recorded verdict changes; the
// check is only real now. Sharing this function between both paths is what keeps the two
// accounts from drifting apart again.
func finalizeRun(res *runResult, tr *transcript, ts *taskSpec, facts *storeFacts, factsErr error, mode string, regrade bool) {
	baseline := isBaseline(mode)
	res.ProtocolNotApplicable = baseline
	if baseline {
		// These are not computed for a baseline run, because none of them is observable:
		// an execution with no open record is not a violation when no record can be open.
		// The zero values stand with the flag above them, and the rollup renders "n/a" so
		// no reader can average them into a rate.
		res.Unprescribed = 0
		res.FirstUpstreamPrescribed = false
		res.LatePrescribe = false
		if facts != nil {
			res.InvalidRun = fmt.Sprintf("baseline run produced an evidence store at %s: an endpoint participated", facts.Dir)
		}
		// Named for what it is. "transcript" would imply the transcript-derived counts are
		// the runner's best available account of protocol behaviour; in a baseline run they
		// are not a weak account of anything, because there was no protocol to account for.
		res.CountsFrom = "no_store"
	} else {
		res.Unprescribed = tr.unprescribedCalls()
		res.FirstUpstreamPrescribed = tr.firstUpstreamPrescribed
		res.LatePrescribe = tr.latePrescribe
	}

	gradeRun(res, ts, tr, mode)

	if factsErr != nil {
		if regrade {
			// A regrade that cannot read the store is not an invalid run - the run already
			// happened - but its verdict can no longer be trusted, which is a failure.
			res.Failures = append(res.Failures, "evidence store unreadable: "+factsErr.Error())
			res.Success = len(res.Failures) == 0 && res.InvalidRun == ""
			return
		}
		res.InvalidRun = "evidence store unreadable: " + factsErr.Error()
		res.Success = false
		return
	}
	if baseline {
		return
	}
	// The signed record outranks the runner's inference, and may add clauses (an enforcement
	// hole, a broken chain) that no transcript predicate contains.
	before := len(res.Failures)
	applyStoreFacts(res, facts, mode)
	if facts != nil {
		res.Unprescribed = facts.Unprescribed
	}
	if len(res.Failures) != before {
		res.Success = len(res.Failures) == 0 && res.InvalidRun == ""
	}
}

// execute runs one tool call against the endpoint and updates the transcript.
func execute(ctx context.Context, ep *mcpClient, tr *transcript, name string, args map[string]any) toolEvent {
	ev := toolEvent{Name: name, Args: args, OpOpen: tr.openOp}
	switch name {
	case "evidra_prescribe", "evidra_report":
		ev.Local = true
	default:
		tr.upstreamCalls++
		if tr.upstreamCalls == 1 {
			// Voluntary coverage asks only one question: was a record open
			// before the first real action (§10).
			tr.firstUpstreamPrescribed = tr.openOp != ""
		}
	}
	res, err := ep.callTool(ctx, name, args)
	if err != nil {
		ev.IsError = true
		ev.Text = err.Error()
		tr.add(ev)
		return ev
	}
	ev.Text = res.Text
	ev.IsError = res.IsError

	switch name {
	case "evidra_prescribe":
		var p struct {
			OperationID string `json:"operation_id"`
			State       string `json:"state"`
			Error       string `json:"error"`
		}
		_ = json.Unmarshal([]byte(res.Text), &p)
		if p.Error == "" {
			tr.Prescribes++
			if boolArg(args, "abandon_and_replace") || boolArg(args, "replace_open") {
				tr.Replacements++
			}
			if boolArg(args, "continue_current") {
				tr.ContinueUses++
			}
			if p.OperationID != "" {
				tr.Operations = append(tr.Operations, opRecord{ID: p.OperationID, Objective: strArg(args, "objective")})
				if tr.upstreamCalls > 0 && tr.openOp == "" {
					tr.latePrescribe = true
				}
				tr.openOp = p.OperationID
			}
		}
	case "evidra_report":
		var r reportEvent
		_ = json.Unmarshal([]byte(res.Text), &r)
		// The claim is what the agent submitted; the response only confirms
		// which state the session moved to. Reading status/outcome back out of
		// the response would grade Evidra's echo instead of the agent.
		if v, ok := args["status"].(string); ok {
			r.Status = v
		}
		if v, ok := args["outcome"].(string); ok {
			r.Outcome = v
		}
		r.IsError = res.IsError
		if r.OperationID == "" {
			r.OperationID = tr.openOp
		}
		tr.Reports = append(tr.Reports, r)
		if !res.IsError && (r.State == "reported" || r.State == "already_reported") {
			tr.openOp = ""
		}
	default:
		if strings.Contains(res.Text, "no_open_operation") {
			ev.Blocked = true
			ev.IsError = true
			tr.BlockedCount++
		}
	}
	tr.add(ev)
	return ev
}

func strArg(args map[string]any, key string) string {
	v, _ := args[key].(string)
	return v
}

func boolArg(args map[string]any, key string) bool {
	v, ok := args[key].(bool)
	return ok && v
}

// unprescribedExecutions counts upstream calls that really executed while no
// operation was open: §34's derived class, inferred here from the protocol
// traffic the runner itself observed. The v2 store of §59 step 4 makes the same
// statement authoritative from recorded events.
func (t *transcript) unprescribedCalls() int {
	n := 0
	for _, ev := range t.Events {
		if !ev.Local && !ev.Blocked && !ev.IsError && ev.OpOpen == "" {
			n++
		}
	}
	return n
}

func classifyEndpointExit(err error, ep *mcpClient) string {
	text := ep.stderrText()
	switch {
	case strings.Contains(text, "reserved tool"):
		return "endpoint refused reserved tool name: " + truncate(text, 200)
	case strings.Contains(text, "unknown enforcement mode"):
		return "bad enforce mode: " + truncate(text, 200)
	case strings.Contains(text, "credit"), strings.Contains(text, "balance"):
		return "gateway credit/balance: " + truncate(text, 200)
	default:
		return "endpoint: " + err.Error() + " | stderr: " + truncate(text, 300)
	}
}

// systemContract deliberately says nothing about the protocol. Everything the
// agent knows about prescribe/report comes from the endpoint's own instructions
// and tool descriptions, the same surface a real MCP client sees. The alternative
// — restating the rules in the harness — would measure the runner's prompt
// engineering and inflate voluntary coverage in observe-only mode.
const systemContract = `You complete the user's task using the tools available to you.
Follow the instructions the server publishes and what its tool descriptions say.
Report what actually happened rather than what you hoped happened.
When the task is finished, answer with one short plain-text sentence and no tool call.`

// capToolText keeps an agent's context bounded the way a real client does, while
// saying so explicitly: silently dropping the tail would make a large-result task
// measure the runner's truncation instead of the agent's handling of a big record.
func capToolText(text string, recorderPresent bool) string {
	const cap = 8192
	if len(text) <= cap {
		return text
	}
	// The cap is byte-for-byte identical in every mode - that is the point of the rule, so
	// the large-result task measures the agent's handling of a big payload rather than the
	// runner's truncation. Only the sentence after it differs, because it states where the
	// remainder went, and in a baseline run nothing retained it. Telling a baseline agent
	// about a recorder would put Evidra's vocabulary in the arm built to exclude it.
	where := "not retained by the client"
	if recorderPresent {
		where = "delivered to the recorder"
	}
	return text[:cap] + fmt.Sprintf("\n[… %d bytes truncated by the client; the full result was %s]", len(text)-cap, where)
}

// writeSummary rolls the runs up into per-cell metrics, applies §10's
// thresholds, and writes the artifact set.
// writeSummary rolls the graded runs up into cells, refuses the result if its own
// analytics are inconsistent, and returns the violations so the exit code can carry them.
func writeSummary(dir string, arms []armSpec, tasks []taskSpec, runs []runResult, overheads map[string]int, o options) []string {
	byCell := map[string][]runResult{}
	for _, r := range runs {
		byCell[r.Arm+"/"+r.Mode] = append(byCell[r.Arm+"/"+r.Mode], r)
	}
	var cells []cellMetrics
	keys := make([]string, 0, len(byCell))
	for k := range byCell {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		cells = append(cells, rollupCell(byCell[k]))
	}
	roleOf := map[string]string{}
	for _, a := range arms {
		roleOf[a.ID] = a.Role
	}
	conds := judgeGate(cells, roleOf)
	// Analytics invariants are checked before the gate is judged, and a violation fails
	// the run: an unbounded or double-counted metric would otherwise reach the artifact
	// as a gate result computed from numbers that cannot exist.
	violations := checkRunInvariants(runs, cells, byCell)
	summary := map[string]any{
		"generated_at":                 time.Now().UTC().Format(time.RFC3339),
		"plan":                         "docs/system-design/vnext-mcp-recorder.md §10",
		"task_set":                     "docs/system-design/gate-a-tasks.md",
		"arms":                         arms,
		"tasks":                        len(tasks),
		"runs_planned_per_cell":        o.runs,
		"dry_run":                      o.dryRun,
		"protocol_definition_overhead": overheads,
		"cells":                        cells,
		"gate_conditions":              conds,
		// The baseline is descriptive and must not be able to certify anything, so the
		// summary states in one line which gate the numbers above do and do not speak to.
		"gate_applicability":      gateApplicability(cells),
		"none_vs_off_comparison":  baselineComparisons(cells),
		"comparison_metrics_note": "none vs off is read from the operational columns only; protocol metrics are n/a in a baseline cell and no harm threshold was predeclared",
		"gate_passed":             gatePassed(conds),
		"git_commit":              gitRevision(),
		"invalid_run_reasons":     invalidReasons(runs),
		"invariant_violations":    violations,
		"build_provenance":        provenanceOf(runs),
	}
	raw, _ := json.MarshalIndent(summary, "", "  ")
	_ = os.WriteFile(filepath.Join(dir, "summary.json"), raw, 0o644)

	fmt.Printf("\ncell                       mode  runs  success protoOnly  report  blocked recovery                             mean-turns  upstream(err)  mean dur\n")
	for _, c := range cells {
		fmt.Printf("%s\n", c.tableRow())
	}
	for _, v := range violations {
		fmt.Printf("  [INVARIANT  ] %s\n", v)
	}
	if len(violations) > 0 {
		fmt.Printf("\n%d analytics invariant violation(s): the numbers above are not usable as evidence.\n", len(violations))
	}
	for _, g := range conds {
		if g.Status != "pass" {
			fmt.Printf("  [%-13s] %s / %s: %s (want %s)\n", g.Status, g.Arm, g.Mode, g.Name, g.Required)
		}
	}
	for _, line := range comparisonLines(cells) {
		fmt.Println(line)
	}
	for _, line := range gateApplicabilityLines(cells, conds) {
		fmt.Println(line)
	}
	fmt.Printf("\ngate_passed=%v  artifacts: %s\n", gatePassed(conds), dir)
	return violations
}

func invalidReasons(runs []runResult) map[string]int {
	out := map[string]int{}
	for _, r := range runs {
		if r.InvalidRun != "" {
			out[truncate(r.InvalidRun, 80)]++
		}
	}
	return out
}

func gitRevision() string {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, "git", "rev-parse", "HEAD").Output()
	if err != nil {
		return "unknown"
	}
	return strings.TrimSpace(string(out))
}

func verdict(runs []runResult, violations []string) int {
	if len(violations) > 0 {
		// Distinct from 3 (an invalid run): the harness itself produced analytics that
		// cannot be trusted, which is the more serious failure.
		return 4
	}
	for _, r := range runs {
		if r.InvalidRun != "" {
			return 3
		}
	}
	return 0
}

// provenanceOf collapses the per-run provenance records into what a reader of
// summary.json needs: one entry when the whole run set came from one build, and every
// distinct build plus how many runs it produced when it did not.
func provenanceOf(runs []runResult) []map[string]any {
	type agg struct {
		rec  *binaryProvenance
		runs int
	}
	var order []string
	by := map[string]*agg{}
	for i := range runs {
		p := runs[i].Provenance
		if p == nil {
			continue
		}
		key := p.buildKey()
		if by[key] == nil {
			by[key] = &agg{rec: p}
			order = append(order, key)
		}
		by[key].runs++
	}
	out := make([]map[string]any, 0, len(order))
	for _, key := range order {
		p := by[key].rec
		out = append(out, map[string]any{
			"source_revision":        p.SourceRevision,
			"source_dirty":           p.SourceDirty,
			"endpoint_binary_sha256": p.EndpointSHA256,
			"execution_path":         p.ExecutionPath,
			"fixture_binary_sha256":  p.FixtureSHA256,
			"runner_binary_sha256":   p.RunnerSHA256,
			"models":                 modelsIn(runs, key),
			"runs_from_this_build":   by[key].runs,
		})
	}
	return out
}

func modelsIn(runs []runResult, key string) []string {
	var models []string
	seen := map[string]bool{}
	for _, r := range runs {
		if r.Provenance == nil || r.Provenance.buildKey() != key || r.Provenance.ModelID == "" {
			continue
		}
		if !seen[r.Provenance.ModelID] {
			seen[r.Provenance.ModelID] = true
			models = append(models, r.Provenance.ModelID)
		}
	}
	return models
}

// driveAgent runs the turn loop: it asks the agent for the next action,
// executes every tool call it returns against the endpoint, and records what
// happened. It reports an invalid_run reason rather than touching the result, so
// a gateway failure cannot be mistaken for a model's decision.
func driveAgent(ctx context.Context, ag agent, ep *mcpClient, llmTools []llmTool,
	nameMap map[string]string, task taskSpec, recorderPresent bool, rec func(string, any)) (*transcript, string) {
	tr := &transcript{}
	// The protocol contract reaches the agent the way it reaches every real MCP
	// client: verbatim from initialize.instructions, never paraphrased here.
	serverInstructions := ""
	if v, ok := ep.header["instructions"].(string); ok {
		serverInstructions = v
	}
	rec("instructions", serverInstructions)
	messages := []llmMessage{{Role: "system", Content: systemContract + "\n\nServer instructions:\n" + serverInstructions},
		{Role: "user", Content: task.Goal}}
	maxTurns := task.MaxTurns

	for turn := 1; turn <= maxTurns; turn++ {
		tr.Turns = turn
		out, err := ag.next(ctx, messages, llmTools)
		if err != nil {
			// The error belongs in the artifact even though the run is thrown
			// away: invalid_run rows are the evidence that a cell was
			// contaminated by account state, not by an agent.
			rec("agent_error", map[string]any{"error": err.Error(), "turn": turn})
			var infra *infraFailure
			if errors.As(err, &infra) {
				return tr, infra.reason
			}
			return tr, "model transport: " + err.Error()
		}
		tr.PromptTokens += out.Prompt
		tr.OutputTokens += out.Output
		tr.ReasonTokens += out.Reason
		tr.FinishReasons = append(tr.FinishReasons, out.Finish)
		rec("completion", map[string]any{"turn": turn, "finish": out.Finish, "text": out.Text,
			"calls": out.Calls, "prompt": out.Prompt, "output": out.Output, "reason": out.Reason})

		if len(out.Calls) == 0 {
			// The run ends here: no reminder is sent, because terminal report
			// coverage has to measure whether the agent closed the record
			// because the protocol asked it to.
			rec("final_text", out.Text)
			// No reminder is sent. Terminal report coverage has to measure
			// whether the agent closes the record because the protocol said so,
			// not because the harness nagged it into the answer.
			break
		}
		messages = append(messages, llmMessage{Role: "assistant", Content: out.Text, ToolCalls: out.Calls})
		for _, call := range out.Calls {
			orig := nameMap[call.Function.Name]
			if orig == "" {
				orig = call.Function.Name
			}
			var args map[string]any
			if err := json.Unmarshal([]byte(call.Function.Arguments), &args); err != nil {
				args = nil
			}
			// The scripted agent cannot know an operation id in advance; the
			// runner fills it in so a replay does not need model access.
			for k, v := range args {
				if str, ok := v.(string); ok && str == "{{open}}" {
					args[k] = tr.openOp
				}
			}
			ev := execute(ctx, ep, tr, orig, args)
			rec("tool", ev)
			content := capToolText(ev.Text, recorderPresent)
			if ev.IsError {
				content = "ERROR: " + ev.Text
			}
			messages = append(messages, llmMessage{Role: "tool", Content: content, ToolCallID: call.ID, Name: call.Function.Name})
		}
	}

	return tr, ""
}

// planRun is one cell element: an arm, a mode, a task, and a repeat number.
type planRun struct {
	arm  armSpec
	mode string
	task taskSpec
	run  int
}

// buildQueue enumerates the cells and orders them cheapest-first, so an early
// protocol redesign never spends paid tokens on runs that will be discarded.
func buildQueue(arms []armSpec, tasks []taskSpec, o options) []planRun {
	var queue []planRun
	for _, a := range arms {
		for _, mode := range modesFor(a, o.onlyModes) {
			for _, t := range tasks {
				for r := 1; r <= o.runs; r++ {
					queue = append(queue, planRun{arm: a, mode: mode, task: t, run: r})
					if o.limit > 0 && len(queue) >= o.limit {
						break
					}
				}
				if o.limit > 0 && len(queue) >= o.limit {
					break
				}
			}
		}
	}
	// Cheapest first: free arms before metered ones, so a protocol redesign
	// discovered in the early cells never spends paid tokens on discarded runs.
	sort.SliceStable(queue, func(i, j int) bool {
		ci, cj := costRank(queue[i].arm), costRank(queue[j].arm)
		if ci != cj {
			return ci < cj
		}
		if queue[i].mode != queue[j].mode {
			return queue[i].mode == "all"
		}
		return queue[i].task.ID < queue[j].task.ID
	})
	return queue
}

// printRun gives one line per run: a verdict, whether the protocol clauses held
// separately from the task, and the first reason if not.
func printRun(p planRun, res runResult, status string) {
	// A baseline row prints "n/a" where a wrapped row prints a protocol count, for the same
	// reason the rollup does: "blocked=0" in a run with no enforcer is not an observation.
	var proto, blocked string
	if isBaseline(p.mode) {
		proto, blocked = "-", metricNotApplicable
	} else {
		proto, blocked = "-", fmt.Sprintf("%d", res.Blocked)
		if res.ProtocolOnly {
			proto = "P"
		}
	}
	fmt.Printf("  %s%s %-14s %-4s %-26s run%d turns=%2d blocked=%s dur=%5dms %s\n",
		status, proto, p.arm.ID, p.mode, p.task.ID, p.run, res.Turns, blocked, res.DurationMS,
		truncate(strings.Join(res.Failures, "; "), 60))
}

// prepareOutput allocates the artifact directory and records the task set that
// actually ran. Writing tasks.used.json is not decoration: verdict rules change
// as a result set is read, and a summary without the predicates that produced it
// cannot be re-argued by anyone else.
func prepareOutput(o *options) error {
	if o.outDir == "" {
		o.outDir = filepath.Join("output", "gatea", time.Now().UTC().Format("20060102T150405Z"))
	}
	if err := os.MkdirAll(o.outDir, 0o700); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(o.outDir, "tasks.used.json"), defaultTasksJSON, 0o600)
}
