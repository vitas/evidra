package main

import (
	"bytes"
	"slices"
	"strings"
	"testing"
)

// TestRegistryIsTheThinCLISurface pins §41: the read side of vNext is two commands
// plus version. A removed command reappearing in the usage output would mean the CLI
// advertises behaviour nothing implements any more, which is exactly how the legacy
// surface was still documented after it stopped being the product.
func TestRegistryIsTheThinCLISurface(t *testing.T) {
	names := make([]string, 0, len(orderedCommands))
	for _, c := range orderedCommands {
		names = append(names, c.name)
		if c.run == nil {
			t.Errorf("command %q has no handler", c.name)
		}
		if strings.TrimSpace(c.description) == "" {
			t.Errorf("command %q has an empty description", c.name)
		}
	}
	want := []string{"summarize", "verify", "version"}
	if !slices.Equal(names, want) {
		t.Fatalf("commands = %v, want %v", names, want)
	}
	if _, ok := lookupCommand("scorecard"); ok {
		t.Error("the legacy scorecard command is reachable again")
	}
	var buf bytes.Buffer
	printUsage(&buf)
	out := buf.String()
	listed := usageCommandNames(out)
	for _, removed := range []string{"scorecard", "prescribe", "report", "record", "import", "keygen", "skill"} {
		if slices.Contains(listed, removed) {
			t.Errorf("usage still advertises the removed command %q", removed)
		}
	}
	for _, kept := range want {
		if !slices.Contains(listed, kept) {
			t.Errorf("usage omits %q", kept)
		}
	}
	// The usage text has to say where evidence comes from, since the write side is a
	// different binary.
	if !strings.Contains(out, "evidra-mcp") {
		t.Error("usage does not point at the endpoint that records evidence")
	}
}

func TestRunDispatch(t *testing.T) {
	var out, errOut bytes.Buffer
	if code := run(nil, &out, &errOut); code != 0 {
		t.Fatalf("no args exit = %d, want 0", code)
	}
	if !strings.Contains(out.String(), "COMMANDS:") {
		t.Errorf("no args should print usage to stdout, got %q", out.String())
	}
	out.Reset()
	if code := run([]string{"nope"}, &out, &errOut); code != 2 {
		t.Fatalf("unknown command exit = %d, want 2", code)
	}
	if !strings.Contains(errOut.String(), "unknown command: nope") {
		t.Errorf("unknown command message = %q", errOut.String())
	}
	out.Reset()
	errOut.Reset()
	if code := run([]string{"--version"}, &out, &errOut); code != 0 {
		t.Fatalf("version exit = %d, want 0", code)
	}
	if out.Len() == 0 {
		t.Error("version printed nothing")
	}
	if len(errOut.String()) != 0 {
		t.Errorf("version wrote to stderr: %q", errOut.String())
	}
}

// usageCommandNames reads back the command column of the usage output, so a match is
// a listed command rather than a word inside a sentence.
func usageCommandNames(usage string) []string {
	var names []string
	inCommands := false
	for _, line := range strings.Split(usage, "\n") {
		switch {
		case strings.HasPrefix(line, "COMMANDS:"):
			inCommands = true
		case !inCommands:
		case strings.TrimSpace(line) == "":
			inCommands = false
		default:
			fields := strings.Fields(line)
			if len(fields) >= 2 && strings.HasPrefix(line, "  ") {
				names = append(names, fields[0])
			} else {
				inCommands = false
			}
		}
	}
	return names
}
