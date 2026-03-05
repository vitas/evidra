package sarif

import (
	"encoding/json"
	"fmt"
	"strings"

	"samebits.com/evidra-benchmark/pkg/evidence"
)

type sarifReport struct {
	Runs []sarifRun `json:"runs"`
}

type sarifRun struct {
	Tool struct {
		Driver struct {
			Name    string `json:"name"`
			Version string `json:"version"`
		} `json:"driver"`
	} `json:"tool"`
	Results []sarifResult `json:"results"`
}

type sarifResult struct {
	RuleID  string `json:"ruleId"`
	Level   string `json:"level"`
	Message struct {
		Text string `json:"text"`
	} `json:"message"`
	Locations []struct {
		PhysicalLocation struct {
			ArtifactLocation struct {
				URI string `json:"uri"`
			} `json:"artifactLocation"`
		} `json:"physicalLocation"`
	} `json:"locations"`
}

// Parse extracts findings from SARIF JSON.
func Parse(data []byte) ([]evidence.FindingPayload, error) {
	var report sarifReport
	if err := json.Unmarshal(data, &report); err != nil {
		return nil, fmt.Errorf("sarif.Parse: %w", err)
	}

	var findings []evidence.FindingPayload
	for _, run := range report.Runs {
		toolName := strings.ToLower(strings.TrimSpace(run.Tool.Driver.Name))
		if toolName == "" {
			toolName = "unknown"
		}
		toolVersion := strings.TrimSpace(run.Tool.Driver.Version)
		for _, result := range run.Results {
			resource := ""
			if len(result.Locations) > 0 {
				resource = result.Locations[0].PhysicalLocation.ArtifactLocation.URI
			}
			findings = append(findings, evidence.FindingPayload{
				Tool:        toolName,
				ToolVersion: toolVersion,
				RuleID:      result.RuleID,
				Severity:    mapSeverity(result.Level),
				Resource:    resource,
				Message:     result.Message.Text,
			})
		}
	}

	return findings, nil
}

func mapSeverity(level string) string {
	switch strings.ToLower(strings.TrimSpace(level)) {
	case "critical":
		return "critical"
	case "high", "error":
		return "high"
	case "medium", "warning":
		return "medium"
	case "low", "note":
		return "low"
	default:
		return "info"
	}
}
