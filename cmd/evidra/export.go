package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"samebits.com/evidra/pkg/export"
)

func runExport(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("export", flag.ContinueOnError)
	fs.SetOutput(stderr)

	evidenceDir := fs.String("evidence-dir", "", "evidence directory to export")
	outputDir := fs.String("output", "", "output directory (default: evidence-export-<timestamp>)")
	anonymize := fs.Bool("anonymize", true, "anonymize identifiers")
	includeScorecard := fs.Bool("include-scorecard", false, "include scorecard.json if available")

	if err := fs.Parse(args); err != nil {
		return 2
	}

	if strings.TrimSpace(*evidenceDir) == "" {
		defaultDir := filepath.Join(os.Getenv("HOME"), ".evidra", "evidence")
		if _, err := os.Stat(defaultDir); err == nil {
			*evidenceDir = defaultDir
		} else {
			fmt.Fprintln(stderr, "export: --evidence-dir is required")
			return 2
		}
	}

	if strings.TrimSpace(*outputDir) == "" {
		*outputDir = fmt.Sprintf("evidence-export-%s", time.Now().UTC().Format("20060102-150405"))
	}

	err := export.Export(export.Options{
		EvidenceDir:      *evidenceDir,
		OutputDir:        *outputDir,
		Anonymize:        *anonymize,
		IncludeScorecard: *includeScorecard,
	})
	if err != nil {
		fmt.Fprintf(stderr, "export: %v\n", err)
		return 1
	}

	fmt.Fprintf(stdout, "exported to %s\n", *outputDir)
	if *anonymize {
		fmt.Fprintln(stdout, "  anonymized: yes (identifiers hashed, artifacts stripped)")
	}

	// List bundle contents
	entries, _ := os.ReadDir(*outputDir)
	for _, e := range entries {
		info, _ := e.Info()
		if info != nil {
			fmt.Fprintf(stdout, "  %s (%d bytes)\n", e.Name(), info.Size())
		}
	}

	return 0
}
