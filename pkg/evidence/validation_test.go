package evidence

import (
	"strings"
	"testing"
)

func TestValidateDigest(t *testing.T) {
	t.Parallel()

	valid := "sha256:" + strings.Repeat("a", 64)
	if err := ValidateDigest(valid); err != nil {
		t.Fatalf("valid digest rejected: %v", err)
	}
	if err := ValidateDigest("sha256:not-hex"); err == nil {
		t.Fatal("invalid digest accepted")
	}
}

func TestValidateRiskLevel(t *testing.T) {
	t.Parallel()

	for _, level := range []string{"low", "medium", "high", "critical", "unknown"} {
		if err := ValidateRiskLevel(level); err != nil {
			t.Fatalf("%q rejected: %v", level, err)
		}
	}
	if err := ValidateRiskLevel("severe"); err == nil {
		t.Fatal("invalid risk level accepted")
	}
}

func TestValidateAssessmentStatus(t *testing.T) {
	t.Parallel()

	for _, status := range []AssessmentStatus{AssessmentProvided, AssessmentNotProvided, AssessmentFailed} {
		if err := ValidateAssessmentStatus(status); err != nil {
			t.Fatalf("%q rejected: %v", status, err)
		}
	}
	if err := ValidateAssessmentStatus("partial"); err == nil {
		t.Fatal("invalid assessment status accepted")
	}
}
