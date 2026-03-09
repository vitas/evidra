# Architecture Alignment Remediation Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Align runtime behavior, contracts, and docs around CLI/MCP as the supported path, severity-aware risk resolution, immediate `report` assessment, and explicit experimental self-hosted analytics.

**Architecture:** First make the risk engine and assessment builder authoritative, then rewire all supported surfaces (`run`, `record`, CLI `report`, MCP `report`) to use the same contract. After runtime behavior is correct, downgrade self-hosted analytics to explicit experimental failures and sweep docs/UI/OpenAPI so the repository advertises only what exists.

**Tech Stack:** Go stdlib, existing `internal/detectors`, `internal/risk`, `internal/signal`, `internal/score`, `internal/api`, React/TS landing page, Markdown docs, OpenAPI YAML.

**Design doc:** `docs/plans/2026-03-09-architecture-alignment-remediation-design.md`

---

### Task 1: Make Risk Resolution Severity-Aware

**Files:**
- Modify: `internal/detectors/registry.go`
- Modify: `internal/risk/matrix.go`
- Test: `internal/risk/matrix_test.go`

**Step 1: Write the failing tests**

Add table-driven tests that lock the exact semantics:

```go
func TestElevateRiskLevel_UsesDetectorSeverityMaximum(t *testing.T) {
	tests := []struct {
		name string
		base string
		tags []string
		want string
	}{
		{"no_tags", "medium", nil, "medium"},
		{"critical_detector_overrides_medium_matrix", "medium", []string{"k8s.privileged_container"}, "critical"},
		{"low_detector_does_not_raise_high_matrix", "high", []string{"k8s.writable_rootfs"}, "high"},
		{"high_detector_raises_low_matrix", "low", []string{"ops.kube_system"}, "high"},
		{"unknown_tag_is_ignored", "medium", []string{"unknown.tag"}, "medium"},
	}
	for _, tt := range tests {
		if got := ElevateRiskLevel(tt.base, tt.tags); got != tt.want {
			t.Fatalf("%s: got %q want %q", tt.name, got, tt.want)
		}
	}
}
```

**Step 2: Run test to verify it fails**

Run:

```bash
go test ./internal/risk -run TestElevateRiskLevel_UsesDetectorSeverityMaximum -count=1
```

Expected: FAIL because `ElevateRiskLevel` still treats any tag as a one-step bump.

**Step 3: Write minimal implementation**

Add a detector metadata lookup by tag and use it from `ElevateRiskLevel`:

```go
func BaseSeverityForTag(tag string) (string, bool) {
	for _, d := range All() {
		if d.Tag() == tag {
			return d.BaseSeverity(), true
		}
	}
	return "", false
}

func ElevateRiskLevel(matrixLevel string, riskTags []string) string {
	best := matrixLevel
	bestSev := riskSeverity[matrixLevel]
	for _, tag := range riskTags {
		sev, ok := detectors.BaseSeverityForTag(tag)
		if !ok {
			continue
		}
		if riskSeverity[sev] > bestSev {
			best = sev
			bestSev = riskSeverity[sev]
		}
	}
	return best
}
```

**Step 4: Run tests to verify they pass**

Run:

```bash
go test ./internal/risk ./internal/detectors -count=1
```

Expected: PASS

**Step 5: Commit**

```bash
git add internal/detectors/registry.go internal/risk/matrix.go internal/risk/matrix_test.go
git commit -m "fix: make risk level severity-aware"
```

### Task 2: Extract Shared Assessment Builder

**Files:**
- Create: `internal/assessment/assessment.go`
- Create: `internal/assessment/assessment_test.go`
- Modify: `cmd/evidra/assessment.go`

**Step 1: Write the failing tests**

Create tests for reusable assessment behavior:

```go
func TestBuild_FromSignalResults_PreviewWhenBelowThreshold(t *testing.T) {
	got := BuildFromResults(nil, 1)
	if got.Basis.AssessmentMode != "preview" {
		t.Fatalf("mode=%q want preview", got.Basis.AssessmentMode)
	}
	if got.ScoreBand == "insufficient_data" {
		t.Fatalf("preview mode should use a scored preview band")
	}
}

func TestBuild_FromSignalResults_SufficientAtThreshold(t *testing.T) {
	got := BuildFromResults(nil, score.MinOperations)
	if got.Basis.AssessmentMode != "sufficient" {
		t.Fatalf("mode=%q want sufficient", got.Basis.AssessmentMode)
	}
}
```

**Step 2: Run test to verify it fails**

Run:

```bash
go test ./internal/assessment -count=1
```

Expected: FAIL because the package does not exist yet.

**Step 3: Write minimal implementation**

Move the shared logic out of CLI-only code:

```go
type Basis struct {
	AssessmentMode       string `json:"assessment_mode"`
	Sufficient           bool   `json:"sufficient"`
	TotalOperations      int    `json:"total_operations"`
	SufficientThreshold  int    `json:"sufficient_threshold"`
	PreviewMinOperations int    `json:"preview_min_operations"`
}

type Snapshot struct {
	Score         float64          `json:"score"`
	ScoreBand     string           `json:"score_band"`
	SignalSummary map[string]int   `json:"signal_summary"`
	Confidence    score.Confidence `json:"confidence"`
	Basis         Basis            `json:"basis"`
}

func BuildAtPath(evidencePath, sessionID string) (Snapshot, error) { ... }
func BuildFromResults(results []signal.SignalResult, totalOps int) Snapshot { ... }
```

Leave `cmd/evidra/assessment.go` as a thin wrapper or delete it after callers move.

**Step 4: Run tests to verify they pass**

Run:

```bash
go test ./internal/assessment ./cmd/evidra -count=1
```

Expected: PASS

**Step 5: Commit**

```bash
git add internal/assessment/assessment.go internal/assessment/assessment_test.go cmd/evidra/assessment.go
git commit -m "refactor: extract shared assessment builder"
```

### Task 3: Align CLI and MCP Output Contracts

**Files:**
- Modify: `cmd/evidra/run.go`
- Modify: `cmd/evidra/record.go`
- Modify: `cmd/evidra/main.go`
- Modify: `pkg/mcpserver/server.go`
- Test: `cmd/evidra/run_test.go`
- Test: `cmd/evidra/record_test.go`
- Test: `cmd/evidra/run_record_parity_test.go`
- Test: `pkg/mcpserver/e2e_test.go`
- Test: `pkg/mcpserver/correlation_test.go`

**Step 1: Write the failing tests**

Lock the new contract:

```go
func TestReportOutputIncludesAssessmentSnapshot(t *testing.T) {
	stdout, stderr, exitCode := runEvidra(t, bin,
		"report",
		"--prescription", prescriptionID,
		"--exit-code", "0",
		"--evidence-dir", evidenceDir,
		"--signing-key-path", privPath,
	)
	if exitCode != 0 {
		t.Fatalf("report exit=%d stderr=%s", exitCode, stderr)
	}
	var out map[string]any
	json.Unmarshal([]byte(stdout), &out)
	for _, field := range []string{"score", "score_band", "signal_summary", "basis", "confidence"} {
		if _, ok := out[field]; !ok {
			t.Fatalf("missing %s: %v", field, out)
		}
	}
	if _, ok := out["risk_classification"]; ok {
		t.Fatalf("risk_classification must be removed: %v", out)
	}
}
```

Add an MCP test that asserts `report` returns the same assessment fields and does not emit `signals`.

**Step 2: Run tests to verify they fail**

Run:

```bash
go test ./cmd/evidra ./pkg/mcpserver -count=1
```

Expected: FAIL because CLI `report` and MCP `report` do not return the shared assessment snapshot yet.

**Step 3: Write minimal implementation**

Rewire all supported surfaces to the shared package:

```go
snapshot, err := assessment.BuildAtPath(evidencePath, reportOut.SessionID)
result := map[string]any{
	"ok":            true,
	"report_id":     reportOut.ReportID,
	"prescription_id": opts.prescriptionID,
	"exit_code":     opts.exitCode,
	"verdict":       evidence.VerdictFromExitCode(opts.exitCode),
	"score":         snapshot.Score,
	"score_band":    snapshot.ScoreBand,
	"signal_summary": snapshot.SignalSummary,
	"basis":         snapshot.Basis,
	"confidence":    snapshot.Confidence,
}
```

For MCP:

```go
type ReportOutput struct {
	OK            bool              `json:"ok"`
	ReportID      string            `json:"report_id"`
	Score         float64           `json:"score"`
	ScoreBand     string            `json:"score_band"`
	SignalSummary map[string]int    `json:"signal_summary"`
	Basis         assessment.Basis  `json:"basis"`
	Confidence    score.Confidence  `json:"confidence"`
	Error         *ErrInfo          `json:"error,omitempty"`
}
```

Remove `risk_classification` from `run` and `record` output maps and from parity tests/docs.

**Step 4: Run tests to verify they pass**

Run:

```bash
go test ./cmd/evidra ./pkg/mcpserver -count=1
go test -tags e2e ./tests/e2e -count=1 -timeout=120s
```

Expected: PASS

**Step 5: Commit**

```bash
git add cmd/evidra/run.go cmd/evidra/record.go cmd/evidra/main.go pkg/mcpserver/server.go cmd/evidra/run_test.go cmd/evidra/record_test.go cmd/evidra/run_record_parity_test.go pkg/mcpserver/e2e_test.go pkg/mcpserver/correlation_test.go
git commit -m "feat: return assessment snapshot from report"
```

### Task 4: Make Hosted Analytics Explicitly Experimental

**Files:**
- Create: `internal/api/experimental_analytics.go`
- Modify: `internal/api/scorecard_handler.go`
- Modify: `internal/api/explain_handler.go`
- Modify: `cmd/evidra-api/main.go`
- Test: `internal/api/router_test.go`
- Test: `internal/api/integration_test.go`

**Step 1: Write the failing tests**

Add API tests that assert hosted analytics fail honestly:

```go
func TestRouter_ScorecardExperimentalReturns501(t *testing.T) {
	router := NewRouter(RouterConfig{
		APIKey:        "test-key",
		DefaultTenant: "t1",
		Scorecard:     ExperimentalAnalytics{},
	})
	req := httptest.NewRequest("GET", "/v1/evidence/scorecard", nil)
	req.Header.Set("Authorization", "Bearer test-key")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotImplemented {
		t.Fatalf("expected 501, got %d", rec.Code)
	}
}
```

Mirror the same for `/v1/evidence/explain`.

**Step 2: Run test to verify it fails**

Run:

```bash
go test ./internal/api -count=1
```

Expected: FAIL because handlers currently return `200` with placeholder content.

**Step 3: Write minimal implementation**

Create a sentinel implementation:

```go
type ExperimentalAnalytics struct{}

func (ExperimentalAnalytics) ComputeScorecard(string, string, string, string, int) (interface{}, error) {
	return nil, ErrExperimentalNotImplemented
}

func (ExperimentalAnalytics) ComputeExplain(string, string) (interface{}, error) {
	return nil, ErrExperimentalNotImplemented
}
```

Map that error to `501` in both handlers and wire `cmd/evidra-api/main.go` to use `ExperimentalAnalytics{}` instead of the placeholder store methods.

**Step 4: Run tests to verify they pass**

Run:

```bash
go test ./internal/api ./cmd/evidra-api -count=1
```

Expected: PASS

**Step 5: Commit**

```bash
git add internal/api/experimental_analytics.go internal/api/scorecard_handler.go internal/api/explain_handler.go cmd/evidra-api/main.go internal/api/router_test.go internal/api/integration_test.go
git commit -m "fix: mark hosted analytics as experimental"
```

### Task 5: Add Separate Self-Hosted Status Doc and Sweep Public Docs

**Files:**
- Create: `docs/guides/self-hosted-experimental-status.md`
- Modify: `README.md`
- Modify: `docs/integrations/CLI_REFERENCE.md`
- Modify: `docs/guides/mcp-setup.md`
- Modify: `docs/ARCHITECTURE.md`
- Modify: `docs/system-design/V1_ARCHITECTURE.md`
- Modify: `docs/system-design/V1_RUN_RECORD_CONTRACT.md`

**Step 1: Write the failing documentation checks**

Use `rg` as the failing contract check:

```bash
rg -n "risk_classification|--api-url|signals\\[\\]|Archived \\(implemented\\)|critical\\)" README.md docs
```

Expected before edits: hits in README and design docs showing stale contract language.

**Step 2: Run the check to verify it fails**

Run:

```bash
rg -n "risk_classification|--api-url|signals\\[\\]|Archived \\(implemented\\)|band classification \\(excellent/good/fair/poor/critical\\)" README.md docs
```

Expected: non-zero findings

**Step 3: Write minimal documentation changes**

Create the separate status doc with a support matrix:

```md
## Supported Today
- evidence ingestion
- key issuance
- entry browsing

## Experimental / Not Implemented
- /v1/evidence/scorecard
- /v1/evidence/explain

## Authoritative Analytics Path
- evidra scorecard
- evidra explain
- evidra report
- evidra-mcp report
```

Then update public docs to:

- replace `--api-url` with `--url`
- replace `signals[]` examples with assessment snapshot language
- remove `risk_classification`
- mark self-hosted analytics experimental
- link to `docs/guides/self-hosted-experimental-status.md`

**Step 4: Run checks to verify they pass**

Run:

```bash
rg -n "risk_classification|--api-url|signals\\[\\]|band classification \\(excellent/good/fair/poor/critical\\)" README.md docs
bash scripts/check-doc-commands.sh
```

Expected: first command returns no stale hits; second passes or only reports unrelated pre-existing issues.

**Step 5: Commit**

```bash
git add docs/guides/self-hosted-experimental-status.md README.md docs/integrations/CLI_REFERENCE.md docs/guides/mcp-setup.md docs/ARCHITECTURE.md docs/system-design/V1_ARCHITECTURE.md docs/system-design/V1_RUN_RECORD_CONTRACT.md
git commit -m "docs: align contracts and self-hosted status"
```

### Task 6: Fix UI, OpenAPI, and Remove Misleading Placeholders

**Files:**
- Modify: `ui/src/pages/Landing.tsx`
- Modify: `cmd/evidra-api/static/index.html`
- Modify: `cmd/evidra-api/static/openapi.yaml`
- Modify: `ui/public/openapi.yaml`
- Modify: `internal/detectors/all/all.go`
- Delete: `internal/detectors/terraform/azure/doc.go`
- Delete: `internal/detectors/terraform/gcp/doc.go`

**Step 1: Write the failing drift checks**

Run:

```bash
rg -n "critical < 25|excellent >= 90|good >= 75|fair >= 50|poor >= 25|api-url|signals\\[\\]" ui cmd/evidra-api/static
rg -n "terraform/azure|terraform/gcp" docs/system-design/V1_ARCHITECTURE.md internal/detectors/all/all.go
```

Expected: stale hits in landing page/static docs and placeholder detector references.

**Step 2: Run the checks to verify they fail**

Run:

```bash
rg -n "critical < 25|excellent >= 90|good >= 75|fair >= 50|poor >= 25|api-url|signals\\[\\]" ui cmd/evidra-api/static
```

Expected: non-zero findings

**Step 3: Write minimal implementation**

Update UI/OpenAPI:

- landing page score bands -> `excellent >= 99`, `good >= 95`, `fair >= 90`, `poor < 90`
- self-hosted copy -> experimental analytics warning with link to the new status doc
- OpenAPI -> `501` responses for hosted analytics with experimental wording

Remove dead placeholders:

```go
import (
	_ "samebits.com/evidra-benchmark/internal/detectors/docker"
	_ "samebits.com/evidra-benchmark/internal/detectors/k8s"
	_ "samebits.com/evidra-benchmark/internal/detectors/ops"
	_ "samebits.com/evidra-benchmark/internal/detectors/terraform/aws"
)
```

Delete the empty Azure/GCP placeholder files.

**Step 4: Run checks to verify they pass**

Run:

```bash
go test ./internal/detectors/... ./internal/api -count=1
rg -n "critical < 25|excellent >= 90|good >= 75|fair >= 50|poor >= 25|api-url|signals\\[\\]" ui cmd/evidra-api/static
```

Expected: tests PASS and stale-string grep returns no results.

**Step 5: Commit**

```bash
git add ui/src/pages/Landing.tsx cmd/evidra-api/static/index.html cmd/evidra-api/static/openapi.yaml ui/public/openapi.yaml internal/detectors/all/all.go
git rm internal/detectors/terraform/azure/doc.go internal/detectors/terraform/gcp/doc.go
git commit -m "chore: remove misleading placeholders and stale UI claims"
```

### Task 7: Full Verification and Release Note Draft

**Files:**
- Modify: `CHANGELOG.md`

**Step 1: Write the failing verification checklist**

Create a short checklist entry in `CHANGELOG.md` draft notes for:

- risk model semantics changed
- `report` contract changed
- self-hosted analytics explicitly experimental
- `risk_classification` removed

**Step 2: Run verification commands**

Run:

```bash
go test ./... -count=1
go test -tags e2e ./tests/e2e -count=1 -timeout=120s
make test-signals
make build
```

Expected: PASS

**Step 3: Run repo-wide drift checks**

Run:

```bash
rg -n "risk_classification|--api-url|signals\\[\\]|placeholder response until full|Archived \\(implemented\\)" README.md docs cmd internal ui
rg -n "experimental" README.md docs/guides/self-hosted-experimental-status.md docs/ARCHITECTURE.md cmd/evidra-api/static/openapi.yaml ui/public/openapi.yaml
```

Expected: first command returns no stale claims tied to this remediation; second confirms the experimental status language is present where expected.

**Step 4: Commit**

```bash
git add CHANGELOG.md
git commit -m "docs: record architecture alignment contract changes"
```

**Step 5: Hand off**

When all tasks are complete, request review focused on:

- contract correctness
- doc/runtime consistency
- MCP `report` ergonomics
- hosted analytics honesty
