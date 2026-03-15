# Signal Maturity Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Build a first-cut signal maturity system in `evidra-benchmark` that measures claim readiness for the current signals, stores maturity runs in PostgreSQL, exposes them over REST, and renders an operator-facing UI.

**Architecture:** Add a new `pkg/maturity` package for typed signal maturity models, fixture loading, aggregation, and graduation logic. Keep the first cut deterministic and parent-owned: local benchmark execution writes maturity runs, the API persists and serves them, and the UI visualizes the resulting claim/shadow status. Do not implement `infra-bench` ingestion in this pass; only leave explicit extension points for it.

**Tech Stack:** Go, PostgreSQL migrations/store layer, `net/http`, existing API router/OpenAPI flow, React 19 + React Router + Vitest, static embedded UI.

---

### Task 1: Add typed signal maturity models and graduation logic

**Files:**
- Create: `pkg/maturity/types.go`
- Create: `pkg/maturity/aggregate.go`
- Create: `pkg/maturity/aggregate_test.go`
- Create: `pkg/maturity/testdata/sample-run.json`
- Reference: `internal/store/benchmarks.go`
- Reference: `ui/src/pages/Landing.tsx`

**Step 1: Write the failing tests**

Add tests that prove:
- `protocol_violation`, `retry_loop`, and `blast_radius` can be marked `claim`
- `artifact_drift` can remain `shadow` when precision or stability is below threshold
- unknown status or tier values are rejected

Example test shape:

```go
func TestSummarizeSignal_ClaimGrade(t *testing.T) {
	run := maturity.Run{
		Signals: []maturity.SignalRun{
			{
				Name:              "protocol_violation",
				Tier:              maturity.TierGold,
				Precision:         0.99,
				Recall:            0.97,
				FalsePositiveRate: 0.01,
				Stability:         0.98,
				PositiveCases:     12,
				NegativeCases:     12,
				NearMissCases:     6,
			},
		},
	}

	summary := maturity.Summarize(run)
	if summary.Signals[0].Status != maturity.StatusClaim {
		t.Fatalf("status = %q, want %q", summary.Signals[0].Status, maturity.StatusClaim)
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./pkg/maturity -run TestSummarizeSignal_ClaimGrade -v`

Expected: FAIL because `pkg/maturity` does not exist yet.

**Step 3: Write minimal implementation**

Implement:
- enums for `Tier` and `Status`
- `Run`, `SignalRun`, `Summary`, and `SignalSummary`
- `Summarize` / `SummarizeSignal` helpers
- explicit graduation checks with hard-coded defaults for this pass

Keep thresholds centralized in one struct so later tuning does not spread magic numbers.

**Step 4: Run tests to verify they pass**

Run: `go test ./pkg/maturity -v`

Expected: PASS with the new aggregation tests green.

**Step 5: Commit**

```bash
git add pkg/maturity
git commit -s -m "feat: add signal maturity aggregation"
```

### Task 2: Add deterministic maturity fixtures and local runner output

**Files:**
- Create: `pkg/maturity/dataset.go`
- Create: `pkg/maturity/dataset_test.go`
- Create: `pkg/maturity/run.go`
- Create: `pkg/maturity/run_test.go`
- Create: `tests/maturity/dataset.json`
- Create: `tests/maturity/schema/run.schema.json`
- Create: `tests/maturity/fixtures/protocol_violation/*.json`
- Create: `tests/maturity/fixtures/retry_loop/*.json`
- Create: `tests/maturity/fixtures/blast_radius/*.json`
- Create: `tests/maturity/fixtures/artifact_drift/*.json`
- Create: `tests/maturity/fixtures/new_scope/*.json`
- Create: `tests/maturity/fixtures/repair_loop/*.json`
- Create: `tests/maturity/fixtures/thrashing/*.json`
- Create: `tests/maturity/fixtures/risk_escalation/*.json`
- Modify: `cmd/evidra/benchmark.go`
- Create: `cmd/evidra/benchmark_test.go`

**Step 1: Write the failing tests**

Add tests that prove:
- the dataset loader rejects malformed entries
- `evidra benchmark run --dataset tests/maturity/dataset.json --out <dir>` writes `signal-maturity-run.json`
- the runner reports claim/shadow outputs for all eight signals

Example CLI test shape:

```go
func TestBenchmarkRun_WritesSignalMaturityRun(t *testing.T) {
	outDir := t.TempDir()
	exit := cmdBenchmark([]string{
		"run",
		"--dataset", "tests/maturity/dataset.json",
		"--out", outDir,
	}, io.Discard, io.Discard)
	if exit != 0 {
		t.Fatalf("exit = %d, want 0", exit)
	}
	if _, err := os.Stat(filepath.Join(outDir, "signal-maturity-run.json")); err != nil {
		t.Fatalf("missing signal-maturity-run.json: %v", err)
	}
}
```

**Step 2: Run tests to verify they fail**

Run: `go test -tags experimental ./cmd/evidra -run TestBenchmarkRun_WritesSignalMaturityRun -v`

Expected: FAIL because the benchmark CLI is still a stub.

**Step 3: Write minimal implementation**

Implement:
- a strict dataset loader for deterministic maturity fixtures
- local runner output format: `signal-maturity-run.json`
- a real `benchmark run` path for maturity datasets behind the existing experimental gate

Keep scope tight:
- support only deterministic local execution
- do not add hosted submission flags yet
- do not add `infra-bench` import yet

**Step 4: Run tests to verify they pass**

Run:
- `go test ./pkg/maturity -run TestLoadDataset -v`
- `go test -tags experimental ./cmd/evidra -run TestBenchmarkRun_WritesSignalMaturityRun -v`

Expected: PASS with output file assertions green.

**Step 5: Commit**

```bash
git add pkg/maturity cmd/evidra tests/maturity
git commit -s -m "feat: add deterministic signal maturity runner"
```

### Task 3: Persist maturity runs in PostgreSQL

**Files:**
- Create: `internal/db/migrations/005_signal_maturity_runs.up.sql`
- Create: `internal/store/maturity.go`
- Create: `internal/store/maturity_test.go`
- Modify: `internal/store/benchmarks_test.go`

**Step 1: Write the failing tests**

Add store tests that prove:
- a maturity run and its per-signal rows are saved and read back
- deterministic and real-run evidence lanes are preserved separately
- blocker strings and metric fields round-trip unchanged

Example test shape:

```go
func TestMaturityStore_SaveAndGetRun(t *testing.T) {
	store := newTestMaturityStore(t)
	runID, err := store.SaveRun(ctx, store.MaturityRun{Suite: "signal-maturity"}, []store.MaturitySignalResult{
		{Name: "retry_loop", Tier: "gold", Status: "claim"},
	})
	if err != nil {
		t.Fatal(err)
	}
	run, rows, err := store.GetRun(ctx, tenantID, runID)
	if err != nil {
		t.Fatal(err)
	}
	if run.Suite != "signal-maturity" || len(rows) != 1 {
		t.Fatalf("run=%+v rows=%+v", run, rows)
	}
}
```

**Step 2: Run tests to verify they fail**

Run: `go test ./internal/store -run TestMaturityStore_SaveAndGetRun -v`

Expected: FAIL because the migration and store do not exist yet.

**Step 3: Write minimal implementation**

Add:
- a migration with `signal_maturity_runs` and `signal_maturity_results`
- a dedicated store instead of overloading benchmark tables
- typed store models mirroring `pkg/maturity`

Do not try to generalize benchmark and maturity persistence together in this pass.

**Step 4: Run tests to verify they pass**

Run: `go test ./internal/store -v`

Expected: PASS including the new maturity store tests.

**Step 5: Commit**

```bash
git add internal/db/migrations/005_signal_maturity_runs.up.sql internal/store
git commit -s -m "feat: add signal maturity storage"
```

### Task 4: Expose maturity runs over REST and OpenAPI

**Files:**
- Create: `internal/api/maturity_handler.go`
- Create: `internal/api/maturity_handler_test.go`
- Modify: `internal/api/router.go`
- Modify: `internal/api/router_test.go`
- Modify: `cmd/evidra-api/static/openapi.yaml`
- Modify: `docs/api-reference.md`

**Step 1: Write the failing tests**

Add handler and router tests that prove:
- `POST /v1/maturity/runs` accepts a valid maturity run
- `GET /v1/maturity/signals` returns the current signal summaries
- `GET /v1/maturity/signals/{name}` returns detailed signal rows
- routes remain auth-protected

Example test shape:

```go
func TestHandleMaturityRun_AcceptsValidRun(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/v1/maturity/runs", bytes.NewBufferString(`{
		"suite":"signal-maturity",
		"signals":[{"name":"protocol_violation","tier":"gold","status":"claim"}]
	}`))
	rec := httptest.NewRecorder()

	handleMaturityRun(maturityStoreStub{saveRun: func(...) (string, error) {
		return "run-1", nil
	}}).ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusCreated)
	}
}
```

**Step 2: Run tests to verify they fail**

Run: `go test ./internal/api -run TestHandleMaturityRun_AcceptsValidRun -v`

Expected: FAIL because the handler and routes do not exist yet.

**Step 3: Write minimal implementation**

Implement:
- request/response structs
- router wiring under `/v1/maturity/...`
- OpenAPI entries with the same field names used by `pkg/maturity`
- API docs updates in `docs/api-reference.md`

Keep the first cut read/write limited to the new maturity resource family only.

**Step 4: Run tests to verify they pass**

Run: `go test ./internal/api -v`

Expected: PASS including router coverage for the new endpoints.

**Step 5: Commit**

```bash
git add internal/api cmd/evidra-api/static/openapi.yaml docs/api-reference.md
git commit -s -m "feat: add signal maturity api"
```

### Task 5: Add API client support for maturity runs

**Files:**
- Modify: `pkg/client/client.go`
- Modify: `pkg/client/client_test.go`

**Step 1: Write the failing tests**

Add tests for:
- `SubmitMaturityRun`
- `ListMaturitySignals`
- `GetMaturitySignal`

Example test shape:

```go
func TestSubmitMaturityRun(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/maturity/runs" {
			t.Fatalf("path = %s", r.URL.Path)
		}
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"run_id":"m1","status":"accepted"}`))
	}))
	defer srv.Close()

	c := client.New(client.Config{URL: srv.URL, APIKey: "test"})
	resp, err := c.SubmitMaturityRun(context.Background(), client.MaturityRunRequest{Suite: "signal-maturity"})
	if err != nil || resp.RunID != "m1" {
		t.Fatalf("resp=%+v err=%v", resp, err)
	}
}
```

**Step 2: Run tests to verify they fail**

Run: `go test ./pkg/client -run TestSubmitMaturityRun -v`

Expected: FAIL because the new client methods do not exist.

**Step 3: Write minimal implementation**

Add typed request/response structs and thin client methods matching the maturity API.

Avoid speculative helpers for `infra-bench` import in this pass.

**Step 4: Run tests to verify they pass**

Run: `go test ./pkg/client -v`

Expected: PASS with new maturity client coverage.

**Step 5: Commit**

```bash
git add pkg/client
git commit -s -m "feat: add signal maturity client methods"
```

### Task 6: Add maturity overview and detail pages in the UI

**Files:**
- Create: `ui/src/pages/MaturityOverview.tsx`
- Create: `ui/src/pages/MaturitySignalDetail.tsx`
- Modify: `ui/src/App.tsx`
- Modify: `ui/src/components/Layout.tsx`
- Create: `ui/test/components/MaturityOverview.test.tsx`
- Create: `ui/test/components/MaturitySignalDetail.test.tsx`
- Modify: `ui/test/components/App.test.tsx`

**Step 1: Write the failing tests**

Add UI tests that prove:
- `/maturity` renders rows for all signals
- gold and shadow tiers are visually distinct
- `/maturity/:name` renders blockers and evidence-lane split

Example test shape:

```tsx
it("renders signal maturity rows", () => {
  window.history.pushState({}, "", "/maturity");
  render(<App />);
  expect(screen.getByRole("heading", { name: /signal maturity/i })).toBeInTheDocument();
  expect(screen.getByText("protocol_violation")).toBeInTheDocument();
  expect(screen.getByText("retry_loop")).toBeInTheDocument();
});
```

**Step 2: Run tests to verify they fail**

Run: `cd ui && npm test`

Expected: FAIL because the routes and pages do not exist yet.

**Step 3: Write minimal implementation**

Implement:
- `/maturity`
- `/maturity/:name`
- a simple data-loading layer using mocked data first, then wire to the new API
- a visible deterministic-vs-real-run split on the detail page

Preserve the existing site visual language. Do not redesign the landing page in this pass.

**Step 4: Run tests to verify they pass**

Run:
- `cd ui && npm test`
- `make build-api`

Expected: PASS for Vitest and a successful embedded UI build.

**Step 5: Commit**

```bash
git add ui/src ui/test
git commit -s -m "feat: add signal maturity ui"
```

### Task 7: Document the maturity system end to end

**Files:**
- Create: `docs/guides/signal-maturity.md`
- Modify: `docs/tests-index.md`
- Modify: `docs/ROAD_MAP.md`
- Modify: `docs/product/` relevant positioning doc if one names claim-grade signals
- Modify: `cmd/evidra-api/static/openapi.yaml` if any docs examples are still missing

**Step 1: Write the failing doc check**

Add or extend a lightweight test that ensures the new public guide is referenced from `docs/tests-index.md` or another top-level doc. If there is no existing doc check for this area, add a focused shell test under `tests/` rather than a generic prose validator.

**Step 2: Run the doc check to verify it fails**

Run the new focused doc-check command.

Expected: FAIL until the guide and references exist.

**Step 3: Write the docs**

Document:
- what gold vs shadow means
- how to run the deterministic maturity dataset
- how to read precision / recall / false-positive rate / stability
- what blocks promotion from `shadow` to `claim`
- which parts are not implemented yet (`infra-bench` import)

**Step 4: Run the full verification set**

Run:
- `go test ./pkg/maturity ./internal/store ./internal/api ./pkg/client -v -count=1`
- `go test -tags experimental ./cmd/evidra -v -count=1`
- `cd ui && npm test`
- `make build-api`

Expected: PASS across the new backend, CLI, and UI surfaces.

**Step 5: Commit**

```bash
git add docs tests
git commit -s -m "docs: add signal maturity guide"
```

### Task 8: Final integration verification

**Files:**
- No new files; verify the merged result only

**Step 1: Run the full repo verification**

Run:

```bash
make test
make test-contracts
make test-signals
make build-api
```

Expected: PASS on the fully integrated maturity system.

**Step 2: Manually verify the feature**

Run:

```bash
go build -tags experimental -o /tmp/evidra-exp ./cmd/evidra
EVIDRA_BENCHMARK_EXPERIMENTAL=1 /tmp/evidra-exp benchmark run --dataset tests/maturity/dataset.json --out /tmp/evidra-maturity
```

Expected:
- `/tmp/evidra-maturity/signal-maturity-run.json` exists
- the JSON includes gold and shadow signals
- the CLI exits `0`

**Step 3: Smoke the API locally**

Start the API and submit one stored run with the new client or `curl`.

Expected:
- `POST /v1/maturity/runs` returns `201`
- `GET /v1/maturity/signals` returns summaries
- `GET /v1/maturity/signals/protocol_violation` returns detail

**Step 4: Smoke the UI**

Open `/maturity` and `/maturity/protocol_violation`.

Expected:
- overview table renders
- detail page shows blockers and evidence-lane split

**Step 5: Commit the final integration pass**

```bash
git add .
git commit -s -m "feat: add signal maturity system"
```
