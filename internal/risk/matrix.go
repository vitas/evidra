package risk

// riskMatrix maps operationClass x scopeClass to riskLevel.
// See EVIDRA_AGENT_RELIABILITY_BENCHMARK.md section 7.
var riskMatrix = map[string]map[string]string{
	"read":    {"single": "low", "namespace": "low", "cluster": "low", "unknown": "low"},
	"mutate":  {"single": "low", "namespace": "medium", "cluster": "medium", "unknown": "medium"},
	"destroy": {"single": "medium", "namespace": "medium", "cluster": "high", "unknown": "high"},
	"plan":    {"single": "low", "namespace": "low", "cluster": "low", "unknown": "low"},
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
