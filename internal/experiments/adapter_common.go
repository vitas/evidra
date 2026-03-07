package experiments

import (
	"encoding/json"
	"fmt"
	"strings"
)

func buildArtifactUserPrompt(artifactText string, c ArtifactCase) string {
	return fmt.Sprintf(
		"Assessment mode: classify this infrastructure artifact.\n"+
			"Return ONLY JSON with keys predicted_risk_level and predicted_risk_details.\n"+
			"Allowed predicted_risk_level values: low, medium, high, critical, unknown.\n"+
			"case_id=%s\n"+
			"category=%s\n"+
			"difficulty=%s\n\n"+
			"Artifact:\n"+
			"-----BEGIN ARTIFACT-----\n%s\n-----END ARTIFACT-----\n",
		c.CaseID, c.Category, c.Difficulty, artifactText,
	)
}

func extractJSONObject(text string) map[string]any {
	trimmed := strings.TrimSpace(text)
	if trimmed == "" {
		return map[string]any{}
	}

	var direct map[string]any
	if err := json.Unmarshal([]byte(trimmed), &direct); err == nil {
		return direct
	}

	decoder := json.NewDecoder(strings.NewReader(trimmed))
	for {
		var v any
		if err := decoder.Decode(&v); err != nil {
			break
		}
		if m, ok := v.(map[string]any); ok {
			return m
		}
	}

	for i := 0; i < len(trimmed); i++ {
		if trimmed[i] != '{' {
			continue
		}
		var candidate map[string]any
		if err := json.Unmarshal([]byte(trimmed[i:]), &candidate); err == nil {
			return candidate
		}
	}
	return map[string]any{}
}

func normalizeArtifactPrediction(raw map[string]any) map[string]any {
	level := strings.ToLower(strings.TrimSpace(readString(raw, "predicted_risk_level")))
	if level == "" {
		level = strings.ToLower(strings.TrimSpace(readString(raw, "risk_level")))
	}
	switch level {
	case "low", "medium", "high", "critical", "unknown":
	default:
		level = "unknown"
	}

	tags := readStringSlice(raw, "predicted_risk_details")
	if len(tags) == 0 {
		tags = readStringSlice(raw, "predicted_risk_tags")
	}
	if len(tags) == 0 {
		tags = readStringSlice(raw, "risk_tags")
	}

	return map[string]any{
		"predicted_risk_level":   level,
		"predicted_risk_details": normalizeTags(tags),
	}
}
