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
	fs.StringVar(&o.onlyModes, "modes-only", "", "comma-separated modes: all,off")
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
	if o.regrade != "" {
		if err := regrade(o.regrade, tasks); err != nil {
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

	if err := preflight(ctx, &o, arms); err != nil {
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

	runs := collectRuns(results)
	writeSummary(o.outDir, arms, tasks, runs, overheads, o)
	return verdict(runs)
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

func modesFor(a armSpec, only string) []string {
	want := splitList(only)
	var out []string
	for _, m := range a.Modes {
		if len(want) > 0 && !inList(m, want) {
			continue
		}
		out = append(out, m)
	}
	return out
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
func preflight(ctx context.Context, o *options, arms []armSpec) error {
	if _, err := os.Stat(o.mcpBin); err != nil {
		return fmt.Errorf("endpoint binary %s: %w", o.mcpBin, err)
	}
	if _, err := os.Stat(o.fixtureBin); err != nil {
		return fmt.Errorf("fixture binary %s: %w", o.fixtureBin, err)
	}
	// Measuring a stale build produces a run set that looks like data about the code
	// under review and is actually data about whatever was compiled last. The Go tests
	// build their own endpoint binary, so they can pass while this runner measures an
	// older one; see cmd/evidra-gatea/freshness.go.
	if !o.allowStaleBuild {
		if err := checkBinaryFreshness([]binaryFreshness{
			{binary: o.mcpBin, sources: []string{"cmd/evidra-mcp", "pkg/proxy", "pkg/evidence", "pkg/report"}},
			{binary: o.fixtureBin, sources: []string{"cmd/evidra-fixture"}},
		}); err != nil {
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
	res := runResult{Arm: arm.ID, ArmModelID: arm.APIModelID, Mode: mode, Task: task.ID, Run: runNo}
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

	epArgs := []string{"--proxy", "--enforce=" + mode, "--evidence-dir", filepath.Join(dir, "evidence"), "--", o.fixtureBin}
	epArgs = append(epArgs, task.FixtureArgs...)
	ep, err := startMCP(runCtx, o.mcpBin, epArgs...)
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

	llmTools := make([]llmTool, 0, len(tools))
	nameMap := map[string]string{}
	for _, t := range tools {
		safe := sanitizeToolName(t.Name)
		nameMap[safe] = t.Name
		llmTools = append(llmTools, toolFor(safe, t.Description, t.InputSchema))
	}

	var ag agent
	if o.dryRun {
		ag = &scriptedAgent{plan: scriptedPlan(task, o.script)}
	} else {
		key, err := loadKey(arm.KeyRef, "")
		if err != nil {
			res.InvalidRun = "key: " + err.Error()
			return res
		}
		ag = newModelAgent(arm, key, o.maxTokens)
	}

	tr, invalid := driveAgent(runCtx, ag, ep, llmTools, nameMap, task, rec)
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
	res.Unprescribed = tr.unprescribedCalls()
	facts, err := readStoreFacts(dir)
	if err != nil {
		res.InvalidRun = "evidence store unreadable: " + err.Error()
	}
	applyStoreFacts(&res, facts, mode)
	if facts != nil {
		res.Unprescribed = facts.Unprescribed
	}
	res.FirstUpstreamPrescribed = tr.firstUpstreamPrescribed
	res.LatePrescribe = tr.latePrescribe
	res.Failures = task.evaluate(tr)
	res.ProtocolOnlyFails = task.protocolOnlyFails(tr)
	res.ProtocolOnly = len(res.ProtocolOnlyFails) == 0 && res.InvalidRun == ""
	res.Success = len(res.Failures) == 0 && res.InvalidRun == ""
	rec("transcript", tr)
	rec("verdict", res)

	raw, _ := json.MarshalIndent(res, "", "  ")
	_ = os.WriteFile(filepath.Join(dir, "result.json"), raw, 0o644)
	return res
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
func capToolText(text string) string {
	const cap = 8192
	if len(text) <= cap {
		return text
	}
	return text[:cap] + fmt.Sprintf("\n[… %d bytes truncated by the client; the full result was delivered to the recorder]", len(text)-cap)
}

// writeSummary rolls the runs up into per-cell metrics, applies §10's
// thresholds, and writes the artifact set.
func writeSummary(dir string, arms []armSpec, tasks []taskSpec, runs []runResult, overheads map[string]int, o options) {
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
		"gate_passed":                  gatePassed(conds),
		"git_commit":                   gitRevision(),
		"invalid_run_reasons":          invalidReasons(runs),
	}
	raw, _ := json.MarshalIndent(summary, "", "  ")
	_ = os.WriteFile(filepath.Join(dir, "summary.json"), raw, 0o644)

	fmt.Printf("\ncell                       mode  runs  success  protoOnly  report  blocked  recovery\n")
	for _, c := range cells {
		fmt.Printf("%-26s %-5s %4d %7d %9d %7d   %7d   %s\n", c.Arm, c.Mode, c.Runs, c.TaskSuccess,
			c.ProtocolOnlySuccess, c.TerminalReportCover, c.BlockedAttempts, c.RecoveryRate)
	}
	for _, g := range conds {
		if g.Status != "pass" {
			fmt.Printf("  [%-13s] %s / %s: %s (want %s)\n", g.Status, g.Arm, g.Mode, g.Name, g.Required)
		}
	}
	fmt.Printf("\ngate_passed=%v  artifacts: %s\n", gatePassed(conds), dir)
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

func verdict(runs []runResult) int {
	for _, r := range runs {
		if r.InvalidRun != "" {
			return 3
		}
	}
	return 0
}

// driveAgent runs the turn loop: it asks the agent for the next action,
// executes every tool call it returns against the endpoint, and records what
// happened. It reports an invalid_run reason rather than touching the result, so
// a gateway failure cannot be mistaken for a model's decision.
func driveAgent(ctx context.Context, ag agent, ep *mcpClient, llmTools []llmTool,
	nameMap map[string]string, task taskSpec, rec func(string, any)) (*transcript, string) {
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
			content := capToolText(ev.Text)
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
	proto := "-"
	if res.ProtocolOnly {
		proto = "P"
	}
	fmt.Printf("  %s%s %-14s %-4s %-26s run%d turns=%2d blocked=%d %s\n",
		status, proto, p.arm.ID, p.mode, p.task.ID, p.run, res.Turns, res.Blocked,
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
