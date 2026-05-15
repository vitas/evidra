package evidence

import (
	"fmt"
	"regexp"
	"strings"
)

var digestPattern = regexp.MustCompile(`^sha256:[a-f0-9]{64}$`)

// ValidateDigest validates optional sha256:<hex> digests used by evidence payloads.
func ValidateDigest(v string) error {
	if strings.TrimSpace(v) == "" {
		return nil
	}
	if !digestPattern.MatchString(v) {
		return fmt.Errorf("digest must match sha256:<64 lowercase hex>")
	}
	return nil
}

// ValidateRiskLevel validates optional risk levels supplied by assessment providers.
func ValidateRiskLevel(level string) error {
	switch level {
	case "", "low", "medium", "high", "critical", "unknown":
		return nil
	default:
		return fmt.Errorf("invalid risk level %q", level)
	}
}

// ValidateAssessmentStatus validates optional assessment status values.
func ValidateAssessmentStatus(status AssessmentStatus) error {
	switch status {
	case "", AssessmentProvided, AssessmentNotProvided, AssessmentFailed:
		return nil
	default:
		return fmt.Errorf("invalid assessment status %q", status)
	}
}
