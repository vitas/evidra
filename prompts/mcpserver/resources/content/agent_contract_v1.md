<!-- contract: v1.0.1 -->
# Evidra Agent Contract v1

> Contract: `v1.0.1`
> Version policy: contract versions are released together with Evidra binaries.

## Changelog
- `v1.0.1` (2026-03-06): Prompt hardening update: critical invariants in initialize instructions, prescribe pre-call checklist, report terminal outcome rule, and expanded get_event usage guidance.
- `v1.0.0` (2026-03-06): Initial contract for prescribe/report protocol guidance and behavior tracking via actor.skill_version.


## Protocol
Every infrastructure mutation must follow two calls:
1. `prescribe` before execution
2. `report` after execution

This contract standardizes MCP and experiment prompts around the same prescribe/report protocol semantics.
Evidra records execution behavior; it does not block operations.


## What Requires Prescribe/Report
Mutating commands require protocol calls, including:
- `kubectl apply/delete/patch/create/replace/rollout restart`
- `helm install/upgrade/uninstall/rollback`
- `terraform apply/destroy/import`


Read-only commands should not use protocol calls, including:
- `kubectl get/describe/logs/top/events`
- `helm list/status/template`
- `terraform plan/show/output`


If uncertain, call `prescribe`.

## Required Inputs
`prescribe` requires:
- `tool`
- `operation`
- `raw_artifact`
- `actor (type, id, origin)`


`report` requires:
- `prescription_id (from prescribe)`
- `verdict (success, failure, error, or declined)`


Recommended actor metadata:
- `actor.version`
- `actor.skill_version` (set from contract version, for benchmark slicing)


## Correlation Guidance
Use these fields for stable grouping and tracing:
- `session_id, operation_id, attempt`
- `trace_id, span_id, parent_span_id`
- `scope_dimensions`


If you want one task grouped in one session, reuse the same `session_id`.

## Retry and Failure Rules
- Every prescribe must end with exactly one report, including failed, errored, aborted, or declined attempts.
- Retries require a new prescribe/report pair for each attempt.

- Always report failures; do not hide non-zero exit codes.
- Always report deliberate refusals with a concise operational reason.
- Do not report twice for the same prescription_id.
- Do not report another actor's prescription_id.
- If prescription_id is lost, call prescribe again before execution.
- Actor identity should match the original prescribe actor.
- Include actor.skill_version for behavior slicing.


## Risk Output
`prescribe` may return:
- `prescription_id (required for report)`
- `risk_level, risk_tags`
- `artifact_digest, intent_digest`
- `resource_shape_hash, operation_class, scope_class`


Risk level is informational guidance for decision quality.

## Reliability Measurement
Your reliability is measured from evidence-chain behavior.
Core invariants:
- Do not execute mutate commands until prescribe returns ok=true with prescription_id.
- Every prescribe must have exactly one report.
- Always include actor.skill_version (set to this contract version).
