package experiments

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
)

type mcpKubectlAgent struct{}

func (a *mcpKubectlAgent) Name() string { return "mcp-kubectl" }

func (a *mcpKubectlAgent) RunExecution(ctx context.Context, req ExecutionAgentRequest) (ExecutionAgentResult, error) {
	rt, err := initMCPRuntime(ctx, req)
	if err != nil {
		return ExecutionAgentResult{}, err
	}

	actor := buildMCPActor(req)
	prescribeBody, callErr := rt.prescribe(ctx, req, actor)
	if callErr != nil {
		return callErr.result, callErr.err
	}
	prescriptionID := readString(prescribeBody, "prescription_id")
	if prescriptionID == "" {
		return rt.missingPrescriptionID(), errors.New("missing prescription_id")
	}

	riskLevel, riskTags := extractPrescribeRisk(prescribeBody)
	cmdExitCode := runShellCommand(ctx, req.Scenario.ExecuteCommand)

	reportBody, callErr := rt.report(ctx, prescriptionID, cmdExitCode, actor, req, riskLevel, riskTags)
	if callErr != nil {
		return callErr.result, callErr.err
	}

	reportOK := readBool(reportBody, "ok")
	output := buildExecutionOutput(req, reportBody, prescriptionID, cmdExitCode, riskLevel, riskTags, reportOK)
	if !reportOK {
		return rt.reportNotOK(output), errors.New("report returned ok=false")
	}
	return rt.success(output), nil
}

type mcpRuntime struct {
	inspectorBin string
	mcpConfig    string
	mcpServer    string
	envLabel     string
	req          ExecutionAgentRequest
	stdout       strings.Builder
	stderr       strings.Builder
	rawEvents    []string
}

type mcpCallError struct {
	result ExecutionAgentResult
	err    error
}

func initMCPRuntime(ctx context.Context, req ExecutionAgentRequest) (*mcpRuntime, error) {
	rt := &mcpRuntime{
		inspectorBin: emptyTo(os.Getenv("EVIDRA_MCP_INSPECTOR_BIN"), "npx"),
		mcpConfig:    emptyTo(os.Getenv("EVIDRA_MCP_CONFIG"), filepath.Join("tests", "inspector", "mcp-config.json")),
		mcpServer:    emptyTo(os.Getenv("EVIDRA_MCP_SERVER"), "evidra"),
		envLabel:     strings.TrimSpace(os.Getenv("EVIDRA_ENVIRONMENT")),
		req:          req,
	}
	if err := rt.validate(ctx); err != nil {
		return nil, err
	}
	return rt, nil
}

func (rt *mcpRuntime) validate(ctx context.Context) error {
	if err := requireCommand(rt.inspectorBin); err != nil {
		return err
	}
	if err := requireCommand("bash"); err != nil {
		return err
	}
	if err := requireCommand("kubectl"); err != nil {
		return err
	}
	if _, err := os.Stat(rt.req.Scenario.ArtifactPath); err != nil {
		return fmt.Errorf("artifact not found: %s", rt.req.Scenario.ArtifactPath)
	}
	if _, err := os.Stat(rt.mcpConfig); err != nil {
		return fmt.Errorf("MCP config not found: %s", rt.mcpConfig)
	}
	if err := ensureEvidraMCPBinary(ctx); err != nil {
		return err
	}
	if strings.TrimSpace(os.Getenv("EVIDRA_SIGNING_MODE")) == "" {
		_ = os.Setenv("EVIDRA_SIGNING_MODE", "optional") // best-effort: os.Setenv only fails on invalid key
	}
	return nil
}

func buildMCPActor(req ExecutionAgentRequest) map[string]any {
	actorID := emptyTo(os.Getenv("EVIDRA_ACTOR_ID"), req.ModelID)
	if actorID == "" {
		actorID = "execution-agent"
	}
	return map[string]any{
		"type":          emptyTo(os.Getenv("EVIDRA_ACTOR_TYPE"), "agent"),
		"id":            actorID,
		"origin":        emptyTo(os.Getenv("EVIDRA_ACTOR_ORIGIN"), "execution-runner"),
		"skill_version": req.Prompt.ContractVersion,
	}
}

func (rt *mcpRuntime) prescribe(ctx context.Context, req ExecutionAgentRequest, actor map[string]any) (map[string]any, *mcpCallError) {
	artifactRaw, err := os.ReadFile(req.Scenario.ArtifactPath)
	if err != nil {
		return nil, &mcpCallError{err: fmt.Errorf("read artifact: %w", err)}
	}
	args := map[string]any{
		"tool":         req.Scenario.Tool,
		"operation":    req.Scenario.Operation,
		"raw_artifact": string(artifactRaw),
		"actor":        actor,
	}
	raw, err := rt.call(ctx, "prescribe", args)
	if err != nil {
		return nil, &mcpCallError{result: rt.prescribeCallFailed(), err: err}
	}
	body := extractInspectorBody(raw)
	if !readBool(body, "ok") {
		return nil, &mcpCallError{result: rt.prescribeNotOK(body), err: errors.New("prescribe returned ok=false")}
	}
	return body, nil
}

func (rt *mcpRuntime) report(
	ctx context.Context,
	prescriptionID string,
	cmdExitCode int,
	actor map[string]any,
	req ExecutionAgentRequest,
	riskLevel string,
	riskTags []string,
) (map[string]any, *mcpCallError) {
	args := map[string]any{
		"prescription_id": prescriptionID,
		"exit_code":       cmdExitCode,
		"actor":           actor,
	}
	raw, err := rt.call(ctx, "report", args)
	if err != nil {
		return nil, &mcpCallError{
			result: rt.reportCallFailed(cmdExitCode, prescriptionID, riskLevel, riskTags),
			err:    err,
		}
	}
	return extractInspectorBody(raw), nil
}

func (rt *mcpRuntime) call(ctx context.Context, phase string, args map[string]any) (string, error) {
	raw, errOut, err := callInspectorTool(ctx, rt.inspectorBin, rt.mcpConfig, rt.mcpServer, rt.envLabel, phase, args)
	rt.stderr.WriteString(errOut)
	if err != nil {
		return "", err
	}
	rt.stdout.WriteString(raw)
	rt.rawEvents = append(rt.rawEvents, encodeRawEvent(phase, raw))
	return raw, nil
}

func (rt *mcpRuntime) prescribeCallFailed() ExecutionAgentResult {
	return ExecutionAgentResult{
		Output: map[string]any{
			"prescribe_ok":  false,
			"report_ok":     false,
			"tool":          rt.req.Scenario.Tool,
			"operation":     rt.req.Scenario.Operation,
			"artifact_path": rt.req.Scenario.ArtifactPath,
			"execute_cmd":   rt.req.Scenario.ExecuteCommand,
			"error_phase":   "prescribe",
		},
		StdoutLog: rt.stdout.String(),
		StderrLog: "agent-cmd-mcp-kubectl: FAIL prescribe call failed\n" + rt.stderr.String(),
		RawStream: strings.Join(rt.rawEvents, "\n"),
	}
}

func (rt *mcpRuntime) prescribeNotOK(body map[string]any) ExecutionAgentResult {
	return ExecutionAgentResult{
		Output: map[string]any{
			"prescribe_ok":  false,
			"report_ok":     false,
			"tool":          rt.req.Scenario.Tool,
			"operation":     rt.req.Scenario.Operation,
			"artifact_path": rt.req.Scenario.ArtifactPath,
			"execute_cmd":   rt.req.Scenario.ExecuteCommand,
			"error_phase":   "prescribe",
			"response":      body,
		},
		StdoutLog: rt.stdout.String(),
		StderrLog: "agent-cmd-mcp-kubectl: FAIL prescribe returned ok=false\n",
		RawStream: strings.Join(rt.rawEvents, "\n"),
	}
}

func (rt *mcpRuntime) missingPrescriptionID() ExecutionAgentResult {
	return ExecutionAgentResult{
		Output: map[string]any{
			"prescribe_ok": true,
			"report_ok":    false,
			"error_phase":  "prescribe",
		},
		StdoutLog: rt.stdout.String(),
		StderrLog: "agent-cmd-mcp-kubectl: FAIL prescribe response missing prescription_id\n",
		RawStream: strings.Join(rt.rawEvents, "\n"),
	}
}

func (rt *mcpRuntime) reportCallFailed(cmdExitCode int, prescriptionID, riskLevel string, riskTags []string) ExecutionAgentResult {
	return ExecutionAgentResult{
		Output: map[string]any{
			"prescribe_ok":    true,
			"report_ok":       false,
			"exit_code":       cmdExitCode,
			"prescription_id": prescriptionID,
			"risk_level":      riskLevel,
			"risk_tags":       riskTags,
			"error_phase":     "report",
		},
		StdoutLog: rt.stdout.String(),
		StderrLog: "agent-cmd-mcp-kubectl: FAIL report call failed\n" + rt.stderr.String(),
		RawStream: strings.Join(rt.rawEvents, "\n"),
	}
}

func buildExecutionOutput(req ExecutionAgentRequest, reportBody map[string]any, prescriptionID string, cmdExitCode int, riskLevel string, riskTags []string, reportOK bool) map[string]any {
	return map[string]any{
		"prescribe_ok":    true,
		"report_ok":       reportOK,
		"exit_code":       cmdExitCode,
		"prescription_id": prescriptionID,
		"report_id":       readString(reportBody, "report_id"),
		"risk_level":      riskLevel,
		"risk_tags":       riskTags,
		"tool":            req.Scenario.Tool,
		"operation":       req.Scenario.Operation,
		"artifact_path":   req.Scenario.ArtifactPath,
		"execute_cmd":     req.Scenario.ExecuteCommand,
		"report_response": reportBody,
	}
}

func extractPrescribeRisk(prescribeBody map[string]any) (string, []string) {
	riskLevel := strings.TrimSpace(readString(prescribeBody, "effective_risk"))
	if riskLevel == "" {
		riskLevel = strings.TrimSpace(readString(prescribeBody, "risk_level"))
	}

	riskTags := readStringSlice(prescribeBody, "risk_tags")
	if len(riskTags) > 0 {
		return emptyTo(riskLevel, "unknown"), normalizeTags(riskTags)
	}

	rawInputs, ok := prescribeBody["risk_inputs"].([]any)
	if !ok {
		return emptyTo(riskLevel, "unknown"), nil
	}

	for _, raw := range rawInputs {
		input, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		if strings.TrimSpace(readString(input, "source")) != "evidra/native" {
			continue
		}
		if riskLevel == "" {
			riskLevel = strings.TrimSpace(readString(input, "risk_level"))
		}
		return emptyTo(riskLevel, "unknown"), normalizeTags(readStringSlice(input, "risk_tags"))
	}

	return emptyTo(riskLevel, "unknown"), nil
}

func (rt *mcpRuntime) reportNotOK(output map[string]any) ExecutionAgentResult {
	return ExecutionAgentResult{
		Output:    output,
		StdoutLog: rt.stdout.String(),
		StderrLog: "agent-cmd-mcp-kubectl: FAIL report returned ok=false\n",
		RawStream: strings.Join(rt.rawEvents, "\n"),
	}
}

func (rt *mcpRuntime) success(output map[string]any) ExecutionAgentResult {
	return ExecutionAgentResult{
		Output:    output,
		StdoutLog: rt.stdout.String(),
		StderrLog: rt.stderr.String(),
		RawStream: strings.Join(rt.rawEvents, "\n"),
	}
}

func requireCommand(name string) error {
	if _, err := exec.LookPath(name); err != nil {
		return fmt.Errorf("missing required command: %s", name)
	}
	return nil
}

func ensureEvidraMCPBinary(ctx context.Context) error {
	if _, err := exec.LookPath("evidra-mcp"); err == nil {
		return nil
	}
	localPath := filepath.Join("bin", "evidra-mcp")
	if _, err := os.Stat(localPath); err == nil {
		path := os.Getenv("PATH")
		return os.Setenv("PATH", filepath.Join(".", "bin")+string(os.PathListSeparator)+path)
	}
	if _, err := exec.LookPath("go"); err != nil {
		return errors.New("evidra-mcp not found and go is unavailable to build it")
	}
	cmd := exec.CommandContext(ctx, "go", "build", "-o", localPath, "./cmd/evidra-mcp")
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("build evidra-mcp: %v: %s", err, stderr.String())
	}
	path := os.Getenv("PATH")
	return os.Setenv("PATH", filepath.Join(".", "bin")+string(os.PathListSeparator)+path)
}

func callInspectorTool(
	ctx context.Context,
	inspectorBin, configPath, serverName, envLabel, tool string,
	args map[string]any,
) (stdout string, stderr string, err error) {
	cmdArgs := []string{"-y", "@modelcontextprotocol/inspector", "--cli", "--config", configPath, "--server", serverName}
	if envLabel != "" {
		cmdArgs = append(cmdArgs, "-e", "EVIDRA_ENVIRONMENT="+envLabel)
	}
	cmdArgs = append(cmdArgs, "--method", "tools/call", "--tool-name", tool)

	keys := make([]string, 0, len(args))
	for key := range args {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		val := formatInspectorArg(args[key])
		cmdArgs = append(cmdArgs, "--tool-arg", key+"="+val)
	}

	cmd := exec.CommandContext(ctx, inspectorBin, cmdArgs...)
	var outBuf, errBuf bytes.Buffer
	cmd.Stdout = &outBuf
	cmd.Stderr = &errBuf
	runErr := cmd.Run()
	if runErr != nil {
		if errors.Is(runErr, context.DeadlineExceeded) || errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return outBuf.String(), errBuf.String(), context.DeadlineExceeded
		}
		return outBuf.String(), errBuf.String(), runErr
	}
	return outBuf.String(), errBuf.String(), nil
}

func formatInspectorArg(v any) string {
	switch t := v.(type) {
	case string:
		return t
	default:
		b, err := json.Marshal(t)
		if err != nil {
			return fmt.Sprintf("%v", t)
		}
		return string(b)
	}
}

func extractInspectorBody(raw string) map[string]any {
	obj := extractJSONObject(raw)
	if len(obj) == 0 {
		return map[string]any{}
	}
	if structured, ok := obj["structuredContent"].(map[string]any); ok {
		return structured
	}
	content, ok := obj["content"].([]any)
	if ok && len(content) > 0 {
		first, ok := content[0].(map[string]any)
		if ok {
			text := readString(first, "text")
			parsed := extractJSONObject(text)
			if len(parsed) > 0 {
				return parsed
			}
		}
	}
	return obj
}

func readBool(m map[string]any, key string) bool {
	v, ok := m[key]
	if !ok || v == nil {
		return false
	}
	b, ok := v.(bool)
	return ok && b
}

func encodeRawEvent(phase, payload string) string {
	line, err := json.Marshal(map[string]any{"phase": phase, "payload": payload})
	if err != nil {
		return `{"phase":"` + phase + `","payload":""}`
	}
	return string(line)
}

func runShellCommand(ctx context.Context, command string) int {
	cmd := exec.CommandContext(ctx, "bash", "-lc", command)
	err := cmd.Run()
	if err == nil {
		return 0
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		return exitErr.ExitCode()
	}
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(ctx.Err(), context.DeadlineExceeded) {
		return 124
	}
	return 1
}
