# V1 Run/Record Contract

**Status:** Normative for v1 CLI ingestion/orchestration  
**Date:** 2026-03-09

---

## 1. Scope

This contract defines:

1. `evidra run` orchestration boundary
2. `evidra record` ingestion schema
3. required output/basis fields for first-use value

Design rule:

- `run` is orchestration, not a second engine.
- `record` validates structured input, then uses the same lifecycle pipeline as `run`.

---

## 2. Normative Boundary

- `run` = Evidra executes and observes the command live.
- `record` = Evidra ingests a completed automation execution from structured input.

Both modes must produce equivalent prescribe/report semantics for equivalent operations.

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
- `exit_code`
- `duration_ms`

Conditional requirement:

- at least one of:
  - `raw_artifact` (string)
  - `canonical_action` (JSON object)

Optional fields:

- `attempt`
- `actor.provenance`
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

For `run` and `record`, output must include:

- `risk_level`
- `score`
- `score_band`
- `signal_summary`
- `basis` (preview/sufficient indicator and operation-count context)
- `confidence`

A separate score-band alias must not be emitted. It is not a distinct runtime concept.

`evidra report` returns an immediate assessment snapshot for the session:

- `prescription_id`
- `exit_code`
- `verdict`
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
