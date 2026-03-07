# Evidra Roadmap

## Shipped

### v0.3.0 (current)
- Canonicalization adapters: K8s (kubectl, oc, helm), Terraform, Docker, generic
- Seven behavioral signal detectors
- Weighted reliability scoring with bands and confidence
- Evidence chain: append-only JSONL, hash-linked, Ed25519 signed
- CLI: run, record, prescribe, report, scorecard, explain, compare, validate, ingest-findings, keygen
- MCP server: prescribe, report, get_event (stdio transport)
- OTLP metrics export
- GoReleaser + Homebrew + Docker

## Next

### v0.4.0
- ArgoCD-specific canonicalization adapter
- Benchmark dataset engine (currently gated behind experimental build tag)
- Configurable MinOperations threshold
- Azure and GCP Terraform detectors

### v0.5.0
- Centralized API backend for multi-node ingestion
- Postgres evidence store
- Receipt/outbox pattern for distributed writes
