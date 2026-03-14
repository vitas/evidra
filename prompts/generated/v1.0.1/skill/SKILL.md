---
name: evidra
description: "Use this skill when the user asks you to EXECUTE infrastructure mutations — commands that create, modify, or delete real resources. This includes: kubectl apply/delete/patch/create/replace/scale/rollout restart, helm install/upgrade/uninstall/rollback, terraform apply/destroy/import, docker run/rm/stop/kill, docker-compose up/down, podman/nerdctl equivalents, or any IaC tool that changes live infrastructure state. Also trigger when the user mentions Evidra, evidence chains, prescribe/report protocol, or reliability scorecards. DO NOT trigger for: writing Dockerfiles, writing Ansible/Terraform code without executing it, CI/CD pipeline setup, kubectl get/describe/logs (read-only), helm list/status/template, terraform plan/show, explaining infrastructure concepts, or writing tests for infrastructure tools. The key distinction is EXECUTING mutations vs WRITING code or READING state."
---
<!-- contract: v1.0.1 -->

# Evidra MCP Protocol for Infrastructure Operations

Evidra is a flight recorder for AI infrastructure agents. It measures operational reliability across CI pipelines, scripts, and AI agents without blocking operations. When the Evidra MCP server is connected, you have access to `prescribe` and `report` tools that record your infrastructure decisions into a tamper-evident evidence chain.

The evidence chain enables reliability scoring, behavioral signal detection, and auditability. Your reliability score improves when you follow the protocol consistently — and degrades when you skip steps, hide failures, or break the prescribe/report pairing.

## The Protocol

Every infrastructure **mutation** follows two steps:

```
1. prescribe  →  get prescription_id + risk assessment
2. [execute the command]
3. report     →  record verdict + get reliability score
```

Read-only operations skip the protocol entirely.

## Critical Invariants

These three rules are non-negotiable:

- **Do not execute mutate commands until prescribe returns ok=true with prescription_id.**
- **Every prescribe must have exactly one report.**
- **Always include actor.skill_version (set to this contract version).**

Breaking these invariants triggers protocol violation signals that degrade your reliability score.

## What Requires Prescribe/Report

### Mutate operations (MUST use protocol)

| Tool | Mutating operations |
|------|-------------------|
| kubectl | apply, delete, patch, create, replace, rollout restart |
| helm | install, upgrade, uninstall, rollback |
| terraform | apply, destroy, import |
| docker/podman/nerdctl | run, create, rm, stop, kill, build, push |
| docker-compose/compose | up, down, start, stop, build |
| kustomize | (via kubectl apply) |
| oc (OpenShift) | same as kubectl |

Contract classification:

Mutation examples (must use prescribe/report):
- kubectl apply/delete/patch/create/replace/rollout restart
- helm install/upgrade/uninstall/rollback
- terraform apply/destroy/import

Read-only examples (skip protocol):
- kubectl get/describe/logs/top/events
- helm list/status/template
- terraform plan/show/output

**When unsure whether a command mutates state, call `prescribe`.** It's always safe to prescribe — the cost is one extra call. The cost of skipping is a protocol violation signal.

## Calling Prescribe

Record intent BEFORE an infrastructure operation that creates, modifies, or deletes resources.

### Required fields

```json
{
  "tool": "kubectl",
  "operation": "apply",
  "raw_artifact": "<the full YAML/JSON/command content>",
  "actor": {
    "type": "agent",
    "id": "your-agent-id",
    "origin": "mcp-stdio",
    "skill_version": "v1.0.1"
  }
}
```

Required inputs:
- **tool**
- **operation**
- **raw_artifact**
- **actor (type, id, origin)**

### Pre-call checklist
- tool/operation/raw_artifact must be non-empty.
- actor.type/actor.id/actor.origin must be present.
- actor.skill_version should be set to this contract version.
- Keep session/operation/trace identifiers stable within one task.

### Optional but recommended fields

```json
{
  "session_id": "stable-session-id",
  "operation_id": "unique-per-operation",
  "environment": "production",
  "scope_dimensions": {
    "cluster": "prod-us-east-1",
    "namespace": "default"
  }
}
```

Keep `session_id` stable within one task. Use `scope_dimensions` to provide cluster/namespace/account/region context.

### What prescribe returns

```json
{
  "ok": true,
  "prescription_id": "01ABC...",
  "risk_inputs": [
    {
      "source": "evidra/native",
      "risk_level": "high",
      "risk_tags": ["k8s.privileged_container"]
    }
  ],
  "effective_risk": "high",
  "artifact_digest": "sha256:...",
  "intent_digest": "sha256:...",
  "operation_class": "mutate",
  "scope_class": "production",
  "resource_count": 1
}
```

Returns:
- prescription_id (required for report)
- risk_inputs, effective_risk
- artifact_digest, intent_digest
- resource_shape_hash, operation_class, scope_class

### If prescribe fails

**CRITICAL: If prescribe returns ok=false, do not execute the infrastructure command. Investigate the error before retrying.**

The error codes are:
- `parse_error` — the artifact couldn't be parsed (check format)
- `invalid_input` — required fields are missing
- `internal_error` — server-side issue

## Executing the Command

After prescribe returns `ok=true`, execute the infrastructure command normally. Capture:
- The **exit code** (0 for success, non-zero for failure)
- Whether any error occurred

## Calling Report

Record the terminal verdict AFTER an infrastructure operation completes or is intentionally declined.

### Success

```json
{
  "prescription_id": "01ABC...",
  "verdict": "success",
  "exit_code": 0,
  "actor": {
    "type": "agent",
    "id": "your-agent-id",
    "origin": "mcp-stdio",
    "skill_version": "v1.0.1"
  }
}
```

### Failure

```json
{
  "prescription_id": "01ABC...",
  "verdict": "failure",
  "exit_code": 1,
  "actor": { "type": "agent", "id": "your-agent-id", "origin": "mcp-stdio", "skill_version": "v1.0.1" }
}
```

Always report failures with the actual non-zero exit code. Hiding failures triggers artifact drift and protocol violation signals.

### Declined (intentional refusal)

When you decide **not** to execute after prescribing (e.g., risk is too high):

```json
{
  "prescription_id": "01ABC...",
  "verdict": "declined",
  "decision_context": {
    "trigger": "risk_threshold_exceeded",
    "reason": "Critical risk: privileged container in production"
  },
  "actor": { "type": "agent", "id": "your-agent-id", "origin": "mcp-stdio", "skill_version": "v1.0.1" }
}
```

Note: `exit_code` is **forbidden** for declined verdicts. Use `decision_context` instead.

### Terminal outcome rule
- Every prescribe must end with exactly one report, including failed, errored, aborted, or declined attempts.
- Retries require a new prescribe/report pair for each attempt.

### Rules
- Always report failures; do not hide non-zero exit codes.
- Always report deliberate refusals with a concise operational reason.
- Do not report twice for the same prescription_id.
- Do not report another actor's prescription_id.
- If prescription_id is lost, call prescribe again before execution.
- Actor identity should match the original prescribe actor.
- Include actor.skill_version for behavior slicing.
- exit_code is required for success/failure/error verdicts and forbidden for declined verdicts.
- On retry, call prescribe again to get a new prescription_id before re-executing. Each attempt is a separate prescribe/report pair.

### What report returns

```json
{
  "ok": true,
  "report_id": "01DEF...",
  "verdict": "success",
  "score": 95.5,
  "score_band": "excellent",
  "signal_summary": {
    "protocol_violation": 0,
    "artifact_drift": 0,
    "retry_loop": 0,
    "blast_radius": 0,
    "new_scope": 0,
    "repair_loop": 0,
    "thrashing": 0,
    "risk_escalation": 0
  }
}
```

The `score` (0-100) reflects your operational reliability within the current session. Non-zero signals indicate behavioral patterns that reduce trust.

## Retry Handling

When retrying a failed operation:
1. Call `prescribe` again (new prescription_id for each attempt)
2. Execute the command
3. Call `report` with the new prescription_id

Each attempt gets its own prescribe/report pair. Do **not** reuse a previous prescription_id.

## Risk Tags Reference

Prescribe may return risk tags indicating specific concerns detected in the artifact:

### Kubernetes
| Tag | Severity | What it means |
|-----|----------|---------------|
| `k8s.privileged_container` | critical | Container runs with privileged=true |
| `k8s.hostpath_mount` | high | Pod mounts host filesystem path |
| `k8s.run_as_root` | high | Container runs as UID 0 |
| `k8s.host_namespace_escape` | high | Pod uses hostNetwork/hostPID/hostIPC |
| `k8s.docker_socket` | high | Pod mounts /var/run/docker.sock |
| `k8s.dangerous_capabilities` | high | Unsafe Linux capabilities (SYS_ADMIN, NET_ADMIN) |
| `k8s.cluster_admin_binding` | critical | ClusterRoleBinding grants cluster-admin |
| `k8s.writable_rootfs` | high | Root filesystem is writable |
| `ops.kube_system` | high | Targets kube-system namespace |
| `ops.namespace_delete` | high | Deleting a namespace |

### Docker/Compose
| Tag | Severity | What it means |
|-----|----------|---------------|
| `docker.privileged` | critical | Service runs with privileged=true |
| `docker.host_network` | high | Service uses host network mode |
| `docker.socket_mount` | high | Service mounts Docker socket |

### Terraform (AWS)
| Tag | Severity | What it means |
|-----|----------|---------------|
| `aws_iam.wildcard_policy` | critical | IAM policy grants Action:* and Resource:* |
| `terraform.iam_wildcard_policy` | high | IAM policy uses wildcard action or resource |
| `terraform.s3_public_access` | high | S3 bucket missing public access block |
| `aws.rds_public` | high | RDS instance publicly accessible |
| `aws.ebs_unencrypted` | high | EBS volume without encryption |
| `aws.security_group_open` | high | Security group allows unrestricted ingress |

### Operations (cross-tool)
| Tag | Severity | What it means |
|-----|----------|---------------|
| `ops.mass_delete` | critical | Deleting more than 10 resources at once |

## Behavioral Signals

Evidra monitors eight behavioral signals. Understanding them helps you maintain a high reliability score:

- **protocol_violation** — Missing report for a prescribe, or executing without prescribing
- **artifact_drift** — The artifact you actually applied differs from what you prescribed
- **retry_loop** — Same intent retried multiple times (may indicate a loop)
- **blast_radius** — Large number of resources affected (>5 in non-production)
- **new_scope** — First operation in a production scope
- **repair_loop** — Repeated failure-then-fix cycles in a session
- **thrashing** — Rapid create/delete oscillation on the same resources
- **risk_escalation** — Operations exceeding the originally prescribed risk level

## Setup

### MCP Configuration (Claude Code / Claude Desktop)

Add to your MCP config (e.g., `~/.claude.json` or Claude Desktop settings):

```json
{
  "mcpServers": {
    "evidra": {
      "command": "evidra-mcp",
      "args": ["--evidence-dir", "~/.evidra/evidence", "--environment", "production"]
    }
  }
}
```

For development with optional signing:
```json
{
  "mcpServers": {
    "evidra": {
      "command": "evidra-mcp",
      "args": ["--evidence-dir", "~/.evidra/evidence", "--signing-mode", "optional"]
    }
  }
}
```

### Docker

```bash
docker run -v ~/.evidra/evidence:/evidence \
  -e EVIDRA_EVIDENCE_DIR=/evidence \
  -e EVIDRA_ENVIRONMENT=production \
  evidra-mcp
```

### Environment Variables

| Variable | Purpose | Default |
|----------|---------|---------|
| `EVIDRA_EVIDENCE_DIR` | Evidence storage path | `~/.evidra/evidence` |
| `EVIDRA_ENVIRONMENT` | Environment label | (none) |
| `EVIDRA_SIGNING_KEY` | Base64 Ed25519 private key | (none) |
| `EVIDRA_SIGNING_KEY_PATH` | Path to signing key file | (none) |
| `EVIDRA_SIGNING_MODE` | `strict` or `optional` | `strict` |
| `EVIDRA_RETRY_TRACKER` | Enable retry loop detection | `false` |
| `EVIDRA_URL` | API URL for online mode | (offline) |
| `EVIDRA_API_KEY` | API authentication key | (none) |

## Quick Decision Flowchart

```
Is this an infrastructure command?
├─ No  → Just execute it. No protocol needed.
└─ Yes → Does it mutate state?
         ├─ No  (get/describe/logs/plan/show) → Execute directly.
         ├─ Yes (apply/delete/create/destroy)  → prescribe → execute → report
         └─ Unsure → prescribe → execute → report (safe default)
```
