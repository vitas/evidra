# Evidra Benchmark

[![CI](https://github.com/vitas/evidra-benchmark/actions/workflows/ci.yml/badge.svg?branch=main)](https://github.com/vitas/evidra-benchmark/actions/workflows/ci.yml)
[![Release](https://github.com/vitas/evidra-benchmark/actions/workflows/release.yml/badge.svg)](https://github.com/vitas/evidra-benchmark/actions/workflows/release.yml)
[![Latest Release](https://img.shields.io/github/v/release/vitas/evidra-benchmark)](https://github.com/vitas/evidra-benchmark/releases/latest)
[![Go Report Card](https://goreportcard.com/badge/samebits.com/evidra-benchmark)](https://goreportcard.com/report/samebits.com/evidra-benchmark)
[![License](https://img.shields.io/badge/License-Apache_2.0-blue.svg)](https://opensource.org/licenses/Apache-2.0)

**Flight recorder and reliability benchmark for infrastructure automation**

Evidence and reliability metrics for AI agents, CI pipelines, and IaC workflows.

Works with:

- AI DevOps agents (via MCP server)
- CI pipelines
- Terraform workflows
- Kubernetes manifests

## Contents

- [How It Works](#how-it-works)
- [Signals](#signals)
- [Quick Start](#quick-start)
- [Architecture](#architecture)
- [Documentation](#documentation)
- [License](#license)

---

## How It Works

1. **Prescribe** — before any infrastructure operation, record intent
2. **Execute** — run kubectl apply, terraform apply, helm upgrade, etc.
3. **Report** — after execution, record the outcome

Every prescribe/report pair generates an evidence record. Evidra never blocks operations — it records and measures.

---

## Signals

Evidra computes five behavioral signals from the evidence chain:

| Signal | What it detects |
|---|---|
| Protocol Violation | Missing prescriptions or reports, duplicate reports, cross-actor reports |
| Artifact Drift | Artifact changed between prescribe and execution |
| Retry Loop | Same operation repeated many times in a short window |
| Blast Radius | Destructive operations affecting many resources |
| New Scope | First-time tool/operation combination |

Signals are aggregated into a weighted reliability scorecard: `score = 100 × (1 - penalty)`.

---

## Quick Start

```bash
# Build
make build

# Prescribe before an operation
evidra prescribe --tool terraform --operation apply --artifact plan.json

# Report after execution
evidra report --prescription <id> --exit-code 0

# Generate scorecard
evidra scorecard --actor agent-1 --period 30d
```

### Scanner Context (Defaults: Trivy + Kubescape)

```bash
# Trivy default (Terraform/IaC)
trivy config . --format sarif > scanner_report.sarif
evidra prescribe --tool terraform --artifact plan.json --scanner-report scanner_report.sarif

# Kubescape default (Kubernetes)
kubescape scan . --format sarif --output scanner_report_k8s.sarif
evidra prescribe --tool kubectl --artifact manifest.yaml --scanner-report scanner_report_k8s.sarif
```

One SARIF ingestion contract: `--scanner-report` for both scanners.

### Install

```bash
# Homebrew (macOS / Linux)
brew install samebits/tap/evidra-mcp
brew install samebits/tap/evidra

# Or download from GitHub Releases
# https://github.com/vitas/evidra-benchmark/releases/latest
```

### MCP Server (for AI agents)

```bash
# Run as MCP server on stdio
evidra-mcp --evidence-dir ~/.evidra/evidence

# Or via Docker
docker build -t evidra-mcp:dev -f Dockerfile .
```

---

## Architecture

```
raw artifact → adapter (k8s/tf/generic) → canonical action → risk detectors → prescription
                                                                                    ↓
exit code + prescription_id ────────────────────────────────────────────────→ report
                                                                                    ↓
                                                              evidence chain → signals → scorecard
```

Three binaries:

| Binary | Purpose |
|---|---|
| `evidra` | CLI: scorecard, compare, prescribe, report |
| `evidra-mcp` | MCP server for AI agents (stdio transport) |

---

## Documentation

### Architecture & Design

- [Architecture Overview](docs/system-design/EVIDRA_ARCHITECTURE_OVERVIEW.md) — system diagram, component map, data flow
- [Inspector Model Architecture](docs/system-design/done/EVIDRA_INSPECTOR_MODEL_ARCHITECTURE.md) — why Evidra observes instead of enforcing
- [Architecture Review](docs/system-design/done/EVIDRA_ARCHITECTURE_REVIEW.md) — gap analysis and trade-offs
- [Architecture Recommendation](docs/system-design/done/EVIDRA_ARCHITECTURE_RECOMMENTATION_V1.md) — v1 architecture decisions

### Specifications

- [Integration Protocol v1.0](docs/system-design/EVIDRA_PROTOCOL.md) — session/run lifecycle, correlation model, scope dimensions, actor identity, findings ingestion
- [Agent Reliability Benchmark](docs/system-design/EVIDRA_AGENT_RELIABILITY_BENCHMARK.md) — protocol, signals, scoring formula, Prometheus metrics
- [Signal Spec](docs/system-design/EVIDRA_SIGNAL_SPEC.md) — formal definitions of all five signals
- [Canonicalization Contract v1](docs/system-design/CANONICALIZATION_CONTRACT_V1.md) — adapter interface, digest model, compatibility rules
- [Canonicalization Test Strategy](docs/system-design/EVIDRA_CANONICALIZATION_TEST_STRATEGY.md) — golden corpus, determinism testing
- [End-to-End Example](docs/system-design/EVIDRA_END_TO_END_EXAMPLE_v2.md) — full prescribe/report walkthrough

### Product & Strategy

- [Product Positioning](docs/product/EVIDRA_PRODUCT_POSITIONING.md) — market position and value proposition
- [Roadmap](docs/product/EVIDRA_ROADMAP.md) — release plan and milestones
- [Strategic Direction](docs/product/EVIDRA_STRATEGIC_DIRECTION.md) — long-term vision
- [Strategic Moat & Standardization](docs/system-design/done/EVIDRA_STRATEGIC_MOAT_AND_STANDARDIZATION.md) — competitive positioning
- [Integration Roadmap](docs/system-design/done/EVIDRA_INTEGRATION_ROADMAP.md) — tool integration plan
- [Scanner SARIF Quickstart](docs/integrations/SCANNER_SARIF_QUICKSTART.md) — Trivy + Kubescape defaults with one contract
### Backlog

- [Threat Model](docs/system-design/backlog/EVIDRA_THREAT_MODEL.md) — security considerations
- [Anti-Goodhart Addendum](docs/system-design/backlog/ANTI_GOODHART_BACKLOG_ADDENDUM.md) — gaming resistance

---

## License

Licensed under the [Apache License 2.0](LICENSE).
