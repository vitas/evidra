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
	case "prescribe":
		return cmdPrescribe(args[1:], stdout, stderr)
	case "report":
		return cmdReport(args[1:], stdout, stderr)
	case "validate":
		return cmdValidate(args[1:], stdout, stderr)
	case "ingest-findings":
		return cmdIngestFindings(args[1:], stdout, stderr)
	case "keygen":
		return cmdKeygen(args[1:], stdout, stderr)
	case "benchmark":
		return cmdBenchmark(args[1:], stdout, stderr)
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
	sc := score.Compute(results, totalOps, 0.0)

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
	sc := score.Compute(results, totalOps, 0.0)

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
		return 2
	}

	sessionID := *sessionIDFlag

	if *artifactFlag == "" || *toolFlag == "" {
		fmt.Fprintln(stderr, "prescribe requires --artifact and --tool")
		return 2
	}

	signer, err := resolveSigner(*signingKeyFlag, *signingKeyPathFlag, *signingModeFlag)
	if err != nil {
		fmt.Fprintf(stderr, "resolve signer: %v\n", err)
		return 1
	}

	data, err := os.ReadFile(*artifactFlag)
	if err != nil {
		fmt.Fprintf(stderr, "read artifact: %v\n", err)
		return 1
	}

	actorID := *actorFlag
	if actorID == "" {
		actorID = "cli"
	}
	actor := evidence.Actor{Type: "cli", ID: actorID, Provenance: "cli"}

	evidencePath := resolveEvidencePath(*evidenceFlag)
	var preCanon *canon.CanonicalAction
	if *canonicalActionFlag != "" {
		preCanon = &canon.CanonicalAction{}
		if err := json.Unmarshal([]byte(*canonicalActionFlag), preCanon); err != nil {
			fmt.Fprintf(stderr, "parse --canonical-action: %v\n", err)
			return 1
		}
	}

	svc := lifecycle.NewService(lifecycle.Options{
		EvidencePath:     evidencePath,
		Signer:           signer,
		BestEffortWrites: evidenceWriteBestEffortEnabled(),
	})
	prescOut, err := svc.Prescribe(context.Background(), lifecycle.PrescribeInput{
		Actor:           actor,
		Tool:            *toolFlag,
		Operation:       *operationFlag,
		RawArtifact:     data,
		Environment:     *envFlag,
		CanonicalAction: preCanon,
		SessionID:       sessionID,
		OperationID:     *operationIDFlag,
		Attempt:         *attemptFlag,
	})
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

	// Write scanner findings as evidence entries.
	if *scannerFlag != "" {
		sarifData, err := os.ReadFile(*scannerFlag)
		if err != nil {
			fmt.Fprintf(stderr, "read scanner report: %v\n", err)
			return 1
		}
		findings, err := sarif.Parse(sarifData)
		if err != nil {
			fmt.Fprintf(stderr, "parse scanner report: %v\n", err)
			return 1
		}
		writtenFindings := 0
		for _, f := range findings {
			findingPayload, _ := json.Marshal(f)
			lastHash, _ := evidence.LastHashAtPath(evidencePath)
			findingEntry, err := evidence.BuildEntry(evidence.EntryBuildParams{
				Type:           evidence.EntryTypeFinding,
				SessionID:      prescOut.SessionID,
				OperationID:    *operationIDFlag,
				Attempt:        *attemptFlag,
				TraceID:        prescOut.TraceID,
				Actor:          prescOut.Actor,
				ArtifactDigest: prescOut.ArtifactDigest,
				Payload:        findingPayload,
				PreviousHash:   lastHash,
				SpecVersion:    version.SpecVersion,
				AdapterVersion: version.Version,
				Signer:         signer,
			})
			if err != nil {
				fmt.Fprintf(stderr, "warning: build finding entry failed for rule %s: %v\n", f.RuleID, err)
				continue
			}
			if err := evidence.AppendEntryAtPath(evidencePath, findingEntry); err != nil {
				fmt.Fprintf(stderr, "warning: write finding entry failed for rule %s: %v\n", f.RuleID, err)
				continue
			}
			writtenFindings++
		}
		result["findings_count"] = writtenFindings
	}

	enc := json.NewEncoder(stdout)
	enc.SetIndent("", "  ")
	if err := enc.Encode(result); err != nil {
		fmt.Fprintf(stderr, "encode prescription: %v\n", err)
		return 1
	}
	return 0
}

func cmdReport(args []string, stdout, stderr io.Writer) int {
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
		return 2
	}

	sessionID := *sessionIDFlag

	if *prescriptionFlag == "" {
		fmt.Fprintln(stderr, "report requires --prescription")
		return 2
	}

	signer, err := resolveSigner(*signingKeyFlag, *signingKeyPathFlag, *signingModeFlag)
	if err != nil {
		fmt.Fprintf(stderr, "resolve signer: %v\n", err)
		return 1
	}

	evidencePath := resolveEvidencePath(*evidenceFlag)

	actorID := *actorFlag
	if actorID == "" {
		actorID = "cli"
	}

	actor := evidence.Actor{Type: "cli", ID: actorID, Provenance: "cli"}

	var externalRefs []evidence.ExternalRef
	if *externalRefsFlag != "" {
		if err := json.Unmarshal([]byte(*externalRefsFlag), &externalRefs); err != nil {
			fmt.Fprintf(stderr, "parse --external-refs: %v\n", err)
			return 1
		}
	}

	svc := lifecycle.NewService(lifecycle.Options{
		EvidencePath:     evidencePath,
		Signer:           signer,
		BestEffortWrites: evidenceWriteBestEffortEnabled(),
	})
	reportOut, err := svc.Report(context.Background(), lifecycle.ReportInput{
		PrescriptionID: *prescriptionFlag,
		ExitCode:       *exitCodeFlag,
		ArtifactDigest: *artifactDigestFlag,
		Actor:          actor,
		ExternalRefs:   externalRefs,
		SessionID:      sessionID,
		OperationID:    *operationIDFlag,
	})
	if err != nil {
		if lifecycle.ErrorCode(err) == lifecycle.ErrCodeNotFound {
			fmt.Fprintf(stderr, "prescription %s not found in evidence\n", *prescriptionFlag)
			return 1
		}
		fmt.Fprintf(stderr, "report: %v\n", err)
		return 1
	}

	result := map[string]interface{}{
		"ok":              true,
		"report_id":       reportOut.ReportID,
		"prescription_id": *prescriptionFlag,
		"exit_code":       *exitCodeFlag,
		"verdict":         evidence.VerdictFromExitCode(*exitCodeFlag),
	}
	enc := json.NewEncoder(stdout)
	enc.SetIndent("", "  ")
	if err := enc.Encode(result); err != nil {
		fmt.Fprintf(stderr, "encode report: %v\n", err)
		return 1
	}
	return 0
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

func evidenceWriteBestEffortEnabled() bool {
	return strings.EqualFold(strings.TrimSpace(os.Getenv("EVIDRA_EVIDENCE_WRITE_MODE")), "best_effort")
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
		return 2
	}

	sessionID := *sessionIDFlag
	if sessionID == "" {
		sessionID = evidence.GenerateSessionID()
	}

	if *sarifFlag == "" {
		fmt.Fprintln(stderr, "ingest-findings requires --sarif")
		return 2
	}

	signer, err := resolveSigner(*signingKeyFlag, *signingKeyPathFlag, *signingModeFlag)
	if err != nil {
		fmt.Fprintf(stderr, "resolve signer: %v\n", err)
		return 1
	}

	sarifData, err := os.ReadFile(*sarifFlag)
	if err != nil {
		fmt.Fprintf(stderr, "read sarif: %v\n", err)
		return 1
	}
	findings, err := sarif.Parse(sarifData)
	if err != nil {
		fmt.Fprintf(stderr, "parse sarif: %v\n", err)
		return 1
	}

	// Override tool_version from CLI flag if provided.
	if *toolVersionFlag != "" {
		for i := range findings {
			findings[i].ToolVersion = *toolVersionFlag
		}
	}

	// Compute artifact digest if artifact provided
	var artifactDigest string
	if *artifactFlag != "" {
		artifactData, err := os.ReadFile(*artifactFlag)
		if err != nil {
			fmt.Fprintf(stderr, "read artifact: %v\n", err)
			return 1
		}
		artifactDigest = canon.ComputeArtifactDigest(artifactData)
	}

	evidencePath := resolveEvidencePath(*evidenceFlag)
	actorID := *actorFlag
	if actorID == "" {
		actorID = "cli"
	}
	actor := evidence.Actor{Type: "cli", ID: actorID, Provenance: "cli"}

	traceID := sessionID
	written := 0
	for _, f := range findings {
		findingPayload, _ := json.Marshal(f)
		lastHash, _ := evidence.LastHashAtPath(evidencePath)
		entry, err := evidence.BuildEntry(evidence.EntryBuildParams{
			Type:           evidence.EntryTypeFinding,
			SessionID:      sessionID,
			TraceID:        traceID,
			Actor:          actor,
			ArtifactDigest: artifactDigest,
			Payload:        findingPayload,
			PreviousHash:   lastHash,
			SpecVersion:    version.SpecVersion,
			AdapterVersion: version.Version,
			Signer:         signer,
		})
		if err != nil {
			fmt.Fprintf(stderr, "warning: build finding entry failed for rule %s: %v\n", f.RuleID, err)
			continue
		}
		if err := evidence.AppendEntryAtPath(evidencePath, entry); err != nil {
			fmt.Fprintf(stderr, "warning: write finding entry failed for rule %s: %v\n", f.RuleID, err)
			continue
		}
		written++
	}

	result := map[string]interface{}{
		"ok":              true,
		"findings_count":  written,
		"artifact_digest": artifactDigest,
	}
	enc := json.NewEncoder(stdout)
	enc.SetIndent("", "  ")
	if err := enc.Encode(result); err != nil {
		fmt.Fprintf(stderr, "encode result: %v\n", err)
		return 1
	}
	return 0
}

func printUsage(w io.Writer) {
	fmt.Fprintln(w, "evidra-benchmark — flight recorder for infrastructure automation")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "COMMANDS:")
	fmt.Fprintln(w, "  scorecard         Generate reliability scorecard for an actor")
	fmt.Fprintln(w, "  explain           Explain signals contributing to a score")
	fmt.Fprintln(w, "  compare           Compare reliability scores between actors")
	fmt.Fprintln(w, "  prescribe         Analyze artifact before execution")
	fmt.Fprintln(w, "  report            Record outcome after execution")
	fmt.Fprintln(w, "  validate          Validate evidence chain integrity and signatures")
	fmt.Fprintln(w, "  ingest-findings   Ingest SARIF scanner findings as evidence entries")
	fmt.Fprintln(w, "  benchmark         Benchmark dataset command group (stub)")
	fmt.Fprintln(w, "  keygen            Generate Ed25519 signing keypair")
	fmt.Fprintln(w, "  version           Print version information")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Run 'evidra <command> --help' for command-specific flags.")
}
