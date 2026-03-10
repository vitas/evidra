package score

import (
	"embed"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"samebits.com/evidra-benchmark/pkg/version"
)

const ScoringProfileEnvVar = "EVIDRA_SCORING_PROFILE"

type Profile struct {
	ID                      string                  `json:"id"`
	MinOperations           int                     `json:"min_operations"`
	Weights                 map[string]float64      `json:"weights"`
	ScoreCaps               []ScoreCap              `json:"score_caps"`
	Confidence              ConfidencePolicy        `json:"confidence"`
	Bands                   []Band                  `json:"bands"`
	SignalProfileThresholds SignalProfileThresholds `json:"signal_profile_thresholds"`
}

type ScoreCap struct {
	Signal   string  `json:"signal"`
	RateGT   float64 `json:"rate_gt"`
	MaxScore float64 `json:"max_score"`
}

type ConfidencePolicy struct {
	ProtocolViolationRateGT  float64 `json:"protocol_violation_rate_gt"`
	ProtocolViolationLevel   string  `json:"protocol_violation_level"`
	ProtocolViolationCeiling float64 `json:"protocol_violation_score_ceiling"`
	ExternalPctGT            float64 `json:"external_pct_gt"`
	ExternalLevel            string  `json:"external_level"`
	ExternalScoreCeiling     float64 `json:"external_score_ceiling"`
	DefaultLevel             string  `json:"default_level"`
	DefaultScoreCeiling      float64 `json:"default_score_ceiling"`
}

type Band struct {
	Name     string  `json:"name"`
	MinScore float64 `json:"min_score"`
}

type SignalProfileThresholds struct {
	LowMax    float64 `json:"low_max"`
	MediumMax float64 `json:"medium_max"`
}

//go:embed profiles/*.json
var embeddedProfiles embed.FS

var embeddedDefaultProfile = mustLoadDefaultProfile()

var MinOperations = embeddedDefaultProfile.MinOperations

var DefaultWeights = cloneWeights(embeddedDefaultProfile.Weights)

func LoadDefaultProfile() (Profile, error) {
	return loadEmbeddedProfile(fmt.Sprintf("profiles/default.%s.json", version.ScoringVersion))
}

func ResolveProfile(overridePath string) (Profile, error) {
	if overridePath == "" {
		overridePath = strings.TrimSpace(os.Getenv(ScoringProfileEnvVar))
	}
	if overridePath == "" {
		return cloneProfile(embeddedDefaultProfile), nil
	}
	return loadProfileFromPath(overridePath)
}

func mustLoadDefaultProfile() Profile {
	profile, err := LoadDefaultProfile()
	if err != nil {
		panic(fmt.Sprintf("load default scoring profile: %v", err))
	}
	return profile
}

func loadEmbeddedProfile(path string) (Profile, error) {
	data, err := embeddedProfiles.ReadFile(path)
	if err != nil {
		return Profile{}, fmt.Errorf("read embedded scoring profile %s: %w", path, err)
	}
	return parseProfile(data)
}

func loadProfileFromPath(path string) (Profile, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Profile{}, fmt.Errorf("read scoring profile %s: %w", path, err)
	}
	return parseProfile(data)
}

func parseProfile(data []byte) (Profile, error) {
	var profile Profile
	if err := json.Unmarshal(data, &profile); err != nil {
		return Profile{}, fmt.Errorf("parse scoring profile: %w", err)
	}
	if err := validateProfile(profile); err != nil {
		return Profile{}, err
	}
	return cloneProfile(profile), nil
}

func validateProfile(profile Profile) error {
	if profile.ID == "" {
		return fmt.Errorf("validate scoring profile: missing id")
	}
	if profile.MinOperations <= 0 {
		return fmt.Errorf("validate scoring profile: min_operations must be > 0")
	}
	if len(profile.Bands) == 0 {
		return fmt.Errorf("validate scoring profile: bands must not be empty")
	}
	if profile.Weights == nil {
		return fmt.Errorf("validate scoring profile: weights must not be nil")
	}
	return nil
}

func cloneProfile(profile Profile) Profile {
	cloned := profile
	cloned.Weights = cloneWeights(profile.Weights)
	cloned.ScoreCaps = append([]ScoreCap(nil), profile.ScoreCaps...)
	cloned.Bands = append([]Band(nil), profile.Bands...)
	return cloned
}

func cloneWeights(weights map[string]float64) map[string]float64 {
	cloned := make(map[string]float64, len(weights))
	for name, weight := range weights {
		cloned[name] = weight
	}
	return cloned
}

func (profile Profile) Weight(name string) float64 {
	return profile.Weights[name]
}
