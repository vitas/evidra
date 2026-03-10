package score

import "samebits.com/evidra-benchmark/internal/signal"

// Scorecard holds the computed reliability score and its components.
type Scorecard struct {
	TotalOperations  int                      `json:"total_operations"`
	Signals          map[string]int           `json:"signals"`
	Rates            map[string]float64       `json:"rates"`
	SignalProfiles   map[string]SignalProfile `json:"signal_profiles"`
	Penalty          float64                  `json:"penalty"`
	Score            float64                  `json:"score"`
	Band             string                   `json:"band"`
	Sufficient       bool                     `json:"sufficient"`
	Confidence       Confidence               `json:"confidence"`
	ScoringProfileID string                   `json:"scoring_profile_id"`
}

// SignalProfile captures qualitative signal severity from rate.
type SignalProfile struct {
	Level string `json:"level"`
}

// Compute calculates a reliability scorecard from signal results.
// externalPct is the fraction of entries canonicalized externally (0.0 when all are adapter-canonicalized).
func Compute(results []signal.SignalResult, totalOps int, externalPct float64) Scorecard {
	return ComputeWithProfile(cloneProfile(embeddedDefaultProfile), results, totalOps, externalPct)
}

// ComputeWithMinOperations calculates a reliability scorecard with a configurable
// sufficiency threshold. If minOps <= 0, MinOperations is used.
func ComputeWithMinOperations(results []signal.SignalResult, totalOps int, externalPct float64, minOps int) Scorecard {
	return ComputeWithProfileAndMinOperations(cloneProfile(embeddedDefaultProfile), results, totalOps, externalPct, minOps)
}

func ComputeWithProfile(profile Profile, results []signal.SignalResult, totalOps int, externalPct float64) Scorecard {
	return ComputeWithProfileAndMinOperations(profile, results, totalOps, externalPct, profile.MinOperations)
}

func ComputeWithProfileAndMinOperations(profile Profile, results []signal.SignalResult, totalOps int, externalPct float64, minOps int) Scorecard {
	if minOps <= 0 {
		minOps = profile.MinOperations
	}

	sc := Scorecard{
		TotalOperations:  totalOps,
		Signals:          make(map[string]int),
		Rates:            make(map[string]float64),
		SignalProfiles:   make(map[string]SignalProfile),
		ScoringProfileID: profile.ID,
	}

	for _, r := range results {
		sc.Signals[r.Name] = r.Count
	}

	if totalOps < minOps {
		sc.Score = -1
		sc.Band = "insufficient_data"
		sc.Confidence = computeConfidence(profile, externalPct, 0.0)
		return sc
	}

	sc.Sufficient = true
	var penalty float64
	for name, count := range sc.Signals {
		rate := float64(count) / float64(totalOps)
		sc.Rates[name] = rate
		sc.SignalProfiles[name] = SignalProfile{Level: signalProfileLevelFor(profile, rate)}
		weight, ok := profile.Weights[name]
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

	for _, cap := range profile.ScoreCaps {
		if sc.Rates[cap.Signal] > cap.RateGT && sc.Score > cap.MaxScore {
			sc.Score = cap.MaxScore
		}
	}

	sc.Band = scoreBandFor(profile, sc.Score)
	sc.Confidence = computeConfidence(profile, externalPct, sc.Rates["protocol_violation"])

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
	return computeConfidence(cloneProfile(embeddedDefaultProfile), externalPct, violationRate)
}

func computeConfidence(profile Profile, externalPct, violationRate float64) Confidence {
	if violationRate > profile.Confidence.ProtocolViolationRateGT {
		return Confidence{
			Level:        profile.Confidence.ProtocolViolationLevel,
			ScoreCeiling: profile.Confidence.ProtocolViolationCeiling,
		}
	}
	if externalPct > profile.Confidence.ExternalPctGT {
		return Confidence{
			Level:        profile.Confidence.ExternalLevel,
			ScoreCeiling: profile.Confidence.ExternalScoreCeiling,
		}
	}
	return Confidence{
		Level:        profile.Confidence.DefaultLevel,
		ScoreCeiling: profile.Confidence.DefaultScoreCeiling,
	}
}

func scoreBandFor(profile Profile, score float64) string {
	for _, band := range profile.Bands {
		if score >= band.MinScore {
			return band.Name
		}
	}
	return profile.Bands[len(profile.Bands)-1].Name
}

func signalProfileLevelFor(profile Profile, rate float64) string {
	switch {
	case rate == 0:
		return "none"
	case rate < profile.SignalProfileThresholds.LowMax:
		return "low"
	case rate < profile.SignalProfileThresholds.MediumMax:
		return "medium"
	default:
		return "high"
	}
}
