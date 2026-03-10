<!-- contract: v1.0.1 -->
# Evidra Runtime Experiment Agent Contract v1

> Contract: `v1.0.1`
> Version policy: contract versions are released together with Evidra binaries.

## Changelog
- `v1.0.1` (2026-03-06): Prompt hardening update: critical invariants in initialize instructions, prescribe pre-call checklist, report terminal outcome rule, and expanded get_event usage guidance.
- `v1.0.0` (2026-03-06): Initial contract for prescribe/report protocol guidance and behavior tracking via actor.skill_version.


## Purpose
- This contract standardizes MCP and experiment prompts around the same prescribe/report protocol semantics.
- Evidra records execution behavior; it does not block operations.


## Protocol Rules (Execution Mode)
- Every infrastructure mutation must call prescribe before execution and report after execution or explicit refusal.
- Mutate commands require protocol calls; read-only commands do not.
- If uncertain mutate vs read-only, call prescribe.
- Every prescribe must have exactly one report.
- Retries require a new prescribe/report pair for each attempt.
- Failures must be reported with non-zero exit_code.
- Deliberate refusals must be reported with verdict=declined, decision_context.trigger, and decision_context.reason.
- Do not report another actor's prescription_id.
- Do not report the same prescription_id twice.
- Include actor.skill_version for behavior slicing.


## Output Rules (Assessment Mode)
- In assessment mode, output exactly one JSON object.
- JSON must contain predicted risk level and predicted risk details.
- No markdown, prose, or code fences in assessment output.


Required JSON keys:
- `predicted_risk_level`
- `predicted_risk_details`
