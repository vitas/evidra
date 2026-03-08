package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"samebits.com/evidra-benchmark/internal/canon"
	"samebits.com/evidra-benchmark/internal/config"
	ievsigner "samebits.com/evidra-benchmark/internal/evidence"
	"samebits.com/evidra-benchmark/internal/lifecycle"
	"samebits.com/evidra-benchmark/internal/pipeline"
	"samebits.com/evidra-benchmark/internal/sarif"
	"samebits.com/evidra-benchmark/internal/score"
	"samebits.com/evidra-benchmark/internal/signal"
	"samebits.com/evidra-benchmark/pkg/evidence"
	"samebits.com/evidra-benchmark/pkg/mode"
	"samebits.com/evidra-benchmark/pkg/version"
)

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		printUsage(stdout)
		return 0
	}

	switch args[0] {
	case "version":
		fmt.Fprintf(stdout, "evidra-benchmark %s (commit: %s, built: %s)\n",
			version.Version, version.Commit, version.Date)
		return 0
	case "scorecard":
		return cmdScorecard(args[1:], stdout, stderr)
	case "explain":
		return cmdExplain(args[1:], stdout, stderr)
	case "compare":
		return cmdCompare(args[1:], stdout, stderr)
	case "run":
		return cmdRun(args[1:], stdout, stderr)
	case "prescribe":
		return cmdPrescribe(args[1:], stdout, stderr)
	case "report":
		return cmdReport(args[1:], stdout, stderr)
	case "record":
		return cmdRecord(args[1:], stdout, stderr)
	case "validate":
		return cmdValidate(args[1:], stdout, stderr)
	case "ingest-findings":
		return cmdIngestFindings(args[1:], stdout, stderr)
	case "keygen":
		return cmdKeygen(args[1:], stdout, stderr)
	case "prompts":
		return cmdPrompts(args[1:], stdout, stderr)
	case "detectors":
		return cmdDetectors(args[1:], stdout, stderr)
	case "help", "--help", "-h":
		printUsage(stdout)
		return 0
	default:
		fmt.Fprintf(stderr, "unknown command: %s\n", args[0])
		printUsage(stderr)
		return 2
	}
}

func cmdScorecard(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("scorecard", flag.ContinueOnError)
	fs.SetOutput(stderr)
	actorFlag := fs.String("actor", "", "Actor ID to generate scorecard for")
	periodFlag := fs.String("period", "30d", "Time period (e.g. 30d)")
	evidenceFlag := fs.String("evidence-dir", "", "Evidence directory")
	ttlFlag := fs.String("ttl", signal.DefaultTTL.String(), "TTL for unreported prescription detection")
	toolFlag := fs.String("tool", "", "Filter by tool name")
	scopeFlag := fs.String("scope", "", "Filter by scope class")
	sessionIDFlag := fs.String("session-id", "", "Filter by session ID")
	minOpsFlag := fs.Int("min-operations", score.MinOperations, "Minimum operations required before score is considered sufficient")
	if err := fs.Parse(args); err != nil {
		return 2
	}

	ttlDuration, err := time.ParseDuration(*ttlFlag)
	if err != nil {
		fmt.Fprintf(stderr, "invalid --ttl value: %v\n", err)
		return 2
	}

	evidencePath := resolveEvidencePath(*evidenceFlag)

	entries, err := evidence.ReadAllEntriesAtPath(evidencePath)
	if err != nil {
		fmt.Fprintf(stderr, "Error reading evidence: %v\n", err)
		return 1
	}

	filtered := filterEntries(entries, *actorFlag, *periodFlag, *sessionIDFlag)

	signalEntries, err := pipeline.EvidenceToSignalEntries(filtered)
	if err != nil {
		fmt.Fprintf(stderr, "Error converting evidence: %v\n", err)
		return 1
	}

	if *toolFlag != "" || *scopeFlag != "" {
		var toolScopeFiltered []signal.Entry
		for _, e := range signalEntries {
			if *toolFlag != "" && e.Tool != *toolFlag {
				continue
			}
			if *scopeFlag != "" && e.ScopeClass != *scopeFlag {
				continue
			}
			toolScopeFiltered = append(toolScopeFiltered, e)
		}
		signalEntries = toolScopeFiltered
	}

	totalOps := countPrescriptions(signalEntries)
	results := signal.AllSignals(signalEntries, ttlDuration)
	sc := score.ComputeWithMinOperations(results, totalOps, 0.0, *minOpsFlag)

	output := struct {
		score.Scorecard
		ActorID        string `json:"actor_id,omitempty"`
		SessionID      string `json:"session_id,omitempty"`
		Period         string `json:"period"`
		ScoringVersion string `json:"scoring_version"`
		SpecVersion    string `json:"spec_version"`
		EvidraVersion  string `json:"evidra_version"`
		GeneratedAt    string `json:"generated_at"`
	}{
		Scorecard:      sc,
		ActorID:        *actorFlag,
		SessionID:      *sessionIDFlag,
		Period:         *periodFlag,
		ScoringVersion: version.SpecVersion,
		SpecVersion:    version.SpecVersion,
		EvidraVersion:  version.Version,
		GeneratedAt:    time.Now().UTC().Format(time.RFC3339),
	}

	enc := json.NewEncoder(stdout)
	enc.SetIndent("", "  ")
	if err := enc.Encode(output); err != nil {
		fmt.Fprintf(stderr, "encode scorecard: %v\n", err)
		return 1
	}
	return 0
}

func cmdExplain(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("explain", flag.ContinueOnError)
	fs.SetOutput(stderr)
	actorFlag := fs.String("actor", "", "Actor ID to explain")
	periodFlag := fs.String("period", "30d", "Time period (e.g. 30d)")
	evidenceFlag := fs.String("evidence-dir", "", "Evidence directory")
	ttlFlag := fs.String("ttl", signal.DefaultTTL.String(), "TTL for unreported prescription detection")
	toolFlag := fs.String("tool", "", "Filter by tool name")
	scopeFlag := fs.String("scope", "", "Filter by scope class")
	sessionIDFlag := fs.String("session-id", "", "Filter by session ID")
	minOpsFlag := fs.Int("min-operations", score.MinOperations, "Minimum operations required before score is considered sufficient")
	if err := fs.Parse(args); err != nil {
		return 2
	}

	ttlDuration, err := time.ParseDuration(*ttlFlag)
	if err != nil {
		fmt.Fprintf(stderr, "invalid --ttl value: %v\n", err)
		return 2
	}

	evidencePath := resolveEvidencePath(*evidenceFlag)

	entries, err := evidence.ReadAllEntriesAtPath(evidencePath)
	if err != nil {
		fmt.Fprintf(stderr, "Error reading evidence: %v\n", err)
		return 1
	}

	filtered := filterEntries(entries, *actorFlag, *periodFlag, *sessionIDFlag)

	signalEntries, err := pipeline.EvidenceToSignalEntries(filtered)
	if err != nil {
		fmt.Fprintf(stderr, "Error converting evidence: %v\n", err)
		return 1
	}

	if *toolFlag != "" || *scopeFlag != "" {
		var toolScopeFiltered []signal.Entry
		for _, e := range signalEntries {
			if *toolFlag != "" && e.Tool != *toolFlag {
				continue
			}
			if *scopeFlag != "" && e.ScopeClass != *scopeFlag {
				continue
			}
			toolScopeFiltered = append(toolScopeFiltered, e)
		}
		signalEntries = toolScopeFiltered
	}

	totalOps := countPrescriptions(signalEntries)
	results := signal.AllSignals(signalEntries, ttlDuration)
	sc := score.ComputeWithMinOperations(results, totalOps, 0.0, *minOpsFlag)

	type SignalDetail struct {
		Signal     string         `json:"signal"`
		Count      int            `json:"count"`
		Weight     float64        `json:"weight"`
		Rate       float64        `json:"rate"`
		EntryIDs   []string       `json:"entry_ids,omitempty"`
		SubSignals map[string]int `json:"sub_signals,omitempty"`
	}

	var details []SignalDetail
	for _, r := range results {
		rate := 0.0
		if totalOps > 0 {
			rate = float64(r.Count) / float64(totalOps)
		}
		weight := score.DefaultWeights[r.Name]
		detail := SignalDetail{
			Signal:   r.Name,
			Count:    r.Count,
			Weight:   weight,
			Rate:     rate,
			EntryIDs: r.EventIDs,
		}
		if r.Name == "protocol_violation" {
			subMap := make(map[string]int)
			pvEvents := signal.DetectProtocolViolationEvents(signalEntries, ttlDuration)
			for _, ev := range pvEvents {
				subMap[ev.SubSignal]++
			}
			detail.SubSignals = subMap
		}
		details = append(details, detail)
	}

	output := struct {
		Score         float64        `json:"score"`
		Band          string         `json:"band"`
		TotalOps      int            `json:"total_operations"`
		Signals       []SignalDetail `json:"signals"`
		EvidraVersion string         `json:"evidra_version"`
		GeneratedAt   string         `json:"generated_at"`
	}{
		Score:         sc.Score,
		Band:          sc.Band,
		TotalOps:      totalOps,
		Signals:       details,
		EvidraVersion: version.Version,
		GeneratedAt:   time.Now().UTC().Format(time.RFC3339),
	}

	enc := json.NewEncoder(stdout)
	enc.SetIndent("", "  ")
	if err := enc.Encode(output); err != nil {
		fmt.Fprintf(stderr, "encode explain: %v\n", err)
		return 1
	}
	return 0
}

func cmdCompare(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("compare", flag.ContinueOnError)
	fs.SetOutput(stderr)
	actorsFlag := fs.String("actors", "", "Comma-separated actor IDs to compare")
	periodFlag := fs.String("period", "30d", "Time period (e.g. 30d)")
	evidenceFlag := fs.String("evidence-dir", "", "Evidence directory")
	toolFlag := fs.String("tool", "", "Filter by tool name")
	scopeFlag := fs.String("scope", "", "Filter by scope class")
	sessionIDFlag := fs.String("session-id", "", "Filter by session ID")
	if err := fs.Parse(args); err != nil {
		return 2
	}

	actors := strings.Split(*actorsFlag, ",")
	if len(actors) < 2 {
		fmt.Fprintln(stderr, "compare requires at least 2 actors (--actors A,B)")
		return 2
	}

	evidencePath := resolveEvidencePath(*evidenceFlag)
	entries, err := evidence.ReadAllEntriesAtPath(evidencePath)
	if err != nil {
		fmt.Fprintf(stderr, "Error reading evidence: %v\n", err)
		return 1
	}

	type actorScore struct {
		ActorID  string                `json:"actor_id"`
		Score    float64               `json:"score"`
		Band     string                `json:"band"`
		TotalOps int                   `json:"total_operations"`
		Profile  score.WorkloadProfile `json:"workload_profile"`
	}

	var scorecards []actorScore
	for _, actorID := range actors {
		filtered := filterEntries(entries, actorID, *periodFlag, *sessionIDFlag)
		signalEntries, err := pipeline.EvidenceToSignalEntries(filtered)
		if err != nil {
			fmt.Fprintf(stderr, "Error converting evidence for %s: %v\n", actorID, err)
			return 1
		}
		if *toolFlag != "" || *scopeFlag != "" {
			var toolScopeFiltered []signal.Entry
			for _, e := range signalEntries {
				if *toolFlag != "" && e.Tool != *toolFlag {
					continue
				}
				if *scopeFlag != "" && e.ScopeClass != *scopeFlag {
					continue
				}
				toolScopeFiltered = append(toolScopeFiltered, e)
			}
			signalEntries = toolScopeFiltered
		}
		totalOps := countPrescriptions(signalEntries)
		results := signal.AllSignals(signalEntries, signal.DefaultTTL)
		sc := score.Compute(results, totalOps, 0.0)
		profile := score.BuildProfile(signalEntries)

		scorecards = append(scorecards, actorScore{
			ActorID:  actorID,
			Score:    sc.Score,
			Band:     sc.Band,
			TotalOps: sc.TotalOperations,
			Profile:  profile,
		})
	}

	// Compute pairwise overlap
	var overlap float64
	if len(scorecards) >= 2 {
		overlap = score.WorkloadOverlap(scorecards[0].Profile, scorecards[1].Profile)
	}

	result := map[string]interface{}{
		"actors":           scorecards,
		"workload_overlap": overlap,
		"generated_at":     time.Now().UTC().Format(time.RFC3339),
	}

	enc := json.NewEncoder(stdout)
	enc.SetIndent("", "  ")
	if err := enc.Encode(result); err != nil {
		fmt.Fprintf(stderr, "encode comparison: %v\n", err)
		return 1
	}
	return 0
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

func parsePrescribeFlags(args []string, stderr io.Writer) (prescribeFlags, int) {
	fs := flag.NewFlagSet("prescribe", flag.ContinueOnError)
	fs.SetOutput(stderr)
	artifactFlag := fs.String("artifact", "", "Path to artifact file (YAML or JSON)")
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
	if *artifactFlag == "" || *toolFlag == "" {
		fmt.Fprintln(stderr, "prescribe requires --artifact and --tool")
		return prescribeFlags{}, 2
	}

	return prescribeFlags{
		artifactPath:        *artifactFlag,
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

func cmdReport(args []string, stdout, stderr io.Writer) int {
	opts, code := parseReportFlags(args, stderr)
	if code != 0 {
		return code
	}

	cmd, err := prepareReportCommand(opts)
	if err != nil {
		fmt.Fprintf(stderr, "%v\n", err)
		return 1
	}

	reportOut, err := cmd.service.Report(context.Background(), cmd.input)
	if err != nil {
		if lifecycle.ErrorCode(err) == lifecycle.ErrCodeNotFound {
			fmt.Fprintf(stderr, "prescription %s not found in evidence\n", opts.prescriptionID)
			return 1
		}
		fmt.Fprintf(stderr, "report: %v\n", err)
		return 1
	}

	result := map[string]interface{}{
		"ok":              true,
		"report_id":       reportOut.ReportID,
		"prescription_id": opts.prescriptionID,
		"exit_code":       opts.exitCode,
		"verdict":         evidence.VerdictFromExitCode(opts.exitCode),
	}
	return writeJSON(stdout, stderr, "encode report", result)
}

type reportFlags struct {
	prescriptionID string
	exitCode       int
	evidenceDir    string
	actorID        string
	artifactDigest string
	externalRefs   string
	sessionID      string
	operationID    string
	signingKey     string
	signingKeyPath string
	signingMode    string
}

type reportCommand struct {
	service *lifecycle.Service
	input   lifecycle.ReportInput
}

func parseReportFlags(args []string, stderr io.Writer) (reportFlags, int) {
	fs := flag.NewFlagSet("report", flag.ContinueOnError)
	fs.SetOutput(stderr)
	prescriptionFlag := fs.String("prescription", "", "Prescription event ID")
	exitCodeFlag := fs.Int("exit-code", 0, "Exit code of the operation")
	evidenceFlag := fs.String("evidence-dir", "", "Evidence directory")
	actorFlag := fs.String("actor", "", "Actor ID")
	artifactDigestFlag := fs.String("artifact-digest", "", "Artifact digest for drift detection")
	externalRefsFlag := fs.String("external-refs", "", "External references JSON array (e.g. '[{\"type\":\"github_run\",\"id\":\"123\"}]')")
	sessionIDFlag := fs.String("session-id", "", "Session/run boundary ID")
	operationIDFlag := fs.String("operation-id", "", "Operation identifier")
	signingKeyFlag := fs.String("signing-key", "", "Base64-encoded Ed25519 signing key")
	signingKeyPathFlag := fs.String("signing-key-path", "", "Path to PEM-encoded Ed25519 signing key")
	signingModeFlag := fs.String("signing-mode", "", "Signing mode: strict (default) or optional")
	if err := fs.Parse(args); err != nil {
		return reportFlags{}, 2
	}
	if *prescriptionFlag == "" {
		fmt.Fprintln(stderr, "report requires --prescription")
		return reportFlags{}, 2
	}

	return reportFlags{
		prescriptionID: *prescriptionFlag,
		exitCode:       *exitCodeFlag,
		evidenceDir:    *evidenceFlag,
		actorID:        *actorFlag,
		artifactDigest: *artifactDigestFlag,
		externalRefs:   *externalRefsFlag,
		sessionID:      *sessionIDFlag,
		operationID:    *operationIDFlag,
		signingKey:     *signingKeyFlag,
		signingKeyPath: *signingKeyPathFlag,
		signingMode:    *signingModeFlag,
	}, 0
}

func prepareReportCommand(opts reportFlags) (reportCommand, error) {
	svc, _, _, err := newLifecycleServiceForCommand(opts.evidenceDir, opts.signingKey, opts.signingKeyPath, opts.signingMode)
	if err != nil {
		return reportCommand{}, err
	}

	externalRefs, err := parseExternalRefsFlag(opts.externalRefs)
	if err != nil {
		return reportCommand{}, err
	}

	actorID := opts.actorID
	if actorID == "" {
		actorID = "cli"
	}
	actor := evidence.Actor{Type: "cli", ID: actorID, Provenance: "cli"}

	return reportCommand{
		service: svc,
		input: lifecycle.ReportInput{
			PrescriptionID: opts.prescriptionID,
			ExitCode:       opts.exitCode,
			ArtifactDigest: opts.artifactDigest,
			Actor:          actor,
			ExternalRefs:   externalRefs,
			SessionID:      opts.sessionID,
			OperationID:    opts.operationID,
		},
	}, nil
}

func newLifecycleServiceForCommand(evidenceDir, signingKey, signingKeyPath, signingMode string) (*lifecycle.Service, string, evidence.Signer, error) {
	writeMode, err := config.ResolveEvidenceWriteMode("")
	if err != nil {
		return nil, "", nil, fmt.Errorf("resolve evidence write mode: %w", err)
	}

	signer, err := resolveSigner(signingKey, signingKeyPath, signingMode)
	if err != nil {
		return nil, "", nil, fmt.Errorf("resolve signer: %w", err)
	}

	evidencePath := resolveEvidencePath(evidenceDir)
	svc := lifecycle.NewService(lifecycle.Options{
		EvidencePath:     evidencePath,
		Signer:           signer,
		BestEffortWrites: writeMode == config.EvidenceWriteModeBestEffort,
	})
	return svc, evidencePath, signer, nil
}

func parseCanonicalActionFlag(raw string) (*canon.CanonicalAction, error) {
	if raw == "" {
		return nil, nil
	}

	preCanon := &canon.CanonicalAction{}
	if err := json.Unmarshal([]byte(raw), preCanon); err != nil {
		return nil, fmt.Errorf("parse --canonical-action: %w", err)
	}
	return preCanon, nil
}

func parseExternalRefsFlag(raw string) ([]evidence.ExternalRef, error) {
	if raw == "" {
		return nil, nil
	}

	var externalRefs []evidence.ExternalRef
	if err := json.Unmarshal([]byte(raw), &externalRefs); err != nil {
		return nil, fmt.Errorf("parse --external-refs: %w", err)
	}
	return externalRefs, nil
}

// resolveSigner creates a Signer from explicit flags or environment variables.
// Returns an error when mode is strict and no key is configured.
func resolveSigner(keyBase64, keyPath, modeRaw string) (evidence.Signer, error) {
	mode, err := config.ResolveSigningMode(modeRaw)
	if err != nil {
		return nil, err
	}

	if keyBase64 == "" {
		keyBase64 = strings.TrimSpace(os.Getenv("EVIDRA_SIGNING_KEY"))
	}
	if keyPath == "" {
		keyPath = strings.TrimSpace(os.Getenv("EVIDRA_SIGNING_KEY_PATH"))
	}

	noKey := keyBase64 == "" && keyPath == ""
	if noKey && mode == config.SigningModeStrict {
		return nil, fmt.Errorf("signing key required in strict mode: set --signing-key, --signing-key-path, EVIDRA_SIGNING_KEY, EVIDRA_SIGNING_KEY_PATH, or use --signing-mode optional")
	}

	s, err := ievsigner.NewSigner(ievsigner.SignerConfig{
		KeyBase64: keyBase64,
		KeyPath:   keyPath,
		DevMode:   noKey && mode == config.SigningModeOptional,
	})
	if err != nil {
		return nil, fmt.Errorf("resolveSigner: %w", err)
	}
	return s, nil
}

func resolveEvidencePath(explicit string) string {
	if explicit != "" {
		return explicit
	}
	if v := strings.TrimSpace(os.Getenv("EVIDRA_EVIDENCE_DIR")); v != "" {
		return v
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return filepath.Join(os.TempDir(), ".evidra", "evidence")
	}
	return filepath.Join(home, ".evidra", "evidence")
}

func filterEntries(entries []evidence.EvidenceEntry, actor, period, sessionID string) []evidence.EvidenceEntry {
	cutoff := parsePeriodCutoff(period)
	var filtered []evidence.EvidenceEntry
	for _, e := range entries {
		if actor != "" && e.Actor.ID != actor {
			continue
		}
		if !cutoff.IsZero() && e.Timestamp.Before(cutoff) {
			continue
		}
		if sessionID != "" && e.SessionID != sessionID {
			continue
		}
		filtered = append(filtered, e)
	}
	return filtered
}

func parsePeriodCutoff(period string) time.Time {
	if period == "" {
		return time.Time{}
	}
	now := time.Now().UTC()
	if len(period) < 2 {
		return time.Time{}
	}
	unit := period[len(period)-1]
	val := 0
	_, _ = fmt.Sscanf(period[:len(period)-1], "%d", &val)
	if val <= 0 {
		return time.Time{}
	}
	switch unit {
	case 'd':
		return now.AddDate(0, 0, -val)
	case 'h':
		return now.Add(-time.Duration(val) * time.Hour)
	default:
		return time.Time{}
	}
}

func countPrescriptions(entries []signal.Entry) int {
	count := 0
	for _, e := range entries {
		if e.IsPrescription {
			count++
		}
	}
	return count
}

func cmdValidate(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("validate", flag.ContinueOnError)
	fs.SetOutput(stderr)
	evidenceFlag := fs.String("evidence-dir", "", "Evidence directory")
	pubKeyFlag := fs.String("public-key", "", "PEM file with Ed25519 public key (enables signature verification)")
	if err := fs.Parse(args); err != nil {
		return 2
	}

	evidencePath := resolveEvidencePath(*evidenceFlag)

	// Always validate hash chain
	if err := evidence.ValidateChainAtPath(evidencePath); err != nil {
		fmt.Fprintf(stderr, "chain validation failed: %v\n", err)
		return 1
	}

	if *pubKeyFlag != "" {
		pubKey, err := ievsigner.LoadPublicKeyPEM(*pubKeyFlag)
		if err != nil {
			fmt.Fprintf(stderr, "load public key: %v\n", err)
			return 1
		}
		if err := evidence.ValidateChainWithSignatures(evidencePath, pubKey); err != nil {
			fmt.Fprintf(stderr, "signature validation failed: %v\n", err)
			return 1
		}
		fmt.Fprintln(stdout, "chain valid: hashes and signatures verified")
	} else {
		fmt.Fprintln(stdout, "chain valid: hashes verified (no public key provided, signatures not checked)")
	}
	return 0
}

func cmdIngestFindings(args []string, stdout, stderr io.Writer) int {
	opts, code := parseIngestFindingsFlags(args, stderr)
	if code != 0 {
		return code
	}

	cmd, err := prepareIngestFindingsCommand(opts)
	if err != nil {
		fmt.Fprintf(stderr, "%v\n", err)
		return 1
	}

	written := appendFindingsAsEvidence(cmd.findings, findingAppendConfig{
		evidencePath:   cmd.evidencePath,
		signer:         cmd.signer,
		sessionID:      cmd.sessionID,
		traceID:        cmd.sessionID,
		actor:          cmd.actor,
		artifactDigest: cmd.artifactDigest,
	}, stderr)

	result := map[string]interface{}{
		"ok":              true,
		"findings_count":  written,
		"artifact_digest": cmd.artifactDigest,
	}
	return writeJSON(stdout, stderr, "encode result", result)
}

type ingestFindingsFlags struct {
	sarifPath      string
	artifactPath   string
	toolVersion    string
	evidenceDir    string
	actorID        string
	sessionID      string
	signingKey     string
	signingKeyPath string
	signingMode    string
}

type ingestFindingsCommand struct {
	findings       []evidence.FindingPayload
	evidencePath   string
	artifactDigest string
	actor          evidence.Actor
	sessionID      string
	signer         evidence.Signer
}

func parseIngestFindingsFlags(args []string, stderr io.Writer) (ingestFindingsFlags, int) {
	fs := flag.NewFlagSet("ingest-findings", flag.ContinueOnError)
	fs.SetOutput(stderr)
	sarifFlag := fs.String("sarif", "", "Path to SARIF scanner report")
	artifactFlag := fs.String("artifact", "", "Path to artifact file (for artifact_digest linking)")
	toolVersionFlag := fs.String("tool-version", "", "Override tool version for all ingested findings")
	evidenceFlag := fs.String("evidence-dir", "", "Evidence directory")
	actorFlag := fs.String("actor", "", "Actor ID")
	sessionIDFlag := fs.String("session-id", "", "Session/run boundary ID")
	signingKeyFlag := fs.String("signing-key", "", "Base64-encoded Ed25519 signing key")
	signingKeyPathFlag := fs.String("signing-key-path", "", "Path to PEM-encoded Ed25519 signing key")
	signingModeFlag := fs.String("signing-mode", "", "Signing mode: strict (default) or optional")
	if err := fs.Parse(args); err != nil {
		return ingestFindingsFlags{}, 2
	}
	if *sarifFlag == "" {
		fmt.Fprintln(stderr, "ingest-findings requires --sarif")
		return ingestFindingsFlags{}, 2
	}

	return ingestFindingsFlags{
		sarifPath:      *sarifFlag,
		artifactPath:   *artifactFlag,
		toolVersion:    *toolVersionFlag,
		evidenceDir:    *evidenceFlag,
		actorID:        *actorFlag,
		sessionID:      *sessionIDFlag,
		signingKey:     *signingKeyFlag,
		signingKeyPath: *signingKeyPathFlag,
		signingMode:    *signingModeFlag,
	}, 0
}

func prepareIngestFindingsCommand(opts ingestFindingsFlags) (ingestFindingsCommand, error) {
	sessionID := opts.sessionID
	if sessionID == "" {
		sessionID = evidence.GenerateSessionID()
	}

	signer, err := resolveSigner(opts.signingKey, opts.signingKeyPath, opts.signingMode)
	if err != nil {
		return ingestFindingsCommand{}, fmt.Errorf("resolve signer: %w", err)
	}

	findings, err := loadSARIFFindings(opts.sarifPath, "sarif")
	if err != nil {
		return ingestFindingsCommand{}, err
	}
	if opts.toolVersion != "" {
		for i := range findings {
			findings[i].ToolVersion = opts.toolVersion
		}
	}

	artifactDigest := ""
	if opts.artifactPath != "" {
		artifactData, err := os.ReadFile(opts.artifactPath)
		if err != nil {
			return ingestFindingsCommand{}, fmt.Errorf("read artifact: %w", err)
		}
		artifactDigest = canon.ComputeArtifactDigest(artifactData)
	}

	actorID := opts.actorID
	if actorID == "" {
		actorID = "cli"
	}

	return ingestFindingsCommand{
		findings:       findings,
		evidencePath:   resolveEvidencePath(opts.evidenceDir),
		artifactDigest: artifactDigest,
		actor: evidence.Actor{
			Type:       "cli",
			ID:         actorID,
			Provenance: "cli",
		},
		sessionID: sessionID,
		signer:    signer,
	}, nil
}

func loadSARIFFindings(path, sourceLabel string) ([]evidence.FindingPayload, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", sourceLabel, err)
	}

	findings, err := sarif.Parse(data)
	if err != nil {
		return nil, fmt.Errorf("parse %s: %w", sourceLabel, err)
	}
	return findings, nil
}

type findingAppendConfig struct {
	evidencePath   string
	signer         evidence.Signer
	sessionID      string
	operationID    string
	attempt        int
	traceID        string
	actor          evidence.Actor
	artifactDigest string
}

func appendFindingsAsEvidence(findings []evidence.FindingPayload, cfg findingAppendConfig, stderr io.Writer) int {
	traceID := cfg.traceID
	if traceID == "" {
		traceID = cfg.sessionID
	}

	written := 0
	for _, finding := range findings {
		findingPayload, _ := json.Marshal(finding)
		lastHash, _ := evidence.LastHashAtPath(cfg.evidencePath)
		entry, err := evidence.BuildEntry(evidence.EntryBuildParams{
			Type:           evidence.EntryTypeFinding,
			SessionID:      cfg.sessionID,
			OperationID:    cfg.operationID,
			Attempt:        cfg.attempt,
			TraceID:        traceID,
			Actor:          cfg.actor,
			ArtifactDigest: cfg.artifactDigest,
			Payload:        findingPayload,
			PreviousHash:   lastHash,
			SpecVersion:    version.SpecVersion,
			AdapterVersion: version.Version,
			Signer:         cfg.signer,
		})
		if err != nil {
			fmt.Fprintf(stderr, "warning: build finding entry failed for rule %s: %v\n", finding.RuleID, err)
			continue
		}
		if err := evidence.AppendEntryAtPath(cfg.evidencePath, entry); err != nil {
			fmt.Fprintf(stderr, "warning: write finding entry failed for rule %s: %v\n", finding.RuleID, err)
			continue
		}
		written++
	}
	return written
}

func writeJSON(stdout, stderr io.Writer, context string, payload interface{}) int {
	enc := json.NewEncoder(stdout)
	enc.SetIndent("", "  ")
	if err := enc.Encode(payload); err != nil {
		fmt.Fprintf(stderr, "%s: %v\n", context, err)
		return 1
	}
	return 0
}

// forwardEvidence resolves the operating mode and, if online, best-effort
// forwards session evidence entries to the Evidra API.
func forwardEvidence(url, apiKey string, offline, fallbackOffline bool, timeout time.Duration, evidencePath, sessionID string, stderr io.Writer) {
	fallbackPolicy := ""
	if fallbackOffline {
		fallbackPolicy = "offline"
	}
	if v := os.Getenv("EVIDRA_FALLBACK"); v != "" && fallbackPolicy == "" {
		fallbackPolicy = v
	}

	resolved, err := mode.Resolve(mode.Config{
		URL:            url,
		APIKey:         apiKey,
		FallbackPolicy: fallbackPolicy,
		ForceOffline:   offline,
		Timeout:        timeout,
	})
	if err != nil {
		fmt.Fprintf(stderr, "warning: mode resolve: %v\n", err)
		return
	}
	if !resolved.IsOnline {
		return
	}

	entries, err := evidence.ReadAllEntriesAtPath(evidencePath)
	if err != nil {
		fmt.Fprintf(stderr, "warning: read evidence for forwarding: %v\n", err)
		return
	}

	var toForward []json.RawMessage
	for _, e := range entries {
		if sessionID != "" && e.SessionID != sessionID {
			continue
		}
		raw, marshalErr := json.Marshal(e)
		if marshalErr != nil {
			continue
		}
		toForward = append(toForward, raw)
	}
	if len(toForward) == 0 {
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	if len(toForward) == 1 {
		if _, fwdErr := resolved.Client.Forward(ctx, toForward[0]); fwdErr != nil {
			fmt.Fprintf(stderr, "warning: forward evidence: %v\n", fwdErr)
		}
	} else {
		if _, fwdErr := resolved.Client.Batch(ctx, toForward); fwdErr != nil {
			fmt.Fprintf(stderr, "warning: batch forward evidence: %v\n", fwdErr)
		}
	}
}

func printUsage(w io.Writer) {
	fmt.Fprintln(w, "evidra-benchmark — reliability benchmark for infrastructure automation")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "COMMANDS:")
	fmt.Fprintln(w, "  scorecard         Generate reliability scorecard for an actor")
	fmt.Fprintln(w, "  explain           Explain signals contributing to a score")
	fmt.Fprintln(w, "  compare           Compare reliability scores between actors")
	fmt.Fprintln(w, "  run               Execute command live and record lifecycle outcome")
	fmt.Fprintln(w, "  prescribe         Analyze artifact before execution")
	fmt.Fprintln(w, "  report            Record outcome after execution")
	fmt.Fprintln(w, "  record            Ingest completed automation operation from structured input")
	fmt.Fprintln(w, "  validate          Validate evidence chain integrity and signatures")
	fmt.Fprintln(w, "  ingest-findings   Ingest SARIF scanner findings as evidence entries")
	fmt.Fprintln(w, "  prompts           Prompt contract generation and verification")
	fmt.Fprintln(w, "  detectors         Detector registry command group")
	fmt.Fprintln(w, "  keygen            Generate Ed25519 signing keypair")
	fmt.Fprintln(w, "  version           Print version information")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Run 'evidra <command> --help' for command-specific flags.")
}
