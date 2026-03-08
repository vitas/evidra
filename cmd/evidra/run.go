package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"time"

	"samebits.com/evidra-benchmark/internal/lifecycle"
	"samebits.com/evidra-benchmark/pkg/evidence"
)

type runFlags struct {
	artifactPath        string
	tool                string
	operation           string
	environment         string
	evidenceDir         string
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

type runCommand struct {
	service        *lifecycle.Service
	evidencePath   string
	prescribeInput lifecycle.PrescribeInput
	wrapped        []string
}

type runMetricsPayload struct {
	Tool           string
	Environment    string
	ExitCode       int
	DurationMs     int64
	ScoreBand      string
	AssessmentMode string
	SignalSummary  map[string]int
}

var emitRunMetricsHook = emitOperationMetrics

func cmdRun(args []string, stdout, stderr io.Writer) int {
	opts, wrappedCmd, code := parseRunFlags(args, stderr)
	if code != 0 {
		return code
	}

	cmd, err := prepareRunCommand(opts, wrappedCmd)
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
		fmt.Fprintf(stderr, "run process: %v\n", err)
		return 1
	}

	assessment, err := buildOperationAssessment(
		cmd.evidencePath,
		opResult.ReportOutput.SessionID,
		opResult.PrescribeOutput.RiskLevel,
	)
	if err != nil {
		fmt.Fprintf(stderr, "run assessment: %v\n", err)
		return 1
	}

	if err := emitRunMetricsHook(context.Background(), runMetricsPayload{
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
		"ok":                  true,
		"session_id":          opResult.ReportOutput.SessionID,
		"operation_id":        cmd.prescribeInput.OperationID,
		"prescription_id":     opResult.PrescribeOutput.PrescriptionID,
		"report_id":           opResult.ReportOutput.ReportID,
		"exit_code":           exitCode,
		"verdict":             evidence.VerdictFromExitCode(exitCode),
		"duration_ms":         durationMs,
		"risk_classification": assessment.RiskClassification,
		"risk_level":          assessment.RiskLevel,
		"risk_tags":           opResult.PrescribeOutput.RiskTags,
		"score":               assessment.Score,
		"score_band":          assessment.ScoreBand,
		"signal_summary":      assessment.SignalSummary,
		"basis":               assessment.Basis,
		"confidence":          assessment.Confidence,
	}
	if writeJSON(stdout, stderr, "encode run", result) != 0 {
		return 1
	}

	// Best-effort forward evidence to API if online.
	forwardEvidence(opts.url, opts.apiKey, opts.offline, opts.fallbackOffline, opts.timeout, cmd.evidencePath, opResult.ReportOutput.SessionID, stderr)

	return exitCode
}

func parseRunFlags(args []string, stderr io.Writer) (runFlags, []string, int) {
	flagArgs, wrappedCmd := splitRunArgs(args)

	fs := flag.NewFlagSet("run", flag.ContinueOnError)
	fs.SetOutput(stderr)
	artifactFlag := fs.String("artifact", "", "Path to artifact file (YAML or JSON)")
	toolFlag := fs.String("tool", "", "Tool name (kubectl, terraform)")
	operationFlag := fs.String("operation", "apply", "Operation (apply, delete, plan)")
	envFlag := fs.String("environment", "", "Environment (production, staging, development)")
	evidenceFlag := fs.String("evidence-dir", "", "Evidence directory")
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
		return runFlags{}, nil, 2
	}

	if *artifactFlag == "" || *toolFlag == "" {
		fmt.Fprintln(stderr, "run requires --artifact and --tool")
		return runFlags{}, nil, 2
	}
	if len(fs.Args()) > 0 {
		fmt.Fprintln(stderr, "run requires '--' before wrapped command")
		return runFlags{}, nil, 2
	}
	if len(wrappedCmd) == 0 {
		fmt.Fprintln(stderr, "run requires wrapped command after '--'")
		return runFlags{}, nil, 2
	}

	return runFlags{
		artifactPath:        *artifactFlag,
		tool:                *toolFlag,
		operation:           *operationFlag,
		environment:         *envFlag,
		evidenceDir:         *evidenceFlag,
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

func splitRunArgs(args []string) ([]string, []string) {
	for i, a := range args {
		if a == "--" {
			return args[:i], args[i+1:]
		}
	}
	return args, nil
}

func prepareRunCommand(opts runFlags, wrapped []string) (runCommand, error) {
	svc, evidencePath, _, err := newLifecycleServiceForCommand(opts.evidenceDir, opts.signingKey, opts.signingKeyPath, opts.signingMode)
	if err != nil {
		return runCommand{}, err
	}

	data, err := os.ReadFile(opts.artifactPath)
	if err != nil {
		return runCommand{}, fmt.Errorf("read artifact: %w", err)
	}

	preCanon, err := parseCanonicalActionFlag(opts.canonicalActionJSON)
	if err != nil {
		return runCommand{}, err
	}

	actorID := opts.actorID
	if actorID == "" {
		actorID = "cli"
	}
	actor := evidence.Actor{Type: "cli", ID: actorID, Provenance: "cli"}

	return runCommand{
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
