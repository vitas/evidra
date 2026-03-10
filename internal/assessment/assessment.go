package assessment

import (
	"fmt"

	"samebits.com/evidra-benchmark/internal/pipeline"
	"samebits.com/evidra-benchmark/internal/score"
	"samebits.com/evidra-benchmark/internal/signal"
	"samebits.com/evidra-benchmark/pkg/evidence"
)

const (
	AssessmentModePreview    = "preview"
	AssessmentModeSufficient = "sufficient"
	PreviewMinOperations     = 1
)

type Basis struct {
	AssessmentMode       string `json:"assessment_mode"`
	Sufficient           bool   `json:"sufficient"`
	TotalOperations      int    `json:"total_operations"`
	SufficientThreshold  int    `json:"sufficient_threshold"`
	PreviewMinOperations int    `json:"preview_min_operations"`
}

type Snapshot struct {
	Score            float64          `json:"score"`
	ScoreBand        string           `json:"score_band"`
	ScoringProfileID string           `json:"scoring_profile_id"`
	SignalSummary    map[string]int   `json:"signal_summary"`
	Confidence       score.Confidence `json:"confidence"`
	Basis            Basis            `json:"basis"`
}

func BuildAtPath(evidencePath, sessionID string) (Snapshot, error) {
	entries, err := evidence.ReadAllEntriesAtPath(evidencePath)
	if err != nil {
		return Snapshot{}, fmt.Errorf("read evidence for assessment: %w", err)
	}

	filtered := filterEntries(entries, sessionID)
	signalEntries, err := pipeline.EvidenceToSignalEntries(filtered)
	if err != nil {
		return Snapshot{}, fmt.Errorf("convert evidence for assessment: %w", err)
	}

	results := signal.AllSignals(signalEntries, signal.DefaultTTL)
	totalOps := countPrescriptions(signalEntries)
	return BuildFromResults(results, totalOps), nil
}

func BuildFromResults(results []signal.SignalResult, totalOps int) Snapshot {
	profile, err := score.ResolveProfile("")
	if err != nil {
		profile, _ = score.LoadDefaultProfile()
	}
	strict := score.ComputeWithProfile(profile, results, totalOps, 0.0)
	preview := score.ComputeWithProfileAndMinOperations(profile, results, totalOps, 0.0, PreviewMinOperations)

	selected := strict
	mode := AssessmentModeSufficient
	if !strict.Sufficient {
		selected = preview
		mode = AssessmentModePreview
	}

	return Snapshot{
		Score:            selected.Score,
		ScoreBand:        selected.Band,
		ScoringProfileID: selected.ScoringProfileID,
		SignalSummary:    selected.Signals,
		Confidence:       selected.Confidence,
		Basis: Basis{
			AssessmentMode:       mode,
			Sufficient:           strict.Sufficient,
			TotalOperations:      totalOps,
			SufficientThreshold:  profile.MinOperations,
			PreviewMinOperations: PreviewMinOperations,
		},
	}
}

func filterEntries(entries []evidence.EvidenceEntry, sessionID string) []evidence.EvidenceEntry {
	if sessionID == "" {
		return entries
	}

	filtered := make([]evidence.EvidenceEntry, 0, len(entries))
	for _, e := range entries {
		if e.SessionID == sessionID {
			filtered = append(filtered, e)
		}
	}
	return filtered
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
