<!-- contract: v1.4.0 -->
# prescribe_smart — Lightweight Intent Recording

Record intent BEFORE an infrastructure mutation when you know the target resource but don't have artifact bytes.

## Template

```json
{
  "tool": "kubectl",
  "operation": "apply",
  "resource": "deployment/web",
  "namespace": "default",
  "actor": {
    "type": "agent",
    "id": "your-agent-id",
    "origin": "mcp-stdio",
    "skill_version": "1.4.0"
  },
  "session_id": "stable-per-task",
  "environment": "production",
  "scope_dimensions": {
    "cluster": "prod-us-east-1",
    "namespace": "default"
  }
}
```

## Required Fields

| Field | Description |
|-------|-------------|
| `tool` | Infrastructure tool: kubectl, helm, terraform |
| `operation` | Action: apply, delete, patch, install, upgrade, destroy |
| `resource` | Target: deployment/web, service/api, namespace/prod |
| `actor.type` | Always "agent" |
| `actor.id` | Your agent identifier |
| `actor.origin` | How you connect: "mcp-stdio" |
| `actor.skill_version` | Contract version |

## Optional Fields

| Field | Description |
|-------|-------------|
| `namespace` | Kubernetes namespace (when applicable) |
| `session_id` | Keep stable within one task |
| `environment` | production, staging, development |
| `scope_dimensions` | cluster, namespace, account, region context |
| `operation_id` | Unique per operation for correlation |

## Response

Returns `prescription_id` (required for report), `effective_risk`, and `risk_inputs` with detected risk tags.

## When to Use

Use prescribe_smart for: kubectl apply/delete/patch, helm install/upgrade, terraform apply/destroy. Skip for read-only: kubectl get/describe/logs, helm list/status, terraform plan/show.
