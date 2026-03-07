package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"

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
}

type recordCommand struct {
	service *lifecycle.Service
	input   automationevent.RecordInput
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

	result := map[string]interface{}{
		"ok":               true,
		"contract_version": cmd.input.ContractVersion,
		"session_id":       opResult.ReportOutput.SessionID,
		"operation_id":     cmd.input.OperationID,
		"prescription_id":  opResult.PrescribeOutput.PrescriptionID,
		"report_id":        opResult.ReportOutput.ReportID,
		"exit_code":        cmd.input.ExitCode,
		"verdict":          evidence.VerdictFromExitCode(cmd.input.ExitCode),
		"duration_ms":      cmd.input.DurationMs,
		"risk_level":       opResult.PrescribeOutput.RiskLevel,
		"risk_tags":        opResult.PrescribeOutput.RiskTags,
	}
	return writeJSON(stdout, stderr, "encode record", result)
}

func parseRecordFlags(args []string, stderr io.Writer) (recordFlags, int) {
	fs := flag.NewFlagSet("record", flag.ContinueOnError)
	fs.SetOutput(stderr)
	inputFlag := fs.String("input", "-", "Path to record JSON file ('-' for stdin)")
	evidenceFlag := fs.String("evidence-dir", "", "Evidence directory")
	signingKeyFlag := fs.String("signing-key", "", "Base64-encoded Ed25519 signing key")
	signingKeyPathFlag := fs.String("signing-key-path", "", "Path to PEM-encoded Ed25519 signing key")
	signingModeFlag := fs.String("signing-mode", "", "Signing mode: strict (default) or optional")
	if err := fs.Parse(args); err != nil {
		return recordFlags{}, 2
	}

	return recordFlags{
		inputPath:      *inputFlag,
		evidenceDir:    *evidenceFlag,
		signingKey:     *signingKeyFlag,
		signingKeyPath: *signingKeyPathFlag,
		signingMode:    *signingModeFlag,
	}, 0
}

func prepareRecordCommand(opts recordFlags) (recordCommand, error) {
	svc, _, _, err := newLifecycleServiceForCommand(opts.evidenceDir, opts.signingKey, opts.signingKeyPath, opts.signingMode)
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
		service: svc,
		input:   in,
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
