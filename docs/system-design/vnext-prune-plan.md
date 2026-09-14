# vNext prune plan — deletion-ready inventory, and the gate that blocks it

Plan reference: §43 and §59 step 10 of [vnext-mcp-recorder.md](vnext-mcp-recorder.md).

## Why this is a plan and not a diff

§43 is explicit about ordering:

> After Gates A–C are green, remove from the branch. … Deletion happens after the new
> vertical path works. Do not use deletion as the first milestone.

Those gates are not green, and the measurements are on record in this repository:

- **Gate A** (`gate-a-results.md`): 80 runs, and the strong-model bar failed — the
  paid ceiling arm reached 11/15 task success against a bar of `>= 15/16`, terminal
  report coverage 13/15 against `>= 15/16`. Enforcement never fired in 80 runs
  (0 blocks), so `recovery_after_first_block` is `not_measurable` in every cell.
- **Gate B** (`vnext-gate-b-results.md`): passing on the supported profile through
  the fixture, with the fidelity claim bounded to that profile. No third-party server
  has been wrapped.
- **Gate C** (`vnext-gate-c-reconciliation-probe.md`): explicitly **not passed** —
  no real operational server, no reader who did not build it, and no session in the
  sample where declaration and observation diverge.

Deleting the legacy graph now would remove the only code whose behaviour is measured
by the hosted-path tests, while the replacement path still has an open question at
its centre (does reconciliation tell a human anything an ordinary log does not?).
That is the exact failure mode §43 warns against. So the work here is to make the
deletion one command list, executable the moment the gates are green, and to keep
this branch honest about the fact that it has not been executed.

## Execution status

| Step | Scope | Commit |
|---|---|---|
| 1 | `cmd/evidra` trimmed to `summarize`, `verify`, `version` | `8351312` |
| 2 | hosted chain: `cmd/evidra-api`, `internal/{api,apiutil,auth,db,store,analytics*,analyticsdb,analyticsvc,ingest,gitops,automationevent}`, `Dockerfile.api` | `cf7e2cb` |
| 3 | `evidra-mcp` endpoint-only; 11 legacy flags gone | `318bcd5` |
| 4 | `pkg/mcpserver`, `internal/{lifecycle,assessment,evidence,config,telemetry}`, `pkg/{mode,client}`, `tests/{inspector,e2e,contracts,testutil}` | `6c28a28` |
| 5 | legacy relay: `pkg/proxy/{proxy,evidence,detect}.go` + tests, `--legacy-proxy`, `runProxyMode` | this commit |
| 6 | v1 evidence shapes in `pkg/evidence`, `pkg/evlock`, `pkg/export`, `internal/{canon,assess,risk,score,detectors,signal,pipeline,sarif,promptfactory}` and their fixtures | pending |
| 7 | documentation pass: `docs/ARCHITECTURE.md`, `CLAUDE.md`, README, `examples/kagent`, `tests/test_*.sh` doc guards | pending |

`ui/` is retained by decision, not oversight: §43 removes its runtime coupling (done in
step 2), and the sources are the asset earmarked for a later repo move that this branch
is instructed not to perform.

## Inventory: what leaves the vNext product path

Everything below is already outside the CI-supported set except where noted, so the
prune is mostly `git rm` plus removing exclusions from `.github/workflows/ci-vnext.yml`
(`VNEXT_UNSUPPORTED`, line 26).

| §43 item | Where it lives |
|---|---|
| `run_command`, `collect_diagnostics`, `write_file`, `describe_tool`, `prescribe_smart`, `prescribe_full`, smart output | `pkg/mcpserver/` (tools + `schemas/`), `internal/promptfactory/` (renders and validates those tool names), `tests/inspector/cases/*` |
| risk | `internal/risk/`, `internal/assess/` |
| score | `internal/score/`, `internal/analytics/`, `internal/analyticsvc/` |
| canonicalization | `internal/canon/`, `tests/canon_fixtures/` |
| detectors | `internal/detectors/`, `internal/assessment/` |
| behavioral signal engine | `internal/signal/`, `internal/pipeline/` |
| SARIF input | `internal/sarif/` |
| hosted analytics | `internal/analyticsdb/` |
| PostgreSQL | `internal/db/` |
| accounts/auth platform | `internal/auth/`, `internal/store/` |
| general API | `internal/api/`, `internal/apiutil/`, `cmd/evidra-api/` |
| old UI/landing runtime coupling | `ui/`, `cmd/evidra-api/static/` |
| old proxy mutation heuristics | `pkg/proxy/detect.go` — exported `ClassifyCommand`, `ClassifyToolName`, `IsMutation` — plus `runProxyMode` in `cmd/evidra-mcp`, and their tests `TestClassifyCommand`, `TestIsMutation`, `TestProxyRelayRequests_TracksGenericMutationTool`, `TestProxyRelayResponses_UsesStructuredExitCode` (all currently passing inside the CI-supported set, so they must go in the same commit as the relay rather than outliving it as tests of a path nobody can reach) |
| old evidence adapters not used by v2 | `pkg/evidence` v1 shapes: `entry.go` (`Actor`, entry/segment APIs), `internal/evidence/` signer, `pkg/evlock/`, `internal/lifecycle/` (guarded by CI; deleting it also removes `cmd/evidra`'s `prescribe`/`report`/`record`/`scorecard`/`explain`/`compare`/`import`/`validate` commands, which are the legacy CLI surface) |

Also removed with the above: `--legacy-proxy`, `--url`/`--environment`/`--retry-tracker`/
`--signing-mode` and the direct MCP-server modes of `cmd/evidra-mcp`; `pkg/client`
(API HTTP client); `pkg/mode`; `pkg/export`; `internal/telemetry` if nothing in the
vNext path emits metrics.

## What must survive the prune

- `cmd/evidra-mcp` merged endpoint (`--proxy`, `--enforce=all|off`) and everything it
  reaches: `pkg/proxy/endpoint*.go`, `pkg/evidence` v2, `pkg/report`.
- `cmd/evidra` limited to `summarize`, `verify`, `version` (§41's thin CLI).
- `cmd/evidra-fixture` (the conformance sensor Gate B measures against) and
  `cmd/evidra-gatea` (the experiment runner).
- `.github/workflows/ci-vnext.yml`, with the exclusion regex rewritten so the
  supported set becomes "everything that is left" rather than a list of exemptions.

## Order that keeps CI green on every commit

1. Delete the unreachable hosted leaf packages (`internal/api`, `internal/db`,
   `internal/auth`, `internal/store`, `internal/analytics*`, `cmd/evidra-api`, `ui/`),
   then `make tidy` and drop them from `VNEXT_UNSUPPORTED`.
2. Delete the analysis chain (`canon`, `assess`, `risk`, `score`, `detectors`,
   `signal`, `pipeline`, `assessment`, `sarif`, `automationevent`, `ingest`,
   `gitops`, `promptfactory`, `telemetry`, `client`, `mode`) and the `cmd/evidra`
   commands that only they use.
3. Delete `pkg/mcpserver` and the legacy MCP modes plus their flags, keeping
   `--legacy-proxy` until the step after it, so a user can still reach the old relay
   while it is being compared.
4. Delete the legacy relay inside `pkg/proxy` **with its tests in the same commit**
   (`detect.go`, `runProxyMode`, the four tests named above), then remove
   `--legacy-proxy` itself. The vNext endpoint shares no code with `detect.go`, which is
   why this can be a single removal rather than a refactoring.
5. Delete the v1 evidence shapes (`pkg/evidence/entry.go`, `internal/evidence`,
   `pkg/evlock`, `internal/lifecycle`) once nothing imports them; finish with the
   `docs/ARCHITECTURE.md` rewrite, which currently describes the pre-vNext product.

Each step must end with: `go build ./...`, `go test -count=1 ./...`,
`go test -race ./pkg/evidence/... ./pkg/proxy/... ./cmd/evidra-fixture`,
`golangci-lint run ./...`, and a Gate A re-grade (`--regrade`) to prove the counts did
not move.

## What unblocked it, and one correction to the order above

The plan's bars were revised from measurement (§45) and §43's precondition was rewritten
to Gate A + Gate B, with Gate C governing merge/no-merge rather than deletion (§43, §47).
That decision was taken explicitly by the human directing this work, not inferred from
convenience, and §43 still forbids using deletion as a milestone: the deletion is
allowed, not obligatory.

One dependency changes the step order written above. `cmd/evidra` — which must survive as
`summarize|verify|version` — currently imports `internal/store`, `internal/analytics`,
`internal/analyticsdb` and `internal/sarif`. So the legacy commands have to be trimmed out
of `cmd/evidra` **before** the hosted chain can be deleted, not after:

1. Trim `cmd/evidra` to `summarize`, `verify`, `version` and delete the command files that
   only the legacy chain used.
2. Delete the hosted leaf packages (`internal/api`, `internal/apiutil`, `internal/auth`,
   `internal/store`, `internal/db`, `internal/analytics*`, `internal/ingest`,
   `internal/gitops`, `cmd/evidra-api`, `ui/`).
3. Delete the analysis chain (`canon`, `assess`, `risk`, `score`, `detectors`, `signal`,
   `pipeline`, `assessment`, `sarif`, `automationevent`, `promptfactory`, `telemetry`,
   `client`, `mode`) and its fixtures.
4. Delete `pkg/mcpserver` and the legacy MCP modes plus their flags.
5. Delete the legacy relay inside `pkg/proxy` with its tests in the same commit
   (`detect.go`, `runProxyMode`), then remove `--legacy-proxy`. The vNext endpoint shares
   no code with `detect.go`, which is why this is a removal rather than a refactoring.
6. Delete `examples/kagent/` content that documents the removed API and direct-MCP
   integration, or rewrite it against `evidra-mcp --proxy`. It is untracked local
   material today, so the choice is "re-adding" rather than "deleting".
7. Delete the v1 evidence shapes (`pkg/evidence/entry.go`, `internal/evidence`,
   `pkg/evlock`, `internal/lifecycle`) once nothing imports them, then rewrite
   `docs/ARCHITECTURE.md`, which still describes the pre-vNext product.

Each step ends with `go build ./...`, `go test -count=1 ./...`,
`go test -race ./pkg/evidence/... ./pkg/proxy/... ./cmd/evidra-fixture`,
`golangci-lint run ./...`, `go mod tidy`, and a Gate A re-grade (`--regrade`) to prove the
counts did not move.
