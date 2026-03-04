package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"samebits.com/evidra-benchmark/pkg/mcpserver"
	"samebits.com/evidra-benchmark/pkg/version"
)

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("evidra-mcp", flag.ContinueOnError)
	fs.SetOutput(stderr)
	showVersion := fs.Bool("version", false, "Print version information and exit")
	evidenceFlag := fs.String("evidence-dir", "", "Path to store evidence records")
	environmentFlag := fs.String("environment", "", "Environment label (production, staging, development)")
	retryFlag := fs.Bool("retry-tracker", false, "Enable retry loop tracking")
	helpFlag := fs.Bool("help", false, "Show help")

	if err := fs.Parse(args); err != nil {
		return 2
	}
	if *showVersion {
		fmt.Fprintf(stdout, "evidra-mcp %s (commit: %s, built: %s)\n",
			version.Version, version.Commit, version.Date)
		return 0
	}
	if *helpFlag {
		printHelp(stderr)
		return 0
	}

	evidencePath := resolveEvidencePath(*evidenceFlag)
	environment := resolveEnvironment(*environmentFlag)

	server := mcpserver.NewServer(mcpserver.Options{
		Name:         "evidra-benchmark",
		Version:      version.Version,
		EvidencePath: evidencePath,
		Environment:  environment,
		RetryTracker: *retryFlag || envBool("EVIDRA_RETRY_TRACKER", false),
	})

	logger := log.New(stderr, "", log.LstdFlags)
	logger.Printf("evidra-mcp running (evidence: %s, env: %s)", evidencePath, environment)

	if err := server.Run(context.Background(), &mcp.StdioTransport{}); err != nil {
		fmt.Fprintf(stderr, "run mcp server: %v\n", err)
		return 1
	}
	return 0
}

func resolveEvidencePath(explicit string) string {
	if explicit != "" {
		return explicit
	}
	if v := strings.TrimSpace(os.Getenv("EVIDRA_EVIDENCE_DIR")); v != "" {
		return v
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return filepath.Join(os.TempDir(), ".evidra", "evidence")
	}
	return filepath.Join(home, ".evidra", "evidence")
}

func resolveEnvironment(explicit string) string {
	if explicit != "" {
		return explicit
	}
	if v := strings.TrimSpace(os.Getenv("EVIDRA_ENVIRONMENT")); v != "" {
		return v
	}
	return ""
}

func envBool(key string, fallback bool) bool {
	v := strings.TrimSpace(strings.ToLower(os.Getenv(key)))
	switch v {
	case "1", "true", "yes":
		return true
	case "0", "false", "no":
		return false
	}
	return fallback
}

func printHelp(w io.Writer) {
	fmt.Fprintln(w, "evidra-mcp — benchmark flight recorder for AI agent infrastructure operations.")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "USAGE:")
	fmt.Fprintln(w, "  evidra-mcp [flags]")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "FLAGS:")
	fmt.Fprintln(w, "  --evidence-dir <dir>    Where to store evidence chain (default: ~/.evidra/evidence)")
	fmt.Fprintln(w, "  --environment <label>   Environment label (production, staging, development)")
	fmt.Fprintln(w, "  --retry-tracker         Enable retry loop tracking")
	fmt.Fprintln(w, "  --version               Print version and exit")
	fmt.Fprintln(w, "  --help                  Show this help")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "ENVIRONMENT:")
	fmt.Fprintln(w, "  EVIDRA_EVIDENCE_DIR     Override evidence directory")
	fmt.Fprintln(w, "  EVIDRA_ENVIRONMENT      Default environment label")
	fmt.Fprintln(w, "  EVIDRA_RETRY_TRACKER    Enable retry tracking (true/false)")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "TOOLS:")
	fmt.Fprintln(w, "  prescribe   Analyze artifact BEFORE execution (returns risk + prescription_id)")
	fmt.Fprintln(w, "  report      Record outcome AFTER execution (exit code + signals)")
	fmt.Fprintln(w, "  get_event   Look up evidence record by event_id")
}
