package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"strings"
	"time"

	"samebits.com/evidra-benchmark/internal/pipeline"
	"samebits.com/evidra-benchmark/internal/score"
	"samebits.com/evidra-benchmark/internal/signal"
	"samebits.com/evidra-benchmark/pkg/evidence"
)

func cmdCompare(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("compare", flag.ContinueOnError)
	fs.SetOutput(stderr)
	actorsFlag := fs.String("actors", "", "Comma-separated actor IDs to compare")
	periodFlag := fs.String("period", "30d", "Time period (e.g. 30d)")
	evidenceFlag := fs.String("evidence-dir", "", "Evidence directory")
	toolFlag := fs.String("tool", "", "Filter by tool name")
	scopeFlag := fs.String("scope", "", "Filter by scope class")
	sessionIDFlag := fs.String("session-id", "", "Filter by session ID")
	scoringProfileFlag := fs.String("scoring-profile", "", "Path to scoring profile JSON")
	if err := fs.Parse(args); err != nil {
		return 2
	}

	actors := strings.Split(*actorsFlag, ",")
	if len(actors) < 2 {
		fmt.Fprintln(stderr, "compare requires at least 2 actors (--actors A,B)")
		return 2
	}

	profile, err := resolveCommandScoringProfile(*scoringProfileFlag)
	if err != nil {
		fmt.Fprintf(stderr, "%v\n", err)
		return 1
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
		signalEntries = filterSignalEntriesByToolAndScope(signalEntries, *toolFlag, *scopeFlag)
		totalOps := countPrescriptions(signalEntries)
		results := signal.AllSignals(signalEntries, signal.DefaultTTL)
		sc := score.ComputeWithProfile(profile, results, totalOps, 0.0)
		profile := score.BuildProfile(signalEntries)

		scorecards = append(scorecards, actorScore{
			ActorID:  actorID,
			Score:    sc.Score,
			Band:     sc.Band,
			TotalOps: sc.TotalOperations,
			Profile:  profile,
		})
	}

	overlap := 0.0
	if len(scorecards) >= 2 {
		overlap = score.WorkloadOverlap(scorecards[0].Profile, scorecards[1].Profile)
	}

	result := map[string]interface{}{
		"actors":             scorecards,
		"workload_overlap":   overlap,
		"scoring_profile_id": profile.ID,
		"generated_at":       time.Now().UTC().Format(time.RFC3339),
	}

	enc := json.NewEncoder(stdout)
	enc.SetIndent("", "  ")
	if err := enc.Encode(result); err != nil {
		fmt.Fprintf(stderr, "encode comparison: %v\n", err)
		return 1
	}
	return 0
}
