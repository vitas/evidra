package main

// Reading the recorder's own store, so the Gate A counts stop being inferred.
//
// The runner used to derive "blocked attempts" and "unprescribed executions" from
// the protocol traffic it generated itself. That is workable but weak: it measures
// what the runner believes happened, and it cannot distinguish a refusal from a call
// that was never forwarded for reasons the runner does not model. The store answers
// both questions as a signed record, so when a store exists the runner defers to it.

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"time"

	"samebits.com/evidra/pkg/evidence"
)

// storeFacts is the recorder's account of one run.
type storeFacts struct {
	Dir             string `json:"dir"`
	Records         int    `json:"records"`
	Blocked         int    `json:"blocked_attempts"`
	Executions      int    `json:"executions"`
	Unprescribed    int    `json:"unprescribed_executions"`
	Prescribed      int    `json:"operations_prescribed"`
	Reported        int    `json:"operations_reported"`
	ChainValid      bool   `json:"chain_valid"`
	SignatureValid  bool   `json:"signature_valid"`
	Coverage        string `json:"evidence_coverage"`
	DegradedWindows int    `json:"degraded_windows"`
}

// readStoreFacts reconciles one run directory with the recorder's own events.
// It returns nil when no store was written, which is the shape a run takes when the
// caller did not ask for evidence; a store that exists but cannot be read is an
// error, because treating a broken recorder as "no recorder" would quietly drop the
// measurement that the recorder exists for.
func readStoreFacts(runDir string) (*storeFacts, error) {
	dirs, err := filepath.Glob(filepath.Join(runDir, "evidence", "*", "events.jsonl"))
	if err != nil {
		return nil, err
	}
	if len(dirs) == 0 {
		return nil, nil
	}
	if len(dirs) > 1 {
		return nil, fmt.Errorf("store: %d recorder stores in %s, want one run per store", len(dirs), runDir)
	}
	dir := filepath.Dir(dirs[0])
	report, err := evidence.VerifyStore(dir, time.Time{})
	if err != nil {
		return nil, err
	}
	events, _, err := evidence.ReadStore(dir, time.Time{})
	if err != nil {
		return nil, err
	}
	facts := &storeFacts{
		Dir: dir, Records: report.Records, ChainValid: report.ChainValid,
		SignatureValid: report.SignatureValid, Coverage: report.Coverage,
	}
	reported := map[string]bool{}
	prescribed := map[string]bool{}
	for i := range events {
		ev := &events[i]
		switch ev.EventType {
		case evidence.EventProtocolViolation:
			facts.Blocked++
		case evidence.EventOperationPrescribed:
			prescribed[ev.OperationID] = true
		case evidence.EventOperationReported:
			reported[ev.OperationID] = true
		case evidence.EventExecutionStarted:
			facts.Executions++
			if ev.OperationID == "" {
				facts.Unprescribed++
			}
		case evidence.EventRecorderDegraded:
			facts.DegradedWindows++
		}
	}
	facts.Prescribed = len(prescribed)
	for op := range reported {
		if op != "" {
			facts.Reported++
		}
	}
	return facts, nil
}

// applyStoreFacts lets the signed record override the transcript-derived counts,
// and turns an enforcement hole into a per-run failure instead of a statistic.
func applyStoreFacts(res *runResult, facts *storeFacts, mode string) {
	if facts == nil {
		res.CountsFrom = "transcript"
		return
	}
	res.Store = facts
	res.CountsFrom = "store"
	res.Blocked = facts.Blocked
	// In enforce=all every forwarded call must sit inside an operation. A number
	// here means the recorder watched something get through that it should have
	// refused, which is a product defect rather than an agent failure.
	if mode == "all" && facts.Unprescribed > 0 {
		res.Failures = append(res.Failures, fmt.Sprintf(
			"enforcement hole: %d executions reached upstream with no open operation", facts.Unprescribed))
	}
	if !facts.ChainValid || !facts.SignatureValid {
		res.Failures = append(res.Failures, "evidence chain or signature invalid for this run")
	}
}

// storeFactsJSON is kept out of the summary unless a store existed.
func (f *storeFacts) String() string {
	raw, err := json.Marshal(f)
	if err != nil {
		return fmt.Sprintf("store facts unencodable: %v", err)
	}
	return string(raw)
}
