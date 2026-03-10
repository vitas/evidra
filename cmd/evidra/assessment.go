package main

import (
	"samebits.com/evidra-benchmark/internal/assessment"
	"samebits.com/evidra-benchmark/internal/score"
	"samebits.com/evidra-benchmark/internal/signal"
)

const (
	assessmentModePreview    = assessment.AssessmentModePreview
	assessmentModeSufficient = assessment.AssessmentModeSufficient
)

type assessmentBasis = assessment.Basis

type operationAssessment struct {
	RiskLevel        string           `json:"risk_level"`
	Score            float64          `json:"score"`
	ScoreBand        string           `json:"score_band"`
	ScoringProfileID string           `json:"scoring_profile_id"`
	SignalSummary    map[string]int   `json:"signal_summary"`
	Confidence       score.Confidence `json:"confidence"`
	Basis            assessmentBasis  `json:"basis"`
}

func buildOperationAssessment(evidencePath, sessionID, riskLevel string) (operationAssessment, error) {
	snapshot, err := assessment.BuildAtPath(evidencePath, sessionID)
	if err != nil {
		return operationAssessment{}, err
	}
	return assessmentFromSnapshot(snapshot, riskLevel), nil
}

func buildOperationAssessmentWithProfile(evidencePath, sessionID, riskLevel string, profile score.Profile) (operationAssessment, error) {
	snapshot, err := assessment.BuildAtPathWithProfile(evidencePath, sessionID, profile)
	if err != nil {
		return operationAssessment{}, err
	}
	return assessmentFromSnapshot(snapshot, riskLevel), nil
}

func buildAssessment(results []signal.SignalResult, totalOps int, riskLevel string) operationAssessment {
	snapshot := assessment.BuildFromResults(results, totalOps)
	return assessmentFromSnapshot(snapshot, riskLevel)
}

func buildAssessmentWithProfile(results []signal.SignalResult, totalOps int, riskLevel string, profile score.Profile) operationAssessment {
	snapshot := assessment.BuildFromResultsWithProfile(profile, results, totalOps)
	return assessmentFromSnapshot(snapshot, riskLevel)
}

func assessmentFromSnapshot(snapshot assessment.Snapshot, riskLevel string) operationAssessment {
	return operationAssessment{
		RiskLevel:        riskLevel,
		Score:            snapshot.Score,
		ScoreBand:        snapshot.ScoreBand,
		ScoringProfileID: snapshot.ScoringProfileID,
		SignalSummary:    snapshot.SignalSummary,
		Confidence:       snapshot.Confidence,
		Basis:            snapshot.Basis,
	}
}
