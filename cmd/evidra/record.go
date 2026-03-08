package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"time"

	"samebits.com/evidra-benchmark/internal/automationevent"
	"samebits.com/evidra-benchmark/internal/canon"
	"samebits.com/evidra-benchmark/internal/lifecycle"
	"samebits.com/evidra-benchmark/pkg/evidence"
)

type recordFlags struct {
	inputPath      string
	evidenceDir    string
	signingKey     string
	signingKeyPath string
	signingMode    string
	// Mode flags
	url             string
	apiKey          string
	offline         bool
	fallbackOffline bool
	timeout         time.Duration
}

type recordCommand struct {
	service      *lifecycle.Service
	evidencePath string
	input        automationevent.RecordInput
}

func cmdRecord(args []string, stdout, stderr io.Writer) int {
	opts, code := parseRecordFlags(args, stderr)
	if code != 0 {
		return code
	}

	cmd, err := prepareRecordCommand(opts)
	if err != nil {
		fmt.Fprintf(stderr, "%v\n", err)
		return 2
	}

	artifactBytes := []byte(cmd.input.RawArtifact)
	if len(artifactBytes) == 0 {
		artifactBytes = cmd.input.CanonicalAction
	}

	preCanon, err := parseRecordCanonicalAction(cmd.input.CanonicalAction)
	if err != nil {
		fmt.Fprintf(stderr, "%v\n", err)
		return 2
	}

	prescIn := lifecycle.PrescribeInput{
		Actor:           cmd.input.Actor,
		Tool:            cmd.input.Tool,
		Operation:       cmd.input.Operation,
		RawArtifact:     artifactBytes,
		Environment:     cmd.input.Environment,
		CanonicalAction: preCanon,
		SessionID:       cmd.input.SessionID,
		OperationID:     cmd.input.OperationID,
		Attempt:         cmd.input.Attempt,
	}
	processor := NewOperationProcessor(cmd.service)
	opResult, err := processor.Process(context.Background(), OperationRequest{
		PrescribeInput: prescIn,
		ExitCode:       cmd.input.ExitCode,
		ReportActor:    cmd.input.Actor,
		SessionID:      cmd.input.SessionID,
		OperationID:    cmd.input.OperationID,
	})
	if err != nil {
		fmt.Fprintf(stderr, "record process: %v\n", err)
		return 1
	}

	assessment, err := buildOperationAssessment(
		cmd.evidencePath,
		opResult.ReportOutput.SessionID,
		opResult.PrescribeOutput.RiskLevel,
	)
	if err != nil {
		fmt.Fprintf(stderr, "record assessment: %v\n", err)
		return 1
	}

	if err := emitOperationMetrics(context.Background(), runMetricsPayload{
		Tool:           cmd.input.Tool,
		Environment:    cmd.input.Environment,
		ExitCode:       cmd.input.ExitCode,
		DurationMs:     cmd.input.DurationMs,
		ScoreBand:      assessment.ScoreBand,
		AssessmentMode: assessment.Basis.AssessmentMode,
		SignalSummary:  assessment.SignalSummary,
	}); err != nil {
		fmt.Fprintf(stderr, "warning: metrics export failed: %v\n", err)
	}

	result := map[string]interface{}{
		"ok":                  true,
		"contract_version":    cmd.input.ContractVersion,
		"session_id":          opResult.ReportOutput.SessionID,
		"operation_id":        cmd.input.OperationID,
		"prescription_id":     opResult.PrescribeOutput.PrescriptionID,
		"report_id":           opResult.ReportOutput.ReportID,
		"exit_code":           cmd.input.ExitCode,
		"verdict":             evidence.VerdictFromExitCode(cmd.input.ExitCode),
		"duration_ms":         cmd.input.DurationMs,
		"risk_classification": assessment.RiskClassification,
		"risk_level":          assessment.RiskLevel,
		"risk_tags":           opResult.PrescribeOutput.RiskTags,
		"score":               assessment.Score,
		"score_band":          assessment.ScoreBand,
		"signal_summary":      assessment.SignalSummary,
		"basis":               assessment.Basis,
		"confidence":          assessment.Confidence,
	}
	code = writeJSON(stdout, stderr, "encode record", result)

	// Best-effort forward evidence to API if online.
	forwardEvidence(opts.url, opts.apiKey, opts.offline, opts.fallbackOffline, opts.timeout, cmd.evidencePath, opResult.ReportOutput.SessionID, stderr)

	return code
}

func parseRecordFlags(args []string, stderr io.Writer) (recordFlags, int) {
	fs := flag.NewFlagSet("record", flag.ContinueOnError)
	fs.SetOutput(stderr)
	inputFlag := fs.String("input", "-", "Path to record JSON file ('-' for stdin)")
	evidenceFlag := fs.String("evidence-dir", "", "Evidence directory")
	signingKeyFlag := fs.String("signing-key", "", "Base64-encoded Ed25519 signing key")
	signingKeyPathFlag := fs.String("signing-key-path", "", "Path to PEM-encoded Ed25519 signing key")
	signingModeFlag := fs.String("signing-mode", "", "Signing mode: strict (default) or optional")
	urlFlag := fs.String("url", os.Getenv("EVIDRA_URL"), "Evidra API URL")
	apiKeyFlag := fs.String("api-key", os.Getenv("EVIDRA_API_KEY"), "Evidra API key")
	offlineFlag := fs.Bool("offline", false, "Force offline mode")
	fallbackOfflineFlag := fs.Bool("fallback-offline", false, "Fall back to offline on API failure")
	timeoutFlag := fs.Duration("timeout", 30*time.Second, "API request timeout")
	if err := fs.Parse(args); err != nil {
		return recordFlags{}, 2
	}

	return recordFlags{
		inputPath:       *inputFlag,
		evidenceDir:     *evidenceFlag,
		signingKey:      *signingKeyFlag,
		signingKeyPath:  *signingKeyPathFlag,
		signingMode:     *signingModeFlag,
		url:             *urlFlag,
		apiKey:          *apiKeyFlag,
		offline:         *offlineFlag,
		fallbackOffline: *fallbackOfflineFlag,
		timeout:         *timeoutFlag,
	}, 0
}

func prepareRecordCommand(opts recordFlags) (recordCommand, error) {
	svc, evidencePath, _, err := newLifecycleServiceForCommand(opts.evidenceDir, opts.signingKey, opts.signingKeyPath, opts.signingMode)
	if err != nil {
		return recordCommand{}, err
	}

	data, err := readRecordInputData(opts.inputPath)
	if err != nil {
		return recordCommand{}, err
	}

	var in automationevent.RecordInput
	if err := json.Unmarshal(data, &in); err != nil {
		return recordCommand{}, fmt.Errorf("parse record input JSON: %w", err)
	}
	if err := automationevent.ValidateRecordInput(in); err != nil {
		return recordCommand{}, err
	}

	return recordCommand{
		service:      svc,
		evidencePath: evidencePath,
		input:        in,
	}, nil
}

func readRecordInputData(inputPath string) ([]byte, error) {
	if inputPath == "" || inputPath == "-" {
		data, err := io.ReadAll(os.Stdin)
		if err != nil {
			return nil, fmt.Errorf("read record input from stdin: %w", err)
		}
		return data, nil
	}

	data, err := os.ReadFile(inputPath)
	if err != nil {
		return nil, fmt.Errorf("read record input file: %w", err)
	}
	return data, nil
}

func parseRecordCanonicalAction(raw json.RawMessage) (*canon.CanonicalAction, error) {
	if len(raw) == 0 {
		return nil, nil
	}

	var preCanon canon.CanonicalAction
	if err := json.Unmarshal(raw, &preCanon); err != nil {
		return nil, fmt.Errorf("parse record canonical_action: %w", err)
	}
	return &preCanon, nil
}
