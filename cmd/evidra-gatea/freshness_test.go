package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func writeGoFile(t *testing.T, path string, modTime time.Time) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("package x\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(path, modTime, modTime); err != nil {
		t.Fatal(err)
	}
}

func TestFreshnessPreflightRefusesAStaleEndpoint(t *testing.T) {
	root := t.TempDir()
	binary := filepath.Join(root, "evidra-mcp")
	now := time.Now()
	writeGoFile(t, binary, now.Add(-time.Hour))
	source := filepath.Join(root, "cmd", "evidra-mcp", "main.go")
	writeGoFile(t, source, now.Add(-time.Minute))

	err := checkBinaryFreshness([]binaryFreshness{{binary: binary, sources: []string{filepath.Dir(source)}}})
	if err == nil {
		t.Fatal("a build older than its source was accepted as fresh")
	}
	if !strings.Contains(err.Error(), "main.go") || !strings.Contains(err.Error(), "go build -o") {
		t.Errorf("the error neither names the changed file nor says how to fix it: %v", err)
	}

	// Rebuilding clears it.
	writeGoFile(t, binary, now.Add(time.Minute))
	if err := checkBinaryFreshness([]binaryFreshness{{binary: binary, sources: []string{filepath.Dir(source)}}}); err != nil {
		t.Errorf("a fresh build was rejected: %v", err)
	}
}

func TestFreshnessPreflightIgnoresMissingInputs(t *testing.T) {
	root := t.TempDir()
	// No binary at all is the caller's Stat problem, not a staleness verdict; a
	// missing source tree means the layout moved, not that the build is old.
	if err := checkBinaryFreshness([]binaryFreshness{
		{binary: filepath.Join(root, "absent"), sources: []string{filepath.Join(root, "absent-source")}},
	}); err != nil {
		t.Errorf("missing inputs reported as staleness: %v", err)
	}
}

// TestFreshnessIgnoresTestOnlyEdits keeps the check aimed at what can actually change a
// binary. A guard that blocks an experiment because someone edited a comment in a
// _test.go file gets worked around, and the workaround is worse than no check.
func TestFreshnessIgnoresTestOnlyEdits(t *testing.T) {
	root := t.TempDir()
	binary := filepath.Join(root, "evidra-mcp")
	now := time.Now()
	writeGoFile(t, binary, now.Add(-time.Hour))
	dir := filepath.Join(root, "cmd", "evidra-mcp")
	writeGoFile(t, filepath.Join(dir, "main.go"), now.Add(-2*time.Hour))
	writeGoFile(t, filepath.Join(dir, "main_test.go"), now)

	if err := checkBinaryFreshness([]binaryFreshness{{binary: binary, sources: []string{dir}}}); err != nil {
		t.Errorf("a test-only edit blocked the run: %v", err)
	}
}
