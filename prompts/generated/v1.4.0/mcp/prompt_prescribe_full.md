<!-- contract: v1.4.0 -->
# prescribe_full — Full Artifact Intent Recording

Record intent BEFORE an infrastructure mutation when you have the actual manifest/artifact bytes. Enables drift detection and native detector coverage.

## Template

```json
{
  "tool": "kubectl",
  "operation": "apply",
  "raw_artifact": "apiVersion: apps/v1\nkind: Deployment\nmetadata:\n  name: web\n  namespace: default\nspec:\n  replicas: 3\n  ...",
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
| `raw_artifact` | Full YAML/JSON/HCL content being applied |
| `actor.type` | Always "agent" |
| `actor.id` | Your agent identifier |
| `actor.origin` | How you connect: "mcp-stdio" |
| `actor.skill_version` | Contract version |

## Response

Returns `prescription_id`, `effective_risk`, `risk_inputs` with detected tags, `artifact_digest` for drift tracking, and `resource_count`.

## When to Use Over prescribe_smart

- You have the full manifest (YAML, JSON, HCL)
- You want artifact drift detection (compares what you prescribed vs what you applied)
- You want full native detector coverage (security scanning of the artifact)
