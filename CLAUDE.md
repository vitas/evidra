# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Build & Test Commands

```bash
make build                              # build bin/evidra and bin/evidra-mcp
make test                               # go test ./... -v -count=1
make fmt                                # gofmt -w .
make lint                               # golangci-lint run
make tidy                               # go mod tidy
make golden-update                      # regenerate golden test files
make docker-mcp                         # build MCP server Docker image
make docker-cli                         # build CLI Docker image

go test -run TestFunctionName ./internal/canon/...   # run a single test
go test -race ./...                                  # race detector
```

## Architecture

Evidra Benchmark is a **flight recorder for infrastructure automation** — it observes and measures AI agent and CI pipeline reliability without blocking operations.

**Module:** `samebits.com/evidra-benchmark` (Go 1.23)

### Two binaries

- `cmd/evidra/` — CLI for prescribe, report, scorecard, compare
- `cmd/evidra-mcp/` — MCP server (stdio transport, uses `github.com/modelcontextprotocol/go-sdk`) for AI agents

### Core pipeline

```
raw artifact → canon adapter → CanonicalAction → risk detectors → Prescription
                                                                        ↓
exit code + prescription_id → Report → signal detectors → Scorecard
```

### Key packages

- **`internal/canon/`** — Canonicalization layer. Four adapters (`K8sAdapter`, `TerraformAdapter`, `DockerAdapter`, `GenericAdapter`) normalize tool-specific artifacts into `CanonicalAction`. Adapter selection via `SelectAdapter()` chain with generic fallback. Golden test files in `tests/golden/`.
- **`internal/risk/`** — Risk classification. `riskMatrix` maps `operationClass × scopeClass → riskLevel`. Tag-based risk detectors live in `internal/detectors/` (e.g., privileged containers, wildcard RBAC).
- **`internal/signal/`** — Seven behavioral signal detectors: protocol violation, artifact drift, retry loop, blast radius, new scope, repair loop, thrashing. All operate on `[]Entry` and return `SignalResult`.
- **`internal/score/`** — Weighted penalty scoring (`score = 100 × (1 - penalty)`), workload profile comparison.
- **`pkg/mcpserver/`** — MCP server implementation. Tools: `prescribe`, `report`, `get_event`. JSON schemas embedded from `pkg/mcpserver/schemas/`.
- **`pkg/evidence/`** — Evidence chain persistence (file-based, append-only segments with manifest and locking).
- **`internal/evidence/`** — Evidence builder, HMAC signer, signing payload construction.

### Architecture reference

`docs/system-design/EVIDRA_ARCHITECTURE_OVERVIEW.md` is the **single architecture reference**.
It consolidates key decisions, invariants, and known gaps from the former review and recommendation docs (now archived in `docs/system-design/done/`).

### Conventions

- No web frameworks — stdlib `net/http` only.
- IDs generated with `github.com/oklog/ulid/v2`.
- `internal/` for server-only code, `pkg/` for shared code.
- Golden test pattern: test fixtures in `tests/golden/`, update with `EVIDRA_UPDATE_GOLDEN=1`.
- Canonicalization adapters implement the `canon.Adapter` interface.

### Environment variables

- `EVIDRA_EVIDENCE_DIR` — evidence storage directory (default: `~/.evidra/evidence`)
- `EVIDRA_ENVIRONMENT` — environment label (MCP server only)
- `EVIDRA_RETRY_TRACKER` — enable retry loop tracking (MCP server only)
