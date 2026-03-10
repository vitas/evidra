package main

import (
	"bytes"
	"strings"
	"testing"
)

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
