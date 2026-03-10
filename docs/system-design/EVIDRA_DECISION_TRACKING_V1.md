# Decision Tracking — Design Summary

**Status:** Proposal (v1)
**Scope:** Report payload extension

---

## Problem

Between `prescribe` and `execute` there is a decision moment that is
currently invisible. If an actor receives a risk assessment and decides
not to proceed, that decision disappears from the evidence chain.

This affects AI agents, CI/CD pipelines, and human operators equally.
Without this data, a fundamental question is unanswerable: *does this
actor use risk assessment in its decisions, or does it ignore it?*

---

## Invariant

> One prescription → one report.

This proposal preserves that invariant. A `declined` report is still
a report. The prescription is closed.

---

## Protocol Change

### New Verdict

| Verdict | Meaning |
|---------|---------|
| `success` | Execution completed, operation succeeded (existing) |
| `failure` | Execution completed, operation failed (existing) |
| `error` | Execution path failed; result is not a normal operational outcome (existing) |
| **`declined`** | Execution intentionally not started after assessment |

### Verdict as Input Field

`declined` has no execution and therefore no `exit_code`. To support
this, `verdict` becomes an explicit report input and `exit_code`
becomes optional. When `verdict` is omitted, existing behavior is
preserved by inferring `success` from exit code 0 and `failure` from
non-zero exit codes. `declined` requires no exit code, and any
contradiction between `verdict` and `exit_code` is a validation error.

This is the main implementation cost: `verdict` becomes an input
across CLI, MCP, lifecycle service, and tests.

### Decision Context

A report with verdict `declined` MUST include `decision_context`.
For other verdicts, the field is absent.

```json
{
  "prescription_id": "01JQ...",
  "verdict": "declined",
  "decision_context": {
    "trigger": "risk_threshold_exceeded",
    "reason": "staging-only policy, blast radius covers production"
  }
}
```

Compact form (no free text):

```json
{
  "prescription_id": "01JQ...",
  "verdict": "declined",
  "decision_context": {
    "trigger": "policy_restriction"
  }
}
```

### Decision Context Fields

| Field | Type | Required | Purpose |
|-------|------|----------|---------|
| `trigger` | string | yes | What class of mechanism caused the decline |
| `reason` | string | no | Short human-readable explanation; max 512 chars; must not contain secrets or internal prompts |

### Trigger Vocabulary

`trigger` is a string with a recommended vocabulary. Unknown values
are preserved as-is.

| Value | Meaning |
|-------|---------|
| `risk_threshold_exceeded` | Risk level exceeded actor's threshold |
| `policy_restriction` | Operation violates actor's policy boundary |
| `actor_discretion` | Actor declined based on its own judgment |
| `other` | None of the above; reason carries detail |

---

## Protocol Fit

```
prescribe → assessment
         ├── report (verdict: success)     — executed, succeeded
         ├── report (verdict: failure)     — executed, failed
         ├── report (verdict: error)       — execution error
         └── report (verdict: declined)    — actor chose not to execute
```

The 1:1 invariant is preserved. Existing integrations are unaffected.

---

## Surface Changes

Surface changes are limited to accepting explicit `verdict` and
`decision_context` in report-capable interfaces (MCP, CLI, API).

CLI convenience: `--decline-trigger` and `--decline-reason` flags
populate `decision_context` and imply `verdict=declined`. Specifying
`--verdict declined` without `--decline-trigger` is a validation error.

---

## What This Answers

> "Does my agent understand risk, or does it just execute commands?"

> "How often does our risk gate actually stop deployments?"

> "When our tools said 'this is dangerous', who listened?"

**Intent → Decision → Outcome.** Three points, one evidence chain.

---

## Out of Scope

- Event-level signals and scoring policies derived from decision data.
- Rich analytics taxonomy (decline-by-trigger, proceeded-on-high-risk).
- Multi-actor handoff and escalation semantics.

Decision tracking may enable future analytics and scoring policies,
but those are out of scope for this proposal. Multi-actor workflows
can be modeled through trace lineage (Actor A declines, Actor B opens
a new prescribe in the same `trace_id`) without new protocol
primitives.
