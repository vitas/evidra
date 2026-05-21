# Changelog

## Unreleased

## v0.5.28 — 2026-05-21

### SEO
- Added root sitemap and robots files for search engine crawling.

## v0.5.27 — 2026-05-21

### Site Verification
- Added the Google Search Console verification file to the embedded web UI.

### Landing Page
- Added the open source Evidra Bench repository link to the landing page.

## v0.5.26 — 2026-05-08

## v0.5.25 — 2026-05-08

## v0.5.24 — 2026-05-08

## v0.5.23 — 2026-05-08

## v0.5.22 — 2026-05-06

## v0.5.21 — 2026-05-06

### Repository Scope
- Removed the hosted bench API, runner control plane, and embedded `/bench` UI from the core Evidra API; Evidra Bench now lives in the separate `evidra-infra-bench` repository and at `https://lab.evidra.cc`.

## v0.5.20 — 2026-04-13

## v0.5.19 — 2026-04-13

## v0.5.18 — 2026-03-28

### Bench Hosted Execution
- Added first-class hosted `execution_mode` support to `POST /v1/bench/trigger`, with optional `provider|a2a` selection and default `provider`
- Threaded `execution_mode` through trigger state, runner job persistence, runner claim payloads, OpenAPI, and bench dashboard controls
- Mapped hosted `execution_mode=a2a` to the bench executor's internal `config.adapter=a2a` for direct benchmark execution

### Bench Model Configuration
- `GET /v1/bench/models` — list tenant-visible models with `available` field based on platform env var presence
- `PUT /v1/bench/models/{model_id}/provider` and `DELETE` — tenant provider overrides (disabled until encryption is implemented)
- `PUT /v1/admin/bench/models/{model_id}` — invite-gated platform route for global model defaults
- Trigger handler auto-resolves provider from model catalog when not supplied in request
- Trigger handler validates API key is configured before accepting jobs
- Input validation: upsert requires at least one non-empty field; delete returns 404 for nonexistent providers

### MCP Deferred Tool Loading
- Deferred tool schema registry — `prescribe_smart` and `report` schemas loaded on demand, not at initialize
- `describe_tool` meta-tool for clients to fetch individual tool schemas
- `run_command` path preferred in initialize instructions
- MCP contract bumped to v1.3.0


## v0.5.13 

### Contract v1.2.0
- Contract extended with DevOps operations (run_command, write_file, diagnosis protocol, safety rules)
- MCP prompt templates (prescribe-smart, prescribe-full, diagnosis) generated from contract source
- All prompts generated from `CONTRACT.yaml` — no hardcoded files
- SKILL.md leads with DevOps operations, protocol compressed to essentials

### Skill Rework
- SKILL.md rewritten: DevOps ops first (~600 tokens), protocol second (was ~2000)
- Trigger description updated to include all DevOps ops (read + write)
- MCP prompts: prescribe-smart, prescribe-full, diagnosis as on-demand resources

### Bench Intelligence Endpoints
- `GET /v1/bench/signals` — aggregated signal counts (protocol_violation, retry_loop, blast_radius) from run scorecards
- `GET /v1/bench/regressions` — detects scenario/model pairs where the latest run failed but previous runs passed
- `GET /v1/bench/insights?scenario=X` — failure analysis with check failure stats, model breakdown, behavior metrics (pass vs fail avg turns/tokens/cost)
- `GET /v1/bench/compare/models` — fixed to accept `?models=X,Y,Z&scenarios=A,B` for multi-model matrix comparison (in addition to legacy `?a=X&b=Y` pairwise)
- `POST /v1/bench/scenarios/sync` — upsert scenario metadata from bench CLI

### MCP Modes And Ingest
- Split the MCP lifecycle surface into `prescribe_full` and `prescribe_smart`, with clearer public mode wording around Full Prescribe, Smart Prescribe, and Proxy Observed
- Added authenticated external lifecycle ingest routes for adapter-driven `prescribe` and `report` creation, and routed webhook ingestion through the same shared service
- Extended payload taxonomy so entries now carry execution flavor plus explicit `evidence.kind` and `source.system`


## v0.4.11

### GitOps And Argo CD
- Added controller-first Argo CD integration for self-hosted `evidra-api`, with zero-touch reconciliation capture and explicit `evidra.cc/*` correlation
- Kept GitOps evidence on the standard `prescribe` / `report` lifecycle using `payload.flavor = reconcile`
- Added shared automation event emission for mapped Argo CD webhook and controller-reported lifecycle entries
- Split execution flavor from ingest taxonomy so payloads can also record `evidence.kind` and `source.system`, and renamed `pipeline_stage` to `workflow`
- Refactored `evidra-api` startup initialization to reduce complexity without changing startup behavior

## v0.4.10 

### Benchmark API
- Added benchmark table to landing page UI
- Added input validation for benchmark run suite field
- Capped benchmark list query limit to 100

### Hosted Analytics And API Contracts
- Replayed stored evidence chronologically in hosted scorecard/explain so self-hosted analytics matches CLI/local signal behavior for order-sensitive detectors
- Added required `operation_id` to generic webhook events and used it as the stable prescribe/report lifecycle correlation key
- Moved API key issuance quota checks behind invite-secret validation so rejected onboarding attempts do not burn shared rate-limit budget
- Replaced fire-and-forget `last_used_at` writes on API key lookup with bounded inline updates
- Restored a fixed eight-signal public scorecard contract instead of auto-expanding API output to every registered signal

### MCP
- Fixed the `get_event` MCP tool output contract so stored report events can be returned without structured-output schema validation failure
- Added explicit MCP output schema coverage for `get_event` payload shapes

### MCP And Prompts
- Updated MCP report schema, tool descriptions, prompt contracts, and generated prompt artifacts for explicit verdicts and declined decisions
- MCP now records not only actions but also deliberate refusals with rationale

## v0.4.2

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

## v0.3.1 

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
- [CLI Reference](docs/integrations/cli-reference.md) — unified command reference
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

## v0.3.0

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
