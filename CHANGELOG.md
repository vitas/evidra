# Changelog

## v0.3.0 — 2026-03-05

First public release of Evidra Benchmark.

### Core Pipeline
- Canonicalization adapters: Kubernetes (kubectl, oc, helm), Terraform, Docker (docker, nerdctl, podman), generic fallback
- Risk matrix (operation_class x scope_class) with 7 catastrophic detectors
- Seven behavioral signals: protocol violation, artifact drift, retry loop, blast radius, new scope, repair loop, thrashing
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
