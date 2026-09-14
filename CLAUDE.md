# CLAUDE.md

Guidance for agents working in this repository. More specific instructions (the user, the
plan) take precedence.

Evidra is currently in **vNext**: an MCP execution-evidence recorder. One endpoint wraps one
upstream MCP server, merges two local tools into its tool list, enforces protocol order, and
writes a signed evidence chain that a human reconciles afterwards.

**Read before changing behaviour:**
[`docs/system-design/vnext-mcp-recorder.md`](docs/system-design/vnext-mcp-recorder.md) is the
plan, cited by section number (§7, §13–§24, §34, §38, §41–§47, §59).
[`docs/ARCHITECTURE.md`](docs/ARCHITECTURE.md) is the map;
[`docs/system-design/vnext-prune-plan.md`](docs/system-design/vnext-prune-plan.md) records
what was deleted and why.
[`docs/system-design/vnext-experiment-harness.md`](docs/system-design/vnext-experiment-harness.md)
states the invariants the measurement harness is built around.

## Build & Test

```bash
make build            # bin/evidra, bin/evidra-mcp
make test             # go test ./... -v -count=1
make fmt lint tidy
go build -o bin/evidra-fixture ./cmd/evidra-fixture/    # conformance upstream
go build -o bin/evidra-gatea ./cmd/evidra-gatea/        # experiment runner
go test -race ./pkg/evidence/... ./pkg/proxy/... ./cmd/evidra-fixture
golangci-lint run ./...        # must report zero issues
```

Single case: `go test -run 'TestName' -v ./pkg/proxy/`. Gate artifacts cite tests by name, so
renaming or deleting one means updating the artifact in the same commit — a checklist row
pointing at a function that no longer compiles is not evidence of anything.

## Git

- **Always ask before pushing.** Nothing leaves this branch without the user.
- Sign every commit: `git commit -s` (DCO `Signed-off-by`).
- No history rewrites, no force-pushes, do not touch `main`.
- Stage by path. A blanket `git add -A` once swept untracked scratch into history here;
  `/output/` and `/tmp/` are gitignored because recorder directories hold ephemeral signing
  and digest keys that must not reach history.

## Layout

- `cmd/evidra-mcp/` — the merged endpoint. The only binary that enforces or records.
- `cmd/evidra/` — read side only: `summarize`, `verify`, `version`. A fourth command needs a
  plan change, not a registry entry.
- `pkg/proxy/` — endpoint (`endpoint*.go`). The pre-vNext relay and its mutation heuristics
  are deleted; do not reintroduce content-based classification.
- `pkg/evidence/` — evidence model **v2 only**: `store_v2.go`, `event_v2.go`, `digest_v2.go`,
  `verify_v2.go`, `canon_jcs.go`. One schema, one writer per directory.
- `pkg/report/` — reconciliation into `summary.json`.
- `cmd/evidra-gatea/`, `cmd/evidra-fixture/` — the measurement harness. Not shipped product,
  but **not disposable test code either**: it produces the evidence the product claim rests
  on, so it obeys `docs/system-design/vnext-experiment-harness.md` (build provenance,
  analytics invariants, regrade-don't-hand-read). Arm definitions live in
  `cmd/evidra-gatea/arms.json`.
- `ui/` — retained for a future relocation; nothing in the vNext path imports or serves it.

## Rules the implementation exists to keep

1. Enforcement is protocol-only (§7): `--enforce=all` refuses `tools/call` while no operation
   is open; `--enforce=off` observes. Never gate on argument content, tool names, or
   annotations.
2. §17 ordering is load-bearing: append `execution_started` before forwarding, and
   `execution_finished` **before** relaying the response.
3. §18's store-failure contract has three branches — refuse the call, relay-but-degrade, and
   never acknowledge a report without durable evidence. Do not collapse them into one "log the
   error" path, and never fabricate an upstream failure.
4. Provenance is one of exactly three values (§15). Derived classes such as
   `unprescribed_execution` are read-time computations, never stored event types.
5. Never average across comparison domains (§20, §24): enforcement mode, upstream, and
   `digest_key_id` stay separate in `pkg/report`.
6. Chain validity, signature validity and coverage are three statements; a valid chain can
   still be incomplete.
7. Privacy is a bound (§23, §24): arguments leave the process only as
   `HMAC(digest_key, JCS(args))`; results are hashed up to 4 MiB, else `omitted_oversize`. Raw
   observed payloads and key material never enter the chain or `summary.json`.
8. Recording is opt-in per process: the endpoint has no default evidence directory and says so
   on stderr. `EVIDRA_EVIDENCE_DIR` applies to the read side only. `--actor-id` sets the
   accountable actor on substantive events; lifecycle events stay attributed to the recorder.
9. Do not advertise capabilities the wrapper cannot cover (§44) — prompts, resources and
   completions stay unadvertised unless `--advertise-passthrough`.
10. Out of scope by §44: generic MCP gateway or multi-upstream multiplexing, HTTP/SSE
    transports, domain verification, risk scoring, policy/HITL, legacy compatibility shims.
    No repo split, and no module-path change (`samebits.com/evidra`).

## Measurement discipline

- Report `not_measurable` instead of a number when the data cannot support one (recovery after
  a first block, with zero blocks, is the standing example).
- A gate is not passed because the code exists. Gate C is recorded as **not passed** until a
  real operational upstream and a reader who did not build the summary agree it changed their
  understanding.
- Do not re-run paid model arms beyond what is already spent; free-tier arms reproduce the
  probe artifacts.
- **Regrade; do not hand-read artifacts.** `--regrade` recomputes verdicts from persisted
  transcripts and re-checks the analytics invariants. A script reading `result.json` by hand
  sits outside every guard the harness has — one did report `0/8` because it read a key that
  does not exist.
- Every run records source revision and binary hashes, and the runner refuses a `bin/`
  artifact older than its sources. An invalid experiment does not error; it produces data.
- Deletion is not a milestone (§43): land a removal only when the new vertical path is the
  path being measured, and say in the commit what the removal made impossible to confuse.

## Environment

- `EVIDRA_EVIDENCE_DIR` — default root for `evidra summarize|verify --dir`.
- `EVIDRA_ACTOR_ID` — default for `evidra-mcp --actor-id`.
