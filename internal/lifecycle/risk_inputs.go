package lifecycle

import (
	"fmt"
	"strings"

	"samebits.com/evidra/internal/risk"
	"samebits.com/evidra/pkg/evidence"
)

func buildSARIFRiskInput(src ExternalFindingsSource) evidence.RiskInput {
	var tags []string
	maxLevel := "low"
	seen := make(map[string]bool)
	counts := map[string]int{}

	for _, f := range src.Findings {
		severity := strings.ToLower(strings.TrimSpace(f.Severity))
		counts[severity]++

		tag := strings.ToLower(strings.TrimSpace(f.Tool)) + "." + strings.TrimSpace(f.RuleID)
		if tag != "." && !seen[tag] && (severity == "high" || severity == "critical") {
			seen[tag] = true
			tags = append(tags, tag)
		}

		if risk.SeverityHigherThan(severity, maxLevel) {
			maxLevel = severity
		}
	}

	source := strings.TrimSpace(src.Source)
	if source == "" && len(src.Findings) > 0 {
		tool := strings.ToLower(strings.TrimSpace(src.Findings[0].Tool))
		source = tool
		if version := strings.TrimSpace(src.Findings[0].ToolVersion); version != "" {
			source += "/" + version
		}
	}

	return evidence.RiskInput{
		Source:    source,
		RiskLevel: maxLevel,
		RiskTags:  tags,
		Detail:    buildFindingsSummary(len(src.Findings), counts),
	}
}

func buildFindingsSummary(total int, counts map[string]int) string {
	if total == 0 {
		return ""
	}

	var parts []string
	for _, severity := range []string{"critical", "high", "medium", "low"} {
		if counts[severity] > 0 {
			parts = append(parts, fmt.Sprintf("%d %s", counts[severity], severity))
		}
	}

	return fmt.Sprintf("%d findings (%s)", total, strings.Join(parts, ", "))
}

func computeEffectiveRisk(inputs []evidence.RiskInput) string {
	best := "low"
	for _, ri := range inputs {
		if risk.SeverityHigherThan(ri.RiskLevel, best) {
			best = ri.RiskLevel
		}
	}
	return best
}
