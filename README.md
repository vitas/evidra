# Evidra

[![CI](https://github.com/vitas/evidra/actions/workflows/ci.yml/badge.svg?branch=main)](https://github.com/vitas/evidra/actions/workflows/ci.yml)
[![Release Pipeline](https://github.com/vitas/evidra/actions/workflows/release.yml/badge.svg?event=push)](https://github.com/vitas/evidra/actions/workflows/release.yml)
[![License](https://img.shields.io/badge/License-Apache_2.0-blue.svg)](https://opensource.org/licenses/Apache-2.0)

**Evidra — Flight recorder for Infrastructure Automation**
A new observability layer for CI/CD, IaC, and AI agents.

Evidra records what your automation intended and what actually happened. 
For AI agents, it also records what they decided not to do. 
From this evidence, Evidra computes behavioral signals that answer: is this actor operating reliably?

Aviation became the safest form of transport not because planes stopped failing, but because every flight is recorded, every incident is investigated, and patterns become visible before the next disaster.
AI agents are getting access to production infrastructure. Who is recording their flights?

## How To Use

Two primary operation modes:

- `evidra record` = Evidra executes and observes a command live.
- `evidra import` = Evidra ingests a completed operation from structured input.

Both modes feed the same lifecycle and scoring engine.

Security boundary: `evidra record` executes the wrapped local command directly.
Evidra does not sandbox the wrapped command. Treat it with the same trust model
as direct shell execution; Evidra records evidence around the command, but it
does not contain or block it.

### Install

```bash
# Homebrew
brew install samebits/tap/evidra

# Binary release (Linux/macOS)
curl -fsSL https://github.com/samebits/evidra/releases/latest/download/evidra_$(uname -s | tr '[:upper:]' '[:lower:]')_$(uname -m | sed 's/x86_64/amd64/;s/aarch64/arm64/').tar.gz \
  | tar -xz -C /usr/local/bin evidra

# Build from source
make build    # produces bin/evidra and bin/evidra-mcp
```

### Quick Start (10-minute path)

```bash
# 1) Generate a signing key (strict mode)
evidra keygen
export EVIDRA_SIGNING_KEY=<base64>

# 2) Record one live operation
evidra record \
  -f deploy.yaml \
  --environment staging \
  -- kubectl apply -f deploy.yaml

# 3) View score context
evidra scorecard --period 30d
```

The `record` output includes first useful fields:
- `risk_level`
- `score`
- `score_band`
- `signal_summary`
- `basis` (`preview` vs `sufficient`)
- `confidence`

### CI/CD Ingestion Path

Use `import` when pipelines already run native commands and you only want ingestion:

```bash
evidra import --input record.json
```

Contract details:
- [V1 Record/Import Contract](docs/system-design/V1_RUN_RECORD_CONTRACT.md)

## How It Works

```text
record/import -> prescribe -> report(verdict) -> signals -> scorecard
```

1. Evidra records operation intent (`prescribe`).
2. A terminal outcome or explicit refusal decision is recorded (`report`).
3. Signal engine computes behavior signals from evidence.
4. Score engine calculates reliability score + band + confidence.

Current signals are documented in:
- [Signal Specification](docs/system-design/EVIDRA_SIGNAL_SPEC.md)

## Supported Tools

| Adapter | Tools | Artifact |
|---|---|---|
| **k8s/v1** | kubectl, helm, kustomize, oc (OpenShift) | YAML manifests |
| **terraform/v1** | terraform | Plan JSON (`terraform show -json`) |
| **docker/v1** | docker | Container inspect JSON |
| **generic/v1** | Any (fallback) | Raw bytes — use `--canonical-action` for structured tools |

Full details: [Supported Tools](docs/SUPPORTED_TOOLS.md)

## Core Commands

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

Full flags and subcommands:
- [CLI Reference](docs/integrations/CLI_REFERENCE.md)
- [E2E Testing Map](docs/E2E_TESTING.md)

## MCP Integration Point

Evidra speaks MCP. The MCP server is a flight recorder for AI agents that touch
infrastructure: the agent reports what it intended to do before execution and
what it actually did or intentionally declined to do, with a bounded reason.

```bash
evidra-mcp --evidence-dir ~/.evidra/evidence
```

Details:
- [MCP server schemas](pkg/mcpserver/schemas/)
- [MCP contract prompts](docs/system-design/MCP_CONTRACT_PROMPTS.md)

## CI Integration

### GitHub Actions

```yaml
- name: Setup Evidra
  uses: samebits/evidra/.github/actions/setup-evidra@main
```

### Generic CI (GitLab, Jenkins, CircleCI, etc.)

```bash
curl -fsSL https://github.com/samebits/evidra/releases/latest/download/evidra_$(uname -s | tr '[:upper:]' '[:lower:]')_$(uname -m | sed 's/x86_64/amd64/;s/aarch64/arm64/').tar.gz \
  | tar -xz -C /usr/local/bin evidra
```

Guides:
- [Setup Evidra Action](docs/guides/setup-evidra-action.md)
- [Terraform CI Quickstart](docs/guides/terraform-ci-quickstart.md)

## API Backend (Self-Hosted)

Run the Evidra API backend with Docker Compose for centralized evidence collection.

Self-hosted supports evidence ingestion, key issuance, and entry browsing today. `/v1/evidence/scorecard` and `/v1/evidence/explain` are experimental and currently return `501 Not Implemented`; use CLI/MCP for authoritative analytics. See [Self-Hosted Experimental Status](docs/guides/self-hosted-experimental-status.md).

### Docker Compose Quickstart

```bash
export EVIDRA_API_KEY=my-secret-key
docker compose up --build -d
curl http://localhost:8080/healthz
```

### Environment Variables

| Variable | Purpose | Default |
|---|---|---|
| `DATABASE_URL` | PostgreSQL connection string | *(required)* |
| `EVIDRA_API_KEY` | API key for authenticated endpoints | *(required)* |
| `LISTEN_ADDR` | HTTP listen address | `:8080` |
| `EVIDRA_SIGNING_KEY` | Base64 Ed25519 private key for signing | *(optional)* |
| `EVIDRA_SIGNING_KEY_PATH` | Path to PEM Ed25519 private key | *(optional)* |
| `EVIDRA_SIGNING_MODE` | `strict` or `optional` | `strict` |
| `EVIDRA_INVITE_SECRET` | Secret for key issuance endpoint | *(optional)* |

### Online Mode (CLI)

Point the CLI at the API backend to forward evidence:

```bash
evidra record \
  --url http://localhost:8080 \
  --api-key my-secret-key \
  -f deploy.yaml \
  -- kubectl apply -f deploy.yaml
```

## Docs Map

Architecture and contracts:
- [Public Roadmap](docs/ROAD_MAP.md)
- [Tests Index](docs/TESTS_INDEX.md)
- [E2E Testing Map](docs/E2E_TESTING.md)
- [Shared Artifact Fixtures](tests/artifacts/fixtures/README.md)
- [Architecture Overview](docs/ARCHITECTURE.md)
- [Decision Tracking v1](docs/system-design/EVIDRA_DECISION_TRACKING_V1.md)
- [V1 Architecture](docs/system-design/V1_ARCHITECTURE.md)
- [Protocol](docs/system-design/EVIDRA_PROTOCOL.md)
- [Core Data Model](docs/system-design/EVIDRA_CORE_DATA_MODEL.md)
- [Canonicalization Contract](docs/system-design/CANONICALIZATION_CONTRACT_V1.md)
- [Signal Spec](docs/system-design/EVIDRA_SIGNAL_SPEC.md)
- [Scoring Rationale](docs/system-design/scoring/default.v1.1.0.md)

Operational guides:
- [Acceptance Fixture Status](docs/guides/acceptance-fixture-status.md)
- [Observability Quickstart](docs/guides/observability-quickstart.md)
- [Scanner SARIF Quickstart](docs/integrations/SCANNER_SARIF_QUICKSTART.md)
- [Self-Hosted Experimental Status](docs/guides/self-hosted-experimental-status.md)

## Environment

| Variable | Purpose |
|---|---|
| `EVIDRA_SIGNING_KEY` | Base64 Ed25519 private key |
| `EVIDRA_SIGNING_KEY_PATH` | PEM Ed25519 private key path |
| `EVIDRA_SIGNING_MODE` | `strict` (default) or `optional` |
| `EVIDRA_EVIDENCE_DIR` | Evidence storage directory |
| `EVIDRA_ENVIRONMENT` | Environment label (MCP server) |
| `EVIDRA_EVIDENCE_WRITE_MODE` | `strict` or `best_effort` |
| `EVIDRA_METRICS_TRANSPORT` | `none` (default) or `otlp_http` |
| `EVIDRA_METRICS_OTLP_ENDPOINT` | OTLP HTTP endpoint |
| `EVIDRA_METRICS_TIMEOUT` | Metrics export timeout (duration) |

Local smoke convenience (ephemeral signing):

```bash
export EVIDRA_SIGNING_MODE=optional
make test-mcp-inspector
```

## Build and Test

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
