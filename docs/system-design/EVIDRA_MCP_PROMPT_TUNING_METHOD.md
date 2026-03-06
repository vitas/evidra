# Evidra MCP Prompt Tuning Method

**Status:** Active guidance  
**Last updated:** 2026-03-06  
**Applies to:** `prompts/mcpserver/` contract prompts and MCP tool descriptions

## 1. Purpose

This document defines how to tune Evidra MCP prompts without breaking protocol semantics.
Goal: improve agent compliance (`prescribe`/`report` behavior) while preserving stable wire protocol behavior.

## 2. Prompt Surfaces and Responsibilities

Evidra MCP prompt surfaces have different jobs:

1. `initialize/instructions.txt`
   - High-signal protocol framing.
   - Must include critical invariants and mutate vs read-only boundary.
2. `tools/prescribe_description.txt`
   - Decision guidance for "should I call prescribe now?"
   - Input checklist and fail-safe behavior.
3. `tools/report_description.txt`
   - Terminal outcome rules (exactly one report per prescribe).
   - Failure-path requirements (report non-zero outcomes).
4. `tools/get_event_description.txt`
   - Protocol/audit debug lookup guidance.
5. `resources/content/agent_contract_v1.md`
   - Human-readable contract reference and changelog.

Design rule: tool descriptions must be sufficient even if the agent never opens the resource document.

## 3. Authoring Rules

1. Keep language imperative and testable.
2. Prefer short checklists over narrative paragraphs.
3. Put hard rules first, rationale second.
4. Include explicit negative scope:
   - "Do not call prescribe/report for non-infra tasks."
5. Keep failure paths explicit:
   - lost `prescription_id`
   - failed command
   - retry attempt
6. Keep content protocol-aligned:
   - prompts cannot contradict `docs/system-design/EVIDRA_PROTOCOL.md`.

## 4. Tuning Loop (Repeatable)

1. Baseline
   - Run MCP inspector tests and capture failure set.
2. Classify failures
   - Under-trigger, over-trigger, malformed input, ordering violation, correlation loss.
3. Patch prompts minimally
   - Add only rules that directly address observed failure class.
4. Re-run tests
   - Confirm fixes and check for regressions in unrelated cases.
5. Record outcome
   - Update prompt contract changelog and keep version traceability.

Do not change protocol semantics in prompt-only tuning work.

## 5. Test Strategy

### 5.1 Trigger tests

Should trigger:
- mutate intents (`apply`, `delete`, `patch`, `upgrade`, `destroy`, `rollback`)

Should not trigger:
- read-only diagnostics (`get`, `describe`, `logs`, `plan`, `show`, `status`, `template`)
- non-infrastructure work (documentation, coding tasks, generic analysis)

### 5.2 Functional protocol tests

1. `prescribe` succeeds with required fields.
2. `report` succeeds with valid `prescription_id`.
3. report on unknown `prescription_id` fails.
4. duplicate report fails.
5. cross-actor report fails.
6. retry flow requires fresh prescribe/report pair.

### 5.3 Failure-path tests

1. parser error in artifact returns `parse_error`.
2. failed command still reports non-zero `exit_code`.
3. lost `prescription_id` path requires re-prescribe.

### 5.4 Correlation tests

Validate preservation/consistency of:
- `session_id`
- `trace_id`
- `operation_id`
- actor identity and `actor.skill_version`

### 5.5 Performance/control tests

Track:
- tool-call count for same scenario before/after prompt change
- prompt-induced protocol violation delta
- token/latency drift if available in harness

## 6. Acceptance Gates

Recommended gates before merge:

1. MCP inspector suite passes in local mode.
2. No increase in protocol violation class failures.
3. No contradiction against protocol/system-design docs.
4. Contract version header present in all prompt files.
5. `agent_contract_v1.md` changelog updated.

If hosted/rest mode is disabled by flag, keep hosted tests gated and document that scope in test report.

## 7. Troubleshooting Playbook

### Symptom: agent skips `prescribe` on mutation
- Cause: weak mutate/read-only boundary in prompt text.
- Fix: add explicit mutate command classes and "if unsure call prescribe".

### Symptom: agent reports without valid `prescription_id`
- Cause: missing recovery rule.
- Fix: add lost-id fail-safe in `report_description`.

### Symptom: missing report after command failure
- Cause: success-path bias in prompt wording.
- Fix: add terminal outcome rule in `report_description`.

### Symptom: over-trigger on non-infrastructure work
- Cause: missing negative trigger guidance.
- Fix: add non-infra exclusion lines in initialize/prescribe prompts.

### Symptom: behavior slicing missing
- Cause: `actor.skill_version` not reinforced.
- Fix: include explicit requirement in initialize + tool descriptions and verify ingestion path.

## 8. Versioning Policy for Prompt Changes

Prompts are part of Evidra releases; they are not versioned as a separate product.

1. All prompt files must start with `contract` header.
2. Contract version is carried into `actor.skill_version`.
3. Changelog is kept in `agent_contract_v1.md`.
4. Version bump rules:
   - Patch (`v1.0.0 -> v1.0.1`): wording clarification, no required behavior changes.
   - Minor (`v1.0.x -> v1.1.0`): new guidance that may affect behavior but not wire schema.
   - Major (`v1.x -> v2.0`): new required behavior or protocol contract shift.

## 9. Required Update Points per Prompt Change

1. Update prompt files under `prompts/mcpserver/`.
2. Update `agent_contract_v1.md` changelog.
3. Ensure parser defaults in `prompts/embed.go` and `pkg/mcpserver/server.go` are aligned with latest contract version.
4. Update docs that describe prompt behavior:
   - `docs/system-design/MCP_CONTRACT_PROMPTS.md`
   - `README.md` MCP section (if guidance changed materially)

## 10. Verification Commands

Use these checks before claiming completion:

```bash
go test ./prompts ./pkg/mcpserver -count=1
make test-mcp-inspector-ci
bash scripts/check-doc-commands.sh
```

If CI intentionally scopes e2e to release-only workflows, verify that condition explicitly in workflow docs and job filters.
