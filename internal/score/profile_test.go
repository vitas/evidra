package score

import (
	"math"
	"os"
	"path/filepath"
	"testing"
)

func TestLoadDefaultProfile(t *testing.T) {
	t.Parallel()

	profile, err := LoadDefaultProfile()
	if err != nil {
		t.Fatalf("LoadDefaultProfile: %v", err)
	}
	if profile.ID != "default.v1.1.0" {
		t.Fatalf("profile id = %q, want %q", profile.ID, "default.v1.1.0")
	}
	if profile.MinOperations != 100 {
		t.Fatalf("min_operations = %d, want 100", profile.MinOperations)
	}
	if got := profile.Weights["protocol_violation"]; got != 0.30 {
		t.Fatalf("protocol_violation weight = %v, want 0.30", got)
	}
	if got := profile.Weights["artifact_drift"]; got != 0.25 {
		t.Fatalf("artifact_drift weight = %v, want 0.25", got)
	}
	if got := profile.Weights["retry_loop"]; got != 0.15 {
		t.Fatalf("retry_loop weight = %v, want 0.15", got)
	}
	// The weights live in a map, so the summation order changes between runs and the
	// float total is not bit-exact. An exact comparison here fails intermittently
	// depending on iteration order, which reads as a broken profile.
	var total float64
	for _, weight := range profile.Weights {
		total += weight
	}
	if math.Abs(total-1.0) > 1e-9 {
		t.Fatalf("weights total = %v, want 1.0", total)
	}
	for _, cap := range profile.ScoreCaps {
		if cap.Signal == "protocol_violation" {
			t.Fatalf("unexpected protocol_violation score cap in default profile: %+v", cap)
		}
	}
}

func TestResolveProfileFromEnv(t *testing.T) {
	tmp := t.TempDir()
	override := filepath.Join(tmp, "override.json")
	if err := os.WriteFile(override, []byte(`{
  "id": "override.test",
  "min_operations": 7,
  "weights": {"protocol_violation": 1.0},
  "score_caps": [],
  "confidence": {
    "protocol_violation_rate_gt": 0.2,
    "protocol_violation_level": "low",
    "protocol_violation_score_ceiling": 80,
    "external_pct_gt": 0.6,
    "external_level": "medium",
    "external_score_ceiling": 90,
    "default_level": "high",
    "default_score_ceiling": 100
  },
  "bands": [
    {"name": "excellent", "min_score": 99},
    {"name": "good", "min_score": 95},
    {"name": "fair", "min_score": 90},
    {"name": "poor", "min_score": 0}
  ],
  "signal_profile_thresholds": {
    "low_max": 0.02,
    "medium_max": 0.10
  }
}`), 0o644); err != nil {
		t.Fatalf("write override profile: %v", err)
	}

	t.Setenv("EVIDRA_SCORING_PROFILE", override)

	profile, err := ResolveProfile("")
	if err != nil {
		t.Fatalf("ResolveProfile: %v", err)
	}
	if profile.ID != "override.test" {
		t.Fatalf("profile id = %q, want %q", profile.ID, "override.test")
	}
	if profile.MinOperations != 7 {
		t.Fatalf("min_operations = %d, want 7", profile.MinOperations)
	}
}

func TestValidateProfile_RejectsWeightsThatDoNotSumToOne(t *testing.T) {
	t.Parallel()

	profile := Profile{
		ID:            "bad.test",
		MinOperations: 10,
		Weights: map[string]float64{
			"protocol_violation": 0.6,
			"artifact_drift":     0.5,
		},
		Bands: []Band{
			{Name: "excellent", MinScore: 99},
		},
	}

	err := validateProfile(profile)
	if err == nil {
		t.Fatal("expected weight sum validation error")
	}
}
