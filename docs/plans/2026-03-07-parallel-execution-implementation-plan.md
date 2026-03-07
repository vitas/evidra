# Parallel Execution Implementation Plan (v1)

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Ship one release with deterministic detector growth, Docker coverage, LLM augmentation, and a validated signal-engine score distribution (P0 release gate).

**Architecture:** Execute 4 streams in parallel: Track 1 (detectors), Track 2 (LLM), Track 3 (Docker), Track 4 (signals). Track 4 is the gating stream; others add breadth but do not replace gating criteria.

**Tech Stack:** Go (`cmd/evidra`, `cmd/evidra-exp`), Bash harnesses, MCP inspector tests, prompt contracts, optional REST integration.

---

## Source Baseline

- Strategy source: this document (consolidated from prior parallel execution strategy notes)
- Validation baseline: [`tests/signal-validation/README.md`](../../tests/signal-validation/README.md)
- Validation scripts:
  - [`tests/signal-validation/helpers.sh`](../../tests/signal-validation/helpers.sh)
  - [`tests/signal-validation/validate-signals-engine.sh`](../../tests/signal-validation/validate-signals-engine.sh)

## Consolidated Strategy (migrated)

Key strategic assumptions consolidated here:

- Deterministic detectors are baseline signal quality.
- LLM augmentation is additive breadth, never a release-gate substitute.
- Docker support is first-class for infrastructure automation parity.
- Signal-engine score distribution is the P0 product gate.

Parallel streams:

- **S1 (Detectors):** expand deterministic tag vocabulary.
- **S2 (LLM):** agreement experiments first, REST integration second.
- **S3 (Docker):** Docker detector coverage on top of completed adapter.
- **S4 (Signals):** scripted sequence validation with hard pass/fail gating.

## Delivery Rules

- `P0`: Track 4 signal distribution must pass before release sign-off.
- `P1`: Detector count and Docker/LLM are additive value, not gate substitutes.
- `P1`: Hosted/REST experiment mode remains opt-in and disabled by default.
- `P1`: All new behavior must be covered by deterministic tests (no manual-only verification).
- `P1`: `local-rest` execution is out of release scope for this phase (deferred until framework migration); keep only a documented stub boundary.

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
3. Add workspace cleanup at script start for idempotent reruns. Safety guard: require `WORKSPACE` starts with `/tmp/evidra-signal-validation` (strict prefix, not substring), is non-empty, and is not `/`. Abort with error if guard fails.
4. Add scripted coverage for repair/thrashing and keep artifact-drift coverage in a dedicated sequence.
5. Add `make test-signals` target that runs `validate-signals-engine.sh` so signal validation is wired into CI alongside `make test`.

**Files:**
- Create: [`tests/signal-validation/expected-bands.json`](../../tests/signal-validation/expected-bands.json)
- Modify: [`tests/signal-validation/validate-signals-engine.sh`](../../tests/signal-validation/validate-signals-engine.sh)
- Modify: `Makefile` (add `test-signals` target)

**Verification:**
- `make test-signals`
- Expected: all 8 sequences run, summary table printed, exit 0 only if all scores/signals/comparisons pass expected bands.

### M1: P0 Gate - Signal Engine Distribution (Day 1)

**Outcome:** Signal engine proves meaningful differentiation across scripted behaviors.

1. Run scripted sequences A-H (8 sequences). Operation counts remain lightweight (10-20 ops per sequence), with score sufficiency controlled via `--min-operations` override in validation harness.
2. Save run artifacts (`scorecard` + `explain` outputs) under a dated results folder.
3. Automated assertions (from M0) enforce expected score bands and signal minimums per sequence:
   - A clean: `99-100`
   - B retry: `75-85`
   - C protocol violation: `85-90`
   - D blast radius: `95-99`
   - E scope escalation: `98-100`
   - F repair: `75-85`
   - G thrashing: `70-80`
   - H artifact drift: `84-86`
4. Script emits `summary.json` to results folder with per-sequence scores and pass/fail.

**Files:**
- Create: `experiments/results/signals/<date>/summary.json` (emitted by validation script)
- Create: `experiments/results/signals/<date>/sequence-*.json`

**Verification:**
- `make test-signals`
- `ls experiments/results/signals/*/summary.json` (confirm artifacts emitted)
- Expected: exit 0, 8 distinct signal profiles; comparisons `F_repair > B_retry` and `G_thrash < B_retry` enforced.

### M2: Deterministic Detector Expansion (Week 1)

**Outcome:** Increase deterministic detector coverage from 7 to 16.

1. Implement K8s detectors: `k8s.docker_socket`, `k8s.run_as_root`, `k8s.dangerous_capabilities`, `k8s.cluster_admin_binding`, `k8s.writable_rootfs`, `ops.kube_system`.
2. Implement Terraform/AWS detectors: `tf.security_group_open`, `tf.rds_public`, `tf.unencrypted_volume`.
3. Register detectors in default registry.
4. Add fixture-driven tests and goldens per detector.

**Files:**
- Modify: `internal/detectors/**/*.go` (new detector implementations)
- Modify: `internal/detectors/**/*_test.go`
- Modify: detector registration surface (`internal/detectors/all/all.go`)
- Create/Modify: fixture files under test fixtures directory

**Verification:**
- `go test ./internal/detectors/... -count=1`
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
- Create/Modify: `internal/detectors/docker/*.go`
- Create/Modify: `internal/detectors/docker/*_test.go`
- Modify: `internal/detectors/all/all.go` (ensure docker package imported)

**Verification:**
- `go test ./internal/detectors/docker/... -count=1`
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

1. Run MCP inspector E2E in local-mcp mode.
2. Re-run signal-engine validation and compare against M1 baseline.
3. Verify prompt contracts match implemented behavior.
4. Publish release checklist result table (pass/fail + artifact links).
5. Upload signal-validation outputs (`summary.json` + per-sequence JSON) as CI artifacts for review/audit.

> `local-rest` is out of scope for this phase (see Delivery Rules). Do not claim REST coverage without explicit local-rest run evidence.

**Files:**
- Modify: `.github/workflows/ci.yml` (or release workflow) to upload `experiments/results/signals/**` artifacts from signal-validation runs.
- Modify: `tests/signal-validation/validate-signals-engine.sh` to emit outputs to deterministic paths under `experiments/results/signals/`.

**Verification:**
- `make test-mcp-inspector-ci`
- `make test-signals`
- `make prompts-verify`
- `find experiments/results/signals -name summary.json -print -quit | grep -q .` (confirm local signal outputs exist before sign-off)
- Verify CI run publishes a `signals-validation` artifact from `experiments/results/signals/**` and link that artifact in the release checklist.
- `go test ./... -count=1`

## Tracking Board

- [x] M0 baseline normalization completed (expected-bands.json, CI assertions, workspace safety guard, A-H scripted coverage)
- [x] M1 P0 signal gate passed and artifacts stored (`experiments/results/signals/*/summary.json`, assertions green)
- [x] M2 detectors #8-#16 merged
- [x] M3 Docker risk detectors #17-#19 merged (adapter already complete)
- [ ] M4a LLM experiment completed and results stored
- [ ] M4b REST API integration merged (blocked on REST API work item)
- [ ] M5 release hardening complete (`local-mcp` gate + CI artifact upload for signal-validation outputs)

## Risks And Mitigations

- Risk: signal scores collapse into one band.
  - Mitigation: treat as release blocker; tune weights/thresholds before continuing breadth work.
- Risk: detector growth introduces false positives.
  - Mitigation: fixture-driven positive/negative tests per detector and regression suite reruns.
- Risk: LLM behavior instability.
  - Mitigation: strict output contract validation + graceful fallback to deterministic path.
- Risk: release appears green without reviewable signal-validation evidence.
  - Mitigation: CI artifact upload is mandatory; release checklist must link artifact URLs.

## Exit Criteria

- P0 gate (M1) is green with stored evidence artifacts (8 sequences, all within bands and comparisons).
- All 7 behavioral signals have validation coverage (`protocol_violation`, `artifact_drift`, `retry_loop`, `blast_radius`, `new_scope`, `repair_loop`, `thrashing`).
- Deterministic detector count reaches planned minimum for release scope.
- Docker artifacts are first-class inputs (adapter complete; risk detectors merged).
- LLM experiment (M4a) completed with stored metrics. REST integration (M4b) conditional on external dependency.
- CI (tests + MCP inspector) is green with documented release checklist and published signal-validation artifacts.
