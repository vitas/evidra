package analytics

import (
	"fmt"
	"time"

	"samebits.com/evidra/internal/pipeline"
	"samebits.com/evidra/internal/score"
	"samebits.com/evidra/internal/signal"
	"samebits.com/evidra/pkg/evidence"
	"samebits.com/evidra/pkg/version"
)

type ScorecardSignalRow struct {
	Signal  string  `json:"signal"`
	Count   int     `json:"count"`
	Rate    float64 `json:"rate"`
	Profile string  `json:"profile"`
	Weight  float64 `json:"weight"`
}

type ScorecardOutput struct {
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

type ScorecardView struct {
	Output     ScorecardOutput      `json:"output"`
	SignalRows []ScorecardSignalRow `json:"signal_rows"`
}

type SignalDetail struct {
	Signal     string         `json:"signal"`
	Count      int            `json:"count"`
	Weight     float64        `json:"weight"`
	Rate       float64        `json:"rate"`
	EntryIDs   []string       `json:"entry_ids,omitempty"`
	SubSignals map[string]int `json:"sub_signals,omitempty"`
}

type ExplainOutput struct {
	Score            float64        `json:"score"`
	Band             string         `json:"band"`
	TotalOps         int            `json:"total_operations"`
	ScoringProfileID string         `json:"scoring_profile_id"`
	Signals          []SignalDetail `json:"signals"`
	EvidraVersion    string         `json:"evidra_version"`
	GeneratedAt      string         `json:"generated_at"`
}

func ComputeScorecard(entries []evidence.EvidenceEntry, filters Filters) (ScorecardOutput, error) {
	profile, err := score.ResolveProfile("")
	if err != nil {
		return ScorecardOutput{}, err
	}
	signalEntries, err := filteredSignalEntries(entries, filters)
	if err != nil {
		return ScorecardOutput{}, err
	}

	totalOps := countPrescriptions(signalEntries)
	results := signal.AllSignals(signalEntries, signal.DefaultTTL)
	sc := score.ComputeWithProfileAndMinOperations(profile, results, totalOps, 0.0, filters.MinOperations)

	return buildScorecardView(sc, profile, signalEntries, filters.Actor, filters.SessionID, filters.Period, time.Now().UTC()).Output, nil
}

func ComputeExplain(entries []evidence.EvidenceEntry, filters Filters) (ExplainOutput, error) {
	profile, err := score.ResolveProfile("")
	if err != nil {
		return ExplainOutput{}, err
	}
	signalEntries, err := filteredSignalEntries(entries, filters)
	if err != nil {
		return ExplainOutput{}, err
	}

	totalOps := countPrescriptions(signalEntries)
	results := signal.AllSignals(signalEntries, signal.DefaultTTL)
	sc := score.ComputeWithProfileAndMinOperations(profile, results, totalOps, 0.0, filters.MinOperations)

	details := make([]SignalDetail, 0, len(results))
	for _, result := range results {
		rate := 0.0
		if totalOps > 0 {
			rate = float64(result.Count) / float64(totalOps)
		}
		detail := SignalDetail{
			Signal:   result.Name,
			Count:    result.Count,
			Weight:   profile.Weight(result.Name),
			Rate:     rate,
			EntryIDs: result.EventIDs,
		}
		if result.Name == "protocol_violation" {
			subMap := make(map[string]int)
			pvEvents := signal.DetectProtocolViolationEvents(signalEntries, signal.DefaultTTL)
			for _, ev := range pvEvents {
				subMap[ev.SubSignal]++
			}
			detail.SubSignals = subMap
		}
		details = append(details, detail)
	}

	return ExplainOutput{
		Score:            sc.Score,
		Band:             sc.Band,
		TotalOps:         totalOps,
		ScoringProfileID: sc.ScoringProfileID,
		Signals:          details,
		EvidraVersion:    version.Version,
		GeneratedAt:      time.Now().UTC().Format(time.RFC3339),
	}, nil
}

func filteredSignalEntries(entries []evidence.EvidenceEntry, filters Filters) ([]signal.Entry, error) {
	filtered := filterEntries(entries, filters.Actor, filters.Period, filters.SessionID)
	signalEntries, err := pipeline.EvidenceToSignalEntries(filtered)
	if err != nil {
		return nil, err
	}
	return filterSignalEntriesByToolAndScope(signalEntries, filters.Tool, filters.Scope), nil
}

func filterEntries(entries []evidence.EvidenceEntry, actor, period, sessionID string) []evidence.EvidenceEntry {
	cutoff := parsePeriodCutoff(period)
	filtered := make([]evidence.EvidenceEntry, 0, len(entries))
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

func filterSignalEntriesByToolAndScope(entries []signal.Entry, tool, scope string) []signal.Entry {
	if tool == "" && scope == "" {
		return entries
	}

	filtered := make([]signal.Entry, 0, len(entries))
	for _, entry := range entries {
		if tool != "" && entry.Tool != tool {
			continue
		}
		if scope != "" && entry.ScopeClass != scope {
			continue
		}
		filtered = append(filtered, entry)
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
	for _, entry := range entries {
		if entry.IsPrescription {
			count++
		}
	}
	return count
}

func buildScorecardView(sc score.Scorecard, profile score.Profile, signalEntries []signal.Entry, actorID, sessionID, period string, now time.Time) ScorecardView {
	publicSignals := PublicSignalNames(profile)
	rows := make([]ScorecardSignalRow, 0, len(publicSignals))
	for _, name := range publicSignals {
		count := sc.Signals[name]
		rate := sc.Rates[name]
		if sc.TotalOperations > 0 && rate == 0 && count > 0 {
			rate = float64(count) / float64(sc.TotalOperations)
		}
		level := sc.SignalProfiles[name].Level
		if level == "" {
			level = "none"
		}
		rows = append(rows, ScorecardSignalRow{
			Signal:  name,
			Count:   count,
			Rate:    rate,
			Profile: level,
			Weight:  profile.Weight(name),
		})
	}

	return ScorecardView{
		Output: ScorecardOutput{
			Scorecard:      sc,
			ActorID:        actorID,
			SessionID:      sessionID,
			Period:         period,
			DaysObserved:   countObservedDays(signalEntries),
			ScoringVersion: version.ScoringVersion,
			SpecVersion:    version.SpecVersion,
			EvidraVersion:  version.Version,
			GeneratedAt:    now.Format(time.RFC3339),
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
