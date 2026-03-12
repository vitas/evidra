# MCP Registry Publication

- Status: Guide
- Version: current
- Canonical for: MCP registry release workflow
- Audience: public

Evidra is published to two MCP registries:

- **Docker MCP Registry** for Docker Desktop and Docker MCP Toolkit users
- **MCP Registry** (`registry.modelcontextprotocol.io`) for the official `server.json`-based directory

The goal is to keep both entries aligned to the same current MCP server:

- binary: `evidra-mcp`
- package identity: `io.github.vitas/evidra`
- OCI image: `ghcr.io/vitas/evidra-mcp:<version>`
- tools: `prescribe`, `report`, `get_event`

This guide is both a user guide and a maintainer runbook. It explains what users install from each registry and what maintainers must update to keep both publications current.

## Overview

| Registry | User-facing install surface | Source of truth |
|---|---|---|
| Docker MCP Registry | Docker Desktop MCP catalog / Docker MCP Toolkit | `docker/mcp-registry` fork entry under `servers/evidra/` |
| MCP Registry | `io.github.vitas/evidra` in the official MCP registry | This repo's [`server.json`](../../server.json) |

Both registries should describe the same runtime behavior:

- Evidra is a **flight recorder for AI infrastructure agents**
- it speaks **MCP over stdio**
- it is **local-first** by default
- it can optionally **forward evidence to a self-hosted Evidra API** via `EVIDRA_URL` and `EVIDRA_API_KEY`
- it does **not** act as a fail-closed policy gate or advertise legacy `validate` semantics

## For Users

No matter which registry you install from, the MCP server is the same product: `evidra-mcp`.

The server exposes three MCP tools:

- `prescribe` records intent before an infrastructure mutation and returns the prescription context
- `report` records the terminal outcome or deliberate refusal for that prescription
- `get_event` looks up a previously recorded evidence event

Default behavior is local:

- evidence is written locally
- signing is controlled by `EVIDRA_SIGNING_MODE`, `EVIDRA_SIGNING_KEY`, and `EVIDRA_SIGNING_KEY_PATH`
- environment labeling uses `EVIDRA_ENVIRONMENT`

Optional hosted mode is available when the client lets you pass environment variables or CLI flags:

- `EVIDRA_URL`
- `EVIDRA_API_KEY`

That keeps the MCP runtime local-first while still allowing forwarding into the self-hosted API for centralized evidence and tenant-wide analytics.
If a registry client only handles installation, set those values in the client's post-install MCP configuration.

For full setup examples after installation, see [MCP Setup & Usage Guide](./mcp-setup.md).

## Docker MCP Registry

The Docker MCP Registry entry is **not** maintained in this repo. Its authoritative files live in your `docker/mcp-registry` fork under `servers/evidra/`.

Expected files:

- `server.yaml`
- `tools.json`
- `readme.md`

### What users get

Users discover Evidra through Docker Desktop's MCP catalog and install it as a containerized stdio MCP server.

That entry should describe the current Evidra MCP contract:

- product name: `Evidra`
- repo: `https://github.com/vitas/evidra`
- category: DevOps / infrastructure automation
- tools: `prescribe`, `report`, `get_event`
- positioning: local-first flight recorder for AI agents touching infrastructure

### What maintainers update

Keep the fork entry aligned to the current repo and release:

- `server.yaml`
  - title, description, repository, tags, and source commit
  - runtime env vars users may need
- `tools.json`
  - static list of `prescribe`, `report`, and `get_event`
  - use this to avoid catalog build-time tool discovery failures
- `readme.md`
  - short catalog description
  - link back to the repo and [MCP Setup & Usage Guide](./mcp-setup.md)

Keep the Docker registry entry in current product language:

- use the current Evidra name and repository
- describe the MCP server as a local-first flight recorder
- publish `prescribe`, `report`, and `get_event`
- avoid policy-gate, blocking, or cache-specific legacy copy

### Validation flow

The current local Docker MCP Registry workflow in the fork is:

```bash
cd ~/git/mcp-registry
task validate -- --name evidra
task build -- --tools evidra
```

Those commands are documented in the fork's `CONTRIBUTING.md` and `add_mcp_server.md`.

### Update flow

1. Cut and push the Evidra release in this repo first.
2. Confirm the matching `ghcr.io/vitas/evidra-mcp:<version>` image exists and starts cleanly.
3. Update `servers/evidra/server.yaml`, `tools.json`, and `readme.md` in the `docker/mcp-registry` fork.
4. Run `task validate -- --name evidra`.
5. Run `task build -- --tools evidra`.
6. Open the PR to `docker/mcp-registry`.

## MCP Registry

The official MCP registry entry is driven directly from this repo's [`server.json`](../../server.json). There is no second metadata repo for this path.

Current identity:

- name: `io.github.vitas/evidra`
- title: `Evidra`
- package: `ghcr.io/vitas/evidra-mcp:<version>`
- transport: stdio

The container ownership label is set in [`Dockerfile`](../../Dockerfile):

```dockerfile
LABEL io.modelcontextprotocol.server.name="io.github.vitas/evidra"
```

### What users get

Clients that support the official MCP registry can discover `io.github.vitas/evidra` and install the OCI-backed stdio server described by `server.json`.

This path should stay aligned with the real runtime:

- same OCI package tag as the release
- same tool set: `prescribe`, `report`, `get_event`
- same local-first behavior with optional forwarding to a hosted Evidra API

### What maintainers update

For this registry, the source of truth is already in this repo:

- [`server.json`](../../server.json)
- [`Dockerfile`](../../Dockerfile)
- [`scripts/bump-version.sh`](../../scripts/bump-version.sh)

Version bumps should be done with:

```bash
scripts/bump-version.sh 0.4.7
```

That updates the authoritative release surfaces in this repo, including `server.json`.

### Validation and publish flow

```bash
# Validate the current manifest
mcp-publisher validate server.json

# Authenticate once
mcp-publisher login

# Publish the current server.json
mcp-publisher publish server.json
```

Verify the published entry:

```bash
curl -s "https://registry.modelcontextprotocol.io/v0.1/servers?search=io.github.vitas/evidra" | jq .
```

## Release Checklist

Use this checklist whenever a release should update both registries.

1. Bump the release version in this repo with [`scripts/bump-version.sh`](../../scripts/bump-version.sh).
2. Confirm [`server.json`](../../server.json) points to the new `ghcr.io/vitas/evidra-mcp:<version>` tag.
3. Push the release tag and confirm the GHCR image is available.
4. Validate the MCP Registry manifest with `mcp-publisher validate server.json`.
5. Publish `server.json` with `mcp-publisher publish server.json`.
6. Update the `docker/mcp-registry` fork entry under `servers/evidra/`.
7. Re-run `task validate -- --name evidra` and `task build -- --tools evidra` in the fork.
8. Verify both registries now describe the same product and the same three MCP tools.

## Alignment Rules

Whenever you update either registry entry, keep these invariants true:

- `Evidra` is the product name
- `io.github.vitas/evidra` is the MCP Registry identity
- `ghcr.io/vitas/evidra-mcp:<version>` is the official OCI package
- `prescribe`, `report`, and `get_event` are the published MCP tools
- local-first evidence recording remains the default
- hosted forwarding is optional, not the primary operating mode
