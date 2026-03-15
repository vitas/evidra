package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"time"

	"samebits.com/evidra/internal/experiments"
	"samebits.com/evidra/pkg/version"
)

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		printUsage(stdout)
		return 0
	}

	switch args[0] {
	case "help", "--help", "-h":
		printUsage(stdout)
		return 0
	case "version":
		fmt.Fprintln(stdout, version.BuildString("evidra-exp"))
		return 0
	case "artifact":
		return runArtifactSubcommand(args[1:], stdout, stderr)
	case "execution":
		printExecutionMigrationNotice(stdout)
		return 0
	default:
		fmt.Fprintf(stderr, "unknown command: %s\n", args[0])
		printUsage(stderr)
		return 2
	}
}

func runArtifactSubcommand(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 || isHelpArg(args[0]) {
		printArtifactUsage(stdout)
		return 0
	}
	switch args[0] {
	case "run":
		opts, code := parseArtifactFlags(args[1:], stderr)
		if code != 0 {
			return code
		}
		if err := experiments.RunArtifact(context.Background(), opts, stdout, stderr); err != nil {
			fmt.Fprintf(stderr, "run-agent-experiments: FAIL %v\n", err)
			return 1
		}
		return 0
	case "baseline":
		opts, code := parseArtifactBaselineFlags(args[1:], stderr)
		if code != 0 {
			return code
		}
		if err := experiments.RunArtifactBaseline(context.Background(), opts, stdout, stderr); err != nil {
			fmt.Fprintf(stderr, "run-agent-experiments-baseline: FAIL %v\n", err)
			return 1
		}
		return 0
	default:
		fmt.Fprintf(stderr, "unknown artifact subcommand: %s\n", args[0])
		printArtifactUsage(stderr)
		return 2
	}
}

func parseArtifactFlags(args []string, stderr io.Writer) (experiments.ArtifactRunOptions, int) {
	fs := flag.NewFlagSet("artifact run", flag.ContinueOnError)
	fs.SetOutput(stderr)
	modelID := fs.String("model-id", "", "Model id (required)")
	provider := fs.String("provider", "unknown", "Provider label")
	promptVersion := fs.String("prompt-version", "", "Prompt version label")
	promptFile := fs.String("prompt-file", experiments.DefaultPromptFile, "Prompt file path")
	temperature := fs.String("temperature", "", "Temperature (number)")
	mode := fs.String("mode", "custom", "Execution mode label")
	repeats := fs.Int("repeats", 3, "Repeats per case")
	timeoutSeconds := fs.Int("timeout-seconds", 300, "Per-run timeout in seconds")
	caseFilter := fs.String("case-filter", "", "Regex filter for case_id")
	maxCases := fs.Int("max-cases", 0, "Max selected cases")
	casesDir := fs.String("cases-dir", experiments.DefaultArtifactCasesDir, "Cases directory")
	outDir := fs.String("out-dir", "", "Output directory")
	cleanOutDir := fs.Bool("clean-out-dir", false, "Remove existing files in out-dir before run")
	delayBetweenRuns := fs.String("delay-between-runs", "0s", "Sleep duration between runs (e.g. 2s, 500ms)")
	agent := fs.String("agent", "", "Agent adapter: claude|bifrost|dry-run")
	dryRun := fs.Bool("dry-run", false, "Skip real adapter execution")
	if err := fs.Parse(args); err != nil {
		return experiments.ArtifactRunOptions{}, 2
	}

	var tempPtr *float64
	if strings.TrimSpace(*temperature) != "" {
		v, err := strconv.ParseFloat(*temperature, 64)
		if err != nil {
			fmt.Fprintln(stderr, "--temperature must be numeric")
			return experiments.ArtifactRunOptions{}, 2
		}
		tempPtr = &v
	}
	delayValue, err := time.ParseDuration(strings.TrimSpace(*delayBetweenRuns))
	if err != nil {
		fmt.Fprintln(stderr, "--delay-between-runs must be a duration like 2s or 500ms")
		return experiments.ArtifactRunOptions{}, 2
	}

	return experiments.ArtifactRunOptions{
		ModelID:          strings.TrimSpace(*modelID),
		Provider:         strings.TrimSpace(*provider),
		PromptVersion:    strings.TrimSpace(*promptVersion),
		PromptFile:       strings.TrimSpace(*promptFile),
		Temperature:      tempPtr,
		Mode:             strings.TrimSpace(*mode),
		Repeats:          *repeats,
		TimeoutSeconds:   *timeoutSeconds,
		CaseFilter:       *caseFilter,
		MaxCases:         *maxCases,
		CasesDir:         strings.TrimSpace(*casesDir),
		OutDir:           strings.TrimSpace(*outDir),
		CleanOutDir:      *cleanOutDir,
		DelayBetweenRuns: delayValue,
		Agent:            strings.TrimSpace(*agent),
		DryRun:           *dryRun,
	}, 0
}

func parseArtifactBaselineFlags(args []string, stderr io.Writer) (experiments.ArtifactBaselineRunOptions, int) {
	fs := flag.NewFlagSet("artifact baseline", flag.ContinueOnError)
	fs.SetOutput(stderr)
	modelIDs := fs.String("model-ids", "", "Comma-separated model ids (required)")
	provider := fs.String("provider", "unknown", "Provider label")
	promptVersion := fs.String("prompt-version", "", "Prompt version label")
	promptFile := fs.String("prompt-file", experiments.DefaultPromptFile, "Prompt file path")
	temperature := fs.String("temperature", "", "Temperature (number)")
	mode := fs.String("mode", "custom", "Execution mode label")
	repeats := fs.Int("repeats", 3, "Repeats per case")
	timeoutSeconds := fs.Int("timeout-seconds", 300, "Per-run timeout in seconds")
	caseFilter := fs.String("case-filter", "", "Regex filter for case_id")
	maxCases := fs.Int("max-cases", 0, "Max selected cases")
	casesDir := fs.String("cases-dir", experiments.DefaultArtifactCasesDir, "Cases directory")
	outDir := fs.String("out-dir", "", "Output directory")
	cleanOutDir := fs.Bool("clean-out-dir", false, "Remove existing files in out-dir before run")
	delayBetweenRuns := fs.String("delay-between-runs", "0s", "Sleep duration between runs (e.g. 2s, 500ms)")
	agent := fs.String("agent", "", "Agent adapter: claude|bifrost|dry-run")
	dryRun := fs.Bool("dry-run", false, "Skip real adapter execution")
	if err := fs.Parse(args); err != nil {
		return experiments.ArtifactBaselineRunOptions{}, 2
	}

	ids := splitAndTrimCSV(*modelIDs)
	if len(ids) == 0 {
		fmt.Fprintln(stderr, "--model-ids must contain at least one model id")
		return experiments.ArtifactBaselineRunOptions{}, 2
	}

	var tempPtr *float64
	if strings.TrimSpace(*temperature) != "" {
		v, err := strconv.ParseFloat(*temperature, 64)
		if err != nil {
			fmt.Fprintln(stderr, "--temperature must be numeric")
			return experiments.ArtifactBaselineRunOptions{}, 2
		}
		tempPtr = &v
	}
	delayValue, err := time.ParseDuration(strings.TrimSpace(*delayBetweenRuns))
	if err != nil {
		fmt.Fprintln(stderr, "--delay-between-runs must be a duration like 2s or 500ms")
		return experiments.ArtifactBaselineRunOptions{}, 2
	}

	return experiments.ArtifactBaselineRunOptions{
		ModelIDs:         ids,
		Provider:         strings.TrimSpace(*provider),
		PromptVersion:    strings.TrimSpace(*promptVersion),
		PromptFile:       strings.TrimSpace(*promptFile),
		Temperature:      tempPtr,
		Mode:             strings.TrimSpace(*mode),
		Repeats:          *repeats,
		TimeoutSeconds:   *timeoutSeconds,
		CaseFilter:       *caseFilter,
		MaxCases:         *maxCases,
		CasesDir:         strings.TrimSpace(*casesDir),
		OutDir:           strings.TrimSpace(*outDir),
		CleanOutDir:      *cleanOutDir,
		DelayBetweenRuns: delayValue,
		Agent:            strings.TrimSpace(*agent),
		DryRun:           *dryRun,
	}, 0
}

func isHelpArg(arg string) bool {
	return arg == "help" || arg == "--help" || arg == "-h"
}

func splitAndTrimCSV(value string) []string {
	parts := strings.Split(value, ",")
	seen := make(map[string]struct{}, len(parts))
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		s := strings.TrimSpace(p)
		if s == "" {
			continue
		}
		if _, ok := seen[s]; ok {
			continue
		}
		seen[s] = struct{}{}
		out = append(out, s)
	}
	return out
}

func printUsage(w io.Writer) {
	fmt.Fprintln(w, "evidra-exp <command>")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Commands:")
	fmt.Fprintln(w, "  artifact run       Run artifact-only experiments")
	fmt.Fprintln(w, "  artifact baseline  Run multi-model artifact baseline and aggregate metrics")
	fmt.Fprintln(w, "  version            Print version")
	fmt.Fprintln(w, "  help               Show help")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Execution-mode experiments have moved to evidra-infra-bench:")
	fmt.Fprintln(w, "  infra-bench run --provider claude --model sonnet --scenario ...")
}

func printExecutionMigrationNotice(w io.Writer) {
	fmt.Fprintln(w, "Execution-mode experiments have moved to evidra-infra-bench.")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Use infra-bench for real agent + cluster testing:")
	fmt.Fprintln(w, "  infra-bench run --provider claude --model sonnet --scenario ...")
	fmt.Fprintln(w, "  infra-bench run --provider bifrost --model openai/gpt-4o --scenario ...")
	fmt.Fprintln(w, "  infra-bench lab  # interactive TUI")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "See: https://github.com/vitas/evidra-infra-bench")
}

func printArtifactUsage(w io.Writer) {
	fmt.Fprintln(w, "evidra-exp artifact <run|baseline> [options]")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Run options:")
	fmt.Fprintln(w, "  --model-id <id>            Required model id")
	fmt.Fprintln(w, "  --provider <name>          Provider label (default: unknown)")
	fmt.Fprintln(w, "  --prompt-version <label>   Prompt version label")
	fmt.Fprintln(w, "  --prompt-file <path>       Prompt file (default: prompts/experiments/runtime/system_instructions.txt)")
	fmt.Fprintln(w, "  --temperature <float>      Sampling temperature override")
	fmt.Fprintln(w, "  --mode <name>              Execution mode label (default: custom)")
	fmt.Fprintln(w, "  --repeats <n>              Repeats per case (default: 3)")
	fmt.Fprintln(w, "  --timeout-seconds <n>      Per-run timeout in seconds (default: 300)")
	fmt.Fprintln(w, "  --case-filter <regex>      Regex filter for case_id")
	fmt.Fprintln(w, "  --max-cases <n>            Max selected cases")
	fmt.Fprintln(w, "  --cases-dir <path>         Cases directory (default: tests/benchmark/cases)")
	fmt.Fprintln(w, "  --out-dir <path>           Output directory (default: experiments/results/<timestamp>)")
	fmt.Fprintln(w, "  --clean-out-dir            Remove files in out-dir before run")
	fmt.Fprintln(w, "  --delay-between-runs <d>   Sleep duration between runs (e.g. 2s, 500ms)")
	fmt.Fprintln(w, "  --agent <name>             claude|bifrost|dry-run")
	fmt.Fprintln(w, "  --dry-run                  Skip real adapter execution")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Baseline-only options:")
	fmt.Fprintln(w, "  --model-ids <csv>          Comma-separated model ids (required for baseline)")
	fmt.Fprintln(w, "  --out-dir <path>           Output directory (default baseline root: experiments/results/llm/<timestamp>)")
}
