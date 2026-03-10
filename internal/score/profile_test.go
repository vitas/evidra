package score

import (
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
	if got := profile.Weights["protocol_violation"]; got != 0.35 {
		t.Fatalf("protocol_violation weight = %v, want 0.35", got)
	}
}

func TestResolveProfileFromEnv(t *testing.T) {
	tmp := t.TempDir()
	override := filepath.Join(tmp, "override.json")
	if err := os.WriteFile(override, []byte(`{
  "id": "override.test",
  "min_operations": 7,
  "weights": {"protocol_violation": 0.5},
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
