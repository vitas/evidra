package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"

	"samebits.com/evidra-benchmark/internal/lifecycle"
	"samebits.com/evidra-benchmark/pkg/evidence"
)

type prescribeFlags struct {
	artifactPath        string
	tool                string
	operation           string
	environment         string
	scannerReportPath   string
	evidenceDir         string
	actorID             string
	canonicalActionJSON string
	sessionID           string
	operationID         string
	attempt             int
	signingKey          string
	signingKeyPath      string
	signingMode         string
}

type prescribeCommand struct {
	service      *lifecycle.Service
	input        lifecycle.PrescribeInput
	evidencePath string
	signer       evidence.Signer
}

func cmdPrescribe(args []string, stdout, stderr io.Writer) int {
	opts, code := parsePrescribeFlags(args, stderr)
	if code != 0 {
		return code
	}

	cmd, err := preparePrescribeCommand(opts)
	if err != nil {
		fmt.Fprintf(stderr, "%v\n", err)
		return 1
	}

	prescOut, err := cmd.service.Prescribe(context.Background(), cmd.input)
	if err != nil {
		if lifecycle.ErrorCode(err) == lifecycle.ErrCodeParseError {
			result := map[string]interface{}{
				"ok":          false,
				"parse_error": err.Error(),
			}
			enc := json.NewEncoder(stdout)
			enc.SetIndent("", "  ")
			if encodeErr := enc.Encode(result); encodeErr != nil {
				fmt.Fprintf(stderr, "warning: failed to encode result: %v\n", encodeErr)
			}
			return 1
		}
		fmt.Fprintf(stderr, "prescribe: %v\n", err)
		return 1
	}

	result := map[string]interface{}{
		"ok":              true,
		"prescription_id": prescOut.PrescriptionID,
		"session_id":      prescOut.SessionID,
		"risk_level":      prescOut.RiskLevel,
		"risk_tags":       prescOut.RiskTags,
		"artifact_digest": prescOut.ArtifactDigest,
		"intent_digest":   prescOut.IntentDigest,
		"operation_class": prescOut.OperationClass,
		"scope_class":     prescOut.ScopeClass,
		"canon_version":   prescOut.CanonVersion,
	}

	if opts.scannerReportPath != "" {
		findings, err := loadSARIFFindings(opts.scannerReportPath, "scanner report")
		if err != nil {
			fmt.Fprintf(stderr, "%v\n", err)
			return 1
		}
		result["findings_count"] = appendFindingsAsEvidence(findings, findingAppendConfig{
			evidencePath:   cmd.evidencePath,
			signer:         cmd.signer,
			sessionID:      prescOut.SessionID,
			operationID:    opts.operationID,
			attempt:        opts.attempt,
			traceID:        prescOut.TraceID,
			actor:          prescOut.Actor,
			artifactDigest: prescOut.ArtifactDigest,
		}, stderr)
	}

	return writeJSON(stdout, stderr, "encode prescription", result)
}

func parsePrescribeFlags(args []string, stderr io.Writer) (prescribeFlags, int) {
	fs := flag.NewFlagSet("prescribe", flag.ContinueOnError)
	fs.SetOutput(stderr)
	artifactFlag := fs.String("artifact", "", "Path to artifact file (YAML or JSON)")
	artifactShortFlag := fs.String("f", "", "Path to artifact file (YAML or JSON)")
	toolFlag := fs.String("tool", "", "Tool name (kubectl, terraform)")
	operationFlag := fs.String("operation", "apply", "Operation (apply, delete, plan)")
	envFlag := fs.String("environment", "", "Environment (production, staging, development)")
	scannerFlag := fs.String("scanner-report", "", "SARIF scanner report file")
	evidenceFlag := fs.String("evidence-dir", "", "Evidence directory")
	actorFlag := fs.String("actor", "", "Actor ID (e.g. ci-pipeline-123)")
	canonicalActionFlag := fs.String("canonical-action", "", "Pre-canonicalized action JSON (bypasses adapter)")
	sessionIDFlag := fs.String("session-id", "", "Session/run boundary ID (generated if omitted)")
	operationIDFlag := fs.String("operation-id", "", "Operation identifier")
	attemptFlag := fs.Int("attempt", 0, "Retry attempt counter")
	signingKeyFlag := fs.String("signing-key", "", "Base64-encoded Ed25519 signing key")
	signingKeyPathFlag := fs.String("signing-key-path", "", "Path to PEM-encoded Ed25519 signing key")
	signingModeFlag := fs.String("signing-mode", "", "Signing mode: strict (default) or optional")
	if err := fs.Parse(args); err != nil {
		return prescribeFlags{}, 2
	}
	artifactPath := firstNonEmpty(*artifactFlag, *artifactShortFlag)
	if artifactPath == "" || *toolFlag == "" {
		fmt.Fprintln(stderr, "prescribe requires --artifact and --tool")
		return prescribeFlags{}, 2
	}

	return prescribeFlags{
		artifactPath:        artifactPath,
		tool:                *toolFlag,
		operation:           *operationFlag,
		environment:         *envFlag,
		scannerReportPath:   *scannerFlag,
		evidenceDir:         *evidenceFlag,
		actorID:             *actorFlag,
		canonicalActionJSON: *canonicalActionFlag,
		sessionID:           *sessionIDFlag,
		operationID:         *operationIDFlag,
		attempt:             *attemptFlag,
		signingKey:          *signingKeyFlag,
		signingKeyPath:      *signingKeyPathFlag,
		signingMode:         *signingModeFlag,
	}, 0
}

func preparePrescribeCommand(opts prescribeFlags) (prescribeCommand, error) {
	svc, evidencePath, signer, err := newLifecycleServiceForCommand(opts.evidenceDir, opts.signingKey, opts.signingKeyPath, opts.signingMode)
	if err != nil {
		return prescribeCommand{}, err
	}

	data, err := os.ReadFile(opts.artifactPath)
	if err != nil {
		return prescribeCommand{}, fmt.Errorf("read artifact: %w", err)
	}

	preCanon, err := parseCanonicalActionFlag(opts.canonicalActionJSON)
	if err != nil {
		return prescribeCommand{}, err
	}

	actorID := opts.actorID
	if actorID == "" {
		actorID = "cli"
	}
	actor := evidence.Actor{Type: "cli", ID: actorID, Provenance: "cli"}

	return prescribeCommand{
		service: svc,
		input: lifecycle.PrescribeInput{
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
		evidencePath: evidencePath,
		signer:       signer,
	}, nil
}
