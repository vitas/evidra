# Public Roadmap Design

**Date:** 2026-03-10
**Status:** Approved design
**Owner:** Architecture

## Goal

Create a canonical public roadmap at `docs/ROAD_MAP.md` that serves two
audiences at the same time:

- CNCF sandbox reviewers evaluating project maturity and ecosystem fit
- prospective adopters evaluating whether Evidra is a credible project to use

The roadmap should also remain concrete enough to guide the project's next
execution steps.

## Problem

The current roadmap and strategy documents are fragmented and partially stale.
They emphasize internal feature delivery more than ecosystem adoption,
community trust, and CNCF-aligned maturity. That is not the right shape for a
public roadmap that may be referenced in sandbox materials.

There is also wording drift in older product documents. The new roadmap must
follow the current positioning:

- Evidra is about behavioral reliability for infrastructure automation
- the project is an observability layer, not a policy engine
- CLI and MCP are the authoritative analytics path today
- self-hosted analytics exist but remain experimental
- OpenTelemetry support is partially implemented already and should be
  described as hardening/expansion, not as a net-new addition

## Recommended Structure

`docs/ROAD_MAP.md` should use one public roadmap with two lenses rather than
separate product and CNCF roadmaps.

### Why one roadmap

- avoids drift between internal and external planning
- gives reviewers and adopters the same honest picture
- keeps the project story focused on maturity, ecosystem fit, and adoption
- reduces maintenance overhead compared with parallel roadmaps

## Document Outline

1. Positioning intro
2. Current state
3. Roadmap principles
4. Now (0-6 months)
5. Next (6-12 months)
6. Later (12+ months)
7. How this roadmap supports CNCF sandbox and ecosystem adoption

## Milestone Tracks

Each time horizon should be organized by the same three tracks:

### Product Maturity

Shows how the project becomes more useful and more coherent:

- CLI/MCP hardening
- scorecard and report UX maturity
- clear supported-vs-experimental boundaries
- path toward centralized analytics parity

### Ecosystem Integrations

Shows how Evidra fits inside the CNCF and platform engineering ecosystem:

- expand existing OTLP/OTel metrics/export support
- Prometheus/Grafana adoption assets
- CloudEvents alignment
- SARIF ingestion
- in-toto compatibility
- onboarding assets such as CI integrations

### Community, Governance, and Trust

Shows the maturity signals CNCF reviewers and adopters care about:

- contributor guidance
- release discipline
- security metadata and self-assessment path
- transparency around supported and experimental surfaces
- evidence of adoption readiness

## Content Rules

### Rule 1: No fake certainty

Use time horizons instead of speculative version numbers unless a release is
already committed.

### Rule 2: Separate current capabilities from future work

Already-shipped or partially implemented features belong in `Current State` or
must be phrased as `expand`, `harden`, or `document`.

This is especially important for:

- OTLP/OTel metrics export
- CLI/MCP analytics
- self-hosted ingestion and browsing

### Rule 3: Keep the roadmap adoption-oriented

Milestones should be framed in terms of adoption leverage, ecosystem fit, and
trust building, not only engineering backlog completion.

### Rule 4: Stay honest about the product boundary

CLI and MCP are the authoritative analytics surfaces today.
Self-hosted remains part of the project, but hosted analytics are
experimental until parity exists.

## Initial Milestone Themes

### Now (0-6 months)

- harden the supported CLI/MCP analytics path
- improve scorecard/report usability and first-run experience
- publish Prometheus/Grafana examples and operational assets
- expand and harden existing OTLP/OTel support
- clarify self-hosted experimental boundaries
- tighten security, release, and contributor documentation

### Next (6-12 months)

- deliver a real server-side analytics parity path for self-hosted
- add practical compatibility adapters for CloudEvents, SARIF, and in-toto
- improve CI/CD onboarding and packaging
- add targeted workload/provider coverage without scanner sprawl
- grow contributor and adopter proof points

### Later (12+ months)

- mature centralized or fleet analytics
- deepen ecosystem integrations based on demonstrated demand
- strengthen provenance, export, and enterprise evidence packaging
- scale governance and maintainer sustainability

## Expected Outcome

The final roadmap should read as:

- a believable execution path for the project team
- a credible maturity signal for CNCF sandbox review
- a practical adoption map for platform and observability users
