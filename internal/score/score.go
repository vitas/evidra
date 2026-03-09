package score

import "samebits.com/evidra-benchmark/internal/signal"

// Default signal weights.
var DefaultWeights = map[string]float64{
	"protocol_violation": 0.35,
	"artifact_drift":     0.30,
	"retry_loop":         0.20,
	"blast_radius":       0.10,
	"new_scope":          0.05,
	"repair_loop":        -0.05,
	"thrashing":          0.15,
	"risk_escalation":    0.10,
}

// MinOperations is the minimum number of operations required for scoring.
const MinOperations = 100

// Scorecard holds the computed reliability score and its components.
type Scorecard struct {
	TotalOperations int                      `json:"total_operations"`
	Signals         map[string]int           `json:"signals"`
	Rates           map[string]float64       `json:"rates"`
	SignalProfiles  map[string]SignalProfile `json:"signal_profiles"`
	Penalty         float64                  `json:"penalty"`
	Score           float64                  `json:"score"`
	Band            string                   `json:"band"`
	Sufficient      bool                     `json:"sufficient"`
	Confidence      Confidence               `json:"confidence"`
}

// SignalProfile captures qualitative signal severity from rate.
type SignalProfile struct {
	Level string `json:"level"`
}

// Compute calculates a reliability scorecard from signal results.
// externalPct is the fraction of entries canonicalized externally (0.0 when all are adapter-canonicalized).
func Compute(results []signal.SignalResult, totalOps int, externalPct float64) Scorecard {
	return ComputeWithMinOperations(results, totalOps, externalPct, MinOperations)
}

// ComputeWithMinOperations calculates a reliability scorecard with a configurable
// sufficiency threshold. If minOps <= 0, MinOperations is used.
func ComputeWithMinOperations(results []signal.SignalResult, totalOps int, externalPct float64, minOps int) Scorecard {
	if minOps <= 0 {
		minOps = MinOperations
	}

	sc := Scorecard{
		TotalOperations: totalOps,
		Signals:         make(map[string]int),
		Rates:           make(map[string]float64),
		SignalProfiles:  make(map[string]SignalProfile),
	}

	for _, r := range results {
		sc.Signals[r.Name] = r.Count
	}

	if totalOps < minOps {
		sc.Score = -1
		sc.Band = "insufficient_data"
		sc.Confidence = ComputeConfidence(externalPct, 0.0)
		return sc
	}

	sc.Sufficient = true
	var penalty float64
	for name, count := range sc.Signals {
		rate := float64(count) / float64(totalOps)
		sc.Rates[name] = rate
		sc.SignalProfiles[name] = SignalProfile{Level: signalProfileLevel(rate)}
		weight, ok := DefaultWeights[name]
		if !ok {
			continue
		}
		penalty += weight * rate
	}

	// Clamp penalty to [0, 1]
	if penalty < 0 {
		penalty = 0
	}
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
	sc.Confidence = ComputeConfidence(externalPct, sc.Rates["protocol_violation"])

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

func signalProfileLevel(rate float64) string {
	switch {
	case rate == 0:
		return "none"
	case rate < 0.02:
		return "low"
	case rate < 0.10:
		return "medium"
	default:
		return "high"
	}
}
