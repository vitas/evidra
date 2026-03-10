# Self-Hosted Experimental Status

This document defines the current support boundary for `evidra-api`.

CLI and MCP are the authoritative analytics surfaces today. Self-hosted remains supported for centralized evidence collection, but hosted analytics are still experimental until they reuse the same engine end to end.

## Supported Today

- Evidence ingestion: `/v1/evidence/forward`, `/v1/evidence/batch`, `/v1/evidence/findings`
- Evidence browsing: `/v1/evidence/entries`, `/v1/evidence/entries/{id}`
- Health/readiness, public key, and key issuance endpoints
- CLI/MCP forwarding into centralized storage via `--url` / `EVIDRA_URL`
- Forwarded evidence may include explicit `report.verdict` values, including `declined` with decision context

## Experimental / Not Implemented

- `/v1/evidence/scorecard`
- `/v1/evidence/explain`

These endpoints currently return `501 Not Implemented` with an experimental status message. They are present so the self-hosted surface stays visible, but they are not authoritative analytics yet.

## Authoritative Analytics Path

- `evidra scorecard`
- `evidra explain`
- `evidra run`
- `evidra record`
- `evidra report`
- `evidra-mcp report`

## Practical Guidance

- Use self-hosted when you want centralized evidence collection, API keys, and entry browsing.
- Use CLI or MCP outputs when you need scorecards, explanations, or immediate assessment snapshots for executed or declined decisions.
- Do not treat self-hosted `/v1/evidence/scorecard` or `/v1/evidence/explain` as feature-complete until this page changes.
