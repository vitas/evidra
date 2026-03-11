package api

import (
	"samebits.com/evidra/internal/analytics"
	"samebits.com/evidra/internal/score"
)

type scorecardAPIResponse struct {
	Score          float64                       `json:"score"`
	Band           string                        `json:"band"`
	Basis          string                        `json:"basis"`
	Confidence     string                        `json:"confidence"`
	TotalEntries   int                           `json:"total_entries"`
	SignalSummary  map[string]signalSummaryEntry `json:"signal_summary"`
	Period         string                        `json:"period,omitempty"`
	ScoringVersion string                        `json:"scoring_version,omitempty"`
	GeneratedAt    string                        `json:"generated_at,omitempty"`
}

type signalSummaryEntry struct {
	Detected bool    `json:"detected"`
	Weight   float64 `json:"weight"`
	Count    int     `json:"count"`
}

func toScorecardAPIResponse(out analytics.ScorecardOutput, profile score.Profile) scorecardAPIResponse {
	basis := "insufficient"
	if out.Sufficient {
		basis = "sufficient"
	}

	summary := make(map[string]signalSummaryEntry)
	for _, name := range analytics.PublicSignalNames(profile) {
		count := out.Signals[name]
		summary[name] = signalSummaryEntry{
			Detected: count > 0,
			Weight:   profile.Weight(name),
			Count:    count,
		}
	}

	return scorecardAPIResponse{
		Score:          out.Score,
		Band:           out.Band,
		Basis:          basis,
		Confidence:     out.Confidence.Level,
		TotalEntries:   out.TotalOperations,
		SignalSummary:  summary,
		Period:         out.Period,
		ScoringVersion: out.ScoringVersion,
		GeneratedAt:    out.GeneratedAt,
	}
}
