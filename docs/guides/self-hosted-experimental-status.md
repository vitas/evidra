# Self-Hosted Experimental Status

This document defines the current support boundary for `evidra-api`.

CLI and MCP remain the primary local and agent-facing analytics surfaces. Self-hosted now exposes the same scorecard and explain engine over centralized stored evidence.

## Supported Today

- Evidence ingestion: `/v1/evidence/forward`, `/v1/evidence/batch`, `/v1/evidence/findings`
- Evidence browsing: `/v1/evidence/entries`, `/v1/evidence/entries/{id}`
- Tenant-wide analytics: `/v1/evidence/scorecard`, `/v1/evidence/explain`
- Webhook receivers: `/v1/hooks/argocd`, `/v1/hooks/generic`
- Health/readiness, public key, and key issuance endpoints
- CLI/MCP forwarding into centralized storage via `--url` / `EVIDRA_URL`
- Forwarded evidence may include explicit `report.verdict` values, including `declined` with decision context

## Analytics Contract

- Default scope is tenant-wide over stored evidence.
- Optional narrowing filters:
  - `actor`
  - `tool`
  - `scope`
  - `session_id`
  - `period`
  - `min_operations` on scorecard and explain
- Webhook mapping support:
  - ArgoCD `sync_started` / `sync_completed`
  - Generic `operation_started` / `operation_completed`
- Hosted `compare` is not part of this contract yet.

## Authoritative Analytics Path

- `evidra scorecard`
- `evidra explain`
- `evidra record`
- `evidra import`
- `evidra report`
- `evidra-mcp report`

## Practical Guidance

- Use self-hosted when you want centralized evidence collection, API keys, entry browsing, and tenant-wide analytics over forwarded evidence.
- Use CLI or MCP when you want local-first workflows, immediate command assessment, or agent-native tool invocation.
- Treat hosted `scorecard` and `explain` as the supported analytics surface for stored evidence. Hosted `compare` is still future work.
