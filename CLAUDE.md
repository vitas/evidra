# CLAUDE.md

Guidance for agents working in this repository. More specific instructions (the user, the
plan) take precedence.

Evidra Core is an MCP execution-evidence recorder, not a scoring platform. One endpoint
wraps one upstream MCP server, merges two local tools into its tool list, enforces protocol
order, and writes a signed evidence chain that a human reconciles afterwards.

**Read before changing behaviour:**
[`docs/architecture.md`](docs/architecture.md) defines the runtime boundaries;
[`docs/evidence-format.md`](docs/evidence-format.md) defines the event and trust model;
[`docs/validation.md`](docs/validation.md) records what has and has not been measured;
[`docs/getting-started.md`](docs/getting-started.md) and
[`docs/cli-reference.md`](docs/cli-reference.md) are what a user reads. The approved
design and implementation plan for active work live under `docs/plans/`.

## Build & Test

```bash
make build            # bin/evidra, bin/evidra-mcp
make test             # go test ./cmd/... ./pkg/... -v -count=1
make fmt lint tidy
go build -o bin/evidra-fixture ./cmd/evidra-fixture/    # conformance upstream
go build -o bin/evidra-gatea ./cmd/evidra-gatea/        # experiment runner
go test -race ./pkg/evidence/... ./pkg/proxy/... ./pkg/report/... ./cmd/evidra-fixture/... ./cmd/evidra-gatea/...
golangci-lint run ./...        # must report zero issues
bash tests/run_guards.sh       # every shell guard, no exclusion list
```

CI references are themselves checked: `tests/test_ci_workflows_resolve.sh` fails if a live
workflow names a make target, script, package path, config or `-run` pattern that does not
exist, and `tests/core-workflows.txt` declares every workflow as `enabled` or `disabled`
(a `disabled` workflow must carry its refusal marker). The package graph is an exact declared
set in `tests/core-packages.txt`, not a count floor.

Single case: `go test -run 'TestName' -v ./pkg/proxy/`. Gate artifacts cite tests by name, so
renaming or deleting one means updating the artifact in the same commit — a checklist row
pointing at a function that no longer compiles is not evidence of anything.

## Documents

Everything in the tree is English — code comments, commit messages, gate artifacts, design
documents, and reports about the work. "It is addressed to one reader" is not an exception:
a document under `docs/` is read by whoever opens `docs/` next, and a report they cannot
read is not a record of having found something, only a claim about it. If a claim in a
document goes stale, correct it in place and say what changed; do not carry the stale
sentence forward as though it had been reviewed.

Public documentation is a tested surface: five canonical documents own one topic each
(`docs/getting-started.md`, `docs/cli-reference.md`, `docs/architecture.md`,
`docs/evidence-format.md`, `docs/validation.md`), the guards
`tests/test_public_docs_structure.sh` and `tests/test_public_claims.sh` hold the line, and
history is preserved by Git, not by an archive directory.

Notes written for one reader in another language go in `/local-notes/`, which is gitignored.
The folder is ignored rather than merely unused so the rule holds without anyone
remembering it: a draft written there cannot be swept into a commit by a blanket `git add`,
which is the mechanism that once put 53 JPGs into this repository's history.

## Git

- **Always ask before pushing.** Nothing leaves this repository without the user.
- Sign every commit: `git commit -s` (DCO `Signed-off-by`).
- No history rewrites, no force-pushes, do not touch `main`.
- Stage by path. A blanket `git add -A` once swept untracked scratch into history here;
  `/output/` and `/tmp/` are gitignored because recorder directories hold ephemeral signing
  and digest keys that must not reach history.

## Layout

- `cmd/evidra-mcp/` — the merged endpoint. The only binary that enforces or records.
- `cmd/evidra/` — read side only: `summarize`, `verify`, `version`. A fourth command needs a
  plan change, not a registry entry.
- `pkg/proxy/` — endpoint (`endpoint*.go`). The legacy hosted relay and its mutation
  heuristics are deleted; do not reintroduce content-based classification.
- `pkg/evidence/` — evidence model **v2 only**: `store_v2.go`, `event_v2.go`, `digest_v2.go`,
  `verify_v2.go`, `canon_jcs.go`. One schema, one writer per directory.
- `pkg/report/` — reconciliation into `summary.json`.
- `cmd/evidra-gatea/`, `cmd/evidra-fixture/` — the measurement harness. Not shipped product,
  but **not disposable test code either**: it produces the evidence the product claim rests
  on, so it follows the provenance, analytics-invariant and regrade discipline summarized in
  `docs/validation.md`. Arm definitions live in `cmd/evidra-gatea/arms.json`.
- `ui/` — the static landing surface. Nothing in the evidence path imports or serves it.

## Rules the implementation exists to keep

1. **Enforcement is protocol-only.** `--enforce=all` refuses `tools/call` while no operation
   is open; `--enforce=off` observes. Never gate on argument content, tool names, or
   annotations.
2. **Write ordering is load-bearing:** append `execution_started` before forwarding, and
   `execution_finished` **before** relaying the response.
3. **The store-failure contract has three branches** — refuse the call, relay-but-degrade, and
   never acknowledge a report without durable evidence. Do not collapse them into one "log the
   error" path, and never fabricate an upstream failure.
4. **Provenance is one of exactly three values.** Derived classes such as
   `unprescribed_execution` are read-time computations, never stored event types.
5. **Never average across comparison domains:** enforcement mode, upstream, and
   `digest_key_id` stay separate in `pkg/report`.
6. **Chain validity, signature validity and coverage are three statements;** a valid chain can
   still be incomplete.
7. **Privacy is a bound:** arguments leave the process only as
   `HMAC(digest_key, JCS(args))`; results are hashed up to 4 MiB, else `omitted_oversize`. Raw
   observed payloads and key material never enter the chain or `summary.json`.
8. **Recording is opt-in per process:** the endpoint has no default evidence directory and says so
   on stderr. `EVIDRA_EVIDENCE_DIR` applies to the read side only. `--actor-id` sets the
   accountable actor on substantive events; lifecycle events stay attributed to the recorder.
9. **Do not advertise capabilities the wrapper cannot cover** — prompts, resources and
   completions stay unadvertised unless `--advertise-passthrough`.
10. **Out of scope:** generic MCP gateway or multi-upstream multiplexing, HTTP/SSE
    transports, domain verification, risk scoring, policy/HITL, legacy compatibility shims.
    No repo split or legacy module-path compatibility shims.

## Measurement discipline

- Report `not_measurable` instead of a number when the data cannot support one (recovery after
  a first block, with zero blocks, is the standing example). Report `n/a` when the metric has no
  referent in that cell at all: the harness's `none` baseline arm renders every protocol metric
  that way, and a zero there would read as compliance failure.
- The harness-only mode `none` (agent → `evidra-fixture`, no endpoint) is not a product mode: no
  `evidra-mcp --enforce=none` exists and none may be added. Its grading is operational-only, its
  provenance names no endpoint binary, and it certifies no gate.
- A gate is not passed because the code exists. Gate C is recorded as **not passed** until a
  real operational upstream and a reader who did not build the summary agree it changed their
  understanding.
- Do not re-run paid model arms beyond what is already spent; free-tier arms reproduce the
  probe artifacts.
- A deleted package must be removed from `tests/core-packages.txt` in the same commit that
  deletes it; a workflow step may not outlive its target. Green that checks nothing is worse
  than red that checks something.
- **Regrade; do not hand-read artifacts.** `--regrade` recomputes verdicts from persisted
  transcripts and re-checks the analytics invariants. A script reading `result.json` by hand
  sits outside every guard the harness has — one did report `0/8` because it read a key that
  does not exist.
- Every run records source revision and binary hashes, and the runner refuses a `bin/`
  artifact older than its sources. An invalid experiment does not error; it produces data.
- **Deletion is not a milestone:** land a removal only when the new vertical path is the
  path being measured, and say in the commit what the removal made impossible to confuse.

## Environment

- `EVIDRA_EVIDENCE_DIR` — default root for `evidra summarize|verify --dir`.
- `EVIDRA_ACTOR_ID` — default for `evidra-mcp --actor-id`.
