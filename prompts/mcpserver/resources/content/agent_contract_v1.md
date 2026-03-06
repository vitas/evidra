<!-- contract: v1.0.1 -->
# Evidra Agent Contract v1

> Contract: `v1.0.1`
> Version policy: contract versions are released together with Evidra binaries.

## Changelog
- `v1.0.1` (2026-03-06): Prompt hardening update: critical invariants in initialize instructions, prescribe pre-call checklist, report terminal outcome rule, and expanded get_event usage guidance.
- `v1.0` (2026-03-06): Initial contract for prescribe/report protocol guidance and behavior tracking via `actor.skill_version`.

## Protocol
Every infrastructure mutation must follow two calls:
1. `prescribe` before execution
2. `report` after execution

Evidra records behavior. It does not block operations.

## What Requires Prescribe/Report
Mutating commands require protocol calls, including:
- `kubectl apply|delete|patch|create|replace|rollout restart`
- `helm install|upgrade|uninstall|rollback`
- `terraform apply|destroy|import`

Read-only commands should not use protocol calls, including:
- `kubectl get|describe|logs|top|events`
- `helm list|status|template`
- `terraform plan|show|output`

If uncertain, call `prescribe`.

## Required Inputs
`prescribe` requires:
- `tool`, `operation`, `raw_artifact`, `actor.type`, `actor.id`, `actor.origin`

`report` requires:
- `prescription_id`, `exit_code`

Recommended actor metadata:
- `actor.version`
- `actor.skill_version` (set from contract version, for benchmark slicing)

## Correlation Guidance
Use these fields for stable grouping and tracing:
- `session_id`, `operation_id`, `attempt`
- `trace_id`, `span_id`, `parent_span_id`

If you want one task grouped in one session, reuse the same `session_id`.

## Retry and Failure Rules
- Every `prescribe` must be followed by exactly one `report`.
- Failures must still be reported (`exit_code != 0`).
- Retries require new `prescribe` calls; do not reuse old `prescription_id`.
- If `prescription_id` is lost, call `prescribe` again before execution.
- Do not report another actor's `prescription_id`.
- Do not report the same `prescription_id` twice.

## Risk Output
`prescribe` may return:
- `risk_level` (`low`, `medium`, `high`, `critical`)
- `risk_tags` for known patterns
- digests and canonical metadata for auditability

Risk level is informational guidance for decision quality.

## Reliability Measurement
Your reliability is measured from evidence-chain behavior, including:
- protocol violations
- retry loops
- artifact drift
- blast radius
- new scope transitions

Better protocol compliance improves score quality and comparability.
