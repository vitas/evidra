package experiments

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

func writeJSONL(path string, value any) error {
	b, err := json.Marshal(value)
	if err != nil {
		return err
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	if _, err := f.Write(b); err != nil {
		return err
	}
	_, err = f.Write([]byte("\n"))
	return err
}

func extractRiskPrediction(output map[string]any) (level string, tags []string) {
	level = readString(output, "predicted_risk_level")
	if level == "" {
		level = readString(output, "risk_level")
	}
	tags = readStringSlice(output, "predicted_risk_details")
	if len(tags) == 0 {
		tags = readStringSlice(output, "predicted_risk_tags")
	}
	if len(tags) == 0 {
		tags = readStringSlice(output, "risk_details")
	}
	if len(tags) == 0 {
		tags = readStringSlice(output, "risk_tags")
	}
	return level, normalizeTags(tags)
}

func readString(m map[string]any, key string) string {
	v, ok := m[key]
	if !ok {
		return ""
	}
	switch t := v.(type) {
	case string:
		return t
	case json.Number:
		return t.String()
	default:
		return fmt.Sprintf("%v", t)
	}
}

func readStringSlice(m map[string]any, key string) []string {
	v, ok := m[key]
	if !ok || v == nil {
		return nil
	}
	raw, ok := v.([]any)
	if !ok {
		if cast, ok := v.([]string); ok {
			return cast
		}
		return nil
	}
	out := make([]string, 0, len(raw))
	for _, item := range raw {
		out = append(out, fmt.Sprintf("%v", item))
	}
	return out
}

func writeRunArtifacts(runDir, stdoutPath, stderrPath, outputPath, rawPath string, stdoutText, stderrText string, output map[string]any, rawText string) error {
	if err := os.MkdirAll(runDir, 0o755); err != nil {
		return err
	}
	if err := writeTextFile(stdoutPath, stdoutText); err != nil {
		return err
	}
	if err := writeTextFile(stderrPath, stderrText); err != nil {
		return err
	}
	if err := writeJSONFile(outputPath, output); err != nil {
		return err
	}
	if rawText == "" {
		rawText = ""
	}
	if err := writeTextFile(rawPath, rawText); err != nil {
		return err
	}
	return nil
}

func ensureObjectOutput(output map[string]any, fallback map[string]any) map[string]any {
	if output == nil {
		return fallback
	}
	return output
}

func runPaths(outDir, runID string) (runDir, stdoutPath, stderrPath, outputPath, rawPath, resultPath string) {
	runDir = filepath.Join(outDir, runID)
	stdoutPath = filepath.Join(runDir, "agent_stdout.log")
	stderrPath = filepath.Join(runDir, "agent_stderr.log")
	outputPath = filepath.Join(runDir, "agent_output.json")
	rawPath = filepath.Join(runDir, "agent_raw_stream.jsonl")
	resultPath = filepath.Join(runDir, "result.json")
	return runDir, stdoutPath, stderrPath, outputPath, rawPath, resultPath
}
