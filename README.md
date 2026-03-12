# Evidra

[![CI](https://github.com/vitas/evidra/actions/workflows/ci.yml/badge.svg?branch=main)](https://github.com/vitas/evidra/actions/workflows/ci.yml)
[![Release Pipeline](https://github.com/vitas/evidra/actions/workflows/release.yml/badge.svg?event=push)](https://github.com/vitas/evidra/actions/workflows/release.yml)
[![License](https://img.shields.io/badge/License-Apache_2.0-blue.svg)](https://opensource.org/licenses/Apache-2.0)

**Evidra — Flight recorder for Infrastructure Automation**<br>
**A new observability layer for CI/CD, IaC, and AI agents.**

Evidra records what your automation intended and what actually happened.
For AI agents, it also records what they decided not to do.
From this evidence, Evidra computes behavioral signals that answer: is this actor operating reliably?

Infrastructure automation will not become trustworthy because agents stop making mistakes.
It will become trustworthy because operations are recorded, decisions are explainable, and risky behavior patterns become visible before the next outage.

## What Evidra Is

Evidra is the evidence, signal, and scoring layer for infrastructure automation. It captures intent before execution, records outcomes or explicit declines after execution, stores that lifecycle in a tamper-evident append-only evidence chain, and turns the resulting history into behavioral signals and reliability scorecards.

CLI and MCP are the authoritative analytics surfaces today.

It is one platform with three operating surfaces:

| Surface | What it does |
|---|---|
| `evidra` CLI | Wraps live commands, imports completed operations, and computes local scorecards |
| `evidra-mcp` | Exposes the `prescribe` / `report` lifecycle to MCP-connected automation runtimes |
| Self-hosted API | Centralizes evidence, webhooks, analytics, and dashboarding across teams |

What teams get from Evidra:

- a stable `prescribe` / `report` protocol for infrastructure actions
- risk tags and risk levels at operation time
- behavioral signals such as retry loops, protocol violations, blast radius, and artifact drift
- scorecards for comparing reliability across actors, sessions, and time windows

## How It Works

```text
CLI / MCP / CI / Webhooks
        -> prescribe
        -> execute or decline
        -> report
        -> evidence chain
        -> signals engine
        -> scorecard
```

1. `prescribe` records the intended action, canonical form, digests, and immediate risk classification.
2. `report` records the terminal outcome or an intentional refusal with structured context.
3. Evidence is stored in an append-only chain.
4. The signals engine detects behavior patterns across that chain.
5. Scorecards convert those patterns into a reliability score, band, and confidence level.

Signals and scoring details:

- [Signal Specification](docs/system-design/EVIDRA_SIGNAL_SPEC_V1.md)
- [Scoring Rationale](docs/system-design/scoring/default.v1.1.0.md)

## Where It Fits

Evidra is useful anywhere infrastructure changes need evidence and behavioral accountability:

- CI/CD pipelines that already run `kubectl`, `helm`, or `terraform`
- GitOps and import-based workflows that want ingestion without wrapping execution
- MCP-connected automation and AI-assisted tooling that need explicit before/after lifecycle recording
- platform teams that want centralized evidence, analytics, and actor comparison across environments

## Fastest Path For DevOps

### Install

```bash
# Homebrew
brew install samebits/tap/evidra

# Binary release (Linux/macOS)
curl -fsSL https://github.com/samebits/evidra/releases/latest/download/evidra_$(uname -s | tr '[:upper:]' '[:lower:]')_$(uname -m | sed 's/x86_64/amd64/;s/aarch64/arm64/').tar.gz \
  | tar -xz -C /usr/local/bin evidra

# Build from source
make build
```

### Record One Operation

```bash
evidra keygen
export EVIDRA_SIGNING_KEY=<base64>

evidra record -f deploy.yaml -- kubectl apply -f deploy.yaml
```

### See The Score Context

```bash
evidra scorecard --period 30d
```

The `record` output includes the first useful fields:

- `risk_level`
- `score`
- `score_band`
- `signal_summary`
- `basis` (`preview` vs `sufficient`)
- `confidence`

Security boundary: `evidra record` executes the wrapped local command directly.
Evidra does not sandbox the wrapped command. Treat it with the same trust model
as direct shell execution; Evidra records evidence around the command, but it
does not contain or block it.

## Integration Surfaces

### Local CLI

Two primary CLI modes feed the same lifecycle and scoring engine:

- `evidra record` executes a command live and records the full lifecycle
- `evidra import` ingests a completed operation from structured input

```bash
evidra import --input record.json
```

Core commands:

| Command | Purpose |
|---|---|
| `record` | Execute command live and record lifecycle outcome |
| `import` | Ingest completed operation payload |
| `scorecard` | Compute reliability scorecard |
| `explain` | Show signal-level breakdown |
| `prescribe` | Record pre-execution intent |
| `report` | Record post-execution outcome |
| `validate` | Verify evidence chain and signatures |
| `import-findings` | Ingest SARIF findings |
| `compare` | Compare actor reliability |
| `keygen` | Generate Ed25519 signing keypair |

References:

- [CLI Reference](docs/integrations/cli-reference.md)
- [V1 Record/Import Contract](docs/system-design/EVIDRA_RUN_RECORD_CONTRACT_V1.md)

### MCP Service

Evidra speaks MCP. The MCP service is the same flight recorder lifecycle exposed to automation runtimes: the actor reports what it intended to do before execution and what it actually did or intentionally declined to do afterward.

```bash
evidra-mcp --evidence-dir ~/.evidra/evidence
```

References:

- [MCP server schemas](pkg/mcpserver/schemas/)
- [MCP contract prompts](docs/system-design/MCP_CONTRACT_PROMPTS.md)
- [MCP Registry Publication Guide](docs/guides/mcp-registry-publication.md)

### Self-Hosted API And Webhooks

Run the Evidra backend when you want centralized evidence collection, webhook ingestion, team-wide analytics, and the embedded UI.

```bash
export EVIDRA_API_KEY=my-secret-key
export EVIDRA_INVITE_SECRET=my-invite-secret
docker compose up --build -d
curl http://localhost:8080/healthz
```

The CLI can forward evidence to the backend:

```bash
evidra record --url http://localhost:8080 --api-key my-secret-key -f deploy.yaml -- kubectl apply -f deploy.yaml
```

References:

- [Self-Hosted Setup Guide](docs/guides/self-hosted-setup.md)
- [API Reference](docs/api-reference.md)
- [Setup Evidra Action](docs/guides/setup-evidra-action.md)
- [Terraform CI Quickstart](docs/guides/terraform-ci-quickstart.md)

## Supported Tools

| Adapter | Tools | Artifact |
|---|---|---|
| **k8s/v1** | kubectl, helm, kustomize, oc (OpenShift) | YAML manifests |
| **terraform/v1** | terraform | Plan JSON (`terraform show -json`) |
| **docker/v1** | docker | Container inspect JSON |
| **generic/v1** | Any (fallback) | Raw bytes — use `--canonical-action` for structured tools |

Full details:

- [Supported Tools](docs/supported-tools.md)

## Docs Map

Architecture and contracts:

- [V1 Architecture](docs/system-design/EVIDRA_ARCHITECTURE_V1.md)
- [Protocol](docs/system-design/EVIDRA_PROTOCOL_V1.md)
- [Core Data Model](docs/system-design/EVIDRA_CORE_DATA_MODEL_V1.md)
- [Canonicalization Contract](docs/system-design/EVIDRA_CANONICALIZATION_CONTRACT_V1.md)
- [Signal Spec](docs/system-design/EVIDRA_SIGNAL_SPEC_V1.md)
- [Scoring Rationale](docs/system-design/scoring/default.v1.1.0.md)

Integration and operations:

- [API Reference](docs/api-reference.md)
- [CLI Reference](docs/integrations/cli-reference.md)
- [Supported Tools](docs/supported-tools.md)
- [Observability Quickstart](docs/guides/observability-quickstart.md)
- [Scanner SARIF Quickstart](docs/integrations/scanner-sarif-quickstart.md)
- [Self-Hosted Setup Guide](docs/guides/self-hosted-setup.md)

Developer references:

- [Architecture Overview](docs/ARCHITECTURE.md)
- [E2E Testing Map](docs/E2E_TESTING.md)
- [Tests Index](docs/tests-index.md)
- [Shared Artifact Fixtures](tests/artifacts/fixtures/README.md)

## Development

```bash
make build
make test
make e2e
make test-contracts
make lint
make test-signals
```

## License

Licensed under the [Apache License 2.0](LICENSE).
