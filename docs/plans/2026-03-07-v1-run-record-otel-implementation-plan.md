# V1 Run/Record + OTel Metrics Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Deliver `evidra run` and `evidra record` as adoption-first UX on top of the existing prescribe/report engine, with explicit metrics transport and clear preview-vs-sufficient scoring behavior.

**Architecture:** Keep lifecycle and scoring logic single-source (`internal/lifecycle`, `internal/signal`, `internal/score`). Add thin orchestration adapters (`run`, `record`) plus a strict record contract and bounded-cardinality metrics exporter. Do not create a second scoring engine in CLI.

**Tech Stack:** Go 1.23, existing lifecycle/evidence packages, OpenTelemetry metrics SDK (OTLP/HTTP), JSON contract validation, GitHub composite actions.

---

### Task 1: Define the `record` Contract and Validator (Risk #2)

**Files:**
- Create: `internal/automationevent/record_contract.go`
- Create: `internal/automationevent/record_contract_test.go`
- Create: `docs/system-design/V1_RUN_RECORD_CONTRACT.md`
- Modify: `docs/plans/2026-03-07-v1-adoption-observability-design.md`

**Step 1: Write the failing tests for required fields and contract rules**

```go
func TestRecordContract_Validate_RequiredFields(t *testing.T) {
    in := RecordInput{}
    err := ValidateRecordInput(in)
    if err == nil {
        t.Fatal("expected validation error")
    }
}

func TestRecordContract_Validate_RequiresArtifactOrCanonicalAction(t *testing.T) {
    in := RecordInput{
        ContractVersion: "v1",
        SessionID:       "sess_1",
        OperationID:     "op_1",
        Tool:            "terraform",
        Operation:       "apply",
        Environment:     "staging",
        ExitCode:        0,
        DurationMs:      4200,
    }
    err := ValidateRecordInput(in)
    if err == nil {
        t.Fatal("expected error when both raw_artifact and canonical_action are missing")
    }
}
```

**Step 2: Run test to verify failure**

Run: `go test ./internal/automationevent -run RecordContract -count=1`
Expected: FAIL (missing package/types/functions)

**Step 3: Implement minimal contract + validator**

```go
type RecordInput struct {
    ContractVersion string            `json:"contract_version"`
    SessionID       string            `json:"session_id"`
    OperationID     string            `json:"operation_id"`
    Tool            string            `json:"tool"`
    Operation       string            `json:"operation"`
    Environment     string            `json:"environment"`
    Actor           evidence.Actor    `json:"actor"`
    ExitCode        int               `json:"exit_code"`
    DurationMs      int64             `json:"duration_ms"`
    Attempt         int               `json:"attempt,omitempty"`
    RawArtifact     string            `json:"raw_artifact,omitempty"`
    CanonicalAction json.RawMessage   `json:"canonical_action,omitempty"`
    ScopeDimensions map[string]string `json:"scope_dimensions,omitempty"`
}

func ValidateRecordInput(in RecordInput) error { /* required fields + one-of checks */ }
```

**Step 4: Run tests to pass**

Run: `go test ./internal/automationevent -count=1`
Expected: PASS

**Step 5: Commit**

```bash
git add internal/automationevent docs/system-design/V1_RUN_RECORD_CONTRACT.md docs/plans/2026-03-07-v1-adoption-observability-design.md
git commit -m "feat(contract): add v1 record input contract and validation"
```

---

### Task 2: Add Root `record` Command That Reuses Lifecycle Service

**Files:**
- Modify: `cmd/evidra/main.go`
- Create: `cmd/evidra/record.go`
- Create: `cmd/evidra/record_test.go`

**Step 1: Write failing CLI tests (`record` success + validation failure)**

```go
func TestRecordCommand_WritesPrescribeAndReport(t *testing.T) {
    // arrange temp evidence dir + valid JSON input file
    // run: evidra record --input record.json --evidence-dir ... --signing-key ...
    // assert: output contains session_id/operation_id/prescription_id/report_id
    // assert: evidence contains one prescribe + one report
}

func TestRecordCommand_RejectsInvalidPayload(t *testing.T) {
    // missing required fields
    // assert: exit code 2 and stable validation error
}
```

**Step 2: Run tests to verify failure**

Run: `go test ./cmd/evidra -run RecordCommand -count=1`
Expected: FAIL (`record` command does not exist)

**Step 3: Implement `record` command and mapping to lifecycle**

- Add `case "record": return cmdRecord(args[1:], stdout, stderr)` in dispatcher.
- Parse `--input <path|->`, `--evidence-dir`, signing flags, and optional overrides.
- Parse JSON into `automationevent.RecordInput` and validate.
- Build `lifecycle.PrescribeInput`, call `Prescribe`.
- Build `lifecycle.ReportInput`, call `Report`.
- Emit canonical JSON result with IDs and execution summary.

**Step 4: Run tests to pass**

Run: `go test ./cmd/evidra -run RecordCommand -count=1`
Expected: PASS

**Step 5: Commit**

```bash
git add cmd/evidra/main.go cmd/evidra/record.go cmd/evidra/record_test.go
git commit -m "feat(cli): add root record command using lifecycle pipeline"
```

---

### Task 3: Extract Shared Operation Processor (Guardrail: No Second Engine)

**Files:**
- Create: `cmd/evidra/operation_processor.go`
- Create: `cmd/evidra/operation_processor_test.go`
- Modify: `cmd/evidra/record.go`

**Step 1: Write failing processor tests**

```go
func TestProcessOperation_UsesSingleLifecyclePath(t *testing.T) {
    // fake/spy lifecycle service
    // assert prescribe called once, report called once
}
```

**Step 2: Run tests to fail**

Run: `go test ./cmd/evidra -run ProcessOperation -count=1`
Expected: FAIL

**Step 3: Implement shared processor used by `record` and future `run`**

```go
type OperationProcessor struct { /* service + options */ }
func (p *OperationProcessor) Process(ctx context.Context, req OperationRequest) (OperationResult, error)
```

**Step 4: Run tests to pass**

Run: `go test ./cmd/evidra -run ProcessOperation -count=1`
Expected: PASS

**Step 5: Commit**

```bash
git add cmd/evidra/operation_processor.go cmd/evidra/operation_processor_test.go cmd/evidra/record.go
git commit -m "refactor(cli): add shared operation processor for run/record parity"
```

---

### Task 4: Implement `run` Command (Live Execution Orchestration)

**Files:**
- Modify: `cmd/evidra/main.go`
- Create: `cmd/evidra/run.go`
- Create: `cmd/evidra/run_test.go`

**Step 1: Write failing tests for run flow**

```go
func TestRunCommand_ExecutesCommandAndReportsOutcome(t *testing.T) {
    // run: evidra run --tool kubectl --operation apply --artifact <file> -- sh -c "exit 0"
    // assert: exit code from wrapped command is returned
    // assert: prescribe+report entries exist
}

func TestRunCommand_FailOpenOnMetricsExportError(t *testing.T) {
    // force metrics exporter failure
    // wrapped command still executes and exit code is preserved
}
```

**Step 2: Run tests to fail**

Run: `go test ./cmd/evidra -run RunCommand -count=1`
Expected: FAIL (`run` command missing)

**Step 3: Implement `run` with explicit `--` boundary**

- Parse command arguments after `--`.
- Execute wrapped process via `os/exec`.
- Capture `exit_code`, `duration_ms`.
- Use `OperationProcessor` to emit prescribe/report via existing lifecycle service.
- Return JSON output containing required first-use fields.

**Step 4: Run tests to pass**

Run: `go test ./cmd/evidra -run RunCommand -count=1`
Expected: PASS

**Step 5: Commit**

```bash
git add cmd/evidra/main.go cmd/evidra/run.go cmd/evidra/run_test.go
git commit -m "feat(cli): add run command for live automation orchestration"
```

---

### Task 5: Implement Preview vs Sufficient Assessment (Risk #3)

**Files:**
- Create: `cmd/evidra/assessment.go`
- Create: `cmd/evidra/assessment_test.go`
- Modify: `cmd/evidra/run.go`
- Modify: `cmd/evidra/record.go`

**Step 1: Write failing tests for sufficiency semantics**

```go
func TestAssessment_BelowThresholdMarkedPreview(t *testing.T) {
    // total_ops < MinOperations
    // expect: assessment_mode="preview", sufficient=false
}

func TestAssessment_AtThresholdMarkedSufficient(t *testing.T) {
    // total_ops >= MinOperations
    // expect: assessment_mode="sufficient", sufficient=true
}
```

**Step 2: Run tests to fail**

Run: `go test ./cmd/evidra -run Assessment -count=1`
Expected: FAIL

**Step 3: Implement dual-score view without changing core scorer**

- Keep `internal/score` logic unchanged.
- Compute strict scorecard with default threshold.
- Compute preview scorecard with `min_operations=1`.
- Output includes:
  - `risk_classification`
  - `risk_level`
  - `score`/`score_band`
  - `signal_summary`
  - `basis` (`preview` vs `sufficient`, totals and threshold)

**Step 4: Run tests to pass**

Run: `go test ./cmd/evidra -run Assessment -count=1`
Expected: PASS

**Step 5: Commit**

```bash
git add cmd/evidra/assessment.go cmd/evidra/assessment_test.go cmd/evidra/run.go cmd/evidra/record.go
git commit -m "feat(cli): add preview vs sufficient assessment output"
```

---

### Task 6: Define and Implement Metrics Transport for CLI (Risk #1)

**Files:**
- Create: `internal/telemetry/transport.go`
- Create: `internal/telemetry/transport_test.go`
- Create: `internal/telemetry/otlp_http.go`
- Create: `internal/telemetry/noop.go`
- Create: `internal/config/metrics.go`
- Create: `internal/config/metrics_test.go`
- Modify: `cmd/evidra/run.go`
- Modify: `cmd/evidra/record.go`

**Step 1: Write failing transport/config tests**

```go
func TestResolveMetricsConfig_DefaultsToNone(t *testing.T) {}
func TestOTLPExporter_FlushesAtEnd(t *testing.T) {}
func TestMetricsLabels_BoundedCardinality(t *testing.T) {}
```

**Step 2: Run tests to fail**

Run: `go test ./internal/config ./internal/telemetry -count=1`
Expected: FAIL

**Step 3: Implement explicit transport contract**

- Add config/env:
  - `EVIDRA_METRICS_TRANSPORT=none|otlp_http`
  - `EVIDRA_METRICS_OTLP_ENDPOINT=<url>`
  - `EVIDRA_METRICS_TIMEOUT`
- Add exporter interface and OTLP/HTTP implementation.
- Emit bounded label set only:
  - `tool`, `environment`, `result_class`, `signal_name`, `score_band`, `assessment_mode`
- Emit correlation IDs only in event payload/debug output, not default labels.

**Step 4: Run tests to pass**

Run: `go test ./internal/config ./internal/telemetry -count=1`
Expected: PASS

**Step 5: Commit**

```bash
git add internal/telemetry internal/config/metrics.go internal/config/metrics_test.go cmd/evidra/run.go cmd/evidra/record.go
git commit -m "feat(metrics): add explicit CLI OTLP metrics transport"
```

---

### Task 7: Marketplace-Ready Setup Action + CI Adoption Path

**Files:**
- Create: `.github/actions/setup-evidra/action.yml`
- Modify: `README.md`
- Create: `docs/guides/setup-evidra-action.md`
- Create: `docs/release/setup-evidra-marketplace-checklist.md`

**Step 1: Write failing doc/usage test (string-level snapshot in CLI docs test if present)**

```go
func TestReadme_ReferencesSetupEvidraAction(t *testing.T) {
    // assert README contains setup action section
}
```

**Step 2: Run tests to fail (if no doc test exists, run `bash scripts/check-doc-commands.sh`)**

Run: `bash scripts/check-doc-commands.sh`
Expected: FAIL or missing references

**Step 3: Implement standalone setup action in repo + docs**

- Action only installs/configures evidra binary (no benchmark execution side effects).
- Keep existing benchmark composite action untouched.
- Document migration from `uses: samebits/evidra-benchmark/.github/actions/evidra@main` to setup+explicit commands.
- Add publication checklist for external Marketplace repo (`evidra-io/setup-evidra`).

**Step 4: Run validation**

Run: `bash scripts/check-doc-commands.sh`
Expected: PASS

**Step 5: Commit**

```bash
git add .github/actions/setup-evidra/action.yml README.md docs/guides/setup-evidra-action.md docs/release/setup-evidra-marketplace-checklist.md
git commit -m "feat(ci): add setup-evidra action and adoption docs"
```

---

### Task 8: End-to-End Verification and Regression Gate

**Files:**
- Modify: `cmd/evidra/main_test.go`
- Create: `tests/e2e/run_record_parity_test.go`
- Modify: `tests/signal-validation/README.md`
- Modify: `docs/plans/2026-03-07-v1-adoption-observability-design.md`

**Step 1: Add failing parity and UX requirement tests**

```go
func TestRunAndRecord_ProduceEquivalentSignalsForSameOperation(t *testing.T) {}
func TestRunOutput_ContainsFirstUsefulOutputFields(t *testing.T) {}
```

**Step 2: Run tests to fail**

Run: `go test ./cmd/evidra ./tests/e2e -count=1`
Expected: FAIL

**Step 3: Implement remaining wiring and docs sync**

- Ensure run/record outputs include mandatory fields from design.
- Ensure parity assertions pass.
- Update docs with final command examples.

**Step 4: Full verification**

Run:
- `go test ./... -count=1`
- `bash scripts/check-doc-commands.sh`
- `bash tests/signal-validation/validate-signals-engine.sh`

Expected:
- all commands PASS
- signal harness passes with expected comparisons (`F_repair > B_retry`, `G_thrash < B_retry`)

**Step 5: Commit**

```bash
git add cmd/evidra/main_test.go tests/e2e/run_record_parity_test.go tests/signal-validation/README.md docs/plans/2026-03-07-v1-adoption-observability-design.md
git commit -m "test(e2e): enforce run/record parity and first-use output contract"
```

---

## Implementation Notes and Guardrails

1. `run` is orchestration, not a second engine.
2. `record` must be strictly validated against v1 contract before lifecycle calls.
3. Metrics transport defaults to `none` (zero-setup), explicit opt-in for OTLP.
4. Correlation IDs must not be default high-cardinality metric labels.
5. Keep `MinOperations=100` as canonical sufficiency gate; preview mode is additive UX.
6. Preserve backward compatibility for `prescribe`/`report` workflows.

---

## Definition of Done

1. Root `run` and `record` commands exist, tested, and documented.
2. Record schema is explicit, versioned, validated, and published in system-design docs.
3. CLI metrics transport is defined and implemented (OTLP/HTTP + none).
4. First-use output requirements are enforced by tests.
5. Parity test proves run/record map to same scoring/signals behavior.
6. Setup action is split and documented for Marketplace publication path.
