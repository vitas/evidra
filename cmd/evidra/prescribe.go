package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"time"

	"samebits.com/evidra/internal/lifecycle"
	"samebits.com/evidra/pkg/evidence"
)

type prescribeFlags struct {
	artifactPath        string
	tool                string
	operation           string
	environment         string
	findingsPaths       multiStringFlag
	evidenceDir         string
	actor               actorFlags
	canonicalActionJSON string
	scopeDimensionsJSON string
	sessionID           string
	operationID         string
	attempt             int
	traceID             string
	spanID              string
	parentSpanID        string
	signingKey          string
	signingKeyPath      string
	signingMode         string
	url                 string
	apiKey              string
	offline             bool
	fallbackOffline     bool
	timeout             time.Duration
}

type prescribeCommand struct {
	service      *lifecycle.Service
	input        lifecycle.PrescribeInput
	evidencePath string
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
		"risk_inputs":     prescOut.RiskInputs,
		"effective_risk":  prescOut.EffectiveRisk,
		"artifact_digest": prescOut.ArtifactDigest,
		"intent_digest":   prescOut.IntentDigest,
		"operation_class": prescOut.OperationClass,
		"scope_class":     prescOut.ScopeClass,
		"canon_version":   prescOut.CanonVersion,
	}

	if writeJSON(stdout, stderr, "encode prescription", result) != 0 {
		return 1
	}

	forwardEvidence(opts.url, opts.apiKey, opts.offline, opts.fallbackOffline, opts.timeout, cmd.evidencePath, prescOut.SessionID, stderr)
	return 0
}

func parsePrescribeFlags(args []string, stderr io.Writer) (prescribeFlags, int) {
	fs := flag.NewFlagSet("prescribe", flag.ContinueOnError)
	fs.SetOutput(stderr)
	artifactFlag := fs.String("artifact", "", "Path to artifact file (YAML or JSON)")
	artifactShortFlag := fs.String("f", "", "Path to artifact file (YAML or JSON)")
	toolFlag := fs.String("tool", "", "Tool name (kubectl, terraform)")
	operationFlag := fs.String("operation", "apply", "Operation (apply, delete, plan)")
	envFlag := fs.String("environment", "", "Environment (production, staging, development)")
	var findingsPaths multiStringFlag
	fs.Var(&findingsPaths, "findings", "SARIF findings file (repeatable)")
	evidenceFlag := fs.String("evidence-dir", "", "Evidence directory")
	var actor actorFlags
	bindActorFlags(fs, &actor, "Actor ID (e.g. ci-pipeline-123)")
	canonicalActionFlag := fs.String("canonical-action", "", "Pre-canonicalized action JSON (bypasses adapter)")
	scopeDimensionsFlag := fs.String("scope-dimensions", "", "Scope dimensions JSON object (e.g. '{\"cluster\":\"prod\",\"namespace\":\"default\"}')")
	sessionIDFlag := fs.String("session-id", "", "Session/run boundary ID (generated if omitted)")
	operationIDFlag := fs.String("operation-id", "", "Operation identifier")
	attemptFlag := fs.Int("attempt", 0, "Retry attempt counter")
	traceIDFlag := fs.String("trace-id", "", "Distributed tracing correlation ID")
	spanIDFlag := fs.String("span-id", "", "Trace span identifier")
	parentSpanIDFlag := fs.String("parent-span-id", "", "Parent span identifier")
	signingKeyFlag := fs.String("signing-key", "", "Base64-encoded Ed25519 signing key")
	signingKeyPathFlag := fs.String("signing-key-path", "", "Path to PEM-encoded Ed25519 signing key")
	signingModeFlag := fs.String("signing-mode", "", "Signing mode: strict (default) or optional")
	urlFlag := fs.String("url", os.Getenv("EVIDRA_URL"), "Evidra API URL")
	apiKeyFlag := fs.String("api-key", os.Getenv("EVIDRA_API_KEY"), "Evidra API key")
	offlineFlag := fs.Bool("offline", false, "Force offline mode")
	fallbackOfflineFlag := fs.Bool("fallback-offline", false, "Fall back to offline on API failure")
	timeoutFlag := fs.Duration("timeout", 30*time.Second, "API request timeout")
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
		findingsPaths:       findingsPaths,
		evidenceDir:         *evidenceFlag,
		actor:               actor,
		canonicalActionJSON: *canonicalActionFlag,
		scopeDimensionsJSON: *scopeDimensionsFlag,
		sessionID:           *sessionIDFlag,
		operationID:         *operationIDFlag,
		attempt:             *attemptFlag,
		traceID:             *traceIDFlag,
		spanID:              *spanIDFlag,
		parentSpanID:        *parentSpanIDFlag,
		signingKey:          *signingKeyFlag,
		signingKeyPath:      *signingKeyPathFlag,
		signingMode:         *signingModeFlag,
		url:                 *urlFlag,
		apiKey:              *apiKeyFlag,
		offline:             *offlineFlag,
		fallbackOffline:     *fallbackOfflineFlag,
		timeout:             *timeoutFlag,
	}, 0
}

func preparePrescribeCommand(opts prescribeFlags) (prescribeCommand, error) {
	svc, evidencePath, _, err := newLifecycleServiceForCommand(opts.evidenceDir, opts.signingKey, opts.signingKeyPath, opts.signingMode)
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
	scopeDimensions, err := parseStringMapFlag(opts.scopeDimensionsJSON, "--scope-dimensions")
	if err != nil {
		return prescribeCommand{}, err
	}

	actor := buildActor(opts.actor, "cli", "cli", "cli")

	var externalFindings []lifecycle.ExternalFindingsSource
	for _, path := range opts.findingsPaths {
		findings, err := loadSARIFFindings(path, "findings")
		if err != nil {
			return prescribeCommand{}, err
		}
		externalFindings = append(externalFindings, lifecycle.ExternalFindingsSource{
			Findings: findings,
		})
	}

	intent := evidence.DeclaredIntent{}
	if preCanon == nil {
		intent = evidence.DeclaredIntent{
			Tool:      opts.tool,
			Operation: opts.operation,
			Target:    opts.artifactPath,
		}
	}

	return prescribeCommand{
		service: svc,
		input: lifecycle.PrescribeInput{
			Actor:            actor,
			Intent:           intent,
			Tool:             opts.tool,
			Operation:        opts.operation,
			RawArtifact:      data,
			Environment:      opts.environment,
			CanonicalAction:  preCanon,
			ExternalFindings: externalFindings,
			SessionID:        opts.sessionID,
			OperationID:      opts.operationID,
			Attempt:          opts.attempt,
			TraceID:          opts.traceID,
			SpanID:           opts.spanID,
			ParentSpanID:     opts.parentSpanID,
			ScopeDimensions:  scopeDimensions,
			Flavor:           evidence.FlavorImperative,
			EvidenceKind:     evidence.EvidenceKindDeclared,
			SourceSystem:     "cli",
		},
		evidencePath: evidencePath,
	}, nil
}
