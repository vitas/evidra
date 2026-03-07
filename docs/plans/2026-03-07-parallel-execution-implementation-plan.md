# Parallel Execution Implementation Plan (v1)

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Ship one release with deterministic detector growth, Docker coverage, LLM augmentation, and a validated signal-engine score distribution (P0 release gate).

**Architecture:** Execute 4 streams in parallel from [`PARALLEL_EXECUTION_PLAN.md`](./PARALLEL_EXECUTION_PLAN.md): Track 1 (detectors), Track 2 (LLM), Track 3 (Docker), Track 4 (signals). Track 4 is the gating stream; others add breadth but do not replace gating criteria.

**Tech Stack:** Go (`cmd/evidra`, `cmd/evidra-exp`), Bash harnesses, MCP inspector tests, prompt contracts, optional REST integration.

---

## Source Baseline

- Strategy source: [`PARALLEL_EXECUTION_PLAN.md`](./PARALLEL_EXECUTION_PLAN.md)
- Validation baseline: [`tests/signal-validation/README.md`](../../tests/signal-validation/README.md)
- Validation scripts:
  - [`tests/signal-validation/helpers.sh`](../../tests/signal-validation/helpers.sh)
  - [`tests/signal-validation/validate-signals-engine.sh`](../../tests/signal-validation/validate-signals-engine.sh)

## Delivery Rules

- `P0`: Track 4 signal distribution must pass before release sign-off.
- `P1`: Detector count and Docker/LLM are additive value, not gate substitutes.
- `P1`: Hosted/REST experiment mode remains opt-in and disabled by default.
- `P1`: All new behavior must be covered by deterministic tests (no manual-only verification).

## Stream Map (Parallel)

| Stream | Scope | Priority | Can start | Blocks |
|---|---|---|---|---|
| S4 | Signal-engine validation + score distribution | P0 | Immediately | Release |
| S1 | New deterministic detectors (#8-#16, then #20) | P1 | Immediately | Final detector target |
| S3 | Docker risk detectors (#17-#19) | P1 | After S4 baseline run | Full multi-tool coverage |
| S2a | LLM baseline experiment (agreement measurement) | P1 | After S4 baseline run | LLM breadth metrics |
| S2b | REST API integration (depends on external work item) | P1 | After REST API layer available | LLM-augmented API |

> **Note:** Docker adapter (`internal/canon/docker.go`) is complete and tested. S3 scope is Docker *risk detectors* only.
> **Note:** REST API layer is not yet imported. S2b has a dependency on a separate work item. S2a (experiment) can proceed independently.

## Milestones And Tasks

### M0: Baseline Normalization (Day 0)

**Outcome:** Validation baseline has machine-readable expected ranges and automated pass/fail for CI.

> Scripts and paths are already normalized. The remaining work is CI-grade assertions.

1. Create `tests/signal-validation/expected-bands.json` with per-sequence score ranges.
2. Add automated band assertions to `validate-signals-engine.sh` that exit non-zero on out-of-range scores.
3. Add workspace cleanup (`rm -rf "$WORKSPACE"`) at script start for idempotent reruns.
4. Add Sequence F: artifact drift (prescribe artifact X, report with different shape hash) to cover the `artifact_drift` signal — currently the only signal with zero validation coverage.

**Files:**
- Create: [`tests/signal-validation/expected-bands.json`](../../tests/signal-validation/expected-bands.json)
- Modify: [`tests/signal-validation/validate-signals-engine.sh`](../../tests/signal-validation/validate-signals-engine.sh)

**Verification:**
- `bash tests/signal-validation/validate-signals-engine.sh`
- Expected: all 6 sequences run, summary table printed, exit 0 only if all scores within expected bands.

### M1: P0 Gate - Signal Engine Distribution (Day 1)

**Outcome:** Signal engine proves meaningful differentiation across scripted behaviors.

1. Run scripted sequences A-F (6 sequences, ~110 operations total).
2. Save run artifacts (`scorecard` + `explain` outputs) under a dated results folder.
3. Automated assertions (from M0) enforce expected score bands per sequence:
   - A clean: `90-100`
   - B retry: `50-70`
   - C protocol violation: `40-65`
   - D blast radius: `60-80`
   - E scope escalation: `80-95`
   - F artifact drift: TBD (set after first run)
4. Script emits `summary.json` to results folder with per-sequence scores and pass/fail.

**Files:**
- Create: `experiments/results/signals/<date>/summary.json` (emitted by validation script)
- Create: `experiments/results/signals/<date>/sequence-*.json`

**Verification:**
- `bash tests/signal-validation/validate-signals-engine.sh`
- Expected: exit 0, 6 distinct score profiles; no collapsed distribution.

### M2: Deterministic Detector Expansion (Week 1)

**Outcome:** Increase deterministic detector coverage from 7 to 16.

1. Implement K8s detectors: `k8s.docker_socket`, `k8s.run_as_root`, `k8s.dangerous_capabilities`, `k8s.cluster_admin_binding`, `k8s.writable_rootfs`, `ops.kube_system`.
2. Implement Terraform/AWS detectors: `tf.security_group_open`, `tf.rds_public`, `tf.unencrypted_volume`.
3. Register detectors in default registry.
4. Add fixture-driven tests and goldens per detector.

**Files:**
- Modify: `internal/risk/*.go` (new detector implementations)
- Modify: `internal/risk/*_test.go`
- Modify: detector registration surface (`DefaultDetectors` path)
- Create/Modify: fixture files under test fixtures directory

**Verification:**
- `go test ./internal/risk/... -count=1`
- `go test ./... -count=1`
- Expected: new tags emitted for positive fixtures; no regressions for existing tags.

### M3: Docker Risk Detectors (Week 2 Start)

**Outcome:** Docker risk detectors added; Docker artifacts produce risk tags end-to-end.

> Docker adapter (`internal/canon/docker.go`) and its tests (`docker_test.go`) are already complete.
> Adapter handles docker, nerdctl, podman, lima (command strings) and docker-compose, compose (YAML).
> This milestone is Docker *risk detectors* only.

1. Add Docker detectors: `docker.privileged`, `docker.host_network`, `docker.socket_mount`.
2. Register detectors in `DefaultDetectors()`.
3. Add fixture-driven tests with compose YAML fixtures (positive and negative).

**Files:**
- Create: `internal/risk/docker_detectors.go`
- Create: `internal/risk/docker_detectors_test.go`
- Modify: `internal/risk/detectors.go` (register in `DefaultDetectors`)

**Verification:**
- `go test ./internal/risk/... -count=1`
- Expected: compose artifacts with privileged/host_network/socket_mount emit correct Docker tags; clean compose artifacts emit no Docker tags.

### M4a: LLM Baseline Experiment (Week 2)

**Outcome:** LLM agreement rate measured against Go detectors on curated artifacts.

1. Run baseline agreement experiment (Go vs LLM on curated artifact set).
2. Run multi-model comparison; capture compliance and disagreement metrics.
3. Store experiment results as structured JSON in `experiments/results/llm/`.

**Files:**
- Modify: experiment runner paths (`cmd/evidra-exp`, `internal/experiments`)
- Create: experiment result artifacts under `experiments/results/llm/`

**Verification:**
- `go test ./cmd/evidra-exp ./internal/experiments -count=1`
- Expected: agreement metrics produced; candidate tag counts within expected range.

### M4b: REST API Integration (Week 2+)

**Outcome:** LLM prediction integrated into REST API, safely degradable.

> **Dependency:** REST API layer is not yet imported into this repository. This milestone is blocked until the REST API work item is available. M4a (experiment) proceeds independently.

1. Integrate LLM merge path in REST API behind opt-in flag (`hosted/rest` default OFF).
2. Keep failure mode graceful: if LLM fails, deterministic result still returns.
3. API integration tests with LLM off/on toggle.

**Files:**
- Modify: REST integration layer (when available)
- Modify: docs for run/flags and expected outputs

**Verification:**
- API integration tests with LLM off/on toggle
- Expected: API returns deterministic baseline even when LLM unavailable.

### M5: Release Hardening And Gate Review

**Outcome:** Release candidate is gated by evidence, not assumptions.

1. Run MCP inspector E2E (local + local REST; hosted/rest disabled by default).
2. Re-run signal-engine validation and compare against M1 baseline.
3. Confirm docs and prompts match implemented behavior.
4. Publish release checklist result table (pass/fail + artifact links).

**Verification:**
- `make test-mcp-inspector-ci`
- `bash tests/signal-validation/validate-signals-engine.sh`
- `go test ./... -count=1`

## Tracking Board

- [ ] M0 baseline normalization completed (expected-bands.json, CI assertions, Sequence F for artifact_drift)
- [ ] M1 P0 signal gate passed and artifacts stored (6 sequences, exit 0)
- [ ] M2 detectors #8-#16 merged
- [ ] M3 Docker risk detectors #17-#19 merged (adapter already complete)
- [ ] M4a LLM experiment completed and results stored
- [ ] M4b REST API integration merged (blocked on REST API work item)
- [ ] M5 release hardening complete

## Risks And Mitigations

- Risk: signal scores collapse into one band.
  - Mitigation: treat as release blocker; tune weights/thresholds before continuing breadth work.
- Risk: detector growth introduces false positives.
  - Mitigation: fixture-driven positive/negative tests per detector and regression suite reruns.
- Risk: LLM behavior instability.
  - Mitigation: strict output contract validation + graceful fallback to deterministic path.

## Exit Criteria

- P0 gate (M1) is green with stored evidence artifacts (6 sequences, all within bands).
- All 5 behavioral signals have validation coverage (including artifact_drift via Sequence F).
- Deterministic detector count reaches planned minimum for release scope.
- Docker artifacts are first-class inputs (adapter complete; risk detectors merged).
- LLM experiment (M4a) completed with stored metrics. REST integration (M4b) conditional on external dependency.
- CI (tests + MCP inspector) is green with documented release checklist.
