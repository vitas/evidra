package experiments

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
)

type expectedFilePayload struct {
	CaseID             string   `json:"case_id"`
	Category           string   `json:"category"`
	Difficulty         string   `json:"difficulty"`
	GroundTruthPattern string   `json:"ground_truth_pattern"`
	ArtifactRef        string   `json:"artifact_ref"`
	RiskLevel          string   `json:"risk_level"`
	RiskDetails        []string `json:"risk_details_expected"`
}

func loadArtifactCases(casesDir string, caseFilter string, maxCases int) ([]ArtifactCase, error) {
	pattern, err := compileOptionalRegex(caseFilter)
	if err != nil {
		return nil, fmt.Errorf("compile case filter: %w", err)
	}

	expectedFiles, err := listFilesByName(casesDir, "expected.json")
	if err != nil {
		return nil, fmt.Errorf("list expected files: %w", err)
	}

	cases := make([]ArtifactCase, 0, len(expectedFiles))
	for _, expectedPath := range expectedFiles {
		c, ok := parseArtifactCase(expectedPath, pattern)
		if !ok {
			continue
		}
		cases = append(cases, c)
	}
	sort.Slice(cases, func(i, j int) bool { return cases[i].CaseID < cases[j].CaseID })

	if maxCases > 0 && len(cases) > maxCases {
		cases = cases[:maxCases]
	}
	if len(cases) == 0 {
		return nil, fmt.Errorf("no artifact cases selected")
	}
	return cases, nil
}

func parseArtifactCase(expectedPath string, filterPattern *regexp.Regexp) (ArtifactCase, bool) {
	var payload expectedFilePayload
	b, err := os.ReadFile(expectedPath)
	if err != nil {
		return ArtifactCase{}, false
	}
	if err := json.Unmarshal(b, &payload); err != nil {
		return ArtifactCase{}, false
	}
	if payload.CaseID == "" || payload.ArtifactRef == "" {
		return ArtifactCase{}, false
	}

	if filterPattern != nil && !filterPattern.MatchString(payload.CaseID) {
		return ArtifactCase{}, false
	}

	artifactPath := filepath.Join(filepath.Dir(expectedPath), payload.ArtifactRef)
	if _, err := os.Stat(artifactPath); err != nil {
		return ArtifactCase{}, false
	}

	return ArtifactCase{
		CaseID:              payload.CaseID,
		Category:            emptyTo(payload.Category, "unknown"),
		Difficulty:          emptyTo(payload.Difficulty, "unknown"),
		GroundTruthPattern:  payload.GroundTruthPattern,
		ExpectedRiskLevel:   payload.RiskLevel,
		ExpectedRiskDetails: normalizeTags(payload.RiskDetails),
		ArtifactPath:        artifactPath,
		ExpectedJSONPath:    expectedPath,
	}, true
}

type scenarioFilePayload struct {
	ScenarioID        string   `json:"scenario_id"`
	Category          string   `json:"category"`
	Difficulty        string   `json:"difficulty"`
	Tool              string   `json:"tool"`
	Operation         string   `json:"operation"`
	ArtifactPath      string   `json:"artifact_path"`
	ExecuteCmd        string   `json:"execute_cmd"`
	ExpectedExitCode  *int     `json:"expected_exit_code"`
	ExpectedRiskLevel string   `json:"expected_risk_level"`
	ExpectedRiskTags  []string `json:"expected_risk_tags"`
}

func loadExecutionScenarios(dir string, filter string, maxScenarios int) ([]ExecutionScenario, error) {
	pattern, err := compileOptionalRegex(filter)
	if err != nil {
		return nil, fmt.Errorf("compile scenario filter: %w", err)
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("read scenarios dir: %w", err)
	}

	paths := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}
		paths = append(paths, filepath.Join(dir, entry.Name()))
	}
	sort.Strings(paths)

	scenarios := make([]ExecutionScenario, 0, len(paths))
	for _, path := range paths {
		scenario, ok := parseExecutionScenario(path, pattern)
		if !ok {
			continue
		}
		scenarios = append(scenarios, scenario)
	}
	if maxScenarios > 0 && len(scenarios) > maxScenarios {
		scenarios = scenarios[:maxScenarios]
	}
	if len(scenarios) == 0 {
		return nil, fmt.Errorf("no execution scenarios selected")
	}
	return scenarios, nil
}

func parseExecutionScenario(path string, filterPattern *regexp.Regexp) (ExecutionScenario, bool) {
	var payload scenarioFilePayload
	b, err := os.ReadFile(path)
	if err != nil {
		return ExecutionScenario{}, false
	}
	if err := json.Unmarshal(b, &payload); err != nil {
		return ExecutionScenario{}, false
	}
	if payload.ScenarioID == "" || payload.Tool == "" || payload.Operation == "" || payload.ArtifactPath == "" || payload.ExecuteCmd == "" {
		return ExecutionScenario{}, false
	}
	if filterPattern != nil && !filterPattern.MatchString(payload.ScenarioID) {
		return ExecutionScenario{}, false
	}

	artifactPath, ok := resolveArtifactPath(filepath.Dir(path), payload.ArtifactPath)
	if !ok {
		return ExecutionScenario{}, false
	}

	return ExecutionScenario{
		ScenarioID:        payload.ScenarioID,
		Category:          emptyTo(payload.Category, "unknown"),
		Difficulty:        emptyTo(payload.Difficulty, "unknown"),
		Tool:              payload.Tool,
		Operation:         payload.Operation,
		ArtifactPath:      artifactPath,
		ExecuteCommand:    payload.ExecuteCmd,
		ExpectedExitCode:  payload.ExpectedExitCode,
		ExpectedRiskLevel: payload.ExpectedRiskLevel,
		ExpectedRiskTags:  normalizeTags(payload.ExpectedRiskTags),
		SourceJSONPath:    path,
	}, true
}

func resolveArtifactPath(baseDir, candidate string) (string, bool) {
	if filepath.IsAbs(candidate) {
		if _, err := os.Stat(candidate); err != nil {
			return "", false
		}
		return candidate, true
	}

	joined := filepath.Join(baseDir, candidate)
	if _, err := os.Stat(joined); err == nil {
		return joined, true
	}
	cwdJoined := filepath.Join(".", candidate)
	if _, err := os.Stat(cwdJoined); err == nil {
		return cwdJoined, true
	}
	return "", false
}

func emptyTo(v, fallback string) string {
	if v == "" {
		return fallback
	}
	return v
}
