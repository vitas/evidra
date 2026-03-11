package main

import (
	"bytes"
	"strings"
	"testing"
)

func TestRegisteredCommandsExposeRebrandedCLI(t *testing.T) {
	t.Parallel()

	commands := registeredCommands()

	for _, want := range []string{"record", "import", "import-findings"} {
		if _, ok := commands[want]; !ok {
			t.Fatalf("expected command %q to be registered", want)
		}
	}

	for _, old := range []string{"run", "ingest-findings"} {
		if _, ok := commands[old]; ok {
			t.Fatalf("old command %q should not be registered", old)
		}
	}
}

func TestPrintUsageUsesCurrentProductPositioning(t *testing.T) {
	t.Parallel()

	var out bytes.Buffer
	printUsage(&out)

	got := out.String()
	if !strings.Contains(got, "evidra -- behavioral reliability for infrastructure automation") {
		t.Fatalf("usage header = %q", got)
	}
	if strings.Contains(got, "evidra-benchmark") {
		t.Fatalf("usage should not mention evidra-benchmark: %q", got)
	}
}

func TestPrintUsageShowsRebrandedCommands(t *testing.T) {
	t.Parallel()

	var out bytes.Buffer
	printUsage(&out)

	got := out.String()
	for _, want := range []string{"record", "import", "import-findings"} {
		if !strings.Contains(got, want) {
			t.Fatalf("usage missing %q: %q", want, got)
		}
	}

	for _, old := range []string{"run", "ingest-findings"} {
		if strings.Contains(got, "  "+old+" ") {
			t.Fatalf("usage should not list %q: %q", old, got)
		}
	}
}

func TestCmdVersionUsesCurrentBinaryName(t *testing.T) {
	t.Parallel()

	var out bytes.Buffer
	if code := cmdVersion(nil, &out, &out); code != 0 {
		t.Fatalf("cmdVersion exit code = %d", code)
	}

	got := out.String()
	if !strings.HasPrefix(got, "evidra ") {
		t.Fatalf("version output = %q", got)
	}
	if strings.Contains(got, "evidra-benchmark") {
		t.Fatalf("version output should not mention evidra-benchmark: %q", got)
	}
}
