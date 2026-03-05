# Changelog

## v0.3.0 — 2026-03-05

First public release of Evidra Benchmark.

### Core Pipeline
- Canonicalization adapters: Kubernetes (kubectl, oc, helm), Terraform, generic fallback
- Risk matrix (operation_class x scope_class) with 7 catastrophic detectors
- Five behavioral signals: protocol violation, artifact drift, retry loop, blast radius, new scope
- Weighted reliability scoring with safety floors and band classification

### CLI (`evidra`)
- `prescribe` — record intent before infrastructure operations
- `report` — record outcome after execution
- `scorecard` — compute reliability score from evidence chain
- `explain` — detailed signal breakdown with sub-signals
- `compare` — side-by-side actor comparison with workload overlap
- `--scanner-report` flag for SARIF ingestion (Trivy, Kubescape)
- `--canonical-action` flag for pre-canonicalized actions
- Tool and scope filtering on scorecard/explain/compare

### MCP Server (`evidra-mcp`)
- Stdio transport for AI agent integration
- Tools: prescribe, report, get_event
- Per-operation trace IDs for correlation
- Optional retry loop tracking

### Protocol (v1.0 Foundation)
- Session/run boundary: `session_id` on all evidence entries (optional, auto-generated if omitted)
- Hierarchical tracing: `trace_id`, `span_id`, `parent_span_id` for multi-step agent workflows
- Actor identity: `actor.instance_id` and `actor.version` (optional, not used in metrics)
- Scope dimensions: `scope_dimensions` map for detailed environment metadata (cluster, namespace, account, region)
- Protocol spec: `docs/system-design/EVIDRA_PROTOCOL.md`

### Evidence Chain
- Append-only JSONL with hash-linked entries
- Segmented storage with automatic rotation (5MB default)
- File-based locking for concurrent access

### Build
- Go 1.23 minimum (broad adoption)
- Cross-platform binaries via GoReleaser (linux/darwin/windows, amd64/arm64)
- Homebrew: `brew install samebits/tap/evidra-mcp`
- Docker: `ghcr.io/samebits/evidra-mcp:0.3.0`

### Known Limitations
- ArgoCD uses generic adapter (no Argo-specific metadata)
- MinOperations=100 required for scoring (low-volume actors get `insufficient_data`)
- Signing implemented but not wired into pipeline (v0.4.0)
- No centralized API server (v0.5.0)
