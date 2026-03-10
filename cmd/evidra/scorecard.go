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
	"samebits.com/evidra-benchmark/pkg/version"
)

var scorecardSignalOrder = []string{
	"protocol_violation",
	"artifact_drift",
	"retry_loop",
	"blast_radius",
	"new_scope",
	"repair_loop",
	"thrashing",
	"risk_escalation",
}

type scorecardSignalRow struct {
	Signal  string
	Count   int
	Rate    float64
	Profile string
	Weight  float64
}

type scorecardOutput struct {
	score.Scorecard
	ActorID        string `json:"actor_id,omitempty"`
	SessionID      string `json:"session_id,omitempty"`
	Period         string `json:"period"`
	DaysObserved   int    `json:"days_observed"`
	ScoringVersion string `json:"scoring_version"`
	SpecVersion    string `json:"spec_version"`
	EvidraVersion  string `json:"evidra_version"`
	GeneratedAt    string `json:"generated_at"`
}

type scorecardView struct {
	Output     scorecardOutput
	SignalRows []scorecardSignalRow
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
	prettyFlag := fs.Bool("pretty", false, "Render human-readable ASCII output")
	if err := fs.Parse(args); err != nil {
		return 2
	}

	ttlDuration, err := time.ParseDuration(*ttlFlag)
	if err != nil {
		fmt.Fprintf(stderr, "invalid --ttl value: %v\n", err)
		return 2
	}

	profile, err := score.ResolveProfile("")
	if err != nil {
		fmt.Fprintf(stderr, "resolve scoring profile: %v\n", err)
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
	view := buildScorecardView(sc, profile, signalEntries, *actorFlag, *sessionIDFlag, *periodFlag)

	if *prettyFlag {
		if err := renderPrettyScorecard(stdout, view); err != nil {
			fmt.Fprintf(stderr, "render pretty scorecard: %v\n", err)
			return 1
		}
		return 0
	}

	enc := json.NewEncoder(stdout)
	enc.SetIndent("", "  ")
	if err := enc.Encode(view.Output); err != nil {
		fmt.Fprintf(stderr, "encode scorecard: %v\n", err)
		return 1
	}
	return 0
}

func buildScorecardView(sc score.Scorecard, profile score.Profile, signalEntries []signal.Entry, actorID, sessionID, period string) scorecardView {
	rows := make([]scorecardSignalRow, 0, len(scorecardSignalOrder))
	for _, name := range scorecardSignalOrder {
		count := sc.Signals[name]
		rate := sc.Rates[name]
		if sc.TotalOperations > 0 && rate == 0 && count > 0 {
			rate = float64(count) / float64(sc.TotalOperations)
		}
		level := sc.SignalProfiles[name].Level
		if level == "" {
			level = "none"
		}
		rows = append(rows, scorecardSignalRow{
			Signal:  name,
			Count:   count,
			Rate:    rate,
			Profile: level,
			Weight:  profile.Weight(name),
		})
	}

	return scorecardView{
		Output: scorecardOutput{
			Scorecard:      sc,
			ActorID:        actorID,
			SessionID:      sessionID,
			Period:         period,
			DaysObserved:   countObservedDays(signalEntries),
			ScoringVersion: version.ScoringVersion,
			SpecVersion:    version.SpecVersion,
			EvidraVersion:  version.Version,
			GeneratedAt:    time.Now().UTC().Format(time.RFC3339),
		},
		SignalRows: rows,
	}
}

func countObservedDays(entries []signal.Entry) int {
	days := make(map[string]struct{})
	for _, entry := range entries {
		if !entry.IsPrescription {
			continue
		}
		days[entry.Timestamp.UTC().Format("2006-01-02")] = struct{}{}
	}
	return len(days)
}

func renderPrettyScorecard(w io.Writer, view scorecardView) error {
	var b strings.Builder
	b.WriteString("EVIDRA SCORECARD\n\n")
	b.WriteString(renderASCIITable(
		[]string{"field", "value"},
		[][]string{
			{"actor", displayValue(view.Output.ActorID, "all")},
			{"session", displayValue(view.Output.SessionID, "all")},
			{"period", view.Output.Period},
			{"days_observed", fmt.Sprintf("%d", view.Output.DaysObserved)},
			{"total_operations", fmt.Sprintf("%d", view.Output.TotalOperations)},
			{"score", fmt.Sprintf("%.2f", view.Output.Score)},
			{"band", view.Output.Band},
			{"penalty", fmt.Sprintf("%.4f", view.Output.Penalty)},
			{"sufficient", fmt.Sprintf("%t", view.Output.Sufficient)},
			{"confidence", fmt.Sprintf("%s (ceiling %.0f)", view.Output.Confidence.Level, view.Output.Confidence.ScoreCeiling)},
			{"scoring_profile_id", view.Output.ScoringProfileID},
			{"scoring_version", view.Output.ScoringVersion},
			{"spec_version", view.Output.SpecVersion},
			{"evidra_version", view.Output.EvidraVersion},
		},
	))
	b.WriteString("\n\nSIGNALS\n\n")

	rows := make([][]string, 0, len(view.SignalRows))
	for _, row := range view.SignalRows {
		rows = append(rows, []string{
			row.Signal,
			fmt.Sprintf("%d", row.Count),
			fmt.Sprintf("%.4f", row.Rate),
			row.Profile,
			fmt.Sprintf("%.2f", row.Weight),
		})
	}
	b.WriteString(renderASCIITable(
		[]string{"signal", "count", "rate", "profile", "weight"},
		rows,
	))

	if view.Output.Band == "insufficient_data" {
		b.WriteString("\n\nNOTE: insufficient data for a fully qualified score in this window.")
	}
	if view.Output.Confidence.ScoreCeiling < 100 {
		b.WriteString(fmt.Sprintf("\nNOTE: confidence ceiling %.0f applies.", view.Output.Confidence.ScoreCeiling))
	}
	b.WriteString("\n")

	_, err := io.WriteString(w, b.String())
	return err
}

func renderASCIITable(headers []string, rows [][]string) string {
	widths := make([]int, len(headers))
	for i, header := range headers {
		widths[i] = len(header)
	}
	for _, row := range rows {
		for i, cell := range row {
			if len(cell) > widths[i] {
				widths[i] = len(cell)
			}
		}
	}

	borderParts := make([]string, 0, len(widths))
	for _, width := range widths {
		borderParts = append(borderParts, strings.Repeat("-", width+2))
	}
	border := "+" + strings.Join(borderParts, "+") + "+\n"

	var b strings.Builder
	b.WriteString(border)
	b.WriteString(renderASCIIRow(headers, widths))
	b.WriteString(border)
	for _, row := range rows {
		b.WriteString(renderASCIIRow(row, widths))
	}
	b.WriteString(border)
	return b.String()
}

func renderASCIIRow(row []string, widths []int) string {
	var b strings.Builder
	b.WriteString("|")
	for i, cell := range row {
		b.WriteString(" ")
		b.WriteString(cell)
		if pad := widths[i] - len(cell); pad > 0 {
			b.WriteString(strings.Repeat(" ", pad))
		}
		b.WriteString(" |")
	}
	b.WriteString("\n")
	return b.String()
}

func displayValue(v, fallback string) string {
	if strings.TrimSpace(v) == "" {
		return fallback
	}
	return v
}
