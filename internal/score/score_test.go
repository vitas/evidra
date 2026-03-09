package score

import (
	"math"
	"testing"

	"samebits.com/evidra-benchmark/internal/signal"
)

func TestCompute_PerfectScore(t *testing.T) {
	t.Parallel()

	results := []signal.SignalResult{
		{Name: "protocol_violation", Count: 0},
		{Name: "artifact_drift", Count: 0},
		{Name: "retry_loop", Count: 0},
		{Name: "blast_radius", Count: 0},
		{Name: "new_scope", Count: 0},
	}
	sc := Compute(results, 200, 0.0)

	if !sc.Sufficient {
		t.Fatal("expected sufficient data")
	}
	if sc.Score != 100 {
		t.Errorf("score = %f, want 100", sc.Score)
	}
	if sc.Band != "excellent" {
		t.Errorf("band = %q, want %q", sc.Band, "excellent")
	}
}

func TestCompute_WithViolations(t *testing.T) {
	t.Parallel()

	results := []signal.SignalResult{
		{Name: "protocol_violation", Count: 10},
		{Name: "artifact_drift", Count: 5},
		{Name: "retry_loop", Count: 0},
		{Name: "blast_radius", Count: 0},
		{Name: "new_scope", Count: 0},
	}
	sc := Compute(results, 200, 0.0)

	// penalty = 0.35 * (10/200) + 0.30 * (5/200)
	//         = 0.35 * 0.05 + 0.30 * 0.025
	//         = 0.0175 + 0.0075 = 0.025
	// score = 100 * (1 - 0.025) = 97.5
	if math.Abs(sc.Score-97.5) > 0.001 {
		t.Errorf("score = %f, want 97.5", sc.Score)
	}
	if sc.Band != "good" {
		t.Errorf("band = %q, want %q", sc.Band, "good")
	}
}

func TestCompute_InsufficientData(t *testing.T) {
	t.Parallel()

	results := []signal.SignalResult{
		{Name: "protocol_violation", Count: 0},
	}
	sc := Compute(results, 50, 0.0)

	if sc.Sufficient {
		t.Fatal("expected insufficient data")
	}
	if sc.Band != "insufficient_data" {
		t.Errorf("band = %q, want %q", sc.Band, "insufficient_data")
	}
}

func TestComputeWithMinOperations_OverrideAllowsScoring(t *testing.T) {
	t.Parallel()

	results := []signal.SignalResult{
		{Name: "protocol_violation", Count: 0},
	}
	sc := ComputeWithMinOperations(results, 10, 0.0, 1)

	if !sc.Sufficient {
		t.Fatal("expected sufficient data with minOps override")
	}
	if sc.Band == "insufficient_data" {
		t.Fatalf("band = %q, want scored band", sc.Band)
	}
}

func TestComputeWithMinOperations_InvalidOverrideFallsBackToDefault(t *testing.T) {
	t.Parallel()

	results := []signal.SignalResult{
		{Name: "protocol_violation", Count: 0},
	}
	sc := ComputeWithMinOperations(results, 50, 0.0, 0)

	if sc.Sufficient {
		t.Fatal("expected insufficient data when override is invalid")
	}
	if sc.Band != "insufficient_data" {
		t.Fatalf("band = %q, want %q", sc.Band, "insufficient_data")
	}
}

func TestCompute_ScoreBands(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		violations int
		totalOps   int
		wantBand   string
	}{
		{"excellent", 0, 200, "excellent"},
		{"good", 10, 200, "good"}, // penalty = 0.35 * 0.05 = 0.0175, score = 98.25
		{"fair", 30, 200, "fair"}, // penalty = 0.35 * 0.15 = 0.0525, score = 94.75
		{"poor", 80, 200, "poor"}, // penalty = 0.35 * 0.40 = 0.14, score = 86.0
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			results := []signal.SignalResult{
				{Name: "protocol_violation", Count: tt.violations},
			}
			sc := Compute(results, tt.totalOps, 0.0)
			if sc.Band != tt.wantBand {
				t.Errorf("band = %q, want %q (score = %f)", sc.Band, tt.wantBand, sc.Score)
			}
		})
	}
}

func TestComputeConfidence_High(t *testing.T) {
	t.Parallel()
	conf := ComputeConfidence(0.0, 0.0)
	if conf.Level != "high" {
		t.Errorf("level: got %q, want high", conf.Level)
	}
	if conf.ScoreCeiling != 100 {
		t.Errorf("ceiling: got %.0f, want 100", conf.ScoreCeiling)
	}
}

func TestComputeConfidence_Medium(t *testing.T) {
	t.Parallel()
	conf := ComputeConfidence(0.6, 0.0)
	if conf.Level != "medium" {
		t.Errorf("level: got %q, want medium", conf.Level)
	}
	if conf.ScoreCeiling != 95 {
		t.Errorf("ceiling: got %.0f, want 95", conf.ScoreCeiling)
	}
}

func TestComputeConfidence_Low(t *testing.T) {
	t.Parallel()
	conf := ComputeConfidence(0.0, 0.15)
	if conf.Level != "low" {
		t.Errorf("level: got %q, want low", conf.Level)
	}
	if conf.ScoreCeiling != 85 {
		t.Errorf("ceiling: got %.0f, want 85", conf.ScoreCeiling)
	}
}

func TestCompute_SafetyFloor_ProtocolViolation(t *testing.T) {
	t.Parallel()

	// 15 violations out of 100 = 15% rate, exceeds 10% threshold
	results := []signal.SignalResult{
		{Name: "protocol_violation", Count: 15},
		{Name: "artifact_drift", Count: 0},
		{Name: "retry_loop", Count: 0},
		{Name: "blast_radius", Count: 0},
		{Name: "new_scope", Count: 0},
	}
	sc := Compute(results, 100, 0.0)
	if sc.Score > 90 {
		t.Errorf("protocol_violation > 10%% should cap score at 90, got %.1f", sc.Score)
	}
}

func TestCompute_SafetyFloor_ArtifactDrift(t *testing.T) {
	t.Parallel()

	// 8 drifts out of 100 = 8% rate, exceeds 5% threshold
	results := []signal.SignalResult{
		{Name: "protocol_violation", Count: 0},
		{Name: "artifact_drift", Count: 8},
		{Name: "retry_loop", Count: 0},
		{Name: "blast_radius", Count: 0},
		{Name: "new_scope", Count: 0},
	}
	sc := Compute(results, 100, 0.0)
	if sc.Score > 85 {
		t.Errorf("artifact_drift > 5%% should cap score at 85, got %.1f", sc.Score)
	}
}

func TestCompute_IncludesConfidence(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		violations  int
		totalOps    int
		externalPct float64
		wantLevel   string
		wantCeiling float64
	}{
		{"high_confidence", 0, 200, 0.0, "high", 100},
		{"medium_confidence_external", 0, 200, 0.6, "medium", 95},
		{"low_confidence_violations", 30, 200, 0.0, "low", 85},
		{"insufficient_data_high", 0, 50, 0.0, "high", 100},
		{"insufficient_data_external", 0, 50, 0.6, "medium", 95},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			results := []signal.SignalResult{
				{Name: "protocol_violation", Count: tt.violations},
				{Name: "artifact_drift", Count: 0},
				{Name: "retry_loop", Count: 0},
				{Name: "blast_radius", Count: 0},
				{Name: "new_scope", Count: 0},
			}
			sc := Compute(results, tt.totalOps, tt.externalPct)
			if sc.Confidence.Level != tt.wantLevel {
				t.Errorf("confidence level = %q, want %q", sc.Confidence.Level, tt.wantLevel)
			}
			if sc.Confidence.ScoreCeiling != tt.wantCeiling {
				t.Errorf("confidence score_ceiling = %.0f, want %.0f", sc.Confidence.ScoreCeiling, tt.wantCeiling)
			}
		})
	}
}

func TestCompute_RiskEscalationZero_ScoreUnchanged(t *testing.T) {
	t.Parallel()

	results := []signal.SignalResult{
		{Name: "protocol_violation", Count: 10},
		{Name: "artifact_drift", Count: 5},
		{Name: "retry_loop", Count: 0},
		{Name: "blast_radius", Count: 0},
		{Name: "new_scope", Count: 0},
		{Name: "risk_escalation", Count: 0},
	}
	sc := Compute(results, 200, 0.0)

	// Same math as TestCompute_WithViolations: penalty = 0.025, score = 97.5
	if math.Abs(sc.Score-97.5) > 0.001 {
		t.Errorf("score = %f, want 97.5 (risk_escalation=0 must not affect score)", sc.Score)
	}
}

func TestWorkloadOverlap_Identical(t *testing.T) {
	t.Parallel()

	a := WorkloadProfile{
		Tools:  map[string]bool{"kubectl": true, "terraform": true},
		Scopes: map[string]bool{"production": true},
	}
	overlap := WorkloadOverlap(a, a)
	if math.Abs(overlap-1.0) > 0.001 {
		t.Errorf("overlap = %f, want 1.0", overlap)
	}
}

func TestWorkloadOverlap_Disjoint(t *testing.T) {
	t.Parallel()

	a := WorkloadProfile{
		Tools:  map[string]bool{"kubectl": true},
		Scopes: map[string]bool{"production": true},
	}
	b := WorkloadProfile{
		Tools:  map[string]bool{"terraform": true},
		Scopes: map[string]bool{"staging": true},
	}
	overlap := WorkloadOverlap(a, b)
	if overlap != 0 {
		t.Errorf("overlap = %f, want 0", overlap)
	}
}

func TestWorkloadOverlap_Partial(t *testing.T) {
	t.Parallel()

	a := WorkloadProfile{
		Tools:  map[string]bool{"kubectl": true, "terraform": true},
		Scopes: map[string]bool{"production": true},
	}
	b := WorkloadProfile{
		Tools:  map[string]bool{"kubectl": true},
		Scopes: map[string]bool{"production": true},
	}
	// tool overlap: 1/2 = 0.5, scope overlap: 1/1 = 1.0
	// total: 0.5 * 1.0 = 0.5
	overlap := WorkloadOverlap(a, b)
	if math.Abs(overlap-0.5) > 0.001 {
		t.Errorf("overlap = %f, want 0.5", overlap)
	}
}
