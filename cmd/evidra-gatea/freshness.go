package main

// Refusing to measure a stale build.
//
// The runner shells out to `bin/evidra-mcp`; the Go tests build their own endpoint
// binary in a temp directory (see TestMain in pkg/proxy). Those two sources of truth
// disagree silently whenever a change lands and only one of them is rebuilt, and the
// result is not a failure but a perfectly well-formed set of runs that measured
// something other than the code under review.
//
// This happened for real: an 8-session probe of the in-band report feedback reported
// zero feedback deliveries, because the probe ran against an endpoint binary built
// before the feature existed while the tests proving the feature worked passed the same
// moment. The check below is that incident turned into a precondition.

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// freshnessChecks decides which artifacts a run set may not measure while stale.
//
// The rule is "freshness of what participates": the fixture and the endpoint's own source
// trees are required for wrapped modes, while a baseline run - which starts no endpoint -
// must not be blocked by an endpoint binary it never execs. Refusing a baseline measurement
// because bin/evidra-mcp is old would be the staleness check asserting the thing it is
// supposed to be agnostic about.
func freshnessChecks(modes []string, endpointBin, fixtureBin, runnerBin string) []binaryFreshness {
	checks := []binaryFreshness{
		{binary: fixtureBin, sources: []string{"cmd/evidra-fixture"}},
		// The runner grades the experiment, so a runner older than its own grading code is
		// the same failure as an endpoint older than the feature: the artifact describes a
		// program that is not the one under review.
		{binary: runnerBin, sources: []string{"cmd/evidra-gatea"}},
	}
	if needsEndpointBinary(modes) {
		checks = append([]binaryFreshness{
			{binary: endpointBin, sources: []string{"cmd/evidra-mcp", "pkg/proxy", "pkg/evidence", "pkg/report"}},
		}, checks...)
	}
	return checks
}

// binaryFreshness pairs a built binary with the source trees that must not be newer
// than it.
type binaryFreshness struct {
	binary  string
	sources []string
}

// checkBinaryFreshness returns an error naming the first source file newer than the
// binary it belongs to. A missing binary or missing source tree is not this check's
// problem: the caller already stats the binary, and a source tree that is absent means
// the layout changed, not that the build is stale.
func checkBinaryFreshness(pairs []binaryFreshness) error {
	for _, pair := range pairs {
		info, err := os.Stat(pair.binary)
		if err != nil {
			continue
		}
		built := info.ModTime()
		for _, root := range pair.sources {
			newest, newestName, found, err := newestGoFile(root)
			if err != nil || !found {
				continue
			}
			if newest.After(built) {
				return fmt.Errorf(
					"endpoint binary %s was built %s, but %s changed at %s: rebuild it (%s) before measuring, "+
						"or the run set will describe an older build",
					pair.binary, built.Format("2006-01-02 15:04:05"), newestName,
					newest.Format("2006-01-02 15:04:05"), rebuildHint(pair.binary))
			}
		}
	}
	return nil
}

// newestGoFile walks a tree for the most recently modified non-test .go file. Editing a
// test does not change the binary, and refusing to run an experiment because someone
// fixed a comment in a _test.go file would train the operator to delete the check.
func newestGoFile(root string) (time.Time, string, bool, error) {
	var newest time.Time
	name := ""
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return nil // unreadable corner of the tree: not a staleness signal
		}
		if entry.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return nil
		}
		if info.ModTime().After(newest) {
			newest, name = info.ModTime(), path
		}
		return nil
	})
	if err != nil {
		return time.Time{}, "", false, err
	}
	return newest, name, name != "", nil
}

// rebuildHint prints the command that would have fixed it.
func rebuildHint(binary string) string {
	return "go build -o " + binary + " ./cmd/" + filepath.Base(binary) + "/"
}
