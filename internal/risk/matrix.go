package risk

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

// ElevateRiskLevel returns the matrix risk level elevated by one step when
// catastrophic risk tags are present. No tags → matrix level unchanged.
func ElevateRiskLevel(matrixLevel string, riskTags []string) string {
	if len(riskTags) == 0 {
		return matrixLevel
	}
	cur := riskSeverity[matrixLevel]
	if cur >= 3 {
		return "critical"
	}
	// Elevate by one step
	for level, sev := range riskSeverity {
		if sev == cur+1 {
			return level
		}
	}
	return "critical"
}
