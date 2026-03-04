package score

import "samebits.com/evidra-benchmark/internal/signal"

// Default signal weights.
var DefaultWeights = map[string]float64{
	"protocol_violation": 0.35,
	"artifact_drift":     0.30,
	"retry_loop":         0.20,
	"blast_radius":       0.10,
	"new_scope":          0.05,
}

// MinOperations is the minimum number of operations required for scoring.
const MinOperations = 100

// Scorecard holds the computed reliability score and its components.
type Scorecard struct {
	TotalOperations int                `json:"total_operations"`
	Signals         map[string]int     `json:"signals"`
	Rates           map[string]float64 `json:"rates"`
	Penalty         float64            `json:"penalty"`
	Score           float64            `json:"score"`
	Band            string             `json:"band"`
	Sufficient      bool               `json:"sufficient"`
}

// Compute calculates a reliability scorecard from signal results.
func Compute(results []signal.SignalResult, totalOps int) Scorecard {
	sc := Scorecard{
		TotalOperations: totalOps,
		Signals:         make(map[string]int),
		Rates:           make(map[string]float64),
	}

	for _, r := range results {
		sc.Signals[r.Name] = r.Count
	}

	if totalOps < MinOperations {
		sc.Score = -1
		sc.Band = "insufficient_data"
		return sc
	}

	sc.Sufficient = true
	var penalty float64
	for name, count := range sc.Signals {
		rate := float64(count) / float64(totalOps)
		sc.Rates[name] = rate
		weight, ok := DefaultWeights[name]
		if !ok {
			continue
		}
		penalty += weight * rate
	}

	// Clamp penalty to [0, 1]
	if penalty > 1 {
		penalty = 1
	}
	sc.Penalty = penalty
	sc.Score = 100 * (1 - penalty)

	// Safety floors: cap score when specific signal rates exceed thresholds
	if sc.Rates["protocol_violation"] > 0.10 && sc.Score > 90 {
		sc.Score = 90
	}
	if sc.Rates["artifact_drift"] > 0.05 && sc.Score > 85 {
		sc.Score = 85
	}

	sc.Band = scoreBand(sc.Score)

	return sc
}

// Confidence represents the reliability of a computed score.
type Confidence struct {
	Level        string  `json:"level"`
	ScoreCeiling float64 `json:"score_ceiling"`
}

// ComputeConfidence determines score confidence based on data quality indicators.
// externalPct is the fraction of entries with canon_source="external" (pre-canonicalized).
// violationRate is the protocol_violation rate (violations / total_ops).
func ComputeConfidence(externalPct, violationRate float64) Confidence {
	if violationRate > 0.10 {
		return Confidence{Level: "low", ScoreCeiling: 85}
	}
	if externalPct > 0.50 {
		return Confidence{Level: "medium", ScoreCeiling: 95}
	}
	return Confidence{Level: "high", ScoreCeiling: 100}
}

func scoreBand(score float64) string {
	switch {
	case score >= 99:
		return "excellent"
	case score >= 95:
		return "good"
	case score >= 90:
		return "fair"
	default:
		return "poor"
	}
}
