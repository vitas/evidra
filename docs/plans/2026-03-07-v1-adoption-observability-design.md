# V1 Adoption + Observability Design

**Date:** 2026-03-07  
**Status:** Approved design (brainstorming outcome)  
**Primary KPI:** time-to-first-value <= 10 minutes  
**Architecture Requirement:** observability fit in existing Grafana/Prometheus stacks  
**Next Stage Priority:** pipeline adoption with minimal CI disruption

---

## 1. Problem and Product Direction

The main adoption risk is pipeline friction: if users must redesign CI/CD flows, adoption drops.
V1 therefore prioritizes fast onboarding and immediate value over platform maximalism.

Position for V1:
- Adoption-first execution model
- Existing Evidra engine remains source of scoring/signals truth
- Two operator realities are first-class:
  - local commands (`run`)
  - CI/CD integration (`record`)

This keeps current product momentum while preserving a clean path to a broader Automation Event standard in v1.x.

---

## 2. Design Goals

1. New user can install and see first useful signals/score in <= 10 minutes.
2. Teams can adopt without replacing their observability stack.
3. CI teams can integrate without intrusive command rewrites.
4. `run` and `record` feed one shared scoring/signals engine.
5. Security/compliance hardening is available, but not required on day 1.

---

## 3. Scope Decision

### In scope (V1)

- `evidra run -- <command>` as primary onboarding and local usage path.
- `evidra record` as CI-friendly ingestion path.
- Zero-setup default profile (best-effort evidence/signing).
- OTel/Prometheus **metrics-first** export.
- Correlation fields for drill-down (`session_id`, `operation_id`, `prescription_id`).
- Internal stable Automation Event Contract used by both `run` and `record`.

### Out of scope (V1)

- Full OTel traces/logs implementation as a primary mode.
- Public external spec/SDK standardization.
- Mandatory strict signed evidence for all users by default.

---

## 4. Architecture Overview

### 4.1 Ingestion Interfaces

Normative boundary:
- `run` = Evidra executes and observes the command live.
- `record` = Evidra ingests a completed automation execution from structured input.

1. `run` interface
- Wraps a real command.
- Auto-captures intent/execution/result.
- Auto-links prescribe/report flow.

2. `record` interface
- Accepts structured execution data from CI steps.
- Validates and normalizes payload.
- Submits into the same downstream flow as `run`.

### 4.2 Shared Core (unchanged source of truth)

Both interfaces feed the same existing core components:
- adapters
- detectors
- risk matrix
- evidence chain
- signals engine
- scorecard

No separate scoring implementation is introduced.

### 4.3 Automation Event Contract (internal in V1)

Canonical event model used by both interfaces:
- `Intent`
- `Execution`
- `Result`
- `EvidenceRef`
- `SignalsSnapshot`

This contract is the bridge to future v1.x spec/SDK without requiring current architectural rewrite.

Minimum required fields in v1 (mandatory contract surface):

- `contract_version`
- `session_id`
- `operation_id`
- `tool`
- `operation`
- `actor`
- `environment`
- `exit_code`
- `duration_ms`
- `risk_level`
- `risk_classification`
- `signal_summary`

Normative schema and validation details are defined in:

- `docs/system-design/V1_RUN_RECORD_CONTRACT.md`

---

## 5. Data Flow

### 5.1 `evidra run -- <command>` flow

1. Parse command context (`tool`, `operation`, `environment`, artifact hints).
2. Emit internal `Intent` event.
3. Invoke prescribe pipeline (adapters -> detectors -> risk level).
4. Execute wrapped command.
5. Capture `exit_code`, duration, and execution metadata.
6. Emit `Result` event and invoke report.
7. Export reliability and operational metrics.

### 5.2 `evidra record` flow

1. Receive structured CI payload.
2. Validate against Automation Event Contract.
3. Normalize fields and map into internal operation/evidence model.
4. Run report/scoring/signals path.
5. Export the same metric family used by `run`.

### 5.3 Parity Rule

Equivalent operations processed via `run` and `record` must yield equivalent signals and scorecard outcomes.

---

## 6. Reliability and Failure Behavior

### 6.1 `run` failure policy

- Default is **fail-open**.
- If Evidra capture/export fails, wrapped user command still executes.
- Failures are surfaced via local diagnostics/events.

### 6.2 `record` failure policy

- **Fail-visible** with structured validation errors.
- CI can choose warn/block policy.
- Invalid payloads are never silently dropped.

---

## 7. Profiles and Adoption Roadmap

### 7.1 Profile decision

Default profile for V1:
- zero-setup
- best-effort evidence write/signing
- no mandatory key management

### 7.2 Roadmap progression (approved)

1. `default` (launch): zero-setup adoption profile.
2. `hardened`: optional persistent keys and stronger evidence guarantees.
3. `strict-ci`: policy-gated CI profile with strict signing/completeness.

### 7.3 Scoring sufficiency policy (v1)

- Canonical reliability sufficiency threshold remains `MinOperations=100`.
- V1 first-use UX must not block on 100 operations. Early output is allowed as preview with explicit basis labeling.
- `run` output should distinguish:
  - preview assessment (below sufficiency threshold)
  - sufficient assessment (at or above threshold)
- CLI already supports configurable threshold via `--min-operations`; v1 quickstart should use this deliberately for demos/small teams, while keeping default governance semantics intact.

---

## 8. Observability Fit (V1 requirement)

V1 uses **metrics-first** integration.

Why metrics-first (not traces-first) in v1:

- Fits existing Grafana/Prometheus stacks with the lowest operational friction.
- Faster implementation path for <=10 minute time-to-first-value.
- Lower runtime and ownership complexity for platform teams.
- Traces/logs can be added later on top of the same event contract without redesigning ingestion.

### 8.1 Metric groups

1. Reliability metrics
- score/band distribution
- signal counts and rates
- protocol violations, retry/thrashing, artifact drift trends

2. Adoption metrics
- operations ingested
- sessions scored
- insufficient-scorecard counts

3. Operational metrics
- wrapped command duration
- report latency
- exporter and ingestion errors

### 8.2 Correlation

Every metric/event path should expose correlation IDs where appropriate:
- `session_id`
- `operation_id`
- `prescription_id`

This enables drill-down in existing observability systems without mandatory custom UI.

Cardinality discipline (mandatory):

- Correlation IDs are not high-cardinality metric labels by default.
- Default metric labels must stay bounded (tool, environment, result class, signal name, band).
- Correlation IDs are exposed via controlled drill-down pathways (event payloads, evidence records, optional debug endpoints/exemplars), not broad timeseries dimensions.

### 8.3 Fleet view before centralized API

- Until centralized API rollout, fleet-level visibility is provided via aggregated metrics in existing observability stacks.
- Local JSONL evidence remains the durable local record; fleet dashboards/alerts come from metrics pipelines (Prometheus scrape/remote_write and standard Grafana views).
- v1 does not require a centralized evidence database for initial multi-cluster value, but it keeps compatibility with a future API-backed aggregation layer.

---

## 9. Acceptance Criteria

1. First useful output can be obtained within 10 minutes from install.
2. For v1, `run` first useful output MUST include at minimum:
- `risk_classification`
- `risk_level`
- `score` or `score_band`
- evaluated `signal_summary`
- confidence or basis indicator (for example: operation count sufficiency / evidence basis)
3. If operation count is below sufficiency threshold, output MUST be clearly labeled as preview (not final reliability grade).
4. Metrics appear in existing observability stack with no bespoke UI dependency.
5. `run` and `record` parity holds for equivalent inputs.
6. Default -> strict profile migration is incremental, not architectural.

---

## 10. Testing Strategy (design-level)

1. Contract tests for `run` and `record` event normalization.
2. Parity tests across both ingestion paths.
3. Failure-path tests (`run` fail-open, `record` fail-visible).
4. Metrics schema/cardinality stability tests.
5. KPI test script for <=10 minute time-to-first-value.

---

## 11. Deliverables from This Design

1. System design updates for ingestion model (`run` + `record`) and internal event contract.
2. Implementation plan with phased tasks (A: TTFV, B: observability fit, C: pipeline adoption).
3. 10-minute quickstart documentation.
4. CI integration guide for `record`.
5. OTel metrics reference and starter alerts.
6. CLI command layering and help-text updates reflecting primary/integration/advanced protocol modes.
7. Publish standalone `setup-evidra` GitHub Action (Marketplace-ready) so CI onboarding is one-line and does not depend on local path usage.

---

## 12. Chosen Approach Record

Approach selected during brainstorming:
- **Adoption-first with contracted core**

Why:
- hits primary KPI fastest
- satisfies observability requirement in current customer stacks
- preserves clear evolution path to broader event standard later

---

## 13. CLI UX Model (Integrated from CLI Redesign Review)

This section incorporates user-authored CLI redesign feedback and aligns it to approved V1 goals.

### 13.1 Three command layers

1. Primary runtime capture (first-use path)
- `evidra run -- <command>`
- `evidra scorecard`
- `evidra explain`

2. Integration mode (CI/platform ingestion)
- `evidra record`
- `evidra ingest-findings`

3. Advanced evidence/protocol mode (power users, regulated environments)
- `evidra prescribe`
- `evidra report`
- `evidra validate`
- `evidra keygen`

Design intent:
- Keep cognitive load low for first-use users.
- Preserve protocol depth for audit/compliance workflows.
- Avoid splitting the underlying engine or semantic model.

### 13.2 Command hierarchy target

```text
evidra
  run
  scorecard
  explain
  record
  ingest-findings
  prescribe
  report
  validate
  keygen
  detectors
  prompts
  benchmark
  version
```

This is a UX hierarchy, not a new architecture. Existing command behavior remains backward-compatible unless explicitly deprecated.

For v1 prioritization:
- primary story: `run` / `record` / `scorecard` / `explain`
- secondary story: `benchmark` (experimental/aggregation-oriented, not first-use north star)

### 13.3 Backward compatibility contract

1. `run` is orchestration, not a second engine.
2. Existing `prescribe -> report` workflows remain supported unchanged.
3. `run` is a convenience orchestration path over existing lifecycle primitives.
4. CI teams may choose `record` or existing explicit protocol flows.
5. Migration is additive, not breaking.
