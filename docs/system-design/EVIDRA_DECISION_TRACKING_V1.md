# Decision Tracking v1

**Status:** Implemented (v1 recording only)  
**Scope:** Explicit terminal `report` verdicts across CLI, MCP, and forwarded evidence

---

## Problem

Between `prescribe` and execution there is a decision moment that used to be invisible.
If an agent decides not to proceed because the assessed risk is unacceptable, that
decision disappears from the evidence chain.

The missing evidence is often the most valuable evidence:

- good judgment: the agent refused a dangerous operation for a good reason
- bad judgment: the agent refused a safe operation for a bad reason
- auditability: operators can see not only what happened, but what was intentionally avoided

---

## Invariant

> One prescription -> one report.

Decision tracking preserves that invariant.
A `declined` report is still a `report`.
The prescription is closed by that single terminal event.

---

## v1 Contract

### Explicit verdict

All report-capable surfaces require an explicit `verdict`:

- `success`
- `failure`
- `error`
- `declined`

This is a strict contract, not a backwards-compatible inference layer.

### Declined decision context

When `verdict=declined`, the report MUST include:

- `decision_context.trigger`
- `decision_context.reason`

Rules:

- `exit_code` is forbidden for `declined`
- `reason` is required and bounded to a short operational explanation
- `reason` must not contain secrets or chain-of-thought dumps

Example:

```json
{
  "prescription_id": "01JQ...",
  "verdict": "declined",
  "decision_context": {
    "trigger": "risk_threshold_exceeded",
    "reason": "risk_level=critical and blast_radius covers production namespace"
  }
}
```

### Executed outcomes

For `success`, `failure`, or `error`:

- `exit_code` is required
- `decision_context` is forbidden

---

## Surface Boundary

Supported in v1:

- `evidra report`
- MCP `report`
- forwarded/stored evidence that carries report payloads

Not supported in v1:

- `evidra record` emitting `declined`
- scoring changes based on decision evidence
- new decision-specific signals

`record` remains execution-only by design.

---

## Product Meaning

This changes Evidra from:

- flight recorder for actions

to:

- flight recorder for actions and decisions

The evidence chain can now answer:

- what the agent intended to do
- what it actually did
- what it intentionally chose not to do, and why

---

## Out Of Scope For v1

- decline analytics in `scorecard`
- trigger breakdowns
- judgment-quality scoring
- heuristics for good vs bad refusal reasons

These belong to v2 after real decision evidence exists in the field.
