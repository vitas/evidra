# Evidra — Product Positioning

**Reliability inspector for infrastructure automation and AI agents.**

Evidra observes infrastructure automation, records tamper‑evident evidence, and measures operational reliability.

It does **not enforce policy** and does not block infrastructure actions.
Instead, it produces **verifiable evidence and behavioral signals** that allow teams to understand how automation behaves.

---

## The problem

Infrastructure is increasingly operated by:

- CI/CD pipelines
- Infrastructure‑as‑Code
- internal automation tools
- AI DevOps agents

But organizations cannot answer simple questions:

- Which automation breaks infrastructure most often?
- Which AI agent produces fewer operational anomalies?
- What actually executed during an incident?

Evidra introduces a **standard reliability benchmark for automation**.

---

## What Evidra does

Evidra records automation activity and produces signals describing operational behavior.

Signals include:

- Protocol violations
- Artifact drift
- Retry loops
- Blast radius events
- New scope operations

Signals are aggregated into **automation reliability scorecards**.

---

## Evidence first

Every operation recorded by Evidra produces a cryptographically verifiable evidence record.

Evidence records are:

- append‑only
- hash‑linked
- optionally signed (Ed25519)
- exportable and verifiable offline

Evidence provides the factual basis for reliability signals and incident analysis.

---

## Hosted and offline modes

Evidra works in two modes:

Offline:
- CLI / MCP server
- local evidence chain

Hosted:
- multi‑tenant API
- Postgres evidence storage
- signed evidence records

Both modes share the same **evidence contract**.

---

Evidra is an **inspector** that measures automation behavior.
