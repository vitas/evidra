package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"samebits.com/evidra-benchmark/internal/lifecycle"
	"samebits.com/evidra-benchmark/pkg/evidence"
)

type recordFlags struct {
	artifactPath        string
	tool                string
	operation           string
	environment         string
	evidenceDir         string
	scoringProfile      string
	actorID             string
	canonicalActionJSON string
	sessionID           string
	operationID         string
	attempt             int
	signingKey          string
	signingKeyPath      string
	signingMode         string
	// Mode flags
	url             string
	apiKey          string
	offline         bool
	fallbackOffline bool
	timeout         time.Duration
}

type recordCommand struct {
	service        *lifecycle.Service
	evidencePath   string
	prescribeInput lifecycle.PrescribeInput
	wrapped        []string
}

type operationMetricsPayload struct {
	Tool           string
	Environment    string
	ExitCode       int
	DurationMs     int64
	ScoreBand      string
	AssessmentMode string
	SignalSummary  map[string]int
}

var emitRecordMetricsHook = emitOperationMetrics

func cmdRecord(args []string, stdout, stderr io.Writer) int {
	opts, wrappedCmd, code := parseRecordFlags(args, stderr)
	if code != 0 {
		return code
	}

	cmd, err := prepareRecordCommand(opts, wrappedCmd)
	if err != nil {
		fmt.Fprintf(stderr, "%v\n", err)
		return 2
	}
	profile, err := resolveCommandScoringProfile(opts.scoringProfile)
	if err != nil {
		fmt.Fprintf(stderr, "%v\n", err)
		return 2
	}

	exitCode, durationMs, err := executeWrappedCommand(context.Background(), cmd.wrapped, stderr)
	if err != nil {
		fmt.Fprintf(stderr, "%v\n", err)
		return 1
	}

	processor := NewOperationProcessor(cmd.service)
	opResult, err := processor.Process(context.Background(), OperationRequest{
		PrescribeInput: cmd.prescribeInput,
		ExitCode:       exitCode,
		ReportActor:    cmd.prescribeInput.Actor,
		SessionID:      cmd.prescribeInput.SessionID,
		OperationID:    cmd.prescribeInput.OperationID,
	})
	if err != nil {
		fmt.Fprintf(stderr, "record process: %v\n", err)
		return 1
	}

	assessment, err := buildOperationAssessmentWithProfile(
		cmd.evidencePath,
		opResult.ReportOutput.SessionID,
		opResult.PrescribeOutput.RiskLevel,
		profile,
	)
	if err != nil {
		fmt.Fprintf(stderr, "record assessment: %v\n", err)
		return 1
	}

	if err := emitRecordMetricsHook(context.Background(), operationMetricsPayload{
		Tool:           cmd.prescribeInput.Tool,
		Environment:    cmd.prescribeInput.Environment,
		ExitCode:       exitCode,
		DurationMs:     durationMs,
		ScoreBand:      assessment.ScoreBand,
		AssessmentMode: assessment.Basis.AssessmentMode,
		SignalSummary:  assessment.SignalSummary,
	}); err != nil {
		fmt.Fprintf(stderr, "warning: metrics export failed: %v\n", err)
	}

	result := map[string]interface{}{
		"ok":                 exitCode == 0,
		"session_id":         opResult.ReportOutput.SessionID,
		"operation_id":       cmd.prescribeInput.OperationID,
		"prescription_id":    opResult.PrescribeOutput.PrescriptionID,
		"report_id":          opResult.ReportOutput.ReportID,
		"exit_code":          exitCode,
		"verdict":            evidence.VerdictFromExitCode(exitCode),
		"duration_ms":        durationMs,
		"risk_level":         assessment.RiskLevel,
		"risk_tags":          opResult.PrescribeOutput.RiskTags,
		"score":              assessment.Score,
		"score_band":         assessment.ScoreBand,
		"scoring_profile_id": assessment.ScoringProfileID,
		"signal_summary":     assessment.SignalSummary,
		"basis":              assessment.Basis,
		"confidence":         assessment.Confidence,
	}
	if writeJSON(stdout, stderr, "encode record", result) != 0 {
		return 1
	}

	// Best-effort forward evidence to API if online.
	forwardEvidence(opts.url, opts.apiKey, opts.offline, opts.fallbackOffline, opts.timeout, cmd.evidencePath, opResult.ReportOutput.SessionID, stderr)

	return exitCode
}

func parseRecordFlags(args []string, stderr io.Writer) (recordFlags, []string, int) {
	flagArgs, wrappedCmd := splitRecordArgs(args)

	fs := flag.NewFlagSet("record", flag.ContinueOnError)
	fs.SetOutput(stderr)
	artifactFlag := fs.String("artifact", "", "Path to artifact file (YAML or JSON)")
	artifactShortFlag := fs.String("f", "", "Path to artifact file (YAML or JSON)")
	toolFlag := fs.String("tool", "", "Tool name (kubectl, terraform)")
	operationFlag := fs.String("operation", "", "Operation (apply, delete, plan)")
	envFlag := fs.String("environment", "", "Environment (production, staging, development)")
	evidenceFlag := fs.String("evidence-dir", "", "Evidence directory")
	scoringProfileFlag := fs.String("scoring-profile", "", "Path to scoring profile JSON")
	actorFlag := fs.String("actor", "", "Actor ID (e.g. ci-pipeline-123)")
	canonicalActionFlag := fs.String("canonical-action", "", "Pre-canonicalized action JSON (bypasses adapter)")
	sessionIDFlag := fs.String("session-id", "", "Session/run boundary ID (generated if omitted)")
	operationIDFlag := fs.String("operation-id", "", "Operation identifier")
	attemptFlag := fs.Int("attempt", 0, "Retry attempt counter")
	signingKeyFlag := fs.String("signing-key", "", "Base64-encoded Ed25519 signing key")
	signingKeyPathFlag := fs.String("signing-key-path", "", "Path to PEM-encoded Ed25519 signing key")
	signingModeFlag := fs.String("signing-mode", "", "Signing mode: strict (default) or optional")
	urlFlag := fs.String("url", os.Getenv("EVIDRA_URL"), "Evidra API URL")
	apiKeyFlag := fs.String("api-key", os.Getenv("EVIDRA_API_KEY"), "Evidra API key")
	offlineFlag := fs.Bool("offline", false, "Force offline mode")
	fallbackOfflineFlag := fs.Bool("fallback-offline", false, "Fall back to offline on API failure")
	timeoutFlag := fs.Duration("timeout", 30*time.Second, "API request timeout")
	if err := fs.Parse(flagArgs); err != nil {
		return recordFlags{}, nil, 2
	}

	if len(fs.Args()) > 0 {
		fmt.Fprintln(stderr, "record requires '--' before wrapped command")
		return recordFlags{}, nil, 2
	}
	if wrappedCmd == nil {
		fmt.Fprintln(stderr, "record requires '--' before wrapped command")
		return recordFlags{}, nil, 2
	}
	if len(wrappedCmd) == 0 {
		fmt.Fprintln(stderr, "record requires wrapped command after '--'")
		return recordFlags{}, nil, 2
	}

	tool, operation, err := resolveRecordIntent(*toolFlag, *operationFlag, wrappedCmd)
	if err != nil {
		fmt.Fprintln(stderr, err.Error())
		return recordFlags{}, nil, 2
	}

	return recordFlags{
		artifactPath:        firstNonEmpty(*artifactFlag, *artifactShortFlag),
		tool:                tool,
		operation:           operation,
		environment:         *envFlag,
		evidenceDir:         *evidenceFlag,
		scoringProfile:      *scoringProfileFlag,
		actorID:             *actorFlag,
		canonicalActionJSON: *canonicalActionFlag,
		sessionID:           *sessionIDFlag,
		operationID:         *operationIDFlag,
		attempt:             *attemptFlag,
		signingKey:          *signingKeyFlag,
		signingKeyPath:      *signingKeyPathFlag,
		signingMode:         *signingModeFlag,
		url:                 *urlFlag,
		apiKey:              *apiKeyFlag,
		offline:             *offlineFlag,
		fallbackOffline:     *fallbackOfflineFlag,
		timeout:             *timeoutFlag,
	}, wrappedCmd, 0
}

func splitRecordArgs(args []string) ([]string, []string) {
	for i, a := range args {
		if a == "--" {
			return args[:i], args[i+1:]
		}
	}
	return args, nil
}

func prepareRecordCommand(opts recordFlags, wrapped []string) (recordCommand, error) {
	svc, evidencePath, _, err := newLifecycleServiceForCommand(opts.evidenceDir, opts.signingKey, opts.signingKeyPath, opts.signingMode)
	if err != nil {
		return recordCommand{}, err
	}

	var data []byte
	if opts.artifactPath != "" {
		data, err = os.ReadFile(opts.artifactPath)
		if err != nil {
			return recordCommand{}, fmt.Errorf("read artifact: %w", err)
		}
	}

	preCanon, err := parseCanonicalActionFlag(opts.canonicalActionJSON)
	if err != nil {
		return recordCommand{}, err
	}

	actorID := opts.actorID
	if actorID == "" {
		actorID = "cli"
	}
	actor := evidence.Actor{Type: "cli", ID: actorID, Provenance: "cli"}

	return recordCommand{
		service:      svc,
		evidencePath: evidencePath,
		prescribeInput: lifecycle.PrescribeInput{
			Actor:           actor,
			Tool:            opts.tool,
			Operation:       opts.operation,
			RawArtifact:     data,
			Environment:     opts.environment,
			CanonicalAction: preCanon,
			SessionID:       opts.sessionID,
			OperationID:     opts.operationID,
			Attempt:         opts.attempt,
		},
		wrapped: wrapped,
	}, nil
}

func resolveRecordIntent(explicitTool, explicitOperation string, wrapped []string) (string, string, error) {
	tool := strings.TrimSpace(explicitTool)
	operation := strings.TrimSpace(explicitOperation)

	if len(wrapped) == 0 {
		return "", "", errors.New("wrapped command is required")
	}
	if isShellWrappedCommand(wrapped[0]) {
		if tool == "" || operation == "" {
			return "", "", errors.New("please specify --tool and --operation explicitly")
		}
		return tool, operation, nil
	}

	if tool == "" {
		inferredTool, err := inferRecordTool(wrapped)
		if err != nil {
			return "", "", err
		}
		tool = inferredTool
	}
	if operation == "" {
		inferredOperation, err := inferRecordOperation(tool, wrapped)
		if err != nil {
			return "", "", err
		}
		operation = inferredOperation
	}

	return tool, operation, nil
}

func inferRecordTool(wrapped []string) (string, error) {
	first := normalizeWrappedToolName(wrapped[0])
	switch first {
	case "kubectl", "helm", "terraform", "docker", "argocd", "oc", "kustomize", "pulumi":
		return first, nil
	default:
		return "", fmt.Errorf("unknown tool '%s', please specify --tool and --operation explicitly", first)
	}
}

func inferRecordOperation(tool string, wrapped []string) (string, error) {
	if len(wrapped) < 2 {
		return "", errors.New("please specify --operation explicitly")
	}

	switch normalizeRecordToken(tool) {
	case "kubectl", "oc", "helm", "terraform", "pulumi":
		if strings.HasPrefix(wrapped[1], "-") {
			return "", errors.New("please specify --operation explicitly")
		}
		return wrapped[1], nil
	case "kustomize":
		if wrapped[1] == "build" {
			return "build", nil
		}
	case "docker":
		if wrapped[1] == "build" {
			return "build", nil
		}
		if wrapped[1] == "compose" && len(wrapped) >= 3 && !strings.HasPrefix(wrapped[2], "-") {
			return wrapped[2], nil
		}
	case "argocd":
		if len(wrapped) >= 3 && wrapped[1] == "app" {
			switch wrapped[2] {
			case "sync", "delete", "create":
				return wrapped[2], nil
			}
		}
	}

	return "", errors.New("please specify --operation explicitly")
}

func isShellWrappedCommand(command string) bool {
	switch normalizeWrappedToolName(command) {
	case "sh", "bash", "zsh":
		return true
	default:
		return false
	}
}

func normalizeWrappedToolName(command string) string {
	return strings.TrimSpace(filepath.Base(command))
}

func normalizeRecordToken(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func executeWrappedCommand(ctx context.Context, wrapped []string, stderr io.Writer) (int, int64, error) {
	if len(wrapped) == 0 {
		return 1, 0, fmt.Errorf("wrapped command is required")
	}

	start := time.Now()
	cmd := exec.CommandContext(ctx, wrapped[0], wrapped[1:]...)
	cmd.Stdout = stderr
	cmd.Stderr = stderr
	err := cmd.Run()
	durationMs := time.Since(start).Milliseconds()
	if err == nil {
		return 0, durationMs, nil
	}

	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		return exitErr.ExitCode(), durationMs, nil
	}

	return 1, durationMs, fmt.Errorf("execute wrapped command: %w", err)
}
