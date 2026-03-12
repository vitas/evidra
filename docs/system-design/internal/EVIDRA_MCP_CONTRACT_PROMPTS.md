# Evidra MCP Contract Prompts — Design Document

**Status:** Implemented (generated from canonical source contracts)
**Date:** March 2026
**Replaces:** Current prompts in `prompts/mcpserver/` (inherited from old project)
**Based on:** EVIDRA_AGENT_SKILL.md
**Operational tuning guide:** `docs/system-design/EVIDRA_MCP_PROMPT_TUNING_METHOD.md`
**Source-of-truth model:** `docs/system-design/EVIDRA_PROMPT_FACTORY_SPEC.md`

---

> Note: this document captures design intent and rationale.
> Source of truth is `prompts/source/contracts/<version>/` plus templates.
> Files under `prompts/mcpserver/*` are generated artifacts and should not be edited manually.
> Prompt text examples below are design snapshots; authoritative wording is generated from source contracts.

## 1. Problem

MCP server is a flight recorder for AI infrastructure agents and their decisions.

Current MCP prompts are too thin. The `initialize` instructions are 6 lines. Tool descriptions are 4 lines each. The agent contract is 5 generic sections. This leads to:

- Agents forget to prescribe on later steps (protocol fatigue — no reinforcement)
- Agents don't know which operations need prescribe (no classification list)
- Agents don't prescribe again on retry (no retry protocol)
- Agents don't report failures (instructions say "always" but don't emphasize failure reporting)
- Agents lose prescription_id and proceed without it (no recovery protocol)

The fault injection experiment (F03, F04) will catch these failures. But they should be preventable through better instructions, not just detectable.

---

## 2. Design Principles

MCP tool descriptions are the PRIMARY prompt surface. When an agent connects via MCP, it sees:

1. **Initialize instructions** — once, at connection time
2. **Tool descriptions** — every time the agent considers calling a tool
3. **Resource content** — on-demand, if the agent reads the contract resource

Tool descriptions are read most often. They must be self-contained — an agent that never reads the contract resource should still follow the protocol correctly from tool descriptions alone.

**Constraints:**
- MCP tool descriptions should be 100-200 words (long enough to be complete, short enough to fit in context)
- Initialize instructions should be ≤10 lines (first impression, not a manual)
- Contract resource is the reference — can be longer, but agents may never read it

### Prompt Versioning Policy

- Contract version is embedded in prompt files and released together with Evidra binaries.
- No separate prompt-version document is maintained.
- Every prompt file must start with a contract header comment.
- `agent_contract_v1.md` keeps the contract changelog section.

---

## 3. File: `prompts/mcpserver/initialize/instructions.txt`

```
# contract: v1.0.1

Evidra — Flight recorder for AI infrastructure agents.
It measures operational reliability across CI pipelines, scripts, and AI agents; it does not block operations.
Evidra speaks MCP: any MCP-capable AI agent can report to Evidra out of the box.

PROTOCOL — two calls per infrastructure operation:
1. prescribe — call BEFORE any kubectl/terraform/helm command that modifies resources
2. report — call AFTER the command completes or the agent intentionally declines

Read-only commands (get, describe, logs, plan, status) do NOT need prescribe/report.

Every prescribe MUST have a matching report. Missing reports are protocol violations.
If a command fails, still report with the non-zero exit code. Failures are valuable data.
If the agent intentionally refuses to execute, still report with verdict=declined, a trigger, and a short operational reason.
If you retry a failed operation, call prescribe again before each retry attempt.

Your operational reliability is measured from these calls.
```

**Changes from current:**
- Added explicit list of what needs prescribe vs what doesn't
- Added retry protocol (prescribe again per retry)
- Added failure reporting emphasis
- Removed "risk level" guidance (moved to prescribe tool description where it's contextual)
- 12 lines vs 6 — still compact

---

## 4. File: `prompts/mcpserver/tools/prescribe_description.txt`

```
# contract: v1.0

Record intent BEFORE an infrastructure operation that creates, modifies, or deletes resources.

WHEN TO CALL:
  Must call: kubectl apply, delete, patch, create, replace, rollout restart,
             helm install, upgrade, uninstall, rollback,
             terraform apply, destroy, import,
             ansible-playbook, docker run, docker rm
  Skip:      kubectl get, describe, logs, top, events,
             helm list, status, terraform plan, show,
             cat, ls, grep, curl (read-only commands)

If unsure whether a command modifies state — call prescribe. False positives are harmless.

INPUTS:
  tool: CLI tool name (kubectl, terraform, helm, ansible)
  operation: what you are doing (apply, delete, create, upgrade, rollback, destroy, import)
  raw_artifact: full content of the manifest, plan output, or config being applied
  actor: your identity (type, id, origin)

RETURNS:
  prescription_id — save this, you need it for the report call
  risk_level — informational: low, medium, high, or critical
  risk_tags — detected risk patterns (e.g., k8s.privileged_container)
  artifact_digest — SHA-256 of the artifact you submitted

RISK LEVEL GUIDANCE:
  low/medium: proceed normally
  high: consider logging your reasoning before proceeding
  critical: strongly consider requesting human approval

If prescribe fails, do NOT execute the infrastructure command.
```

**Changes from current:**
- Explicit WHEN TO CALL classification (the most important addition)
- "If unsure, prescribe" principle
- Structured INPUTS/RETURNS format for LLM parsing
- Risk level guidance moved here from initialize (contextual)
- Error handling: prescribe fails → don't execute
- 200 words vs 40 — but every word earns its place

---

## 5. File: `prompts/mcpserver/tools/report_description.txt`

```
# contract: v1.0

Record the terminal verdict AFTER an infrastructure operation completes or is intentionally declined.

ALWAYS call report after every prescribe, whether the operation succeeded, failed, errored, or was intentionally declined.
A missing report is a protocol violation that reduces your reliability score.

INPUTS:
  prescription_id — the ID returned by the preceding prescribe call (required)
  verdict — required terminal outcome: success, failure, error, or declined
  exit_code — command exit code: required for success/failure/error, forbidden for declined
  decision_context.trigger — required for declined
  decision_context.reason — required for declined; short operational explanation
  actor: your identity (must match the prescribe actor)
  artifact_digest — SHA-256 of what was actually applied (optional, enables drift detection)

IMPORTANT RULES:
  1. Every prescribe MUST have exactly one matching report.
  2. Report failures too — exit_code=1 is valuable data, not something to hide.
  3. If you intentionally decline, report verdict=declined with a concise operational reason.
  4. Do NOT report on the same prescription_id twice (duplicate report violation).
  5. Do NOT report on another agent's prescription_id (cross-actor violation).
  6. If you lost the prescription_id, call prescribe again to get a new one,
     then execute, then report on the new one. Do not proceed without a prescription.
  7. If you are retrying a failed operation, call prescribe again first —
     each attempt needs its own prescribe/report pair.

RETURNS:
  ok — true if report was recorded successfully
  report_id — unique identifier for this report
  verdict — echoed terminal verdict
  decision_context — echoed when verdict=declined
  error — error details if ok=false
```

**Changes from current:**
- 6 explicit rules covering all protocol edge cases
- Lost prescription_id recovery protocol (rule 5)
- Retry protocol reinforcement (rule 6)
- Structured INPUTS/RETURNS format
- Emphasis on reporting failures
- 150 words vs 30

---

## 6. File: `prompts/mcpserver/tools/get_event_description.txt`

```
# contract: v1.0

Look up a single evidence record by event_id.

INPUT:
  event_id

RETURNS:
  ok
  entry (when found)
  error (when not found or invalid)
```

**Changes:** New file. Currently get_event has an inline description in server.go. Moving to file for consistency.

---

## 7. File: `prompts/mcpserver/resources/content/agent_contract_v1.md`

```markdown
# Evidra Agent Contract v1

> Contract: `v1.0`
> Version policy: contract version is released with Evidra binaries.

## Changelog
- `v1.0` (2026-03-06): Added contract header and changelog section.

## Protocol

Every infrastructure operation follows two steps:

1. **prescribe** — before execution, record what you intend to do
2. **report** — after execution or refusal, record the terminal verdict

This is the prescribe/report protocol. It creates a signed, tamper-evident
evidence chain of every infrastructure change.

## What Needs Prescribe/Report

Commands that CREATE, MODIFY, or DELETE infrastructure resources:
  kubectl apply, delete, patch, create, replace, rollout restart
  helm install, upgrade, uninstall, rollback
  terraform apply, destroy, import
  ansible-playbook (any run that changes state)

Commands that READ without changing state (skip prescribe/report):
  kubectl get, describe, logs, top, events
  helm list, status, template
  terraform plan, show, output
  any cat, ls, grep, curl, or diagnostic command

Rule: if unsure, prescribe. A false positive costs nothing.
A missing prescribe is a protocol violation.

## Retry Protocol

If an operation fails and you retry:
  1. Do NOT reuse the old prescription_id
  2. Call prescribe again with the current artifact
  3. Execute
  4. Call report with the new prescription_id

Each attempt is a separate prescribe/report pair.
This enables retry loop detection.

## Error Handling

If prescribe returns an error → do not execute the command.
If the command fails → still call report with verdict=failure or verdict=error and the exit code.
If you intentionally decline → call report with verdict=declined plus decision_context.trigger and decision_context.reason.
If you lose the prescription_id → call prescribe again before proceeding.

Never execute infrastructure changes without a valid prescription_id.

## Risk Levels

prescribe returns a risk assessment:
  low — proceed normally
  medium — proceed with awareness
  high — consider logging reasoning before proceeding
  critical — strongly consider requesting human approval

Risk levels are informational. Evidra never blocks operations.

## Risk Tags

prescribe may return risk_tags identifying specific patterns:
  k8s.privileged_container — container has privileged security context
  k8s.hostpath_mount — pod mounts host filesystem path
  k8s.host_namespace_escape — pod uses hostPID, hostIPC, or hostNetwork
  tf.s3_public_access — S3 bucket without complete public access block
  tf.iam_wildcard — IAM policy with wildcard Action or Resource
  ops.mass_delete — operation deletes more than 10 resources

These tags help you make informed decisions about whether to proceed.

## Your Reliability Score

Every prescribe/report pair contributes to your reliability score.
Five signals are measured:

  protocol_violation — did you follow prescribe/report consistently?
  artifact_drift — did you apply what you said you would apply?
  retry_loop — did you get stuck retrying the same failed operation?
  blast_radius — did you run destructive operations on many resources?
  new_scope — did you operate in new tool/environment combinations?

Higher protocol compliance and fewer anomalies = higher score.

## Session Boundaries

If you want operations grouped into one session, pass the same session_id
to every prescribe call. If session_id is omitted, Evidra generates a
new one per prescribe — meaning each operation becomes its own session.

For consistent session grouping, generate one session_id at the start of
your task and reuse it across all prescribe/report calls in that task.
```

**Changes from current:**
- Added: What needs prescribe/report (with full list)
- Added: Retry protocol
- Added: Error handling
- Added: Risk tags explanation
- Added: Reliability score explanation (5 signals)
- Added: Session boundaries
- Removed: nothing (all current content preserved, expanded)
- 300 words vs 120

---

## 8. Implementation Notes

### Where These Are Used in Code

```go
// pkg/mcpserver/server.go

// Initialize instructions — sent once when agent connects
initializeInstructions = loadPromptFile("prompts/mcpserver/initialize/instructions.txt")

// Tool descriptions — sent with tool list
prescribeToolDescription = loadPromptFile("prompts/mcpserver/tools/prescribe_description.txt")
reportToolDescription = loadPromptFile("prompts/mcpserver/tools/report_description.txt")
getEventToolDescription = loadPromptFile("prompts/mcpserver/tools/get_event_description.txt")

// Contract resource — requires adding new resource to server.go:
// URI: evidra://agent/contract (not yet implemented)
// Current resources: evidra://event/{event_id}, evidra://evidence/manifest
agentContractContent = loadPromptFile("prompts/mcpserver/resources/content/agent_contract_v1.md")
```

Currently `server.go` has inline string constants. These should be replaced with file reads (or embeds) from `prompts/`. The schema files already use `//go:embed` — same pattern.

### Contract Tightening

The report contract is now explicit rather than inferred. Agents must send a `verdict` on every report. Refusals are first-class evidence and therefore require structured `decision_context`.

### Versioning

Prompt files do not have a separate version document. Contract version is stored in each prompt file header and is released with Evidra binaries. Agents pass this value in `actor.skill_version` so behavior can be sliced by contract version. If the contract changes materially (new required behavior, not just clarification), bump to `agent_contract_v2.md`.

---

## 9. Prompt Size Budget

| File | Current | New | Budget | Notes |
|------|---------|-----|--------|-------|
| `instructions.txt` | 45 words | 95 words | ≤120 | First impression, compact |
| `prescribe_description.txt` | 40 words | 200 words | ≤250 | Most-read prompt, must be complete |
| `report_description.txt` | 30 words | 150 words | ≤200 | Critical for protocol compliance |
| `get_event_description.txt` | 15 words | 40 words | ≤60 | Simple lookup, minimal |
| `agent_contract_v1.md` | 120 words | 350 words | ≤500 | Reference doc, read on demand |

Total MCP prompt overhead per agent session: ~500 tokens for tool descriptions + ~100 tokens for initialize. The contract resource is only loaded if the agent explicitly reads it.

At $0.50/1M tokens this is $0.0003 per session. Negligible.

---

## 10. Testing

After replacing prompts, verify detectors still work. The fault injection scripts are defined in `FAULT_INJECTION_RUNBOOK.md` and live in the experiment workspace (not in the Evidra repo):

```bash
# From ~/evidra-experiment/ (see FAULT_INJECTION_RUNBOOK.md)
cd ~/evidra-experiment
bash run_all_faults.sh
```

Alternatively, run the existing Evidra test suite to verify nothing broke:

```bash
# From ~/evidra/ (the repo)
make test
make e2e
```

Then run 2-3 agent sessions with different models to observe:
- Does the agent call prescribe before kubectl apply?
- Does the agent call report after failure?
- Does the agent prescribe again on retry?
- Does the agent skip prescribe for kubectl get/describe?

Compare protocol compliance rate (from `evidra explain`) before and after prompt update. The new prompts should measurably improve compliance, especially on weaker models.
