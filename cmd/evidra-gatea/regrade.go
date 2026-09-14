package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// regrade recomputes verdicts from an existing artifact directory, without
// running anything. Two reasons. Verdict rules change as the experiment is
// read - a predicate fix must not require spending tokens again - and a metric
// that cannot be recomputed from the recorded frames is not evidence, it is a
// claim about what the runner believed at the time.
func regrade(o options, tasks []taskSpec) error {
	dir := o.regrade
	byID := map[string]*taskSpec{}
	for i := range tasks {
		byID[tasks[i].ID] = &tasks[i]
	}
	dirs, err := os.ReadDir(filepath.Join(dir, "runs"))
	if err != nil {
		return err
	}
	n := 0
	for _, d := range dirs {
		if !d.IsDir() {
			continue
		}
		runDir := filepath.Join(dir, "runs", d.Name())
		raw, err := os.ReadFile(filepath.Join(runDir, "result.json"))
		if err != nil {
			return err
		}
		var res runResult
		if err := json.Unmarshal(raw, &res); err != nil {
			return err
		}
		task := byID[res.Task]
		if task == nil {
			return fmt.Errorf("%s: unknown task %q", runDir, res.Task)
		}
		tr, err := readTranscript(filepath.Join(runDir, "transcript.jsonl"))
		if err != nil {
			// An artifact without a recorded transcript cannot be regraded, and
			// keeping its old verdict would mix two standards silently.
			return fmt.Errorf("%s: %w", runDir, err)
		}
		res.Failures = task.evaluate(tr)
		res.Success = len(res.Failures) == 0 && res.InvalidRun == ""
		res.ProtocolOnlyFails = task.protocolOnlyFails(tr)
		res.ProtocolOnly = len(res.ProtocolOnlyFails) == 0 && res.InvalidRun == ""
		res.Blocked = tr.BlockedCount
		res.Unprescribed = tr.unprescribedCalls()
		facts, ferr := readStoreFacts(runDir)
		if ferr == nil {
			applyStoreFacts(&res, facts, res.Mode)
			if facts != nil {
				res.Unprescribed = facts.Unprescribed
			}
		} else {
			res.CountsFrom = "transcript"
			res.Failures = append(res.Failures, "evidence store unreadable: "+ferr.Error())
		}
		res.FirstUpstreamPrescribed = tr.firstUpstreamPrescribed
		res.LatePrescribe = tr.latePrescribe
		out, _ := json.MarshalIndent(res, "", "  ")
		if err := os.WriteFile(filepath.Join(runDir, "result.json"), out, 0o644); err != nil {
			return err
		}
		n++
	}
	// Rebuild the rollup from the regraded verdicts so the summary cannot drift
	// away from the per-run artifacts it claims to describe.
	raw, err := os.ReadFile(filepath.Join(dir, "summary.json"))
	if err != nil {
		return err
	}
	var prev struct {
		Arms                       []armSpec      `json:"arms"`
		RunsPlannedPerCell         int            `json:"runs_planned_per_cell"`
		DryRun                     bool           `json:"dry_run"`
		ProtocolDefinitionOverhead map[string]int `json:"protocol_definition_overhead"`
	}
	if err := json.Unmarshal(raw, &prev); err != nil {
		return err
	}
	var runs []runResult
	for _, d := range dirs {
		if !d.IsDir() {
			continue
		}
		b, err := os.ReadFile(filepath.Join(dir, "runs", d.Name(), "result.json"))
		if err != nil {
			return err
		}
		var r runResult
		if err := json.Unmarshal(b, &r); err != nil {
			return err
		}
		if r.Task != "" {
			runs = append(runs, r)
		}
	}
	violations := writeSummary(dir, prev.Arms, tasks, runs, prev.ProtocolDefinitionOverhead,
		options{runs: prev.RunsPlannedPerCell, dryRun: prev.DryRun})
	// Regrade re-derives verdicts from persisted frames, so it is allowed to disagree
	// with the build that produced them. Saying which build that was is the difference
	// between a re-grade and a fabricated re-measurement.
	for _, line := range provenanceDrift(runs, o.mcpBin, o.fixtureBin) {
		fmt.Printf("  [PROVENANCE ] %s\n", line)
	}
	fmt.Printf("regraded %d runs in %s\n", n, dir)
	if len(violations) > 0 {
		return fmt.Errorf("%d analytics invariant violation(s); the rollup above is not evidence", len(violations))
	}
	return nil
}

// readTranscript recovers the transcript object a run recorded about itself.
func readTranscript(path string) (*transcript, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 64*1024*1024)
	var found *transcript
	for sc.Scan() {
		var rec struct {
			Kind    string          `json:"kind"`
			Payload json.RawMessage `json:"payload"`
		}
		if err := json.Unmarshal(sc.Bytes(), &rec); err != nil || rec.Kind != "transcript" {
			continue
		}
		var tr transcript
		if err := json.Unmarshal(rec.Payload, &tr); err != nil {
			return nil, err
		}
		found = &tr
	}
	if found == nil {
		return nil, fmt.Errorf("no transcript record in %s", path)
	}
	return found, sc.Err()
}
