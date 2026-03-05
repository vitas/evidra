# Evidra Benchmark

[![CI](https://github.com/vitas/evidra-benchmark/actions/workflows/ci.yml/badge.svg?branch=main)](https://github.com/vitas/evidra-benchmark/actions/workflows/ci.yml)
[![Release](https://github.com/vitas/evidra-benchmark/actions/workflows/release.yml/badge.svg)](https://github.com/vitas/evidra-benchmark/actions/workflows/release.yml)
[![Latest Release](https://img.shields.io/github/v/release/vitas/evidra-benchmark)](https://github.com/vitas/evidra-benchmark/releases/latest)
[![Go Report Card](https://goreportcard.com/badge/samebits.com/evidra-benchmark)](https://goreportcard.com/report/samebits.com/evidra-benchmark)
[![License](https://img.shields.io/badge/License-Apache_2.0-blue.svg)](https://opensource.org/licenses/Apache-2.0)

**Flight recorder and reliability benchmark for infrastructure automation.**

Evidra observes AI agents, CI pipelines, and IaC workflows — recording evidence, computing behavioral signals, and producing reliability scorecards. It never blocks operations.

---

## How It Works

```
artifact --> prescribe --> execute --> report --> signals --> scorecard
```

1. **Prescribe** — record intent before any infrastructure operation
2. **Execute** — run kubectl, terraform, helm, etc.
3. **Report** — record the outcome after execution

Every prescribe/report pair produces a signed, hash-chained evidence entry.

---

## Signals

Five behavioral signals computed from the evidence chain:

| Signal | Detects |
|--------|---------|
| Protocol Violation | Missing prescriptions/reports, duplicates, cross-actor reports |
| Artifact Drift | Artifact changed between prescribe and execution |
| Retry Loop | Same operation repeated in a short window |
| Blast Radius | Destructive operations affecting many resources |
| New Scope | First-time tool/operation combination |

Scorecard formula: `score = 100 * (1 - weighted_penalty)` with bands: excellent / good / fair / poor.

---

## Install

```bash
# Homebrew (macOS / Linux)
brew install samebits/tap/evidra
brew install samebits/tap/evidra-mcp

# Or download from GitHub Releases
# https://github.com/vitas/evidra-benchmark/releases/latest

# Or build from source
make build    # produces bin/evidra and bin/evidra-mcp
```

---

## Quick Start

### Generate signing keys (required)

```bash
evidra keygen
# Outputs:
#   EVIDRA_SIGNING_KEY=<base64>     (set as env var or use --signing-key)
#   -----BEGIN PUBLIC KEY-----      (save to file for validation)
```

### Prescribe / Report / Scorecard

```bash
export EVIDRA_SIGNING_KEY=<base64 from keygen>

# Before execution
evidra prescribe --tool terraform --operation apply --artifact plan.json \
  --session-id "$SESSION_ID"

# After execution
evidra report --prescription <id> --exit-code 0 --session-id "$SESSION_ID"

# Generate scorecard
evidra scorecard --session-id "$SESSION_ID"

# Explain signal contributions
evidra explain --session-id "$SESSION_ID"

# Compare actors
evidra compare --actors agent-1,agent-2 --period 30d
```

Session IDs are auto-generated (ULID) when `--session-id` is omitted.

### Ingest Scanner Findings (SARIF)

```bash
# Standalone ingestion
evidra ingest-findings --sarif trivy-results.sarif --artifact deploy.yaml

# Or inline with prescribe
evidra prescribe --tool kubectl --artifact manifest.yaml \
  --scanner-report kubescape.sarif
```

Supported scanners: any tool producing [SARIF v2.1.0](https://docs.oasis-open.org/sarif/sarif/v2.1.0/sarif-v2.1.0.html) (Trivy, Kubescape, Checkov, etc.).

### Validate Evidence Chain

```bash
# Hash chain only
evidra validate --evidence-dir ./evidence

# Hash chain + Ed25519 signatures
evidra validate --evidence-dir ./evidence --public-key pub.pem
```

---

## CLI Commands

| Command | Purpose |
|---------|---------|
| `keygen` | Generate Ed25519 signing keypair |
| `prescribe` | Record intent before execution |
| `report` | Record outcome after execution |
| `scorecard` | Generate reliability scorecard |
| `explain` | Explain signals contributing to a score |
| `compare` | Compare reliability across actors |
| `validate` | Validate evidence chain integrity and signatures |
| `ingest-findings` | Ingest SARIF scanner findings as evidence |
| `version` | Print version information |

Run `evidra <command> --help` for command-specific flags.

---

## MCP Server (for AI Agents)

```bash
# Run as MCP server on stdio
evidra-mcp --evidence-dir ~/.evidra/evidence

# Or via Docker
docker build -t evidra-mcp:dev -f Dockerfile .
```

Tools exposed: `prescribe`, `report`, `get_event`. JSON schemas in `pkg/mcpserver/schemas/`.

---

## GitHub Action

```yaml
- name: Run Evidra Benchmark
  uses: samebits/evidra-benchmark/.github/actions/evidra@main
  with:
    evidence-dir: ./evidence
    session-id: ${{ github.run_id }}
    sarif-path: trivy-results.sarif
    public-key: signing.pub.pem
    fail-on-risk: fair    # fail if band is fair or worse
```

---

## Architecture

Two binaries, one evidence chain:

| Binary | Transport | Purpose |
|--------|-----------|---------|
| `evidra` | CLI | Human and CI pipeline interface |
| `evidra-mcp` | stdio (MCP) | AI agent interface |

### Core Pipeline

```
raw artifact -> canon adapter -> CanonicalAction -> risk detectors -> Prescription
                                                                          |
exit code + prescription_id -> Report -> signal detectors -> Scorecard
```

### Key Packages

| Package | Role |
|---------|------|
| `internal/canon/` | Canonicalization (K8s, Terraform, Generic adapters) |
| `internal/risk/` | Risk classification matrix + tag detectors |
| `internal/signal/` | Five behavioral signal detectors |
| `internal/score/` | Weighted penalty scoring + confidence |
| `internal/sarif/` | SARIF v2.1.0 parser (lossy projection) |
| `pkg/evidence/` | Evidence chain: append-only segments, hash chain, Ed25519 signing |
| `pkg/mcpserver/` | MCP server implementation |

### Evidence Model

- Append-only JSONL segments with manifest and file locking
- SHA-256 hash chain (`previous_hash` linkage)
- Ed25519 signatures on every entry (required)
- Session/operation correlation (`session_id`, `operation_id`, `attempt`)
- Nine entry types: `prescribe`, `report`, `finding`, `signal`, `receipt`, `canonicalization_failure`, `session_start`, `session_end`, `annotation`

---

## Documentation

### Normative Specifications

| Document | Status | Scope |
|----------|--------|-------|
| [Core Data Model](docs/system-design/EVIDRA_CORE_DATA_MODEL.md) | Normative | Entry schema, field definitions, frozen enums |
| [Integration Protocol v1.0](docs/system-design/EVIDRA_PROTOCOL.md) | Normative (Draft) | Session lifecycle, correlation, actor identity, findings ingestion |
| [Session/Operation Event Model](docs/system-design/EVIDRA_SESSION_OPERATION_EVENT_MODEL.md) | Normative | Session/operation hierarchy, event taxonomy, OTel/CloudEvents/K8s mapping |
| [Signal Spec](docs/system-design/EVIDRA_SIGNAL_SPEC.md) | Normative | Formal definitions of all five signals |
| [Canonicalization Contract v1](docs/system-design/CANONICALIZATION_CONTRACT_V1.md) | Frozen | Adapter interface, digest model, compatibility rules |
| [Agent Reliability Benchmark](docs/system-design/EVIDRA_AGENT_RELIABILITY_BENCHMARK.md) | Normative | Scoring formula, Prometheus metrics |

### Non-Normative References

| Document | Scope |
|----------|-------|
| [Architecture Overview](docs/system-design/EVIDRA_ARCHITECTURE_OVERVIEW.md) | System diagram, component map, invariants |
| [Benchmark CLI](docs/system-design/EVIDRA_BENCHMARK_CLI.md) | CLI design and command reference |
| [CNCF Standards Alignment](docs/system-design/EVIDRA_CNCF_STANDARDS_ALIGNMENT.md) | CloudEvents, OTel, SARIF, in-toto, OPA mapping |
| [Canonicalization Test Strategy](docs/system-design/EVIDRA_CANONICALIZATION_TEST_STRATEGY.md) | Golden corpus, determinism testing |
| [End-to-End Example](docs/system-design/EVIDRA_END_TO_END_EXAMPLE_v2.md) | Full prescribe/report walkthrough |
| [Scanner SARIF Quickstart](docs/integrations/SCANNER_SARIF_QUICKSTART.md) | Trivy + Kubescape setup guide |

### Product

- [Product Positioning](docs/product/EVIDRA_PRODUCT_POSITIONING.md)
- [Roadmap](docs/product/EVIDRA_ROADMAP.md)
- [Strategic Direction](docs/product/EVIDRA_STRATEGIC_DIRECTION.md)

---

## Environment Variables

| Variable | Purpose |
|----------|---------|
| `EVIDRA_SIGNING_KEY` | Base64-encoded Ed25519 signing key |
| `EVIDRA_SIGNING_KEY_PATH` | Path to PEM-encoded Ed25519 private key |
| `EVIDRA_EVIDENCE_DIR` | Evidence storage directory (default: `~/.evidra/evidence`) |
| `EVIDRA_ENVIRONMENT` | Environment label |
| `EVIDRA_API_URL` | Forward evidence to remote API |
| `EVIDRA_RETRY_TRACKER` | Enable retry loop tracking |

---

## Build & Test

```bash
make build          # bin/evidra + bin/evidra-mcp
make test           # go test ./... -v -count=1
make lint           # golangci-lint run
make fmt            # gofmt -w .
make tidy           # go mod tidy
make golden-update  # regenerate golden test files
```

Requires Go 1.24+.

---

## License

Licensed under the [Apache License 2.0](LICENSE).
