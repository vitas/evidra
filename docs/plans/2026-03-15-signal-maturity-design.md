# Signal Maturity Design

**Date:** 2026-03-15
**Repo:** `evidra-benchmark`
**Status:** Approved design

## Goal

Make signal maturity evidence-driven. `evidra-benchmark` should become the source of truth for which signals are claim-grade, which are still maturing, and why.

## Product Framing

The landing page already draws the correct boundary:

- claim-grade today: `protocol_violation`, `retry_loop`, `blast_radius`
- maturing signals: `artifact_drift`, `new_scope`, `repair_loop`, `thrashing`, `risk_escalation`

The system should preserve that split in code, API, UI, and docs.

## Model

Each signal gets a maturity record with:

- `tier`: `gold` or `shadow`
- `status`: `claim`, `shadow`, or `hold`
- `precision`
- `recall`
- `false_positive_rate`
- `stability`
- positive / negative / near-miss case counts
- `graduation_blockers`
- references to supporting runs and artifacts

`gold` means product-priority and release-blocking. `shadow` means scored and visible, but not safe for public claims yet.

## Evidence Lanes

Two evidence lanes feed the same maturity summary:

1. Deterministic maturity fixtures in `evidra-benchmark`
   These are curated cases used to tune detector semantics, thresholds, and regression safety.

2. Imported real-run observations from `infra-bench`
   These measure repeatability and robustness under real agent behavior.

The UI and reports must keep these lanes visibly separate. A signal that looks good only in fixtures must not be treated as claim-grade.

## API

Add a dedicated maturity API rather than overloading benchmark runs:

- `POST /v1/maturity/runs`
- `GET /v1/maturity/runs/:id`
- `GET /v1/maturity/signals`
- `GET /v1/maturity/signals/:name`

Later:

- `POST /v1/maturity/import/infra-bench`

## UI

First cut:

- Maturity overview page with one row per signal
- Signal detail page with metrics, blockers, deterministic vs real-run evidence split, and raw artifact links

The landing page should eventually read claim status from maturity data instead of hand-maintained copy.

## Promotion Rules

Signals only move from `shadow` to `claim` through explicit graduation rules:

- enough positive, negative, and near-miss cases
- precision above threshold
- false-positive rate below threshold
- stable repeated behavior
- understandable operator-facing explanation

Signals may also be demoted automatically when claim-grade evidence regresses.

## Documentation Requirements

The implementation must update:

- public signal docs
- API docs / OpenAPI
- operator docs for maturity interpretation
- contributor docs for adding new maturity cases

Documentation is part of the feature, not follow-up work.
