package experiments

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// ArtifactBaselineModelSummary captures aggregate metrics for one model run.
type ArtifactBaselineModelSummary struct {
	ModelID            string         `json:"model_id"`
	Provider           string         `json:"provider"`
	RunDir             string         `json:"run_dir"`
	SummaryJSONL       string         `json:"summary_jsonl"`
	RunsTotal          int            `json:"runs_total"`
	PassCount          int            `json:"pass_count"`
	FailCount          int            `json:"fail_count"`
	PassRate           float64        `json:"pass_rate"`
	StatusCounts       map[string]int `json:"status_counts"`
	AvgPrecision       *float64       `json:"avg_precision,omitempty"`
	AvgRecall          *float64       `json:"avg_recall,omitempty"`
	AvgF1              *float64       `json:"avg_f1,omitempty"`
	RiskLevelMatchRate *float64       `json:"risk_level_match_rate,omitempty"`
}

// ArtifactBaselineComparison highlights top-performing models across key metrics.
type ArtifactBaselineComparison struct {
	BestPassRateModel string   `json:"best_pass_rate_model,omitempty"`
	BestPassRate      *float64 `json:"best_pass_rate,omitempty"`
	BestF1Model       string   `json:"best_f1_model,omitempty"`
	BestF1            *float64 `json:"best_f1,omitempty"`
}

// ArtifactBaselineSummary is the machine-readable output for multi-model baseline runs.
type ArtifactBaselineSummary struct {
	SchemaVersion string                         `json:"schema_version"`
	GeneratedAt   string                         `json:"generated_at"`
	OutDir        string                         `json:"out_dir"`
	CasesDir      string                         `json:"cases_dir"`
	CaseFilter    string                         `json:"case_filter,omitempty"`
	MaxCases      int                            `json:"max_cases"`
	Repeats       int                            `json:"repeats"`
	Mode          string                         `json:"mode"`
	Agent         string                         `json:"agent"`
	PromptFile    string                         `json:"prompt_file"`
	PromptVersion string                         `json:"prompt_version,omitempty"`
	ModelCount    int                            `json:"model_count"`
	Models        []ArtifactBaselineModelSummary `json:"models"`
	Comparison    ArtifactBaselineComparison     `json:"comparison"`
}

// RunArtifactBaseline executes the artifact runner across multiple models and writes
// an aggregated summary under opts.OutDir/summary.json.
func RunArtifactBaseline(ctx context.Context, opts ArtifactBaselineRunOptions, stdout, stderr io.Writer) error {
	userProvidedOutDir := strings.TrimSpace(opts.OutDir) != ""
	opts = withArtifactBaselineDefaults(opts)
	if err := validateArtifactBaselineOptions(opts); err != nil {
		return err
	}

	if opts.CleanOutDir {
		cleanTarget := opts.OutDir
		if !userProvidedOutDir {
			cleanTarget = DefaultLLMBaselineOutRoot
		}
		if err := ensureDirClean(cleanTarget); err != nil {
			return err
		}
	}
	if err := os.MkdirAll(opts.OutDir, 0o755); err != nil {
		return fmt.Errorf("mkdir baseline out dir: %w", err)
	}

	fmt.Fprintf(stdout, "run-agent-experiments-baseline: models=%d out_dir=%s\n", len(opts.ModelIDs), opts.OutDir)

	models := make([]ArtifactBaselineModelSummary, 0, len(opts.ModelIDs))
	for idx, modelID := range opts.ModelIDs {
		modelOutDir := filepath.Join(opts.OutDir, safeModelID(modelID))
		fmt.Fprintf(stdout, "run-agent-experiments-baseline: model=%s (%d/%d) out_dir=%s\n", modelID, idx+1, len(opts.ModelIDs), modelOutDir)

		modelOpts := ArtifactRunOptions{
			ModelID:          modelID,
			Provider:         opts.Provider,
			PromptVersion:    opts.PromptVersion,
			PromptFile:       opts.PromptFile,
			Temperature:      opts.Temperature,
			Mode:             opts.Mode,
			Repeats:          opts.Repeats,
			TimeoutSeconds:   opts.TimeoutSeconds,
			CaseFilter:       opts.CaseFilter,
			MaxCases:         opts.MaxCases,
			CasesDir:         opts.CasesDir,
			OutDir:           modelOutDir,
			CleanOutDir:      false,
			DelayBetweenRuns: opts.DelayBetweenRuns,
			Agent:            opts.Agent,
			DryRun:           opts.DryRun,
		}

		if err := RunArtifact(ctx, modelOpts, stdout, stderr); err != nil {
			return fmt.Errorf("model %s failed: %w", modelID, err)
		}

		summary, err := summarizeArtifactModel(modelID, opts.Provider, modelOutDir)
		if err != nil {
			return err
		}
		models = append(models, summary)
	}

	baselineSummary := ArtifactBaselineSummary{
		SchemaVersion: LLMBaselineSchemaVersion,
		GeneratedAt:   time.Now().UTC().Format(time.RFC3339),
		OutDir:        opts.OutDir,
		CasesDir:      opts.CasesDir,
		CaseFilter:    opts.CaseFilter,
		MaxCases:      opts.MaxCases,
		Repeats:       opts.Repeats,
		Mode:          opts.Mode,
		Agent:         opts.Agent,
		PromptFile:    opts.PromptFile,
		PromptVersion: opts.PromptVersion,
		ModelCount:    len(models),
		Models:        models,
		Comparison:    computeBaselineComparison(models),
	}

	summaryPath := filepath.Join(opts.OutDir, "summary.json")
	if err := writeJSONFile(summaryPath, baselineSummary); err != nil {
		return fmt.Errorf("write baseline summary: %w", err)
	}
	fmt.Fprintf(stdout, "run-agent-experiments-baseline: summary=%s\n", summaryPath)
	return nil
}

func withArtifactBaselineDefaults(opts ArtifactBaselineRunOptions) ArtifactBaselineRunOptions {
	if opts.Provider == "" {
		opts.Provider = "unknown"
	}
	if opts.PromptFile == "" {
		opts.PromptFile = DefaultPromptFile
	}
	if opts.CasesDir == "" {
		opts.CasesDir = DefaultArtifactCasesDir
	}
	if opts.Mode == "" {
		opts.Mode = "custom"
	}
	if opts.Repeats <= 0 {
		opts.Repeats = 3
	}
	if opts.TimeoutSeconds <= 0 {
		opts.TimeoutSeconds = 300
	}
	if opts.OutDir == "" {
		opts.OutDir = filepath.Join(DefaultLLMBaselineOutRoot, runStampNow())
	}
	if opts.Agent == "" && opts.DryRun {
		opts.Agent = "dry-run"
	}
	return opts
}

func validateArtifactBaselineOptions(opts ArtifactBaselineRunOptions) error {
	if len(opts.ModelIDs) == 0 {
		return errors.New("--model-ids is required")
	}
	if opts.Agent == "" {
		return errors.New("--agent is required")
	}
	if opts.Repeats < 1 {
		return errors.New("--repeats must be >= 1")
	}
	if opts.TimeoutSeconds < 1 {
		return errors.New("--timeout-seconds must be >= 1")
	}
	if opts.DelayBetweenRuns < 0 {
		return errors.New("--delay-between-runs must be >= 0")
	}
	info, err := os.Stat(opts.CasesDir)
	if err != nil || !info.IsDir() {
		return fmt.Errorf("cases dir not found: %s", opts.CasesDir)
	}
	if _, err := os.Stat(opts.PromptFile); err != nil {
		return fmt.Errorf("prompt file not found: %s", opts.PromptFile)
	}
	return nil
}

func summarizeArtifactModel(modelID, provider, modelOutDir string) (ArtifactBaselineModelSummary, error) {
	summaryPath := filepath.Join(modelOutDir, "summary.jsonl")
	f, err := os.Open(summaryPath)
	if err != nil {
		return ArtifactBaselineModelSummary{}, fmt.Errorf("open summary for model %s: %w", modelID, err)
	}
	defer func() {
		_ = f.Close()
	}()

	type row struct {
		Status     string `json:"status"`
		Pass       bool   `json:"pass"`
		ResultJSON string `json:"result_json"`
	}

	var total, passCount int
	statusCounts := map[string]int{}
	var resultPaths []string

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var r row
		if err := json.Unmarshal([]byte(line), &r); err != nil {
			return ArtifactBaselineModelSummary{}, fmt.Errorf("decode summary row for model %s: %w", modelID, err)
		}
		total++
		if r.Pass {
			passCount++
		}
		statusCounts[r.Status]++
		if strings.TrimSpace(r.ResultJSON) != "" {
			resultPaths = append(resultPaths, r.ResultJSON)
		}
	}
	if err := scanner.Err(); err != nil {
		return ArtifactBaselineModelSummary{}, fmt.Errorf("scan summary for model %s: %w", modelID, err)
	}

	var precisionSum, recallSum, f1Sum float64
	var precisionN, recallN, f1N int
	var levelMatchCount, levelMatchN int

	for _, resultPath := range resultPaths {
		var parsed struct {
			Case struct {
				ExpectedRiskLevel string `json:"expected_risk_level"`
			} `json:"case"`
			Evaluation struct {
				RiskLevelMatch bool     `json:"risk_level_match"`
				Precision      *float64 `json:"precision"`
				Recall         *float64 `json:"recall"`
				F1             *float64 `json:"f1"`
			} `json:"evaluation"`
		}

		b, err := os.ReadFile(resultPath)
		if err != nil {
			return ArtifactBaselineModelSummary{}, fmt.Errorf("read result for model %s: %w", modelID, err)
		}
		if err := json.Unmarshal(b, &parsed); err != nil {
			return ArtifactBaselineModelSummary{}, fmt.Errorf("decode result for model %s: %w", modelID, err)
		}

		if parsed.Evaluation.Precision != nil {
			precisionSum += *parsed.Evaluation.Precision
			precisionN++
		}
		if parsed.Evaluation.Recall != nil {
			recallSum += *parsed.Evaluation.Recall
			recallN++
		}
		if parsed.Evaluation.F1 != nil {
			f1Sum += *parsed.Evaluation.F1
			f1N++
		}

		if strings.TrimSpace(parsed.Case.ExpectedRiskLevel) != "" {
			levelMatchN++
			if parsed.Evaluation.RiskLevelMatch {
				levelMatchCount++
			}
		}
	}

	passRate := 0.0
	if total > 0 {
		passRate = float64(passCount) / float64(total)
	}

	return ArtifactBaselineModelSummary{
		ModelID:            modelID,
		Provider:           provider,
		RunDir:             modelOutDir,
		SummaryJSONL:       summaryPath,
		RunsTotal:          total,
		PassCount:          passCount,
		FailCount:          total - passCount,
		PassRate:           passRate,
		StatusCounts:       statusCounts,
		AvgPrecision:       avgOrNil(precisionSum, precisionN),
		AvgRecall:          avgOrNil(recallSum, recallN),
		AvgF1:              avgOrNil(f1Sum, f1N),
		RiskLevelMatchRate: avgOrNil(float64(levelMatchCount), levelMatchN),
	}, nil
}

func avgOrNil(sum float64, n int) *float64 {
	if n <= 0 {
		return nil
	}
	v := sum / float64(n)
	return &v
}

func computeBaselineComparison(models []ArtifactBaselineModelSummary) ArtifactBaselineComparison {
	var cmp ArtifactBaselineComparison
	if len(models) == 0 {
		return cmp
	}

	bestPass := models[0]
	for _, m := range models[1:] {
		if m.PassRate > bestPass.PassRate || (m.PassRate == bestPass.PassRate && m.ModelID < bestPass.ModelID) {
			bestPass = m
		}
	}
	cmp.BestPassRateModel = bestPass.ModelID
	cmp.BestPassRate = &bestPass.PassRate

	var bestF1 *ArtifactBaselineModelSummary
	for i := range models {
		m := &models[i]
		if m.AvgF1 == nil {
			continue
		}
		if bestF1 == nil || *m.AvgF1 > *bestF1.AvgF1 || (*m.AvgF1 == *bestF1.AvgF1 && m.ModelID < bestF1.ModelID) {
			bestF1 = m
		}
	}
	if bestF1 != nil {
		cmp.BestF1Model = bestF1.ModelID
		cmp.BestF1 = bestF1.AvgF1
	}
	return cmp
}
