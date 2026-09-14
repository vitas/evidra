package main

import "fmt"

// scriptedPlan turns a task spec into the protocol-correct call sequence a
// cooperating agent would produce. It is what --dry-run replays, and it is the
// check that every predicate in tasks.json is actually satisfiable: a task no
// scripted run can pass is a broken task, not a hard model question.
// script variants: compliant follows the protocol, late acts before it
// prescribes (the shape §10 measures blocked attempts and unprescribed
// executions from), noreport never closes the record.
const (
	scriptCompliant   = "compliant"
	scriptLate        = "late"
	scriptNoReport    = "noreport"
	scriptNoPrescribe = "noprescribe"
)

// scriptedPlan in baseline mode omits every protocol call: prescribe and report are not
// steps an agent can take against a server that does not offer them, and the variants whose
// whole subject is a protocol shape (late, noreport, noprescribe) have no meaning without it.
// So the baseline plan is the operational work, which is also what proves a task's operational
// predicates are satisfiable with no Evidra involved.
func scriptedPlan(task taskSpec, variant string, baseline bool) []plannedCall {
	var plan []plannedCall
	if baseline {
		var work []plannedCall
		for _, want := range task.ExpectTools {
			n := want.Min
			if n < 1 {
				n = 1
			}
			for i := 0; i < n; i++ {
				work = append(work, plannedCall{Tool: want.Tool, Args: argsFor(want)})
			}
		}
		return work
	}
	report := func(status, outcome string) {
		if status == "" {
			status = "completed"
		}
		if outcome == "" {
			outcome = "achieved"
		}
		plan = append(plan, plannedCall{Tool: "evidra_report", Args: map[string]any{
			"operation_id": "{{open}}", "status": status, "outcome": outcome,
		}})
	}

	if task.ID == "change-of-mind" {
		plan = append(plan,
			plannedCall{Tool: "evidra_prescribe", Args: map[string]any{
				"objective":        "restart checkout",
				"expected_outcome": "checkout restarted",
			}},
			plannedCall{Tool: "restart", Args: map[string]any{"service": "checkout"}},
			plannedCall{Tool: "evidra_prescribe", Args: map[string]any{
				"objective":           "restart search",
				"expected_outcome":    "search restarted",
				"abandon_and_replace": true,
			}},
			plannedCall{Tool: "restart", Args: map[string]any{"service": "search"}},
		)
		report("completed", "achieved")
		return plan
	}

	prescribe := plannedCall{Tool: "evidra_prescribe", Args: map[string]any{
		"objective":        task.Title,
		"expected_outcome": fmt.Sprintf("%s done", task.ID),
	}}
	var work []plannedCall
	for _, want := range task.ExpectTools {
		n := want.Min
		if n < 1 {
			n = 1
		}
		for i := 0; i < n; i++ {
			work = append(work, plannedCall{Tool: want.Tool, Args: argsFor(want)})
		}
	}
	if len(work) > 0 {
		first := work[0]
		switch variant {
		case scriptLate:
			// Act first, get blocked, prescribe, retry the blocked call, then
			// finish: this is the recovery path §10's recovery rate counts.
			// In observe-only mode nothing is blocked, so the retry shows up as
			// a duplicate action; use --modes-only all to read the recovery
			// shape from this script.
			plan = append(plan, first, prescribe, first)
			work = work[1:]
		case scriptNoPrescribe:
			// Never prescribes: every upstream call is unprescribed, which is
			// how observe-only coverage and sessions_with_no_prescribe get
			// exercised without a model.
			plan = append(plan, work...)
			return plan
		case scriptNoReport:
			plan = append(plan, prescribe)
			plan = append(plan, work...)
			return plan
		default:
			plan = append(plan, prescribe)
		}
	} else {
		plan = append(plan, prescribe)
	}
	plan = append(plan, work...)
	status, outcome := "", ""
	if r := task.RequireReport; r != nil {
		if len(r.StatusIn) > 0 {
			status = r.StatusIn[0]
		}
		if len(r.OutcomeIn) > 0 {
			outcome = r.OutcomeIn[0]
		}
	}
	report(status, outcome)
	return plan
}

// argsFor builds the smallest argument set that satisfies a predicate.
func argsFor(w toolExpectation) map[string]any {
	args := map[string]any{}
	for k, v := range w.ArgsContain {
		args[k] = v
	}
	for k, v := range w.ArgsMin {
		args[k] = v
	}
	if len(args) == 0 {
		switch w.Tool {
		case "restart":
			args["service"] = "payments"
		case "slow":
			args["delay_ms"] = float64(5000)
			args["label"] = "gatea"
		case "big":
			args["bytes"] = float64(12582912)
		}
	}
	return args
}
