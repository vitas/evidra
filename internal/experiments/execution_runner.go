package experiments

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"
)

func RunExecution(ctx context.Context, opts ExecutionRunOptions, stdout, stderr io.Writer) error {
	opts = withExecutionDefaults(opts)
	if err := validateExecutionOptions(opts); err != nil {
		return err
	}

	promptInfo, err := resolvePromptInfo(opts.PromptFile, opts.PromptVersion)
	if err != nil {
		return err
	}

	if opts.CleanOutDir {
		if err := ensureDirClean(opts.OutDir); err != nil {
			return err
		}
	}
	if err := os.MkdirAll(opts.OutDir, 0o755); err != nil {
		return fmt.Errorf("mkdir out dir: %w", err)
	}
	summaryPath := filepath.Join(opts.OutDir, "summary.jsonl")
	if err := os.WriteFile(summaryPath, nil, 0o644); err != nil {
		return err
	}

	scenarios, err := loadExecutionScenarios(opts.ScenariosDir, opts.ScenarioFilter, opts.MaxScenarios)
	if err != nil {
		return err
	}
	agent, err := newExecutionAgent(opts.Agent)
	if err != nil {
		return err
	}
	useDryStatus := opts.DryRun || agent.Name() == "dry-run"

	fmt.Fprintf(stdout,
		"run-agent-execution-experiments: selected_scenarios=%d repeats=%d out_dir=%s mode=%s\n",
		len(scenarios), opts.Repeats, opts.OutDir, opts.Mode,
	)

	counters := RunCounters{}
	statusCounts := map[string]int{}
	runStamp := runStampNow()

	for _, scenario := range scenarios {
		for repeat := 1; repeat <= opts.Repeats; repeat++ {
			counters.Total++
			runID := fmt.Sprintf("%s-%s-%s-r%d", runStamp, safeModelID(opts.ModelID), scenario.ScenarioID, repeat)
			oneStatus, evalPass, err := runExecutionScenario(ctx, opts, promptInfo, agent, scenario, repeat, runID, summaryPath, useDryStatus)
			if err != nil {
				return err
			}
			statusCounts[oneStatus]++
			applyStatus(&counters, oneStatus)
			if evalPass {
				counters.EvalPass++
			} else {
				counters.EvalFail++
			}
			fmt.Fprintf(stdout, "run-agent-execution-experiments: run_id=%s status=%s pass=%v result=%s\n", runID, oneStatus, evalPass, filepath.Join(opts.OutDir, runID, "result.json"))
		}
	}

	printExecutionSummary(stdout, counters, summaryPath, statusCounts)
	return nil
}

func withExecutionDefaults(opts ExecutionRunOptions) ExecutionRunOptions {
	if opts.Provider == "" {
		opts.Provider = "unknown"
	}
	if opts.PromptFile == "" {
		opts.PromptFile = DefaultPromptFile
	}
	if opts.ScenariosDir == "" {
		opts.ScenariosDir = DefaultExecutionScenarios
	}
	if opts.Mode == "" {
		opts.Mode = "local-mcp"
	}
	if opts.Repeats <= 0 {
		opts.Repeats = 1
	}
	if opts.TimeoutSeconds <= 0 {
		opts.TimeoutSeconds = 600
	}
	if opts.OutDir == "" {
		opts.OutDir = filepath.Join(DefaultArtifactOutRoot, runStampNow()+DefaultExecutionOutSuffix)
	}
	if opts.Agent == "" {
		if opts.DryRun {
			opts.Agent = "dry-run"
		}
	}
	return opts
}

func validateExecutionOptions(opts ExecutionRunOptions) error {
	if opts.ModelID == "" {
		return errors.New("--model-id is required")
	}
	if opts.Agent == "" {
		return errors.New("--agent is required")
	}
	if opts.Repeats < 1 {
		return errors.New("--repeats must be >= 1")
	}
	if opts.TimeoutSeconds < 1 {
		return errors.New("--timeout-seconds must be >= 1")
	}
	info, err := os.Stat(opts.ScenariosDir)
	if err != nil || !info.IsDir() {
		return fmt.Errorf("scenarios dir not found: %s", opts.ScenariosDir)
	}
	if _, err := os.Stat(opts.PromptFile); err != nil {
		return fmt.Errorf("prompt file not found: %s", opts.PromptFile)
	}
	return nil
}

func runExecutionScenario(
	parent context.Context,
	opts ExecutionRunOptions,
	prompt PromptInfo,
	agent ExecutionAgent,
	scenario ExecutionScenario,
	repeat int,
	runID string,
	summaryPath string,
	forceDry bool,
) (status string, evalPass bool, err error) {
	runDir, stdoutPath, stderrPath, outputPath, rawPath, resultPath := runPaths(opts.OutDir, runID)
	start := time.Now().UTC()
	status = "success"
	agentExitCode := 0

	result, status, agentExitCode := executeExecutionAgent(parent, opts, prompt, agent, scenario, repeat, runID, rawPath, outputPath, forceDry)
	output := ensureObjectOutput(result.Output, map[string]any{
		"prescribe_ok": false,
		"report_ok":    false,
		"status":       status,
		"agent_exit":   agentExitCode,
	})

	if err := writeRunArtifacts(runDir, stdoutPath, stderrPath, outputPath, rawPath, result.StdoutLog, result.StderrLog, output, result.RawStream); err != nil {
		return "", false, err
	}

	prescribeOK, _ := output["prescribe_ok"].(bool)
	reportOK, _ := output["report_ok"].(bool)
	observedLevel, observedTags := extractRiskPrediction(output)
	observedExit := readOptionalInt(output, "exit_code")
	eval := evaluateExecution(prescribeOK, reportOK, scenario.ExpectedExitCode, observedExit, scenario.ExpectedRiskLevel, observedLevel, scenario.ExpectedRiskTags, observedTags)

	end := time.Now().UTC()
	resultObj := buildExecutionResult(resultPath, runID, opts, prompt, scenario, runDir, stdoutPath, stderrPath, outputPath, rawPath, status, agentExitCode, repeat, start, end, output, eval)
	if err := writeJSONFile(resultPath, resultObj); err != nil {
		return "", false, err
	}

	summaryRow := map[string]any{
		"run_id":      runID,
		"scenario_id": scenario.ScenarioID,
		"status":      status,
		"pass":        eval.Pass,
		"result_json": resultPath,
	}
	if err := writeJSONL(summaryPath, summaryRow); err != nil {
		return "", false, err
	}
	return status, eval.Pass, nil
}

func executeExecutionAgent(
	parent context.Context,
	opts ExecutionRunOptions,
	prompt PromptInfo,
	agent ExecutionAgent,
	scenario ExecutionScenario,
	repeat int,
	runID, rawPath, outputPath string,
	forceDry bool,
) (ExecutionAgentResult, string, int) {
	if forceDry {
		res, _ := (&dryRunAgent{}).RunExecution(parent, ExecutionAgentRequest{})
		return res, "dry_run", 0
	}

	ctx, cancel := context.WithTimeout(parent, time.Duration(opts.TimeoutSeconds)*time.Second)
	defer cancel()
	req := ExecutionAgentRequest{
		Scenario:        scenario,
		ModelID:         opts.ModelID,
		Provider:        opts.Provider,
		Prompt:          prompt,
		TimeoutSeconds:  opts.TimeoutSeconds,
		RunID:           runID,
		RepeatIndex:     repeat,
		RawStreamOut:    rawPath,
		AgentOutputPath: outputPath,
	}
	res, err := agent.RunExecution(ctx, req)
	if err == nil {
		return res, "success", 0
	}
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(ctx.Err(), context.DeadlineExceeded) {
		return res, "timeout", 124
	}
	if res.StderrLog == "" {
		res.StderrLog = err.Error() + "\n"
	}
	return res, "failure", 1
}

func buildExecutionResult(
	resultPath, runID string,
	opts ExecutionRunOptions,
	prompt PromptInfo,
	scenario ExecutionScenario,
	runDir, stdoutPath, stderrPath, outputPath, rawPath, status string,
	agentExitCode, repeat int,
	start, end time.Time,
	agentResult map[string]any,
	eval ExecutionEvaluation,
) map[string]any {
	return map[string]any{
		"schema_version":   ExecutionResultSchema,
		"run_id":           runID,
		"started_at":       start.Format(time.RFC3339),
		"finished_at":      end.Format(time.RFC3339),
		"duration_seconds": int(end.Sub(start).Seconds()),
		"mode":             opts.Mode,
		"agent_result":     agentResult,
		"evaluation":       eval,
		"model": map[string]any{
			"id":                      opts.ModelID,
			"provider":                opts.Provider,
			"prompt_version":          prompt.Version,
			"prompt_file":             prompt.File,
			"prompt_contract_version": prompt.ContractVersion,
			"repeat_index":            repeat,
		},
		"scenario": map[string]any{
			"id":                  scenario.ScenarioID,
			"category":            scenario.Category,
			"difficulty":          scenario.Difficulty,
			"tool":                scenario.Tool,
			"operation":           scenario.Operation,
			"artifact_path":       scenario.ArtifactPath,
			"execute_cmd":         scenario.ExecuteCommand,
			"expected_exit_code":  scenario.ExpectedExitCode,
			"expected_risk_level": scenario.ExpectedRiskLevel,
			"expected_risk_tags":  scenario.ExpectedRiskTags,
			"source_json_path":    scenario.SourceJSONPath,
		},
		"execution": map[string]any{
			"agent_cmd":       "agent:" + opts.Agent,
			"timeout_seconds": opts.TimeoutSeconds,
			"status":          status,
			"agent_exit_code": agentExitCode,
		},
		"artifacts": map[string]any{
			"run_dir":          runDir,
			"stdout_log":       stdoutPath,
			"stderr_log":       stderrPath,
			"agent_output":     outputPath,
			"agent_raw_stream": rawPath,
			"result_json":      resultPath,
		},
	}
}

func readOptionalInt(m map[string]any, key string) *int {
	v, ok := m[key]
	if !ok || v == nil {
		return nil
	}
	switch n := v.(type) {
	case float64:
		i := int(n)
		return &i
	case int:
		i := n
		return &i
	case json.Number:
		i64, err := n.Int64()
		if err != nil {
			return nil
		}
		i := int(i64)
		return &i
	default:
		return nil
	}
}

func printExecutionSummary(stdout io.Writer, counters RunCounters, summaryPath string, statusCounts map[string]int) {
	fmt.Fprintln(stdout)
	fmt.Fprintln(stdout, "run-agent-execution-experiments: summary")
	fmt.Fprintf(stdout, "  total:       %d\n", counters.Total)
	fmt.Fprintf(stdout, "  success:     %d\n", counters.Success)
	fmt.Fprintf(stdout, "  failure:     %d\n", counters.Failure)
	fmt.Fprintf(stdout, "  timeout:     %d\n", counters.Timeout)
	fmt.Fprintf(stdout, "  dry_run:     %d\n", counters.DryRun)
	fmt.Fprintf(stdout, "  eval_pass:   %d\n", counters.EvalPass)
	fmt.Fprintf(stdout, "  eval_fail:   %d\n", counters.EvalFail)
	fmt.Fprintf(stdout, "  index:       %s\n", summaryPath)
	fmt.Fprintln(stdout)
	fmt.Fprintln(stdout, "run-agent-execution-experiments: top statuses from summary")
	writeStatusCounts(stdout, statusCounts)
}
