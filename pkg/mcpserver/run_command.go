package mcpserver

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"samebits.com/evidra/pkg/evidence"
	"samebits.com/evidra/pkg/proxy"
	promptdata "samebits.com/evidra/prompts"
)

// RunCommandInput is the input for the run_command tool.
type RunCommandInput struct {
	Command string `json:"command"`
}

// RunCommandOutput is the output for the run_command tool.
type RunCommandOutput struct {
	OK       bool   `json:"ok"`
	Output   string `json:"output"`
	ExitCode int    `json:"exit_code"`
	Mutation bool   `json:"mutation"`
	Error    string `json:"error,omitempty"`
}

type runCommandHandler struct {
	service         *MCPService
	kubeconfigPath  string
	actorID         string
	allowedPrefixes []string
	blockedSubs     []string
}

// defaultAllowedPrefixes restricts which commands the LLM can execute.
var defaultAllowedPrefixes = []string{
	"kubectl", "helm", "argocd", "kind", "terraform", "aws", "kustomize",
	"cat", "echo", "grep", "head", "tail", "wc", "ls", "find", "openssl",
	"jq", "yq",
}

// defaultBlockedSubcommands blocks interactive, exec, and dangerous subcommands.
// kubectl exec and kubectl run are blocked entirely — they execute arbitrary
// commands inside pods, bypassing the allowlist.
var defaultBlockedSubcommands = []string{
	"kubectl edit ",
	"kubectl exec ",
	"kubectl attach ",
	"kubectl port-forward ",
	"kubectl proxy",
	"kubectl run ",
	"kubectl debug ",
	"helm shell",
	"terraform console",
}

// runCommandInputSchema is the JSON Schema for the run_command tool input.
var runCommandInputSchema = map[string]any{
	"type":     "object",
	"required": []any{"command"},
	"properties": map[string]any{
		"command": map[string]any{
			"type":        "string",
			"description": "Shell command to execute (e.g., 'kubectl get pods -n demo')",
		},
	},
}

const defaultRunCommandToolDescription = `Execute kubectl, helm, terraform, or aws commands with token-efficient output summaries.

Investigate before fixing:
- kubectl get pods -n demo
- kubectl describe pod web-abc -n demo
- kubectl logs web-abc -n demo --tail=50

Fix and verify:
- kubectl patch deployment/web -n demo --type=merge -p '{"spec":{"template":{"spec":{}}}}'
- kubectl rollout status deployment/web -n demo --timeout=60s

Mutations are automatically recorded as evidence. Use prescribe_smart or prescribe_full explicitly when you need tighter control before execution.`

func runCommandToolDescription() string {
	content, err := promptdata.Read(promptdata.MCPRunCommandDescriptionPath)
	if err != nil {
		return defaultRunCommandToolDescription
	}
	return promptdata.StripContractHeader(content)
}

func runCommandOutputSchema() map[string]any {
	schema, err := loadSchema(runCommandOutputSchemaBytes, "schemas/run_command.output.schema.json")
	if err != nil {
		panic(err)
	}
	return schema
}

// Handle executes an infrastructure command with validation, auto-evidence for mutations,
// and smart output formatting.
func (h *runCommandHandler) Handle(
	ctx context.Context,
	_ *mcp.CallToolRequest,
	input RunCommandInput,
) (*mcp.CallToolResult, RunCommandOutput, error) {
	output := h.execute(ctx, input)
	result := &mcp.CallToolResult{}
	if !output.OK {
		result.IsError = true
	}
	return result, output, nil
}

func (h *runCommandHandler) execute(ctx context.Context, input RunCommandInput) RunCommandOutput {
	command := strings.TrimSpace(input.Command)
	if command == "" {
		return RunCommandOutput{OK: false, Error: "command is required"}
	}

	if err := validateRunCommand(command, h.allowedPrefixes, h.blockedSubs); err != nil {
		return RunCommandOutput{OK: false, Error: err.Error()}
	}

	isMutation := proxy.IsMutation(command)

	// Evidence for mutations: prefer linking to a prior model-issued
	// prescription (real claim vs action); fall back to an auto-derived,
	// explicitly flagged prescription so the chain is never silent.
	var prescriptionID string
	if isMutation && h.service != nil {
		pid, evidenceErr := h.evidenceForMutation(ctx, command)
		if evidenceErr != nil {
			return RunCommandOutput{OK: false, Error: evidenceErr.Error(), Mutation: true}
		}
		prescriptionID = pid
	}

	// Execute command — use direct exec, not bash -c, to prevent shell injection.
	args, err := parseCommand(command)
	if err != nil {
		return RunCommandOutput{OK: false, Error: err.Error(), Mutation: isMutation}
	}
	cmd := exec.CommandContext(ctx, args[0], args[1:]...)
	cmd.Env = os.Environ()
	if h.kubeconfigPath != "" {
		cmd.Env = append(cmd.Env, "KUBECONFIG="+h.kubeconfigPath)
	}
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err = cmd.Run()
	exitCode := 0
	if err != nil {
		exitCode = exitCodeFromError(err)
	}

	// Combine output.
	rawOutput := stdout.String()
	if stderr.Len() > 0 {
		if rawOutput != "" {
			rawOutput += "\n"
		}
		rawOutput += stderr.String()
	}

	// Auto-report for mutations.
	if prescriptionID != "" && h.service != nil {
		ec := exitCode
		verdict := evidence.Verdict("success")
		if exitCode != 0 {
			verdict = evidence.Verdict("failure")
		}
		h.service.ReportCtx(ctx, ReportInput{
			PrescriptionID: prescriptionID,
			Verdict:        verdict,
			ExitCode:       &ec,
			Actor:          InputActor{Type: "proxy", ID: "run_command", Origin: "mcp"},
		})
	}

	// Smart formatting.
	formatted := FormatSmartOutput(command, rawOutput, exitCode)

	return RunCommandOutput{
		OK:       exitCode == 0,
		Output:   formatted,
		ExitCode: exitCode,
		Mutation: isMutation,
	}
}

// evidenceForMutation resolves the prescription id an executed mutation
// should report against: first an unconsumed model-issued claim covering the
// same normalized action, otherwise a fresh server-derived prescription
// flagged auto_prescribed (see claim_cache.go). A nil error with an empty id
// means "no evidence plumbing possible for this command" (non-mutating per
// classifier, or prescription write declined); execution is unaffected.
func (h *runCommandHandler) evidenceForMutation(ctx context.Context, command string) (string, error) {
	prescribeInput, ok, err := deriveAutoPrescribeInput(command, h.actorID)
	if err != nil {
		return "", err
	}
	if !ok {
		return "", nil
	}
	if linked, found := h.service.takeClaim(prescribeInput); found {
		return linked, nil
	}
	if out := h.service.PrescribeCtx(ctx, prescribeInput); out.OK {
		return out.PrescriptionID, nil
	}
	return "", nil
}

// validateRunCommand checks that a command starts with an allowed prefix
// and does not match any blocked interactive subcommand.
// shellMetachars are characters that enable command chaining/injection in bash.
var shellMetachars = []string{";", "&&", "||", "|", "`", "$(", "${", ">", "<", "\n"}

func validateRunCommand(command string, allowed, blocked []string) error {
	trimmed := strings.TrimSpace(command)

	// Reject shell metacharacters — prevents command injection via chaining.
	for _, meta := range shellMetachars {
		if strings.Contains(trimmed, meta) {
			return fmt.Errorf("command contains shell metacharacter %q — only single commands allowed", meta)
		}
	}

	// Check blocked subcommands.
	for _, b := range blocked {
		if trimmed == strings.TrimSpace(b) || strings.HasPrefix(trimmed, b) {
			return fmt.Errorf("command %q is blocked (interactive/dangerous)", truncateCmd(trimmed, 60))
		}
	}

	// Check allowlist.
	for _, prefix := range allowed {
		if trimmed == prefix || strings.HasPrefix(trimmed, prefix+" ") {
			return nil
		}
	}
	return fmt.Errorf("command %q not in allowlist", truncateCmd(trimmed, 50))
}

func exitCodeFromError(err error) int {
	if err == nil {
		return 0
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		return exitErr.ExitCode()
	}
	return 1
}

func truncateCmd(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}

// RegisterRunCommand registers the run_command tool on the given MCP server.
// It is only registered when the server is not in evidence-only mode.
func RegisterRunCommand(server *mcp.Server, svc *MCPService, kubeconfigPath string, actorID string) {
	handler := &runCommandHandler{
		service:         svc,
		kubeconfigPath:  kubeconfigPath,
		actorID:         actorID,
		allowedPrefixes: defaultAllowedPrefixes,
		blockedSubs:     defaultBlockedSubcommands,
	}

	mcp.AddTool(server, &mcp.Tool{
		Name:        "run_command",
		Title:       "Execute Infrastructure Command",
		Description: runCommandToolDescription(),
		Annotations: &mcp.ToolAnnotations{
			Title:           "Run Command",
			ReadOnlyHint:    false,
			IdempotentHint:  false,
			DestructiveHint: boolPtr(true),
			OpenWorldHint:   boolPtr(true),
		},
		InputSchema:  runCommandInputSchema,
		OutputSchema: runCommandOutputSchema(),
	}, handler.Handle)
}
