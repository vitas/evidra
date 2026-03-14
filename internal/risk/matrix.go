package risk

import (
	"samebits.com/evidra/internal/canon"
	"samebits.com/evidra/internal/detectors"
	_ "samebits.com/evidra/internal/detectors/all"
)

// riskMatrix maps operationClass x scopeClass (environment-based) to riskLevel.
// See docs/system-design/EVIDRA_ARCHITECTURE_V1.md Risk Matrix layer notes.
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

// SeverityHigherThan reports whether severity a outranks severity b.
// Unrecognized severities never outrank recognized ones.
func SeverityHigherThan(a, b string) bool {
	aSeverity, ok := riskSeverity[a]
	if !ok {
		return false
	}
	bSeverity, ok := riskSeverity[b]
	if !ok {
		bSeverity = -1
	}
	return aSeverity > bSeverity
}

// RiskLevel returns the risk level for the given operation and scope classes.
// Unknown combinations default to "high".
func RiskLevel(operationClass, scopeClass string) string {
	scopeClass = canon.NormalizeScopeClass(scopeClass)
	row, ok := riskMatrix[operationClass]
	if !ok {
		return "medium"
	}
	level, ok := row[scopeClass]
	if !ok {
		return "medium"
	}
	return level
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
