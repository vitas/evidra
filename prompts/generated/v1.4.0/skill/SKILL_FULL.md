---
name: evidra
description: "Use this skill when the user asks you to perform DevOps operations — diagnosing, fixing, or managing infrastructure using kubectl, helm, terraform, or aws commands. This includes: running kubectl get/describe/logs/apply/delete/patch, helm install/upgrade/status/list, terraform plan/apply/destroy/import, aws CLI commands, or any infrastructure investigation and remediation workflow. Also trigger when the user mentions Evidra, evidence chains, prescribe/report protocol, reliability scorecards, or run_command. DO NOT trigger for: writing Dockerfiles, writing Ansible/Terraform code without executing it, CI/CD pipeline setup, explaining infrastructure concepts, or writing tests for infrastructure tools. The key distinction is OPERATING infrastructure (both reading and mutating) vs WRITING code."
---
<!-- contract: v1.4.0 -->

# Evidra — Full Prescribe MCP Server

evidra-mcp is your infrastructure toolkit. Use `run_command` for kubectl, helm, terraform, and aws operations.

## Full-Prescribe Mode

Install this skill only when the MCP server is started with `--full-prescribe`.
Use `run_command` for investigation and routine operations, but when you have artifact bytes and want explicit control, prefer `prescribe_full` before execution and `report` after execution.

## Diagnosis Protocol

- Investigate before fixing: kubectl describe, get events, check logs
- Make one targeted fix, verify it worked, then stop
- Don't patch resources you didn't investigate

## Safety

- Never delete resources outside the problem scope
- Verify fixes with kubectl get or rollout status
- Check what exists before creating (avoid duplicates)

## Explicit Evidence Recording

Infrastructure mutations executed through `run_command` are always recorded. If you prescribed the same action beforehand with `prescribe_smart` or `prescribe_full`, the recorded evidence links to your claim automatically; otherwise the server records the claim for you and it counts as an `unprescribed_mutation`. For explicit control, call `describe_tool` to inspect `prescribe_smart` and `report`; use `prescribe_full` only when the server exposes it and you have artifact bytes.

### prescribe_full (recommended when artifact bytes are available)

Call with: tool, operation, raw_artifact, actor.
Use this when you have the full YAML/manifest and want drift detection plus artifact-aware assessment.

```json
{
  "tool": "kubectl", "operation": "apply",
  "raw_artifact": "apiVersion: apps/v1\nkind: Deployment\n...",
  "actor": {"type": "agent", "id": "your-id", "origin": "mcp-stdio", "skill_version": "1.4.0"}
}
```

### prescribe_smart (fallback when you do not have artifact bytes)

Call with: tool, operation, resource, namespace, actor.
Use this only when the target is known but the artifact bytes are not available in context.

### report (after every explicit mutation)

Call with: prescription_id, verdict (success/failure/declined), exit_code, actor.
On retry: new prescribe, execute, new report (each attempt is a pair).

### Critical Rules

- Every infrastructure mutation must be recorded as evidence, either automatically via run_command or explicitly via prescribe/report.
- If you use prescribe_smart or prescribe_full, do not execute until the prescribe call returns ok=true with prescription_id.
- Every explicit prescribe must have exactly one report.
- Always include actor.skill_version (set to this contract version) on explicit protocol calls.
- Report failures honestly (non-zero exit_code)
- Declined verdicts use decision_context, not exit_code

## Decision Rule

1. Diagnose first with read-only commands or `run_command`.
2. If you have artifact bytes and the server exposes `prescribe_full`, use it.
3. If not, fall back to `prescribe_smart`.
4. Always call `report` after an explicit prescribe flow.
