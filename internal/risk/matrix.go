package risk

import (
	"strings"

	_ "samebits.com/evidra-benchmark/internal/detectors/all"
	"samebits.com/evidra-benchmark/internal/detectors"
)

// riskMatrix maps operationClass x scopeClass (environment-based) to riskLevel.
// See EVIDRA_AGENT_RELIABILITY_BENCHMARK.md section 7.
var riskMatrix = map[string]map[string]string{
	"read":    {"production": "low", "staging": "low", "development": "low", "unknown": "low"},
	"mutate":  {"production": "high", "staging": "medium", "development": "low", "unknown": "medium"},
	"destroy": {"production": "critical", "staging": "high", "development": "medium", "unknown": "high"},
	"plan":    {"production": "low", "staging": "low", "development": "low", "unknown": "low"},
}

// riskSeverity maps risk levels to numeric severity for comparison.
var riskSeverity = map[string]int{
	"low":      0,
	"medium":   1,
	"high":     2,
	"critical": 3,
}

// RiskLevel returns the risk level for the given operation and scope classes.
// Unknown combinations default to "high".
func RiskLevel(operationClass, scopeClass string) string {
	scopeClass = normalizeScopeClassAlias(scopeClass)
	row, ok := riskMatrix[operationClass]
	if !ok {
		return "high"
	}
	level, ok := row[scopeClass]
	if !ok {
		return "high"
	}
	return level
}

func normalizeScopeClassAlias(scopeClass string) string {
	v := strings.ToLower(strings.TrimSpace(scopeClass))
	switch v {
	case "prod":
		return "production"
	case "stage":
		return "staging"
	case "dev", "test", "sandbox":
		return "development"
	default:
		return v
	}
}

// ElevateRiskLevel returns the highest severity across the matrix-derived risk
// level and any fired detector tags. Unknown tags are ignored.
func ElevateRiskLevel(matrixLevel string, riskTags []string) string {
	best := matrixLevel
	bestSeverity, ok := riskSeverity[matrixLevel]
	if !ok {
		best = "high"
		bestSeverity = riskSeverity[best]
	}

	for _, tag := range riskTags {
		baseSeverity, ok := detectors.BaseSeverityForTag(tag)
		if !ok {
			continue
		}
		sev, ok := riskSeverity[baseSeverity]
		if !ok {
			continue
		}
		if sev > bestSeverity {
			best = baseSeverity
			bestSeverity = sev
		}
	}

	return best
}
