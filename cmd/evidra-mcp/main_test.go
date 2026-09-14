package main

import (
	"bytes"
	"reflect"
	"strings"
	"testing"
)

func TestNormalizeProxyArgs_StripsLeadingSeparator(t *testing.T) {
	got, err := normalizeProxyArgs([]string{"--", "upstream", "--flag"})
	if err != nil {
		t.Fatalf("normalizeProxyArgs: %v", err)
	}
	want := []string{"upstream", "--flag"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("args=%v, want %v", got, want)
	}
}

func TestNormalizeProxyArgs_RejectsMissingCommand(t *testing.T) {
	cases := [][]string{
		nil,
		{},
		{"--"},
	}
	for _, tc := range cases {
		if _, err := normalizeProxyArgs(tc); err == nil {
			t.Fatalf("normalizeProxyArgs(%v): expected error", tc)
		}
	}
}

func TestPrintHelp_DescribesTheEndpointContract(t *testing.T) {
	var out bytes.Buffer
	printHelp(&out)
	help := out.String()

	for _, needle := range []string{
		"--proxy",
		"--enforce",
		"--evidence-dir",
		"--max-message",
		"evidra_prescribe",
		"evidra_report",
		"evidra summarize",
	} {
		if !strings.Contains(help, needle) {
			t.Fatalf("help missing %q:\n%s", needle, help)
		}
	}
	// The removed direct-mode tools must not be advertised: an agent that finds them
	// in help output will call them and get nothing back.
	for _, gone := range []string{"run_command", "collect_diagnostics", "write_file", "describe_tool", "prescribe_full", "prescribe_smart", "full-prescribe"} {
		if strings.Contains(help, gone) {
			t.Errorf("help still advertises the removed %q", gone)
		}
	}
}

func TestRunWithNoWrappingModeFails(t *testing.T) {
	// Standing up without an upstream used to serve the direct MCP tool surface. That
	// surface is gone, so the binary has to say it has nothing to wrap instead of
	// running as something it no longer is.
	var out, errOut bytes.Buffer
	if code := run([]string{"--help"}, &out, &errOut); code != 0 {
		t.Fatalf("--help exit = %d, want 0", code)
	}
	if !strings.Contains(errOut.String(), "--proxy") {
		t.Errorf("--help output lacks the endpoint usage: %q", errOut.String())
	}
	out.Reset()
	errOut.Reset()
	if code := run(nil, &out, &errOut); code != 2 {
		t.Fatalf("no-mode exit = %d, want 2", code)
	}
	if !strings.Contains(errOut.String(), "nothing to wrap") {
		t.Errorf("no-mode message = %q", errOut.String())
	}
}
