# vNext prune record — what left the branch, in which commit, and what each removal made
# impossible to confuse

Plan reference: §43 and §59 step 10 of [vnext-mcp-recorder.md](vnext-mcp-recorder.md).

## Ordering, and how it actually went

§43 originally read "After Gates A–C are green, remove from the branch." That precondition
was revised during execution, and the revision is the honest part of this record:

- **Gate A was never green and still is not.** The official 80-run set failed the bars as
  written (`gate-a-results.md`). §45's bars were later revised from those same measurements,
  and §45 now states plainly that revised bars cannot certify the run set that produced them.
  Mechanically the old set meets every new bar; the verdict stays `gate_passed=false`.
- **§43 was unblocked by an explicit human decision**, not by a gate turning green: enough of
  the protocol surface had been measured to know the vertical path is the path being
  measured, and Gate B was green. `703de15` records that as "decision not drift".
- **Gate B passed** (`vnext-gate-b-results.md`), and its rows were later amended when §43
  steps invalidated the tests they cited.
- **Gate C governs the merge/product decision, not the deletion** (`vnext-gate-c-reconciliation-probe.md`,
  not passed). Deletion was never the milestone §43 warned about; it is bookkeeping.

This document is therefore a **record**: the ordering, the inventory and the step list
below are written in the tense of things that happened, and where a step is genuinely still
open it says so. The inventory below is what left the branch, in
which commit, and what each removal made impossible to confuse. Nothing here is a to-do.

## Execution status

| Step | Scope | Commit |
|---|---|---|
| 1 | `cmd/evidra` trimmed to `summarize`, `verify`, `version` | `8351312` |
| 2 | hosted chain: `cmd/evidra-api`, `internal/{api,apiutil,auth,db,store,analytics*,analyticsdb,analyticsvc,ingest,gitops,automationevent}`, `Dockerfile.api` | `cf7e2cb` |
| 3 | `evidra-mcp` endpoint-only; 11 legacy flags gone | `318bcd5` |
| 4 | `pkg/mcpserver`, `internal/{lifecycle,assessment,evidence,config,telemetry}`, `pkg/{mode,client}`, `tests/{inspector,e2e,contracts,testutil}` | `6c28a28` |
| 5 | legacy relay: `pkg/proxy/{proxy,evidence,detect}.go` + tests, `--legacy-proxy`, `runProxyMode` | `3292230` |
| 6 | the analysis engine, prompt contracts and export layer: `pkg/{evlock,export,execcontract}`, `internal/{canon,assess,risk,score,detectors,signal,pipeline,sarif,promptfactory}`, `prompts/`, `internal/testutil` | `b508052` |
| 6b | v1 evidence shapes: `pkg/evidence` keeps only the v2 model | `3fadc59` |
| 7 | documentation pass: `docs/ARCHITECTURE.md` and `CLAUDE.md` rewritten to the shipped system; `--actor-id` restored; README carries a scope banner instead of a false claim | `bce335f`, `4e72f7c` |
| 8 | pre-vNext **normative** specs deleted rather than archived in place: seven `EVIDRA_*V1` system-design docs, the default scoring profile, both `docs/contracts` V1 contracts, 13 `tests/*.sh` doc guards whose subject was the deleted surface, `tests-index.md`, `E2E_TESTING.md` and three orphaned SARIF fixtures | `76bdee5`, `3476147` |

Still open from step 7 and deliberately not counted as done: `examples/kagent` rewrite-or-drop
and the full README rewrite, both waiting on Gate C rather than on effort.

`ui/` is retained by decision, not oversight: §43 removes its runtime coupling (done in
step 2), and the sources are the asset earmarked for a later repo move that this branch
is instructed not to perform.

## Inventory: what left the vNext product path

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

## The commit order that was used, and why it kept CI green at every step

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

The precondition was revised twice during execution — see "Ordering" above for the same
story in one paragraph — and the final rule, now written into §43 itself, is: **the new vertical path is
the path being measured, and Gate B is green.** Not "Gate A is green" — Gate A's formal
verdict on the official set is and stays `gate_passed=false`, because §45's revised bars
were chosen from that same data and cannot certify it. Gate C governs merge and product,
never deletion (§47).

The decision to prune under that criterion was taken explicitly by the human directing this
work, not inferred from convenience, and §43 still forbids using deletion as a milestone:
deletion was allowed, not obligatory, and each step's commit message says what the removal
made impossible to confuse.

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
   `docs/ARCHITECTURE.md`. Done in `3fadc59` and `bce335f`: `pkg/evidence` is v2-only and
   the architecture and agent-guidance documents describe the shipped system.

Each step ends with `go build ./...`, `go test -count=1 ./...`,
`go test -race ./pkg/evidence/... ./pkg/proxy/... ./cmd/evidra-fixture`,
`golangci-lint run ./...`, `go mod tidy`, and a Gate A re-grade (`--regrade`) to prove the
counts did not move.
