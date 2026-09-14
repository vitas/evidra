# Evidra

Evidra is an open-source MCP execution-evidence recorder. It wraps one upstream
MCP server and records what an agent declared, what the proxy observed, and what
the agent reported.

[![CI](https://github.com/vitas/evidra/actions/workflows/ci.yml/badge.svg?branch=main)](https://github.com/vitas/evidra/actions/workflows/ci.yml)
[![Release Pipeline](https://github.com/vitas/evidra/actions/workflows/release.yml/badge.svg?event=push)](https://github.com/vitas/evidra/actions/workflows/release.yml)
[![License](https://img.shields.io/badge/License-Apache_2.0-blue.svg)](LICENSE)

```text
Agent / MCP client -> Evidra MCP endpoint -> upstream MCP server
                              |
                              v
                  signed local MCP execution evidence
```

## Why Evidra

An MCP response alone does not preserve the difference between intent, activity,
and the agent's account of the result. Evidra keeps those sources separate:

- **Declared** — the objective the agent records before it calls an upstream
  operational tool.
- **Observed** — the request and response that pass through the Evidra MCP
  boundary.
- **Reported** — the terminal status and explanation the agent records when it
  closes the operation.

Evidra reconciles these records without treating any one of them as external
truth. A proxy observation shows traffic at the MCP boundary; an agent declaration
or report remains an agent-provided claim.

## Quick start

Building from source requires Go 1.26 or later.

```bash
make build
go build -o bin/evidra-fixture ./cmd/evidra-fixture
mkdir -p evidence

./bin/evidra-mcp --proxy \
  --evidence-dir ./evidence \
  --server-name fixture \
  -- ./bin/evidra-fixture
```

The command starts a single stdio MCP endpoint and waits for JSON-RPC input. In
normal use, an MCP client owns this process and supplies the configuration. See
[Getting started](docs/getting-started.md) for a complete client configuration
and an end-to-end operation.

## Read the evidence

Each endpoint process creates a recorder directory beneath the evidence root.
Summarize all recorder directories or verify their chains with the reader CLI:

```bash
./bin/evidra summarize --dir ./evidence
./bin/evidra verify --dir ./evidence
```

The summary keeps declarations, observations, and reports distinct. Verification
checks recorder stores rather than deciding whether the operation achieved its
real-world goal.

## What Evidra verifies

Evidra distinguishes several different statements:

- **Chain integrity** checks that stored records remain in their signed,
  hash-linked order.
- **Signature validity** checks records against the public key stored with that
  recorder. A local recorder key does not independently establish an
  organizational identity.
- **Evidence coverage** checks whether expected recorder events exist, including
  paired execution boundaries.
- **Proxy observations** attest to requests and responses visible at the Evidra
  MCP boundary, not to side effects beyond it.
- **Agent declarations and reports** are preserved as attributed claims; their
  presence does not make them independently true.

Evidra does not independently prove that the external system reached the agent's
intended state. It records and reconciles claims and observations available at the
MCP boundary.

## Current scope

- One stdio upstream MCP server per Evidra endpoint process.
- Protocol enforcement with `--enforce=all` (the default), or observe-only
  operation with `--enforce=off`.
- Local evidence directories, with one recorder directory per process.
- MCP protocol versions `2025-06-18`, `2025-03-26`, and `2024-11-05`.
- No required hosted service.
- No built-in operational tool catalogue; upstream tools are exposed unchanged
  alongside `evidra_prescribe` and `evidra_report`.

## Documentation

- [Getting started](docs/getting-started.md)
- [CLI reference](docs/cli-reference.md)
- [Architecture](docs/architecture.md)
- [Evidence format and trust model](docs/evidence-format.md)
- [Validation status and limitations](docs/validation.md)

After the Core documentation, the separate
[Evidra Bench](https://github.com/vitas/evidra-bench) project provides benchmark
workflows built around reproducible evidence.

## Contributing and security

See [CONTRIBUTING.md](CONTRIBUTING.md) for the development workflow and
[SECURITY.md](SECURITY.md) for the vulnerability-reporting process and security
scope.

Evidra is licensed under the [Apache License 2.0](LICENSE).
