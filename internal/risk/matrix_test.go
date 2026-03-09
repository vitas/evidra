package risk

import (
	"testing"

	_ "samebits.com/evidra-benchmark/internal/detectors/all"
)

func TestRiskLevel_KnownCombinations(t *testing.T) {
	t.Parallel()

	tests := []struct {
		opClass    string
		scopeClass string
		want       string
	}{
		{"read", "production", "low"},
		{"read", "staging", "low"},
		{"read", "development", "low"},
		{"read", "unknown", "low"},
		{"mutate", "production", "high"},
		{"mutate", "staging", "medium"},
		{"mutate", "development", "low"},
		{"mutate", "unknown", "medium"},
		{"destroy", "production", "critical"},
		{"destroy", "staging", "high"},
		{"destroy", "development", "medium"},
		{"destroy", "unknown", "high"},
		{"plan", "production", "low"},
		{"plan", "staging", "low"},
		{"plan", "development", "low"},
		{"plan", "unknown", "low"},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.opClass+"_"+tt.scopeClass, func(t *testing.T) {
			t.Parallel()
			if got := RiskLevel(tt.opClass, tt.scopeClass); got != tt.want {
				t.Fatalf("RiskLevel(%q, %q) = %q, want %q", tt.opClass, tt.scopeClass, got, tt.want)
			}
		})
	}
}

func TestRiskLevel_UnknownDefaultsHigh(t *testing.T) {
	t.Parallel()

	tests := []struct {
		opClass    string
		scopeClass string
	}{
		{"nuke", "single"},
		{"mutate", "galaxy"},
		{"nuke", "galaxy"},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.opClass+"_"+tt.scopeClass, func(t *testing.T) {
			t.Parallel()
			if got := RiskLevel(tt.opClass, tt.scopeClass); got != "high" {
				t.Fatalf("RiskLevel(%q, %q) = %q, want high", tt.opClass, tt.scopeClass, got)
			}
		})
	}
}

func TestRiskLevel_ScopeAliasesNormalized(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		opClass    string
		scopeClass string
		want       string
	}{
		{name: "prod_alias", opClass: "mutate", scopeClass: "prod", want: "high"},
		{name: "dev_alias", opClass: "destroy", scopeClass: "dev", want: "medium"},
		{name: "test_alias", opClass: "mutate", scopeClass: "test", want: "low"},
		{name: "sandbox_alias", opClass: "read", scopeClass: "sandbox", want: "low"},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := RiskLevel(tt.opClass, tt.scopeClass); got != tt.want {
				t.Fatalf("RiskLevel(%q,%q)=%q, want %q", tt.opClass, tt.scopeClass, got, tt.want)
			}
		})
	}
}

func TestElevateRiskLevel(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		base     string
		tags     []string
		expected string
	}{
		{"no_tags_nil", "medium", nil, "medium"},
		{"no_tags_empty", "medium", []string{}, "medium"},
		{"critical_detector_overrides_medium_matrix", "medium", []string{"k8s.privileged_container"}, "critical"},
		{"low_detector_does_not_raise_high_matrix", "high", []string{"k8s.writable_rootfs"}, "high"},
		{"high_detector_raises_low_matrix", "low", []string{"ops.kube_system"}, "high"},
		{"medium_detector_raises_low_matrix", "low", []string{"k8s.run_as_root"}, "medium"},
		{"unknown_tag_is_ignored", "medium", []string{"unknown.tag"}, "medium"},
		{"critical_stays_critical", "critical", []string{"ops.mass_delete"}, "critical"},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := ElevateRiskLevel(tt.base, tt.tags); got != tt.expected {
				t.Fatalf("ElevateRiskLevel(%q, %v) = %q, want %q", tt.base, tt.tags, got, tt.expected)
			}
		})
	}
}
