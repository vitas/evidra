package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"time"

	"samebits.com/evidra-benchmark/internal/pipeline"
	"samebits.com/evidra-benchmark/internal/score"
	"samebits.com/evidra-benchmark/internal/signal"
	"samebits.com/evidra-benchmark/pkg/evidence"
	"samebits.com/evidra-benchmark/pkg/version"
)

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
	scoringProfileFlag := fs.String("scoring-profile", "", "Path to scoring profile JSON")
	if err := fs.Parse(args); err != nil {
		return 2
	}

	ttlDuration, err := time.ParseDuration(*ttlFlag)
	if err != nil {
		fmt.Fprintf(stderr, "invalid --ttl value: %v\n", err)
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

	filtered := filterEntries(entries, *actorFlag, *periodFlag, *sessionIDFlag)
	signalEntries, err := pipeline.EvidenceToSignalEntries(filtered)
	if err != nil {
		fmt.Fprintf(stderr, "Error converting evidence: %v\n", err)
		return 1
	}
	signalEntries = filterSignalEntriesByToolAndScope(signalEntries, *toolFlag, *scopeFlag)

	totalOps := countPrescriptions(signalEntries)
	results := signal.AllSignals(signalEntries, ttlDuration)
	sc := score.ComputeWithProfileAndMinOperations(profile, results, totalOps, 0.0, *minOpsFlag)

	type signalDetail struct {
		Signal     string         `json:"signal"`
		Count      int            `json:"count"`
		Weight     float64        `json:"weight"`
		Rate       float64        `json:"rate"`
		EntryIDs   []string       `json:"entry_ids,omitempty"`
		SubSignals map[string]int `json:"sub_signals,omitempty"`
	}

	var details []signalDetail
	for _, result := range results {
		rate := 0.0
		if totalOps > 0 {
			rate = float64(result.Count) / float64(totalOps)
		}
		detail := signalDetail{
			Signal:   result.Name,
			Count:    result.Count,
			Weight:   profile.Weight(result.Name),
			Rate:     rate,
			EntryIDs: result.EventIDs,
		}
		if result.Name == "protocol_violation" {
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
		Score            float64        `json:"score"`
		Band             string         `json:"band"`
		TotalOps         int            `json:"total_operations"`
		ScoringProfileID string         `json:"scoring_profile_id"`
		Signals          []signalDetail `json:"signals"`
		EvidraVersion    string         `json:"evidra_version"`
		GeneratedAt      string         `json:"generated_at"`
	}{
		Score:            sc.Score,
		Band:             sc.Band,
		TotalOps:         totalOps,
		ScoringProfileID: sc.ScoringProfileID,
		Signals:          details,
		EvidraVersion:    version.Version,
		GeneratedAt:      time.Now().UTC().Format(time.RFC3339),
	}

	enc := json.NewEncoder(stdout)
	enc.SetIndent("", "  ")
	if err := enc.Encode(output); err != nil {
		fmt.Fprintf(stderr, "encode explain: %v\n", err)
		return 1
	}
	return 0
}
