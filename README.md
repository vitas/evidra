# Evidra

[![CI](https://github.com/vitas/evidra-benchmark/actions/workflows/ci.yml/badge.svg?branch=main)](https://github.com/vitas/evidra-benchmark/actions/workflows/ci.yml)
[![Release Pipeline](https://github.com/vitas/evidra-benchmark/actions/workflows/release.yml/badge.svg?event=push)](https://github.com/vitas/evidra-benchmark/actions/workflows/release.yml)
[![License](https://img.shields.io/badge/License-Apache_2.0-blue.svg)](https://opensource.org/licenses/Apache-2.0)

**Evidra is a reliability flight recorder and Benchmark for infrastructure automation.**

It measures behavior across CI/CD pipelines, IaC workflows, shell automation, and AI agents.
Evidra records intent + outcome evidence, computes reliability signals, and returns a scorecard.
It is an inspector, not an enforcement gate.

## Positioning

**Evidra — reliability benchmark for infrastructure automation (including AI).**

Evidra does not require AI to be useful.
AI agents are treated as one automation actor type, measured with the same model as CI and scripts.

## How To Use

Two primary operation modes:

- `evidra run` = Evidra executes and observes a command live.
- `evidra record` = Evidra ingests a completed operation from structured input.

Both modes feed the same lifecycle and scoring engine.

### Install

```bash
# Homebrew
brew install samebits/tap/evidra

# Binary release (Linux/macOS)
curl -fsSL https://github.com/samebits/evidra-benchmark/releases/latest/download/evidra_$(uname -s | tr '[:upper:]' '[:lower:]')_$(uname -m | sed 's/x86_64/amd64/;s/aarch64/arm64/').tar.gz \
  | tar -xz -C /usr/local/bin evidra

# Build from source
make build    # produces bin/evidra and bin/evidra-mcp
```

### Quick Start (10-minute path)

```bash
# 1) Generate a signing key (strict mode)
evidra keygen
export EVIDRA_SIGNING_KEY=<base64>

# 2) Run and capture one operation
evidra run \
  --tool kubectl \
  --operation apply \
  --artifact deploy.yaml \
  --environment staging \
  -- -- sh -c "kubectl apply -f deploy.yaml"

# 3) View score context
evidra scorecard --period 30d
```

The `run` output includes first useful fields:
- `risk_classification`
- `risk_level`
- `score` / `score_band`
- `signal_summary`
- `basis` (`preview` vs `sufficient`)

### CI/CD Ingestion Path

Use `record` when pipelines already run native commands and you only want ingestion:

```bash
evidra record --input record.json
```

Contract details:
- [V1 Run/Record Contract](docs/system-design/V1_RUN_RECORD_CONTRACT.md)

## How It Works

```text
run/record -> prescribe -> report -> signals -> scorecard
```

1. Evidra records operation intent (`prescribe`).
2. Operation outcome is recorded (`report`).
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
| `run` | Execute command live and record lifecycle outcome |
| `record` | Ingest completed operation payload |
| `scorecard` | Compute reliability scorecard |
| `explain` | Show signal-level breakdown |
| `prescribe` | Record pre-execution intent |
| `report` | Record post-execution outcome |
| `validate` | Verify evidence chain and signatures |
| `ingest-findings` | Ingest SARIF findings |
| `compare` | Compare actor reliability |
| `keygen` | Generate Ed25519 signing keypair |

Full flags and subcommands:
- [CLI Reference](docs/integrations/CLI_REFERENCE.md)

## MCP Integration Point

Evidra speaks MCP. Any MCP-capable automation client can report to Evidra.

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
  uses: samebits/evidra-benchmark/.github/actions/setup-evidra@main
```

### Generic CI (GitLab, Jenkins, CircleCI, etc.)

```bash
curl -fsSL https://github.com/samebits/evidra-benchmark/releases/latest/download/evidra_$(uname -s | tr '[:upper:]' '[:lower:]')_$(uname -m | sed 's/x86_64/amd64/;s/aarch64/arm64/').tar.gz \
  | tar -xz -C /usr/local/bin evidra
```

Guides:
- [Setup Evidra Action](docs/guides/setup-evidra-action.md)
- [Terraform CI Quickstart](docs/guides/terraform-ci-quickstart.md)

## Docs Map

Architecture and contracts:
- [Architecture Overview](docs/system-design/EVIDRA_ARCHITECTURE_OVERVIEW.md)
- [V1 Architecture](docs/system-design/V1_ARCHITECTURE.md)
- [Protocol](docs/system-design/EVIDRA_PROTOCOL.md)
- [Core Data Model](docs/system-design/EVIDRA_CORE_DATA_MODEL.md)
- [Canonicalization Contract](docs/system-design/CANONICALIZATION_CONTRACT_V1.md)
- [Signal Spec](docs/system-design/EVIDRA_SIGNAL_SPEC.md)

Operational guides:
- [Scanner SARIF Quickstart](docs/integrations/SCANNER_SARIF_QUICKSTART.md)
- [Artifact Runner Guide](docs/experimental/ARTIFACT_RUNNER_GUIDE.md)
- [Signal Validation Guide](docs/experimental/SIGNAL_VALIDATION_GUIDE.md)

Product and roadmap:
- [Product Positioning](docs/product/EVIDRA_PRODUCT_POSITIONING.md)
- [Roadmap](docs/product/EVIDRA_ROADMAP.md)

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
make lint
make test-signals
```

## License

Licensed under the [Apache License 2.0](LICENSE).
