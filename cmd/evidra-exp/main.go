package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"

	"samebits.com/evidra-benchmark/internal/experiments"
	"samebits.com/evidra-benchmark/pkg/version"
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
		fmt.Fprintf(stdout, "evidra-exp %s (commit: %s, built: %s)\n", version.Version, version.Commit, version.Date)
		return 0
	case "artifact":
		return runArtifactSubcommand(args[1:], stdout, stderr)
	case "execution":
		return runExecutionSubcommand(args[1:], stdout, stderr)
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
	if args[0] != "run" {
		fmt.Fprintf(stderr, "unknown artifact subcommand: %s\n", args[0])
		printArtifactUsage(stderr)
		return 2
	}
	opts, code := parseArtifactFlags(args[1:], stderr)
	if code != 0 {
		return code
	}
	if err := experiments.RunArtifact(context.Background(), opts, stdout, stderr); err != nil {
		fmt.Fprintf(stderr, "run-agent-experiments: FAIL %v\n", err)
		return 1
	}
	return 0
}

func runExecutionSubcommand(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 || isHelpArg(args[0]) {
		printExecutionUsage(stdout)
		return 0
	}
	if args[0] != "run" {
		fmt.Fprintf(stderr, "unknown execution subcommand: %s\n", args[0])
		printExecutionUsage(stderr)
		return 2
	}
	opts, code := parseExecutionFlags(args[1:], stderr)
	if code != 0 {
		return code
	}
	if err := experiments.RunExecution(context.Background(), opts, stdout, stderr); err != nil {
		fmt.Fprintf(stderr, "run-agent-execution-experiments: FAIL %v\n", err)
		return 1
	}
	return 0
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

	return experiments.ArtifactRunOptions{
		ModelID:        strings.TrimSpace(*modelID),
		Provider:       strings.TrimSpace(*provider),
		PromptVersion:  strings.TrimSpace(*promptVersion),
		PromptFile:     strings.TrimSpace(*promptFile),
		Temperature:    tempPtr,
		Mode:           strings.TrimSpace(*mode),
		Repeats:        *repeats,
		TimeoutSeconds: *timeoutSeconds,
		CaseFilter:     *caseFilter,
		MaxCases:       *maxCases,
		CasesDir:       strings.TrimSpace(*casesDir),
		OutDir:         strings.TrimSpace(*outDir),
		CleanOutDir:    *cleanOutDir,
		Agent:          strings.TrimSpace(*agent),
		DryRun:         *dryRun,
	}, 0
}

func parseExecutionFlags(args []string, stderr io.Writer) (experiments.ExecutionRunOptions, int) {
	fs := flag.NewFlagSet("execution run", flag.ContinueOnError)
	fs.SetOutput(stderr)
	modelID := fs.String("model-id", "", "Model id (required)")
	provider := fs.String("provider", "unknown", "Provider label")
	promptVersion := fs.String("prompt-version", "", "Prompt version label")
	promptFile := fs.String("prompt-file", experiments.DefaultPromptFile, "Prompt file path")
	scenariosDir := fs.String("scenarios-dir", experiments.DefaultExecutionScenarios, "Scenario directory")
	mode := fs.String("mode", "local-mcp", "Execution mode label")
	repeats := fs.Int("repeats", 1, "Repeats per scenario")
	timeoutSeconds := fs.Int("timeout-seconds", 600, "Per-run timeout in seconds")
	scenarioFilter := fs.String("scenario-filter", "", "Regex filter for scenario_id")
	maxScenarios := fs.Int("max-scenarios", 0, "Max selected scenarios")
	outDir := fs.String("out-dir", "", "Output directory")
	cleanOutDir := fs.Bool("clean-out-dir", false, "Remove existing files in out-dir before run")
	agent := fs.String("agent", "", "Agent adapter: mcp-kubectl|dry-run")
	dryRun := fs.Bool("dry-run", false, "Skip real adapter execution")
	if err := fs.Parse(args); err != nil {
		return experiments.ExecutionRunOptions{}, 2
	}

	return experiments.ExecutionRunOptions{
		ModelID:        strings.TrimSpace(*modelID),
		Provider:       strings.TrimSpace(*provider),
		PromptVersion:  strings.TrimSpace(*promptVersion),
		PromptFile:     strings.TrimSpace(*promptFile),
		ScenariosDir:   strings.TrimSpace(*scenariosDir),
		Mode:           strings.TrimSpace(*mode),
		Repeats:        *repeats,
		TimeoutSeconds: *timeoutSeconds,
		ScenarioFilter: *scenarioFilter,
		MaxScenarios:   *maxScenarios,
		OutDir:         strings.TrimSpace(*outDir),
		CleanOutDir:    *cleanOutDir,
		Agent:          strings.TrimSpace(*agent),
		DryRun:         *dryRun,
	}, 0
}

func isHelpArg(arg string) bool {
	return arg == "help" || arg == "--help" || arg == "-h"
}

func printUsage(w io.Writer) {
	fmt.Fprintln(w, "evidra-exp <command>")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Commands:")
	fmt.Fprintln(w, "  artifact run    Run artifact-only experiments")
	fmt.Fprintln(w, "  execution run   Run execution experiments")
	fmt.Fprintln(w, "  version         Print version")
	fmt.Fprintln(w, "  help            Show help")
}

func printArtifactUsage(w io.Writer) {
	fmt.Fprintln(w, "evidra-exp artifact run [options]")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Options:")
	fmt.Fprintln(w, "  --model-id <id>          Required model id")
	fmt.Fprintln(w, "  --agent <name>           claude|bifrost|dry-run")
	fmt.Fprintln(w, "  --cases-dir <path>       Default: tests/benchmark/cases")
	fmt.Fprintln(w, "  --out-dir <path>         Default: experiments/results/<timestamp>")
	fmt.Fprintln(w, "  --clean-out-dir          Remove files in out-dir before run")
	fmt.Fprintln(w, "  --dry-run                Skip real adapter execution")
}

func printExecutionUsage(w io.Writer) {
	fmt.Fprintln(w, "evidra-exp execution run [options]")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Options:")
	fmt.Fprintln(w, "  --model-id <id>          Required model id")
	fmt.Fprintln(w, "  --agent <name>           mcp-kubectl|dry-run")
	fmt.Fprintln(w, "  --scenarios-dir <path>   Default: tests/experiments/execution-scenarios")
	fmt.Fprintln(w, "  --out-dir <path>         Default: experiments/results/<timestamp>-execution")
	fmt.Fprintln(w, "  --clean-out-dir          Remove files in out-dir before run")
	fmt.Fprintln(w, "  --dry-run                Skip real adapter execution")
}
