<!-- contract: v1.4.0 -->
# Evidra Agent Contract v1

> Contract: `v1.4.0`
> Version policy: contract versions are released together with Evidra binaries.

## Changelog
- `v1.3.0` (2026-03-26): Shifted MCP initialize guidance to a run_command-first workflow, with describe_tool exposing explicit prescribe/report schemas on demand.
- `v1.2.0` (2026-03-24): Unified contract: DevOps operations, MCP prompt templates, and evidence protocol under single source of truth. Added diagnosis flowchart and risk tag reference to contract.
- `v1.1.0` (2026-03-18): Split direct prescribe into prescribe_full and prescribe_smart so artifact-heavy and lightweight workflows have separate MCP prompt surfaces.
- `v1.0.1` (2026-03-06): Prompt hardening update: critical invariants in initialize instructions, prescribe pre-call checklist, report terminal outcome rule, and expanded get_event usage guidance.
- `v1.0.0` (2026-03-06): Initial contract for prescribe/report protocol guidance and behavior tracking via actor.skill_version.


## Default Workflow
1. Use `run_command` for investigation and most mutations. Mutations executed through it are recorded automatically.
2. Use `describe_tool` if you need the full schema for deferred explicit-control tools.
3. For explicit control, call `prescribe_smart` before execution and `report` after execution or explicit refusal.
4. Use `prescribe_full` only when enabled and you have artifact bytes.

This contract standardizes MCP prompts around a run_command-first workflow, with explicit prescribe/report available on demand.
Evidra records execution behavior; it does not block operations.


## What May Use Explicit Prescribe/Report
Mutating commands may use explicit protocol calls when you are not relying on `run_command` auto-evidence or when you need tighter control, including:
- `kubectl apply/delete/patch/create/replace/rollout restart`
- `helm install/upgrade/uninstall/rollback`
- `terraform apply/destroy/import`


Read-only commands should not use protocol calls, including:
- `kubectl get/describe/logs/top/events`
- `helm list/status/template`
- `terraform plan/show/output`


If uncertain, investigate with `run_command` first. If you still need explicit control, call `describe_tool` and then `prescribe_smart`.

## Required Inputs
`prescribe_full` requires:
- `tool`
- `operation`
- `raw_artifact`
- `actor (type, id, origin)`


`prescribe_smart` requires:
- `tool`
- `operation`
- `resource`
- `actor (type, id, origin)`


`report` requires:
- `prescription_id (from prescribe_full or prescribe_smart)`
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
- If prescription_id is lost, call a prescribe tool again before execution.
- Actor identity should match the original prescribe actor.
- Include actor.skill_version for behavior slicing.
- exit_code is required for success/failure/error verdicts and forbidden for declined verdicts.
- On retry, call a prescribe tool again to get a new prescription_id before re-executing. Each attempt is a separate prescribe/report pair.


## Risk Output
Either prescribe tool may return:
- `prescription_id (required for report)`
- `risk_inputs, effective_risk`
- `artifact_digest, intent_digest`
- `resource_shape_hash, operation_class, scope_class`


`effective_risk` is the roll-up guidance. `risk_inputs` shows the underlying instrument panel Evidra saw at prescribe time.

## Reliability Measurement
Your reliability is measured from evidence-chain behavior.
Core invariants:
- Every infrastructure mutation must be recorded as evidence, either automatically via run_command or explicitly via prescribe/report.
- If you use prescribe_smart or prescribe_full, do not execute until the prescribe call returns ok=true with prescription_id.
- Every explicit prescribe must have exactly one report.
- Always include actor.skill_version (set to this contract version) on explicit protocol calls.
