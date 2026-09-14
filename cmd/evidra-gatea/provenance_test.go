package main

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeFile(t *testing.T, path, body string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(body), 0o755); err != nil {
		t.Fatal(err)
	}
}

func sha(body string) string {
	sum := sha256.Sum256([]byte(body))
	return hex.EncodeToString(sum[:])
}

func TestProvenanceRecordsTheExactBinaries(t *testing.T) {
	dir := t.TempDir()
	endpoint := filepath.Join(dir, "evidra-mcp")
	fixture := filepath.Join(dir, "evidra-fixture")
	writeFile(t, endpoint, "endpoint-bytes")
	writeFile(t, fixture, "fixture-bytes")

	p := captureProvenance(endpoint, fixture, "some-model", pathProxy)
	if p.EndpointSHA256 != sha("endpoint-bytes") {
		t.Errorf("endpoint hash = %s, want the sha256 of the file it measured", p.EndpointSHA256)
	}
	if p.FixtureSHA256 != sha("fixture-bytes") {
		t.Errorf("fixture hash = %s", p.FixtureSHA256)
	}
	if p.ModelID != "some-model" {
		t.Errorf("model id = %q", p.ModelID)
	}
	// A run whose revision cannot be resolved must say "unknown" rather than look clean.
	if p.SourceRevision == "" {
		t.Error("source revision is empty; a reader cannot tell unknown from unreleased")
	}
	// A binary that is not there is recorded as absent, not as the hash of nothing.
	if got := captureProvenance(filepath.Join(dir, "gone"), fixture, "m", pathProxy).EndpointSHA256; got != "" {
		t.Errorf("missing binary hashed to %q, want empty", got)
	}
	// The empty value is also produced on purpose for a run whose topology has no endpoint,
	// and those two empties must stay distinguishable: the path says which one happened.
	if p.ExecutionPath != pathProxy {
		t.Errorf("execution path = %q, want %q for a wrapped run", p.ExecutionPath, pathProxy)
	}
}

func TestBuildKeySeparatesARebuiltEndpoint(t *testing.T) {
	dir := t.TempDir()
	endpoint := filepath.Join(dir, "evidra-mcp")
	fixture := filepath.Join(dir, "evidra-fixture")
	writeFile(t, endpoint, "v1")
	writeFile(t, fixture, "same")
	before := captureProvenance(endpoint, fixture, "m", pathProxy)
	writeFile(t, endpoint, "v2")
	after := captureProvenance(endpoint, fixture, "m", pathProxy)

	if before.buildKey() == after.buildKey() {
		t.Fatal("swapping the endpoint binary did not change the build key")
	}
	// The model is not part of build identity: cells vary by arm on purpose.
	other := *before
	other.ModelID = "different-model"
	if other.buildKey() != before.buildKey() {
		t.Error("model id leaked into build identity, which would split every legitimate cell")
	}
}

func cellFor(runs []runResult) cellMetrics { return rollupCell(runs) }

func byCellOf(runs []runResult) map[string][]runResult {
	out := map[string][]runResult{}
	for _, r := range runs {
		out[r.Arm+"/"+r.Mode] = append(out[r.Arm+"/"+r.Mode], r)
	}
	return out
}

// gradingRun builds one run with a transcript that exists, so tests that are not about
// the transcript invariant do not trip it by accident.
func gradingRun(t *testing.T, dir, id string, success, reported bool) runResult {
	t.Helper()
	path := filepath.Join(dir, id+".jsonl")
	writeFile(t, path, `{"kind":"transcript"}`+"\n")
	r := runResult{
		Arm: "arm", ArmModelID: "m", Mode: "all", Task: id, Run: 1,
		Success: success, Reports: boolToInt(reported), Prescribes: 1,
		TranscriptPath: path, CountsFrom: "store",
	}
	return r
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

func TestInvariantsAcceptAConsistentRollup(t *testing.T) {
	dir := t.TempDir()
	runs := []runResult{
		gradingRun(t, dir, "a", true, true),
		gradingRun(t, dir, "b", false, true),
		gradingRun(t, dir, "c", true, false),
	}
	broken := runs[0]
	broken.InvalidRun = "upstream died"
	broken.Success = false
	runs = append(runs, broken)

	byCell := byCellOf(runs)
	cells := []cellMetrics{cellFor(runs)}
	if v := checkRunInvariants(runs, cells, byCell); len(v) != 0 {
		t.Fatalf("a rollup produced by the real code path was rejected: %v", v)
	}
	c := cells[0]
	if c.Runs != 3 || c.InvalidRuns != 1 || c.TaskSuccess != 2 || c.TerminalReportCover != 2 {
		t.Fatalf("unexpected rollup: runs=%d invalid=%d success=%d report=%d",
			c.Runs, c.InvalidRuns, c.TaskSuccess, c.TerminalReportCover)
	}
}

func TestInvariantsCatchAnUnboundedMetric(t *testing.T) {
	dir := t.TempDir()
	runs := []runResult{gradingRun(t, dir, "a", true, true)}
	cell := cellFor(runs)
	// The bug that happened for real: a numerator summed over per-run values with a
	// denominator that was never checked against it.
	cell.TaskSuccess = 28
	violations := checkRunInvariants(runs, []cellMetrics{cell}, byCellOf(runs))
	if len(violations) == 0 {
		t.Fatal("task_success = 28 with 1 run passed the invariants")
	}
	if !strings.Contains(violations[0], "outside [0, runs=") {
		t.Errorf("violation does not state the bound: %v", violations)
	}
}

func TestInvariantsCatchAnAggregateThatDisagreesWithItsRows(t *testing.T) {
	dir := t.TempDir()
	runs := []runResult{
		gradingRun(t, dir, "a", true, true),
		gradingRun(t, dir, "b", true, true),
	}
	cell := cellFor(runs)
	cell.TaskSuccess = 1 // one row was silently dropped by the rollup
	violations := checkRunInvariants(runs, []cellMetrics{cell}, byCellOf(runs))
	if len(violations) == 0 {
		t.Fatal("an aggregate that disagrees with its per-run rows passed")
	}
	joined := strings.Join(violations, "\n")
	if !strings.Contains(joined, "task_success sums to 2 per run but 1 per cell") {
		t.Errorf("violation does not name both sides of the disagreement: %q", joined)
	}
}

func TestInvariantsCatchAComparisonCellBuiltFromTwoBuilds(t *testing.T) {
	dir := t.TempDir()
	one := gradingRun(t, dir, "a", true, true)
	two := gradingRun(t, dir, "b", true, true)
	endpoint := filepath.Join(dir, "evidra-mcp")
	fixture := filepath.Join(dir, "evidra-fixture")
	writeFile(t, endpoint, "first")
	writeFile(t, fixture, "f")
	one.Provenance = captureProvenance(endpoint, fixture, "m", pathProxy)
	writeFile(t, endpoint, "second")
	two.Provenance = captureProvenance(endpoint, fixture, "m", pathProxy)

	runs := []runResult{one, two}
	cells := []cellMetrics{cellFor(runs)}
	violations := checkRunInvariants(runs, cells, byCellOf(runs))
	found := false
	for _, v := range violations {
		if strings.Contains(v, "different builds") {
			found = true
		}
	}
	if !found {
		t.Fatalf("a cell averaging two endpoint builds was not flagged: %v", violations)
	}
}

func TestInvariantsCatchAVerdictWithNoTranscript(t *testing.T) {
	dir := t.TempDir()
	r := gradingRun(t, dir, "a", true, true)
	r.TranscriptPath = filepath.Join(dir, "deleted.jsonl")
	runs := []runResult{r}
	violations := checkRunInvariants(runs, []cellMetrics{cellFor(runs)}, byCellOf(runs))
	if len(violations) == 0 || !strings.Contains(violations[0], "not persisted") {
		t.Fatalf("a graded run whose transcript is gone was accepted: %v", violations)
	}
	// An invalid run has no verdict to protect, so it must not fail on a missing file.
	invalid := gradingRun(t, dir, "b", false, false)
	invalid.InvalidRun = "endpoint exited early"
	invalid.TranscriptPath = filepath.Join(dir, "also-gone.jsonl")
	runs = append(runs, invalid)
	violations = checkRunInvariants(runs, []cellMetrics{cellFor(runs)}, byCellOf(runs))
	for _, v := range violations {
		if strings.Contains(v, "also-gone") {
			t.Errorf("an invalid run was required to have a persisted transcript: %v", v)
		}
	}
}

func TestProvenanceDriftNamesUnattributedAndForeignRuns(t *testing.T) {
	dir := t.TempDir()
	endpoint := filepath.Join(dir, "evidra-mcp")
	fixture := filepath.Join(dir, "evidra-fixture")
	writeFile(t, endpoint, "current")
	writeFile(t, fixture, "fx")

	same := gradingRun(t, dir, "a", true, true)
	same.Provenance = captureProvenance(endpoint, fixture, "m", pathProxy)

	oldEndpoint := filepath.Join(dir, "older")
	writeFile(t, oldEndpoint, "older")
	foreign := gradingRun(t, dir, "b", true, true)
	foreign.Provenance = captureProvenance(oldEndpoint, fixture, "m", pathProxy)

	unattributed := gradingRun(t, dir, "c", true, true)

	drift := provenanceDrift([]runResult{same, foreign, unattributed}, endpoint, fixture)
	joined := strings.Join(drift, "\n")
	if !strings.Contains(joined, "measured endpoint") {
		t.Errorf("a run from another endpoint build was not named: %q", joined)
	}
	if !strings.Contains(joined, "predate provenance recording") {
		t.Errorf("a run with no recorded provenance was reported as if it matched: %q", joined)
	}
	if strings.Contains(joined, "same") {
		t.Errorf("runs from the current build were reported as drift: %q", joined)
	}
	// Nothing foreign: the only run matches the current binaries.
	if d := provenanceDrift([]runResult{same}, endpoint, fixture); len(d) != 0 {
		t.Errorf("drift reported against an identical build: %v", d)
	}
}

// TestInvariantsCatchASelfContradictoryComplianceRecord covers the invariant whose
// absence let a serialization bug stand for an entire archived set. The compliance
// fields were unexported, so --regrade restored them as zero values and reported
// voluntary coverage 0/N everywhere, while `unprescribed_executions` stayed 0 in the
// very same records. Zero satisfies every bound the other families check; only the
// disagreement between two fields of one record was unexplainable.
func TestInvariantsCatchASelfContradictoryComplianceRecord(t *testing.T) {
	dir := t.TempDir()
	base := gradingRun(t, dir, "a", true, true)
	base.UpstreamCalls = 2 // so the guard below is reached at all

	cases := []struct {
		name      string
		mutate    func(*runResult)
		wantCatch bool
	}{
		// The H-0 signature: nothing was refused, nothing ran uncovered, a
		// prescription happened - and yet the first action is reported uncovered.
		{"zeroed coverage", func(r *runResult) { r.FirstUpstreamPrescribed = false }, true},
		{"consistent record", func(r *runResult) { r.FirstUpstreamPrescribed = true }, false},
		// A refused first attempt really is uncovered, and really has no unprescribed
		// execution either: the call never reached the upstream. Not a contradiction.
		{"blocked first attempt", func(r *runResult) {
			r.FirstUpstreamPrescribed = false
			r.Blocked = 1
		}, false},
		// Acting before prescribing with nothing to stop it is a genuine miss, and it
		// is already counted by unprescribed_executions. Not a contradiction.
		{"unprescribed execution", func(r *runResult) {
			r.FirstUpstreamPrescribed = false
			r.Unprescribed = 1
		}, false},
		// No upstream work at all leaves the metric with no referent.
		{"no upstream calls", func(r *runResult) {
			r.FirstUpstreamPrescribed = false
			r.UpstreamCalls = 0
		}, false},
		// A baseline cell has no protocol surface, so the rollup renders n/a and a zero
		// there is a statement about a tool the agent was never given.
		{"baseline run", func(r *runResult) {
			r.FirstUpstreamPrescribed = false
			r.ProtocolNotApplicable = true
		}, false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := base
			tc.mutate(&r)
			v := checkComplianceConsistency([]runResult{r})
			if tc.wantCatch && len(v) == 0 {
				t.Fatal("a self-contradictory compliance record passed the invariants")
			}
			if !tc.wantCatch && len(v) != 0 {
				t.Fatalf("a legitimate record was rejected: %v", v)
			}
			if tc.wantCatch && !strings.Contains(v[0], "cannot all be true") {
				t.Errorf("violation does not name the contradiction: %q", v[0])
			}
		})
	}
}

// TestInvariantsWireComplianceConsistencyIntoTheRollup proves the new family is actually
// composed into checkRunInvariants: an invariant nobody calls is indistinguishable from
// one that never fires.
func TestInvariantsWireComplianceConsistencyIntoTheRollup(t *testing.T) {
	dir := t.TempDir()
	r := gradingRun(t, dir, "a", true, true)
	r.UpstreamCalls = 2
	r.FirstUpstreamPrescribed = false
	runs := []runResult{r}
	v := checkRunInvariants(runs, []cellMetrics{cellFor(runs)}, byCellOf(runs))
	found := false
	for _, line := range v {
		if strings.Contains(line, "cannot all be true") {
			found = true
		}
	}
	if !found {
		t.Fatalf("compliance consistency is not part of the rollup check: %v", v)
	}
}
