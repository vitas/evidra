# Changelog

## v0.4.5 — 2026-03-10

### Decision Tracking
- Added explicit terminal `report verdict` handling across CLI, MCP, and forwarded evidence
- Added `declined` decision recording with required trigger and bounded operational reason
- Preserved the `one prescription -> one report` invariant while making refusal decisions first-class evidence

### MCP And Prompts
- Updated MCP report schema, tool descriptions, prompt contracts, and generated prompt artifacts for explicit verdicts and declined decisions
- MCP now records not only actions but also deliberate refusals with rationale

### Documentation And Product Surfaces
- Added [Decision Tracking v1](docs/system-design/EVIDRA_DECISION_TRACKING_V1.md)
- Updated README, CLI reference, architecture docs, end-to-end examples, landing page, and fallback landing copy to reflect `intent -> decision -> outcome`

### Testing
- Added CLI, lifecycle, and MCP regression coverage for declined decisions
- Updated contracts, signal-validation helpers, inspector fixtures, and docs smoke checks to the explicit verdict contract

## v0.4.4 — 2026-03-10

### Core
- Merged the codebase hardening wave: decomposed the CLI command surface, added governance baseline files, and tightened security and product claims
- Integrated the hardened signal-validation/scoring-profile path on `main`

### Testing
- Rationalized the top-level e2e structure and tightened signal-validation coverage and CI checks

### Release Metadata
- Runtime version and MCP server metadata aligned to `v0.4.4`

## v0.4.3 — 2026-03-10

### Positioning
- Homepage, fallback landing page, and README now lead with `Behavioral reliability for infrastructure automation`
- Category framing updated to `A new observability layer for CI/CD, IaC, and AI agents`
- Landing copy now emphasizes deployment instability outcomes instead of generic scoring language

### Documentation
- Added [Prometheus Positioning Backlog](docs/PROMETHEUS_POSITIONING_BACKLOG.md) to track the next observability-first homepage improvements

### Release Metadata
- Runtime version, MCP server metadata, and release artifacts aligned to `v0.4.3`

## v0.4.2 — 2026-03-09

### Signals
- New signal: `risk_escalation` — detects when an actor's operations exceed their baseline risk level (8th signal, weight 0.10)
- Baseline computed as mode of actor+tool risk levels in 30-day rolling window
- Demotions tracked internally as `risk_demotion` sub-signal (informational, no penalty)
- Signal Spec updated to v1.1

### Telemetry
- `risk_escalation` added to allowed signal names in OTLP metrics export

### Documentation
- [MCP Setup Guide](docs/guides/mcp-setup.md) — install, connect agents (Claude Code, Cursor, Codex, Gemini CLI, OpenClaw), configuration, troubleshooting
- MCP Setup section added to landing page with editor-specific config snippets
- Signal 8 definition added to EVIDRA_SIGNAL_SPEC.md
- All "7 signals" references updated to 8 across docs, UI, and OpenAPI spec
- Architecture overview moved to `docs/ARCHITECTURE.md`

### Testing
- E2e test: staging→production escalation through full CLI pipeline
- Score stability regression test (zero-count risk_escalation does not affect score)

## v0.3.1 — 2026-03-07

### CLI
- `evidra run` — execute commands live and record lifecycle outcome (prescribe + execute + report in one call)
- `evidra record` — ingest completed operations from structured JSON input
- `evidra keygen` — generate Ed25519 signing keypair
- Assessment output includes `score`, `score_band`, `basis` (preview vs sufficient), and `signal_summary`
- `--canonical-action` flag for pre-canonicalized actions (Pulumi, Ansible, CDK escape hatch)
- Kustomize support added to K8s adapter (`--tool kustomize`)

### Observability
- OTLP/HTTP metrics export: `evidra.operation.signal.count` and `evidra.operation.duration_ms`
- Bounded-cardinality labels: tool, environment, result_class, signal_name, score_band, assessment_mode
- Configuration via `EVIDRA_METRICS_TRANSPORT`, `EVIDRA_METRICS_OTLP_ENDPOINT`, `EVIDRA_METRICS_TIMEOUT`
- [Observability Quickstart](docs/guides/observability-quickstart.md) with collector setup and PromQL examples

### Protocol
- Session ID auto-generated at ingress when omitted
- `operation_id` and `attempt` fields on evidence entries
- `session_start`, `session_end`, `annotation` entry types
- Signing enforced on every evidence entry (strict mode default)
- Trace defaults: `trace_id` defaults to `session_id`, optional `span_id`/`parent_span_id`
- Evidence write mode: `strict` (default) or `best_effort`

### Canonicalization
- Docker adapter: docker, nerdctl, podman, compose
- OpenShift resources: DeploymentConfig, Route, BuildConfig, ImageStream
- Noise filtering: managedFields, uid, resourceVersion, creationTimestamp, last-applied-configuration

### Documentation
- [Supported Tools](docs/SUPPORTED_TOOLS.md) reference with adapter matrix and risk detectors
- [Observability Quickstart](docs/guides/observability-quickstart.md) — OTLP setup, Grafana/Prometheus queries, CI examples
- [Terraform CI Quickstart](docs/guides/terraform-ci-quickstart.md)
- [Scanner SARIF Quickstart](docs/integrations/SCANNER_SARIF_QUICKSTART.md) rewritten with run/record patterns
- [CLI Reference](docs/integrations/CLI_REFERENCE.md) — unified command reference
- [Setup Evidra Action](docs/guides/setup-evidra-action.md) — GitHub Actions + generic CI install

### Testing
- Real-world e2e test suite: K8s, Terraform, Helm (Redis, ingress-nginx), ArgoCD, Kustomize, OpenShift
- E2e tests verify actual canonicalization output (resource_count, resource_identity, risk_tags, noise immunity)
- Run/record parity contract tests
- MCP schema-struct parity contract test
- Signal validation scenarios in CI

### CI/CD
- E2e tests gate release pipeline (release-guard → test → e2e → snapshot + docker → goreleaser)
- Homebrew tap publishing via GoReleaser
- Docker image: `ghcr.io/vitas/evidra-mcp`
- `setup-evidra` GitHub Action for CI adoption

### Fixes
- Evidence chain: in-process ID cache for faster entry lookup
- Findings correlation: correct TraceID, attach SessionID/OperationID/Attempt
- Lifecycle flows unified with session invariant enforcement
- Removed dead code (MaxBaseSeverity, RehashEntry, SegmentFiles)

## v0.3.0 — 2026-03-05

First public release of Evidra Benchmark.

### Core Pipeline
- Canonicalization adapters: Kubernetes (kubectl, oc, helm), Terraform, Docker (docker, nerdctl, podman), generic fallback
- Risk matrix (operation_class x scope_class) with 7 catastrophic detectors
- Eight behavioral signals: protocol violation, artifact drift, retry loop, blast radius, new scope, repair loop, thrashing, risk escalation
- Weighted reliability scoring with safety floors and band classification
- Ed25519 evidence signing with strict/optional modes and key generation

### CLI (`evidra`)
- `prescribe` — record intent before infrastructure operations
- `report` — record outcome after execution
- `scorecard` — compute reliability score from evidence chain
- `explain` — detailed signal breakdown with sub-signals
- `compare` — side-by-side actor comparison with workload overlap
- `--scanner-report` flag for SARIF ingestion (Trivy, Kubescape)
- `--canonical-action` flag for pre-canonicalized actions
- Tool and scope filtering on scorecard/explain/compare
- `run` — execute command live and record lifecycle outcome
- `record` — ingest completed operation from structured JSON input
- `validate` — verify evidence chain integrity and signatures
- `ingest-findings` — ingest SARIF scanner findings as evidence entries
- `keygen` — generate Ed25519 signing keypair

### MCP Server (`evidra-mcp`)
- Stdio transport for MCP-based automation integration (including AI agents)
- Tools: prescribe, report, get_event
- Session/trace/span correlation fields for multi-step workflows
- Optional retry loop tracking

### Protocol (v1.0 Foundation)
- Session/run boundary hardened: persisted evidence entries always include `session_id` (generated at ingress when omitted by caller)
- Correlation defaults documented: `trace_id` defaults to `session_id`, with optional `span_id` and `parent_span_id`
- Actor identity: `actor.instance_id` and `actor.version` (optional, not used in metrics)
- Scope dimensions: `scope_dimensions` map for detailed environment metadata (cluster, namespace, account, region)
- Protocol spec: `docs/system-design/EVIDRA_PROTOCOL.md`

### Evidence Chain
- Append-only JSONL with hash-linked entries
- Segmented storage with automatic rotation (5MB default)
- File-based locking for concurrent access

### Build
- Go 1.23 minimum (CI pinned from `go.mod`)
- Cross-platform binaries via GoReleaser (linux/darwin/windows, amd64/arm64)
- Homebrew: `brew install samebits/tap/evidra-mcp`
- Docker: `ghcr.io/vitas/evidra-mcp:0.3.0`

### Known Limitations
- ArgoCD uses generic adapter (no Argo-specific metadata)
- MinOperations=100 required for scoring (low-volume actors get `insufficient_data`)
- Optional signing mode (`EVIDRA_SIGNING_MODE=optional`) uses ephemeral keys and is not durable across restarts
- No centralized API server (v0.5.0)
