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
	"math"
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
	// ExecutionPath says how the run reached the upstream, so a baseline run cannot borrow
	// the endpoint's identity: no endpoint binary participated, and hashing one anyway would
	// let an artifact imply provenance from a program it never started.
	ExecutionPath string `json:"execution_path"`
	CapturedAt    string `json:"captured_at"`
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

// usedEndpoint reports whether this run's topology included the merged endpoint. A method
// rather than a comparison at each call site, because "did an endpoint participate" is the
// question every reader of provenance is actually asking.
func (p *binaryProvenance) usedEndpoint() bool {
	return p != nil && p.ExecutionPath != pathDirectFixture
}

func (p *binaryProvenance) String() string {
	if p == nil {
		return "provenance unrecorded for this run"
	}
	dirty := "clean"
	if p.SourceDirty {
		dirty = "dirty"
	}
	endpoint := fileName(p.EndpointSHA256)
	if !p.usedEndpoint() {
		endpoint = "absent, no endpoint in this topology (" + p.ExecutionPath + ")"
	}
	return fmt.Sprintf("rev %s (%s), endpoint %s, fixture %s", shortRev(p.SourceRevision), dirty,
		endpoint, fileName(p.FixtureSHA256))
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
//
// For a direct-fixture topology the endpoint binary is deliberately not opened. Recording
// bin/evidra-mcp's hash on a run that never started it would attribute the measurement to a
// program that produced nothing, which is the provenance equivalent of a green check that
// tests nothing.
func captureProvenance(endpointBin, fixtureBin, modelID, path string) *binaryProvenance {
	runner := os.Args[0]
	rev, dirty := sourceState()
	endpoint := ""
	if path != pathDirectFixture {
		endpoint = fileSHA256(endpointBin)
	}
	return &binaryProvenance{
		SourceRevision: rev,
		SourceDirty:    dirty,
		EndpointSHA256: endpoint,
		FixtureSHA256:  fileSHA256(fixtureBin),
		RunnerSHA256:   fileSHA256(runner),
		ModelID:        modelID,
		ExecutionPath:  path,
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
	// The baseline's own claims - no protocol traffic, no endpoint in the provenance - are
	// part of the analytics contract, not a presentation preference.
	violations = append(violations, checkBaselineInvariants(cells, byCell)...)
	violations = append(violations, checkOperationalAgreement(runs, cells)...)
	violations = append(violations, checkComparisonAgreement(cells)...)
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
		// A baseline cell has no protocol surface, so its protocol numerators must be
		// exactly zero; anything else means a wrapped run was counted under `none`.
		if !protocolMetricsApplicable(c.Mode) && c.ProtocolOnlySuccess != 0 {
			add("%s: baseline cell reports protocol_only_success = %d", label, c.ProtocolOnlySuccess)
		}
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
// checkOperationalAgreement does for the operational columns what
// checkAgreementWithRows does for the protocol ones: totals must equal the sum of the rows
// they claim to aggregate, and a mean must equal its total over its own denominator. The mean
// check is the one that would have caught 28-over-16 in the first place: a ratio is only
// evidence if it is the ratio of the two numbers printed next to it.
func checkOperationalAgreement(runs []runResult, cells []cellMetrics) []string {
	var violations []string
	add := func(format string, args ...any) {
		violations = append(violations, fmt.Sprintf(format, args...))
	}
	type acc struct {
		runs                            int
		duration                        int64
		turns, calls, errs, prompt, out int
	}
	per := map[string]*acc{}
	for _, r := range runs {
		if !r.valid() {
			continue
		}
		key := r.Arm + "/" + r.Mode
		if per[key] == nil {
			per[key] = &acc{}
		}
		a := per[key]
		a.runs++
		a.duration += r.DurationMS
		a.turns += r.Turns
		a.calls += r.UpstreamCalls
		a.errs += r.UpstreamErrors
		a.prompt += r.PromptTokens
		a.out += r.OutputTokens
	}
	for _, c := range cells {
		a := per[c.Arm+"/"+c.Mode]
		if a == nil {
			a = &acc{}
		}
		label := c.Arm + "/" + c.Mode
		if c.DurationMS != a.duration {
			add("%s: duration_ms_total = %d but the valid rows sum to %d", label, c.DurationMS, a.duration)
		}
		if c.UpstreamCalls != a.calls {
			add("%s: upstream_calls = %d but the valid rows sum to %d", label, c.UpstreamCalls, a.calls)
		}
		if c.UpstreamErrors != a.errs {
			add("%s: upstream_errors = %d but the valid rows sum to %d", label, c.UpstreamErrors, a.errs)
		}
		if c.TurnsTotal != a.turns {
			add("%s: turns_total = %d but the valid rows sum to %d", label, c.TurnsTotal, a.turns)
		}
		if c.PromptTokens != a.prompt {
			add("%s: prompt_tokens_total = %d but the valid rows sum to %d", label, c.PromptTokens, a.prompt)
		}
		if c.OutputTokens != a.out {
			add("%s: completion_tokens_total = %d but the valid rows sum to %d", label, c.OutputTokens, a.out)
		}
		if c.Runs == 0 {
			if c.MeanTurns != 0 || c.MeanDurationMS != 0 {
				add("%s: a cell with no valid runs reports means (%.2f turns, %.0fms); an empty denominator has no average",
					label, c.MeanTurns, c.MeanDurationMS)
			}
			continue
		}
		if d := math.Abs(c.MeanTurns*float64(c.Runs) - float64(c.TurnsTotal)); d > 0.5 {
			add("%s: mean_turns ×runs (%.2f×%d) is %.1f off turns_total (%d)",
				label, c.MeanTurns, c.Runs, d, c.TurnsTotal)
		}
		if d := math.Abs(c.MeanDurationMS*float64(c.Runs) - float64(c.DurationMS)); d > 1.0 {
			add("%s: mean_duration_ms ×runs (%.1f×%d) is %.1f off duration_ms_total (%d)",
				label, c.MeanDurationMS, c.Runs, d, c.DurationMS)
		}
	}
	return violations
}

// checkComparisonAgreement makes the comparison block prove itself against the cells it
// copies. A hand-built pair of columns is where a comparison starts drifting from the
// measurement it claims to summarise.
func checkComparisonAgreement(cells []cellMetrics) []string {
	var violations []string
	byCell := map[string]cellMetrics{}
	for _, c := range cells {
		byCell[c.Arm+"/"+c.Mode] = c
	}
	for _, pair := range baselineComparisons(cells) {
		arm, _ := pair["arm"].(string)
		b, _ := pair["baseline"].(map[string]any)
		o, _ := pair["observe_only"].(map[string]any)
		d, _ := pair["delta"].(map[string]any)
		if b == nil || o == nil || d == nil {
			continue
		}
		for _, side := range []struct {
			col  map[string]any
			cell cellMetrics
		}{{b, byCell[arm+"/"+modeNone]}, {o, byCell[arm+"/"+modeOff]}} {
			if int(colAsFloat(side.col["task_success"])) != side.cell.TaskSuccess {
				violations = append(violations, fmt.Sprintf("%s: comparison reports task_success %v, cell says %d",
					arm, side.col["task_success"], side.cell.TaskSuccess))
			}
			if math.Abs(colAsFloat(side.col["mean_turns"])-side.cell.MeanTurns) > 1e-9 ||
				int(colAsFloat(side.col["duration_ms"])) != int(side.cell.DurationMS) {
				violations = append(violations, fmt.Sprintf("%s: comparison columns do not match the %s cell they name",
					arm, side.cell.Mode))
			}
		}
		if int(colAsFloat(d["task_success"])) != byCell[arm+"/"+modeOff].TaskSuccess-byCell[arm+"/"+modeNone].TaskSuccess {
			violations = append(violations, fmt.Sprintf("%s: reported success delta is not off minus none", arm))
		}
	}
	return violations
}

// colAsFloat reads a number out of an untyped comparison column. int64 must be listed: the
// duration column is an int64 in the struct, and a type the helper forgets returns NaN, which
// compares unequal to everything and turns a correct comparison into an invariant violation.
func colAsFloat(v any) float64 {
	switch n := v.(type) {
	case int:
		return float64(n)
	case int64:
		return float64(n)
	case int32:
		return float64(n)
	case float64:
		return n
	case float32:
		return float64(n)
	default:
		return math.NaN()
	}
}

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
		endpoint := currentEndpoint
		if !r.Provenance.usedEndpoint() {
			// A baseline run has no endpoint to have drifted from; comparing the empty
			// field against this build would report drift that cannot exist.
			endpoint = ""
		}
		if r.Provenance.EndpointSHA256 == endpoint && r.Provenance.FixtureSHA256 == currentFixture {
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
