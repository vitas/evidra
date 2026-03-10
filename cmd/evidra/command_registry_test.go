package main

import (
	"bytes"
	"os"
	"strings"
	"testing"
)

func TestCommandRegistryIncludesPrimaryCommands(t *testing.T) {
	t.Parallel()

	commands := registeredCommands()
	want := []string{
		"scorecard",
		"explain",
		"compare",
		"run",
		"prescribe",
		"report",
		"record",
		"validate",
		"ingest-findings",
		"prompts",
		"detectors",
		"keygen",
		"version",
	}

	if len(commands) != len(want) {
		t.Fatalf("registered command count = %d, want %d", len(commands), len(want))
	}
	for _, name := range want {
		if _, ok := commands[name]; !ok {
			t.Fatalf("registered commands missing %q", name)
		}
	}
}

func TestMainHelpIncludesRegistryCommands(t *testing.T) {
	t.Parallel()

	var out, errBuf bytes.Buffer
	code := run([]string{"help"}, &out, &errBuf)
	if code != 0 {
		t.Fatalf("help exit %d: %s", code, errBuf.String())
	}

	for _, want := range []string{"scorecard", "record", "detectors", "version"} {
		if !strings.Contains(out.String(), want) {
			t.Fatalf("help output missing %q:\n%s", want, out.String())
		}
	}
}

func TestMainGoDoesNotContainExtractedCommandHandlers(t *testing.T) {
	t.Parallel()

	data, err := os.ReadFile("main.go")
	if err != nil {
		t.Fatalf("read main.go: %v", err)
	}

	for _, forbidden := range []string{
		"func cmdScorecard(",
		"func cmdExplain(",
		"func cmdCompare(",
		"func cmdPrescribe(",
		"func cmdReport(",
		"func cmdValidate(",
		"func cmdIngestFindings(",
	} {
		if strings.Contains(string(data), forbidden) {
			t.Fatalf("main.go still contains extracted handler %q", forbidden)
		}
	}
}
