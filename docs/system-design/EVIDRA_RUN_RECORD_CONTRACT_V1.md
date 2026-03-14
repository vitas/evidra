# Evidra Record/Import Contract

- Status: Normative
- Version: v1.0
- Canonical for: record/import orchestration semantics
- Audience: public
- Date: 2026-03-09

---

## 1. Scope

This contract defines:

1. `evidra record` orchestration boundary
2. `evidra import` ingestion schema
3. required output/basis fields for first-use value

Design rule:

- `record` is orchestration, not a second engine.
- `import` validates structured input, then uses the same lifecycle pipeline as `record`.

---

## 2. Normative Boundary

- `record` = Evidra executes and observes the command live.
- `import` = Evidra ingests a completed automation execution from structured input.

Both modes must produce equivalent prescribe/report semantics for equivalent operations.
When the lifecycle input is the same, they should also expose the same prescribe-time
`risk_inputs` panel and `effective_risk` roll-up.

---

## 3. Record Input Schema (v1)

Supported contract version:

- `contract_version: "v1"`

Required fields:

- `contract_version`
- `session_id`
- `operation_id`
- `tool`
- `operation`
- `environment`
- `actor.type`
- `actor.id`
- `actor.provenance`
- `exit_code`
- `duration_ms`

Conditional requirement:

- at least one of:
  - `raw_artifact` (string)
  - `canonical_action` (JSON object)

Optional fields:

- `attempt`
- `actor.instance_id`
- `actor.version`
- `actor.skill_version`

Validation rules:

1. `contract_version` must equal `v1`.
2. `duration_ms >= 0`.
3. `attempt >= 0` when provided.
4. empty/whitespace required fields are invalid.

---

## 4. First Useful Output (v1)

For `record` and `import`, output must include:

- `risk_inputs`
- `effective_risk`
- `score`
- `score_band`
- `signal_summary`
- `basis` (preview/sufficient indicator and operation-count context)
- `confidence`

A separate score-band alias must not be emitted. It is not a distinct runtime concept.

`evidra report` returns an immediate assessment snapshot for the session:

- `prescription_id`
- `verdict`
- `exit_code` (required for `success`/`failure`/`error`, absent for `declined`)
- `decision_context` (required for `declined`, absent otherwise)
- `score`
- `score_band`
- `signal_summary`
- `basis`
- `confidence`

If total operations are below sufficiency threshold, response must be explicitly marked as preview.

---

## 5. Compatibility

This contract is additive and does not change:

- evidence wire format in `pkg/evidence`
- scoring semantics in `internal/score` (`MinOperations=100` remains canonical)
