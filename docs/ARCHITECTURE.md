# Evidra Architecture Overview

## Purpose

This is the architecture entry point for Evidra.
It explains system boundaries and where to find the normative contracts.

This document is non-normative.
Normative behavior is defined in the linked specs.

## One-Sentence Model

**Evidra records automation intent and outcome, then computes reliability signals and scorecards from an append-only evidence chain.**

## Architecture Boundaries

### Ingestion

- `run` = Evidra executes and observes a command live.
- `record` = Evidra ingests a completed operation from structured input.
- `prescribe` / `report` remain the low-level protocol primitives.

### Processing

- Canonicalization adapters transform raw artifacts into canonical actions.
- Risk analysis computes `risk_level` + `risk_tags`.
- Lifecycle writes prescribe/report evidence entries.
- Signal engine computes behavioral signals from evidence.
- Scoring engine computes penalty, score, band, and confidence.

### Storage and Integrity

- Evidence is append-only JSONL segments.
- Hash-linked chain detects tampering.
- Entries are signed (Ed25519) when signing is enabled.

### Export

- CLI and API outputs (scorecard, explain).
- Metrics export via bounded-cardinality labels (`none` or `otlp_http` transport in CLI).

## Component View

```text
Automation (CI / scripts / AI agents)
            |
            | run / record / prescribe / report
            v
     +---------------------+
     |  evidra interfaces  |
     |  CLI + MCP server   |
     +----------+----------+
                |
                v
     +---------------------+
     | lifecycle service   |
     | prescribe -> report |
     +----------+----------+
                |
                v
     +---------------------+
     | evidence chain      |
     | append-only JSONL   |
     +----+-----------+----+
          |           |
          v           v
+----------------+  +----------------+
| signal engine  |  | validation      |
| behavior rates |  | chain/signature |
+-------+--------+  +----------------+
        |
        v
+----------------+
| score engine   |
| score/band/conf|
+----------------+
```

## Data Flow

```text
run/record -> prescribe -> execute/report -> evidence -> signals -> scorecard
```

1. Ingestion path receives raw operation context.
2. `prescribe` computes canonical action and risk context.
3. `report` records operation result.
4. Evidence entries are persisted to chain.
5. Signal detectors evaluate behavior over evidence.
6. Scorecard is computed and returned.

## Current Product Shape (v1)

- Primary UX: `run`, `record`, `scorecard`, `explain`.
- Integration point: MCP server (`evidra-mcp`).
- Evidence-first reliability model with preview/sufficient basis in outputs.
- Metrics-first observability integration.

## Canonical Invariants

1. One scoring/signal engine for all ingestion paths (`run`, `record`, MCP, low-level CLI).
2. Evidence is the single source of truth for replayable scoring.
3. `run` is orchestration, not a second scoring engine.
4. Correlation IDs are not default high-cardinality metric labels.
5. Reliability signals are behavioral indicators, not deny/allow policy enforcement.

## Where To Find Details

Normative contracts:
- [Protocol](system-design/EVIDRA_PROTOCOL.md)
- [Core Data Model](system-design/EVIDRA_CORE_DATA_MODEL.md)
- [Canonicalization Contract](system-design/CANONICALIZATION_CONTRACT_V1.md)
- [Signal Spec](system-design/EVIDRA_SIGNAL_SPEC.md)
- [Session/Operation Event Model](system-design/EVIDRA_SESSION_OPERATION_EVENT_MODEL.md)

System design and implementation mapping:
- [V1 Architecture](system-design/V1_ARCHITECTURE.md)
- [V1 Run/Record Contract](system-design/V1_RUN_RECORD_CONTRACT.md)
- [CNCF Standards Alignment](system-design/EVIDRA_CNCF_STANDARDS_ALIGNMENT.md)

Operational references:
- [CLI Reference](integrations/CLI_REFERENCE.md)
- [End-to-End Example](system-design/EVIDRA_END_TO_END_EXAMPLE_v2.md)
- [Signal Validation Guide](experimental/SIGNAL_VALIDATION_GUIDE.md)
