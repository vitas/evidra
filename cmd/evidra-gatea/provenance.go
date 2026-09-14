package main

// Binary provenance and analytics invariants for the experiment harness.
//
// The harness is not disposable test code: it is the instrument that produces the
// numbers the product claim rests on, so it needs the same discipline as the evidence
// store. Two incidents on this branch make the case better than any argument would.
//
//   - An 8-session probe reported zero deliveries of the in-band feedback and looked
//     like a measurement. The runner had used bin/evidra-mcp built before the feature
//     existed, while the tests that build their own binary passed the same moment. An
//     invalid experiment does not error; it produces data.
//   - A cell metric once printed 28 against a denominator of 16, because per-run values
//     were summed into a numerator that was never bounded. A number that cannot exist
//     survived an entire review cycle because nothing checked the arithmetic.
//
// So every run now records which binaries produced it, and every summary is refused
// unless its analytics satisfy invariants that do not depend on anyone noticing.

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"os/exec"
	"sync"
	"time"
)

// binaryProvenance identifies the exact artifacts a run was measured against.
type binaryProvenance struct {
	SourceRevision string `json:"source_revision"`
	SourceDirty    bool   `json:"source_dirty"`
	EndpointSHA256 string `json:"endpoint_binary_sha256"`
	FixtureSHA256  string `json:"fixture_binary_sha256"`
	RunnerSHA256   string `json:"runner_binary_sha256"`
	ModelID        string `json:"model_id,omitempty"`
	CapturedAt     string `json:"captured_at"`
}

// buildKey is the identity that must be constant inside one comparison cell. Model id is
// deliberately excluded: cells already vary by arm, and the point of this key is "same
// code, different week" versus "different code that looks the same".
func (p *binaryProvenance) buildKey() string {
	if p == nil {
		return "<unrecorded>"
	}
	return p.SourceRevision + "|" + p.EndpointSHA256 + "|" + p.FixtureSHA256 + "|" + p.RunnerSHA256
}

func (p *binaryProvenance) String() string {
	if p == nil {
		return "provenance unrecorded for this run"
	}
	dirty := "clean"
	if p.SourceDirty {
		dirty = "dirty"
	}
	return fmt.Sprintf("rev %s (%s), endpoint %s, fixture %s", shortRev(p.SourceRevision), dirty,
		fileName(p.EndpointSHA256), fileName(p.FixtureSHA256))
}

func shortRev(rev string) string {
	if len(rev) > 12 {
		return rev[:12]
	}
	if rev == "" {
		return "unknown"
	}
	return rev
}

func fileName(sum string) string {
	if len(sum) > 12 {
		return sum[:12]
	}
	if sum == "" {
		return "absent"
	}
	return sum
}

// captureProvenance records the artifacts this process is about to measure with. It is
// called once per run on purpose: a build replaced mid-experiment must show up as two
// provenances inside one cell rather than as a silent average over both.
func captureProvenance(endpointBin, fixtureBin, modelID string) *binaryProvenance {
	runner := os.Args[0]
	rev, dirty := sourceState()
	return &binaryProvenance{
		SourceRevision: rev,
		SourceDirty:    dirty,
		EndpointSHA256: fileSHA256(endpointBin),
		FixtureSHA256:  fileSHA256(fixtureBin),
		RunnerSHA256:   fileSHA256(runner),
		ModelID:        modelID,
		CapturedAt:     time.Now().UTC().Format(time.RFC3339),
	}
}

func fileSHA256(path string) string {
	f, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer func() { _ = f.Close() }()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return ""
	}
	return hex.EncodeToString(h.Sum(nil))
}

// sourceState resolves git identity once per process. captureProvenance runs per graded
// run, and each call would otherwise spawn two git subprocesses - on a machine already
// doing real work, that is the difference between a 2s test package and a 90s one.
var (
	sourceOnce     sync.Once
	cachedRevision string
	cachedDirty    bool
)

func sourceState() (string, bool) {
	sourceOnce.Do(func() {
		cachedRevision, cachedDirty = gitRevision(), gitStatusDirty()
	})
	return cachedRevision, cachedDirty
}

func gitStatusDirty() bool {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, "git", "status", "--porcelain").Output()
	if err != nil {
		return false
	}
	return len(out) > 0
}

// checkRunInvariants refuses a rollup whose own analytics are inconsistent. Each family
// of property lives in its own function, because the whole point is that these are not
// one big opinion but several independent arithmetic facts.
func checkRunInvariants(runs []runResult, cells []cellMetrics, byCell map[string][]runResult) []string {
	var violations []string
	violations = append(violations, checkCellBounds(cells, byCell)...)
	violations = append(violations, checkAgreementWithRows(runs, cells)...)
	violations = append(violations, checkTranscriptsPersist(runs)...)
	return violations
}

// checkCellBounds enforces the bounds on each cell and the one-build-per-cell rule.
func checkCellBounds(cells []cellMetrics, byCell map[string][]runResult) []string {
	var violations []string
	add := func(format string, args ...any) {
		violations = append(violations, fmt.Sprintf(format, args...))
	}
	// 1. Every bounded metric must actually be bounded by its denominator, and the
	//    invalid runs plus the counted runs must be the runs that were graded.
	for _, c := range cells {
		label := c.Arm + "/" + c.Mode
		type bound struct {
			name  string
			value int
		}
		for _, b := range []bound{
			{"task_success", c.TaskSuccess},
			{"terminal_report_coverage", c.TerminalReportCover},
			{"first_attempt_protocol_compliance", c.FirstAttemptComply},
			{"protocol_only_success", c.ProtocolOnlySuccess},
			{"sessions_with_no_prescribe", c.SessionsNoPrescribe},
		} {
			if b.value < 0 || b.value > c.Runs {
				add("%s: %s = %d outside [0, runs=%d]", label, b.name, b.value, c.Runs)
			}
		}
		// The companion metric gates nothing, but it cannot exceed the runs that were
		// valid enough to be scored.
		if c.ProtocolOnlySuccess > c.Runs {
			add("%s: companion protocol_only_success (%d) exceeds valid runs (%d)",
				label, c.ProtocolOnlySuccess, c.Runs)
		}
		if c.BlockedAttempts < 0 {
			add("%s: blocked_attempts = %d, negative counts are not observations", label, c.BlockedAttempts)
		}
		graded := byCell[label]
		if len(graded) != c.Runs+c.InvalidRuns {
			add("%s: cell aggregates %d runs but %d were graded", label, c.Runs+c.InvalidRuns, len(graded))
		}

		// 2. One comparison cell must be one build. A mixed cell averages two programs
		//    into a number that describes neither, and the mix is invisible unless it is
		//    checked.
		var key string
		for _, r := range graded {
			if r.Provenance == nil {
				continue
			}
			if key == "" {
				key = r.Provenance.buildKey()
				continue
			}
			if r.Provenance.buildKey() != key {
				add("%s: runs come from different builds (%s vs %s)", label, key, r.Provenance.buildKey())
				break
			}
		}
	}
	return violations
}

// checkAgreementWithRows makes the aggregate prove itself against the per-run rows. A
// rollup that recomputes something slightly differently from the row loop is exactly how
// a metric of 28 appeared over a denominator of 16.
func checkAgreementWithRows(runs []runResult, cells []cellMetrics) []string {
	var violations []string
	add := func(format string, args ...any) {
		violations = append(violations, fmt.Sprintf(format, args...))
	}
	var (
		sumSuccess, sumReport, sumBlocked, sumRuns, sumInvalid int
	)
	for _, r := range runs {
		if !r.valid() {
			sumInvalid++
			continue
		}
		sumRuns++
		if r.Success {
			sumSuccess++
		}
		if r.Reports > 0 {
			sumReport++
		}
		sumBlocked += r.Blocked
	}
	var aggSuccess, aggReport, aggBlocked, aggRuns, aggInvalid int
	for _, c := range cells {
		aggSuccess += c.TaskSuccess
		aggReport += c.TerminalReportCover
		aggBlocked += c.BlockedAttempts
		aggRuns += c.Runs
		aggInvalid += c.InvalidRuns
	}
	if sumRuns != aggRuns || sumInvalid != aggInvalid {
		add("aggregate: runs counted per cell (%d valid, %d invalid) differ from graded runs (%d valid, %d invalid)",
			aggRuns, aggInvalid, sumRuns, sumInvalid)
	}
	if sumSuccess != aggSuccess {
		add("aggregate: task_success sums to %d per run but %d per cell", sumSuccess, aggSuccess)
	}
	if sumReport != aggReport {
		add("aggregate: terminal_report_coverage sums to %d per run but %d per cell", sumReport, aggReport)
	}
	if sumBlocked != aggBlocked {
		add("aggregate: blocked_attempts sums to %d per run but %d per cell", sumBlocked, aggBlocked)
	}
	return violations
}

// checkTranscriptsPersist requires that a graded verdict still has its transcript. A run
// that cannot be re-read cannot be challenged, which is the entire purpose of the store.
func checkTranscriptsPersist(runs []runResult) []string {
	var violations []string
	for _, r := range runs {
		if !r.valid() || r.TranscriptPath == "" {
			continue
		}
		if _, err := os.Stat(r.TranscriptPath); err != nil {
			violations = append(violations, fmt.Sprintf("%s/%s %s run%d: transcript %s is not persisted (%v)",
				r.Arm, r.Mode, r.Task, r.Run, r.TranscriptPath, err))
		}
	}
	return violations
} // provenanceDrift compares the artifacts a run set was measured against with the ones
// this process would use now. It reports groupings, not per-run spam: what a reader needs
// is whether the regrade ran against the same code, and how much of the set it covers.
func provenanceDrift(runs []runResult, endpointBin, fixtureBin string) []string {
	currentEndpoint, currentFixture := fileSHA256(endpointBin), fileSHA256(fixtureBin)
	type group struct {
		count int
		rev   string
	}
	var order []string
	by := map[string]*group{}
	unrecorded := 0
	for _, r := range runs {
		if r.Provenance == nil {
			unrecorded++
			continue
		}
		if r.Provenance.EndpointSHA256 == currentEndpoint && r.Provenance.FixtureSHA256 == currentFixture {
			continue
		}
		key := shortRev(r.Provenance.SourceRevision) + "|" + fileName(r.Provenance.EndpointSHA256)
		if by[key] == nil {
			by[key] = &group{rev: key}
			order = append(order, key)
		}
		by[key].count++
	}
	var out []string
	for _, key := range order {
		out = append(out, fmt.Sprintf(
			"%d run(s) measured endpoint %s while this build is %s: regraded verdicts are re-readings of that artifact, not of the current code",
			by[key].count, by[key].rev, fileName(currentEndpoint)))
	}
	if unrecorded > 0 {
		out = append(out, fmt.Sprintf(
			"%d run(s) predate provenance recording: their build identity is unknown, so treat them as unattributed rather than as matching this build", unrecorded))
	}
	return out
}
