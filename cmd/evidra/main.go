package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"samebits.com/evidra-benchmark/internal/canon"
	"samebits.com/evidra-benchmark/internal/pipeline"
	"samebits.com/evidra-benchmark/internal/risk"
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
	ttlFlag := fs.String("ttl", "5m", "TTL for unreported prescription detection")
	toolFlag := fs.String("tool", "", "Filter by tool name")
	scopeFlag := fs.String("scope", "", "Filter by scope class")
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

	filtered := filterEntries(entries, *actorFlag, *periodFlag)

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
	results := signal.AllSignals(signalEntries)

	ttlEvents := signal.DetectUnreported(signalEntries, ttlDuration)
	for i, r := range results {
		if r.Name == "protocol_violation" {
			for _, ev := range ttlEvents {
				results[i].Count++
				results[i].EventIDs = append(results[i].EventIDs, ev.EntryRef)
			}
			break
		}
	}

	sc := score.Compute(results, totalOps)

	output := struct {
		score.Scorecard
		ActorID        string `json:"actor_id,omitempty"`
		Period         string `json:"period"`
		ScoringVersion string `json:"scoring_version"`
		SpecVersion    string `json:"spec_version"`
		EvidraVersion  string `json:"evidra_version"`
		GeneratedAt    string `json:"generated_at"`
	}{
		Scorecard:      sc,
		ActorID:        *actorFlag,
		Period:         *periodFlag,
		ScoringVersion: "0.3.0",
		SpecVersion:    "0.3.0",
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
	ttlFlag := fs.String("ttl", "5m", "TTL for unreported prescription detection")
	toolFlag := fs.String("tool", "", "Filter by tool name")
	scopeFlag := fs.String("scope", "", "Filter by scope class")
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

	filtered := filterEntries(entries, *actorFlag, *periodFlag)

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
	results := signal.AllSignals(signalEntries)

	ttlEvents := signal.DetectUnreported(signalEntries, ttlDuration)
	for i, r := range results {
		if r.Name == "protocol_violation" {
			for _, ev := range ttlEvents {
				results[i].Count++
				results[i].EventIDs = append(results[i].EventIDs, ev.EntryRef)
			}
			break
		}
	}

	sc := score.Compute(results, totalOps)

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
			pvEvents := signal.DetectProtocolViolationEvents(signalEntries)
			for _, ev := range pvEvents {
				subMap[ev.SubSignal]++
			}
			for _, ev := range ttlEvents {
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
		filtered := filterEntries(entries, actorID, *periodFlag)
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
		results := signal.AllSignals(signalEntries)
		sc := score.Compute(results, totalOps)
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
	if err := fs.Parse(args); err != nil {
		return 2
	}

	if *artifactFlag == "" || *toolFlag == "" {
		fmt.Fprintln(stderr, "prescribe requires --artifact and --tool")
		return 2
	}

	data, err := os.ReadFile(*artifactFlag)
	if err != nil {
		fmt.Fprintf(stderr, "read artifact: %v\n", err)
		return 1
	}

	cr := canon.Canonicalize(*toolFlag, *operationFlag, *envFlag, data)

	if *canonicalActionFlag != "" {
		var preCanon canon.CanonicalAction
		if err := json.Unmarshal([]byte(*canonicalActionFlag), &preCanon); err != nil {
			fmt.Fprintf(stderr, "parse --canonical-action: %v\n", err)
			return 1
		}
		// Keep tool from flag if not in pre-canon
		if preCanon.Tool == "" {
			preCanon.Tool = *toolFlag
		}
		if preCanon.Operation == "" {
			preCanon.Operation = *operationFlag
		}
		cr.CanonicalAction = preCanon
		cr.RawAction, _ = json.Marshal(preCanon)
		cr.IntentDigest = canon.ComputeIntentDigest(preCanon)
		cr.CanonVersion = "external"
		cr.ParseError = nil
	}

	riskTags := risk.RunAll(cr.CanonicalAction, data)
	riskLevel := risk.ElevateRiskLevel(
		risk.RiskLevel(cr.CanonicalAction.OperationClass, cr.CanonicalAction.ScopeClass),
		riskTags,
	)

	actorID := *actorFlag
	if actorID == "" {
		actorID = "cli"
	}
	actor := evidence.Actor{Type: "cli", ID: actorID, Provenance: "cli"}

	evidencePath := resolveEvidencePath(*evidenceFlag)

	// Handle parse error
	if cr.ParseError != nil {
		// Write canon failure entry
		failPayload, _ := json.Marshal(evidence.CanonFailurePayload{
			ErrorCode:    "parse_error",
			ErrorMessage: cr.ParseError.Error(),
			Adapter:      cr.CanonVersion,
			RawDigest:    cr.ArtifactDigest,
		})
		lastHash, _ := evidence.LastHashAtPath(evidencePath)
		entry, buildErr := evidence.BuildEntry(evidence.EntryBuildParams{
			Type:           evidence.EntryTypeCanonFailure,
			TraceID:        evidence.GenerateTraceID(),
			Actor:          actor,
			ArtifactDigest: cr.ArtifactDigest,
			Payload:        failPayload,
			PreviousHash:   lastHash,
			SpecVersion:    "0.3.0",
			AdapterVersion: version.Version,
		})
		if buildErr == nil {
			evidence.AppendEntryAtPath(evidencePath, entry)
		}

		result := map[string]interface{}{
			"ok":          false,
			"parse_error": cr.ParseError.Error(),
		}
		enc := json.NewEncoder(stdout)
		enc.SetIndent("", "  ")
		enc.Encode(result)
		return 1
	}

	// Build prescription payload
	traceID := evidence.GenerateTraceID()
	var canonActionJSON json.RawMessage
	if cr.RawAction != nil {
		canonActionJSON = cr.RawAction
	} else {
		canonActionJSON, _ = json.Marshal(cr.CanonicalAction)
	}

	canonSource := "adapter"
	if *canonicalActionFlag != "" {
		canonSource = "external"
	}

	prescPayload := evidence.PrescriptionPayload{
		PrescriptionID:  traceID,
		CanonicalAction: canonActionJSON,
		RiskLevel:       riskLevel,
		RiskTags:        riskTags,
		TTLMs:           evidence.DefaultTTLMs,
		CanonSource:     canonSource,
	}
	payloadJSON, _ := json.Marshal(prescPayload)

	lastHash, _ := evidence.LastHashAtPath(evidencePath)
	entry, err := evidence.BuildEntry(evidence.EntryBuildParams{
		Type:           evidence.EntryTypePrescribe,
		TraceID:        traceID,
		Actor:          actor,
		IntentDigest:   cr.IntentDigest,
		ArtifactDigest: cr.ArtifactDigest,
		Payload:        payloadJSON,
		PreviousHash:   lastHash,
		SpecVersion:    "0.3.0",
		CanonVersion:   cr.CanonVersion,
		AdapterVersion: version.Version,
	})
	if err != nil {
		fmt.Fprintf(stderr, "build entry: %v\n", err)
		return 1
	}

	if err := evidence.AppendEntryAtPath(evidencePath, entry); err != nil {
		fmt.Fprintf(stderr, "write evidence: %v\n", err)
		return 1
	}

	result := map[string]interface{}{
		"ok":              true,
		"prescription_id": entry.EntryID,
		"risk_level":      riskLevel,
		"risk_tags":       riskTags,
		"artifact_digest": cr.ArtifactDigest,
		"intent_digest":   cr.IntentDigest,
		"operation_class": cr.CanonicalAction.OperationClass,
		"scope_class":     cr.CanonicalAction.ScopeClass,
		"canon_version":   cr.CanonVersion,
	}

	// Write scanner findings as evidence entries.
	if *scannerFlag != "" {
		evidencePath := resolveEvidencePath(*evidenceFlag)
		actor := *actorFlag

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
		for _, f := range findings {
			findingPayload, _ := json.Marshal(f)
			lastHash, _ := evidence.LastHashAtPath(evidencePath)
			findingEntry, err := evidence.BuildEntry(evidence.EntryBuildParams{
				Type:           evidence.EntryTypeFinding,
				TraceID:        cr.ArtifactDigest,
				Actor:          evidence.Actor{Type: "cli", ID: actor},
				ArtifactDigest: cr.ArtifactDigest,
				Payload:        findingPayload,
				PreviousHash:   lastHash,
				SpecVersion:    "0.3.0",
				AdapterVersion: version.Version,
			})
			if err != nil {
				continue
			}
			evidence.AppendEntryAtPath(evidencePath, findingEntry)
		}
		result["findings_count"] = len(findings)
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
	if err := fs.Parse(args); err != nil {
		return 2
	}

	if *prescriptionFlag == "" {
		fmt.Fprintln(stderr, "report requires --prescription")
		return 2
	}

	evidencePath := resolveEvidencePath(*evidenceFlag)

	// Look up prescription
	_, found, err := evidence.FindEntryByID(evidencePath, *prescriptionFlag)
	if err != nil {
		fmt.Fprintf(stderr, "read evidence: %v\n", err)
		return 1
	}
	if !found {
		fmt.Fprintf(stderr, "prescription %s not found in evidence\n", *prescriptionFlag)
		return 1
	}

	actorID := *actorFlag
	if actorID == "" {
		actorID = "cli"
	}
	actor := evidence.Actor{Type: "cli", ID: actorID, Provenance: "cli"}

	reportID := evidence.GenerateTraceID()
	reportPayload := evidence.ReportPayload{
		ReportID:       reportID,
		PrescriptionID: *prescriptionFlag,
		ExitCode:       *exitCodeFlag,
		Verdict:        evidence.VerdictFromExitCode(*exitCodeFlag),
	}

	if *externalRefsFlag != "" {
		var refs []evidence.ExternalRef
		if err := json.Unmarshal([]byte(*externalRefsFlag), &refs); err != nil {
			fmt.Fprintf(stderr, "parse --external-refs: %v\n", err)
			return 1
		}
		reportPayload.ExternalRefs = refs
	}

	payloadJSON, _ := json.Marshal(reportPayload)

	lastHash, _ := evidence.LastHashAtPath(evidencePath)
	entry, err := evidence.BuildEntry(evidence.EntryBuildParams{
		Type:           evidence.EntryTypeReport,
		TraceID:        reportID,
		Actor:          actor,
		ArtifactDigest: *artifactDigestFlag,
		Payload:        payloadJSON,
		PreviousHash:   lastHash,
		SpecVersion:    "0.3.0",
		AdapterVersion: version.Version,
	})
	if err != nil {
		fmt.Fprintf(stderr, "build entry: %v\n", err)
		return 1
	}

	if err := evidence.AppendEntryAtPath(evidencePath, entry); err != nil {
		fmt.Fprintf(stderr, "write evidence: %v\n", err)
		return 1
	}

	result := map[string]interface{}{
		"ok":              true,
		"report_id":       entry.EntryID,
		"prescription_id": *prescriptionFlag,
		"exit_code":       *exitCodeFlag,
		"verdict":         reportPayload.Verdict,
	}
	enc := json.NewEncoder(stdout)
	enc.SetIndent("", "  ")
	if err := enc.Encode(result); err != nil {
		fmt.Fprintf(stderr, "encode report: %v\n", err)
		return 1
	}
	return 0
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

func filterEntries(entries []evidence.EvidenceEntry, actor, period string) []evidence.EvidenceEntry {
	cutoff := parsePeriodCutoff(period)
	var filtered []evidence.EvidenceEntry
	for _, e := range entries {
		if actor != "" && e.Actor.ID != actor {
			continue
		}
		if !cutoff.IsZero() && e.Timestamp.Before(cutoff) {
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
	fmt.Sscanf(period[:len(period)-1], "%d", &val)
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

func printUsage(w io.Writer) {
	fmt.Fprintln(w, "evidra-benchmark — flight recorder for infrastructure automation")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "COMMANDS:")
	fmt.Fprintln(w, "  scorecard   Generate reliability scorecard for an actor")
	fmt.Fprintln(w, "  explain     Explain signals contributing to a score")
	fmt.Fprintln(w, "  compare     Compare reliability scores between actors")
	fmt.Fprintln(w, "  prescribe   Analyze artifact before execution")
	fmt.Fprintln(w, "  report      Record outcome after execution")
	fmt.Fprintln(w, "  version     Print version information")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Run 'evidra <command> --help' for command-specific flags.")
}
