# Changelog

## Unreleased

Historical entries may reference paths that are no longer present in the working
tree. Those files remain available in the tag or revision that introduced the
entry and in Git history.

### vNext experiment — step 3a: protocol enforcement (branch `vnext/mcp-recorder`)

- `--enforce=all` (default) implements the single rule of §7: with no open operation an upstream `tools/call` is not forwarded, is noted as a `protocol_violation`, and the agent gets a tool result — not a transport error — containing `no_open_operation`, the tool it tried, and the instruction to prescribe and retry. Declared read-only tools are blocked by the same rule; annotations stay reporting data.
- `--enforce=off` forwards everything and never blocks, so prescription coverage in that mode measures voluntary behavior. An unsupported mode (`--enforce=mutations`) fails at startup listing the two valid values.
- Enforcement resumes the moment an operation closes: a report returns the session to blocked state, tested through the fixture's own restart counter so a blocked call is proven never to have reached the upstream.
- `cmd/evidra-mcp` wrapping flags moved into `registerProxyFlags`/`dispatch` to keep `run()` inside the lint budget, and they hold `flag` pointers rather than copies.
- 3 new endpoint tests (blocked-and-proven, observe-only, unknown mode); 14 endpoint tests green with `-race`.
### vNext experiment — step 2 (branch `vnext/mcp-recorder`)

- `evidra-mcp --proxy -- <server>` is now the vNext merged endpoint: one upstream child process plus Evidra's own `evidra_prescribe` / `evidra_report`, appearing to the agent as a single MCP server. The pre-vNext relay stayed reachable as `--legacy-proxy` until the §59 step-10 prune.
- Tool-list composition follows §27: local tools appear on the first upstream page only, upstream cursors and per-tool JSON survive byte-identically, and `notifications/tools/list_changed` is forwarded and re-merged. §6 is enforced at startup — an upstream advertising `evidra_prescribe` or `evidra_report` makes the endpoint refuse to start rather than rename anything (tested through a new `--steal-names` fixture flag).
- `initialize` is composed rather than echoed: client-facing capabilities are the supported profile with untested families advertised down (`--advertise-passthrough` opts them back in), protocol version is negotiated explicitly, and upstream instructions are appended behind a source boundary instead of being overwritten.
- Direction-safe bookkeeping per §28: requests, notifications and responses are classified by method+id shape, per-direction pending tables make a client request id and an upstream server-to-client request id collide harmlessly, and pending client requests are answered with an explicit error when the upstream dies instead of hanging. Framing rejects oversized frames with the session intact.
- §11 and §32 behavior is live in memory: one open operation per process, `operation_already_open` with `continue_current` / `abandon_and_replace`, `operation_id_mismatch`, `no_open_operation`, and an idempotent `already_reported` for a duplicate close. Durable evidence lands with the v2 store in step 4 behind a narrow recorder seam.
- 10 endpoint conformance tests in `pkg/proxy/endpoint_test.go` drive the real fixture and the real CLI as child processes, including the direct-vs-wrapped list equivalence that is step 2's exit criterion.
### vNext — stale "current" reference docs and the guards that measure the working tree

- Deleted `docs/api-reference.md` (REST endpoints of the removed hosted service) and
  `docs/benchmarks/tool-surface-size.md` (measured the `run_command` tool surface that no
  longer exists). Both declared `Status: Reference` / `Version: current`, which is the same
  defect as `Status: Normative` at lower volume. `docs/product/backlog.md` was **left
  alone**: it is untracked, so it is not part of what this branch publishes.
- Every pre-vNext banner-marked document now says so in its own header: `Status: Historical
  (pre-vNext surface)` and `Version: frozen at removal`, replacing headers that claimed
  currency two lines below a banner denying it. `guides/signal-validation.md` and
  `guides/argocd-gitops-integration.md` gained the missing banners.
- `docs/guides/mcp-registry-publication.md` keeps `Version: current` deliberately: the
  server it describes still exists as `evidra-mcp`, and `test_mcp_registry_publication_guide`
  still passes against real content.
- `test_repo_cleanup_hygiene` checked the **working tree** for `.DS_Store`, so it failed on
  Finder noise inside ignored directories — including `output/`, which the recorder writes
  during every experiment. It now checks tracked and staged files, which is what a push
  carries. A repository guard that fails on local noise teaches people to disregard
  repository guards.
- Dead links rewritten in README and `guides/self-hosted-setup.md`.

### vNext — tracked binaries and the ignore rules that hid them

- Stopped tracking `cmd/evidra-gatea/evidra-gatea` (9.7 MB) and `evidra-exp` (9.5 MB): neither
  is referenced by any build, test, workflow or document, and the harness builds into the
  ignored `bin/`. Their objects stay in history by decision, not oversight — `evidra-exp`
  entered through a commit that is an ancestor of `main`, and `evidra-gatea` through an
  already-pushed branch, so removal means rewriting shared history or force-pushing, and
  GitHub keeps either blob SHA-reachable until it purges. The first commit about this claimed
  to untrack the files and had changed only `.gitignore`: a `git add` of the paths by name
  re-staged them, and a tracked file is never un-ignored. Fixed forward, not amended.
- `test_repo_cleanup_hygiene` gained the two checks that make that class visible: no tracked
  file may be one `.gitignore` claims (`git ls-files -i -c --exclude-standard`), and no
  tracked file may exceed 1 MiB. The first found one real hit immediately: `.gitignore` listed
  `CLAUDE.md`, which is tracked and is the guidance file every agent in this repository reads.

### vNext — the CI fix broke CI twice, and both shapes are now checked locally

The second one was a deleted `e2e` job still named by two `needs:` lists in `release.yml`.
GitHub answers that with a run that has no jobs, no step log and a red mark, so the failure was
invisible until someone read the checks API. `tests/ci_workflow_structure.py` checks mappings
that must not be empty, `needs:` targets that must name a job in the same file, and acyclicity
of the needs graph — as a separate program, because the first version of the check was a bash
heredoc that never installed itself and reported PASS over the reference it was written to
catch. All four rules were mutation-tested by re-inserting the broken shapes and watching each
fail with a file, line, job and target named.

### vNext — the harness grew a no-Evidra baseline arm (`mode none`)

`off` and `all` both wrap the upstream, so together they measure what enforcement costs and
nothing about what *introducing Evidra* costs. `cmd/evidra-gatea` now plans a third, harness-only
mode: agent → `evidra-fixture`, no endpoint process, no evidence directory, no protocol tools,
and the task's own `baseline_goal` prompt instead of the wrapped one. There is no
`evidra-mcp --enforce=none` and none was added; `--modes-only` rejects anything outside
`none,off,all` rather than silently planning nothing.

- Grading split into operational clauses (upstream tools, arguments, order, no strays) and
  protocol clauses (`require_report`, prescription-before-action, replacement/abandonment).
  `none` is graded on the operational set only, so a baseline run cannot fail for want of a
  report it was never able to make; wrapped grading is unchanged and asserted to be unchanged.
- Protocol metrics in a `none` cell render `"n/a"`, never `0`, and a `none` cell enters no Gate A
  condition; `summary.json` carries `gate_applicability` saying which cells certify what.
- Provenance gained `execution_path` (`direct_fixture` | `evidra_proxy`); a baseline run records
  `endpoint_binary_sha256: ""` because no endpoint binary participated — hashing one would
  attribute the measurement to a program that produced nothing.
- Freshness now asks what participated: a baseline-only run set does not require
  `bin/evidra-mcp` to be current, and still requires the fixture *and the runner* (the runner's
  own freshness was never checked before).
- `--regrade` works across both topologies and shares one grading function with the live path.

Two latent defects surfaced by that sharing, both recorded rather than quietly fixed:

- **An enforcement hole could not fail a run.** `applyStoreFacts` appended
  `"enforcement hole: N executions reached upstream with no open operation"` (and an invalid
  chain/signature) to `res.Failures`, and the next statement assigned
  `res.Failures = task.evaluate(tr)`, overwriting it. No recorded verdict changes: no row in any
  archived run set (166 rows across `output/gatea/*`) has an unprescribed execution or an
  invalid chain, so the clause never had anything to report. Verified by regrading a copy of the
  official set — 80 rows, 0 verdict diffs.
- **`duration_ms` was declared, marshalled and never assigned**, reading `0` through the whole
  official experiment. It is now measured, totalled per cell, and the mean is invariant-checked
  against its own numerator and denominator.

### vNext — the module graph still required everything the prune deleted

After §43, `go.mod` required `modelcontextprotocol/go-sdk`, `jsonschema-go`, `terraform-json`,
`go-yaml`, `protobuf` and five `otel` modules that no surviving file imports: `go mod tidy` had
never been run against the pruned tree, and nothing checked it. Tidying drops 41 `go.mod` lines
and 92 `go.sum` lines, and leaves one external dependency - `oklog/ulid/v2`.

The cost was not disk space. Dependabot was opening bumps against packages the shipped binary
never linked, those bumps merged green, and each one read as a change to the artifact users run.
`docs/ARCHITECTURE.md` also named the SDK as a dependency in prose describing the current
surface, which is how a false claim survives: it sits in the file that is supposed to be the map.

- `tests/test_module_graph_tidy.sh` (in the guard suite) fails when `go.mod`/`go.sum` are not the
  tidy form of the tree, and restores them afterwards so running the check is not an edit.
  Mutation-tested: with the pre-tidy `go.mod` it fails and prints the diff; with the tidied one,
  13/13 guards pass.

### vNext — the CI fix itself broke CI once, and now that is checked

The first version of the fix below deleted `VNEXT_MIN_PACKAGES` by replacing it with comment
lines, leaving `env:` with nothing under it. That is valid YAML and an invalid workflow: GitHub
created a run with no jobs, no log, and a failure — the least diagnosable shape available.
`tests/test_ci_workflows_resolve.sh` now rejects a mapping that must have entries (`env`,
`with`, `jobs`, `steps`, `permissions`, `outputs`) when only comments follow it, and the rule
was mutation-tested by re-adding the empty block and watching the guard fail. `pull_request:`
and `workflow_dispatch:` are exempt because an empty trigger is legal, which the first draft of
the check got wrong by flagging three valid files.

### vNext — CI was pointing at deleted things

- `ci-vnext.yml` ran `go test -race ./internal/lifecycle/...` after the §43 prune deleted that
  package, and failed the first pushed run of this branch. `ci.yml` called four Make targets
  (`e2e`, `test-contracts`, `test-signals`, `prompts-verify`), five shell guards, a `prompts/`
  directory and a signal-validation artifact path that likewise no longer exist; `release.yml`
  did the same plus uploaded `tests/inspector/out/latest.log`, and `.goreleaser.yaml` still
  built `./cmd/evidra-api`. Both workflows had been edited for pages at a time while their
  contents drifted.
- `tests/test_ci_workflows_resolve.sh` now fails if a live workflow names a Make target,
  script, package path, config file or `-run` test pattern that does not exist — including the
  vacuous case where `-run` matches no tests and the step reports success while testing
  nothing. Every workflow must be declared in `tests/vnext-workflows.txt` as `enabled` or
  `disabled`; a `disabled` one must contain its refusal marker, so disabled cannot rot into
  "silently broken but still runnable". `release.yml` is disabled until Gate C by a first step
  that refuses loudly and says why, because shipping vNext is a human decision, not a cleanup.
- `tests/run_guards.sh` runs every `tests/test_*.sh` with no exclusion list; `ci.yml` calls it.
- `VNEXT_MIN_PACKAGES: 8` is gone, replaced by `tests/vnext-packages.txt` compared against
  `go list ./...` in both directions by `tests/test_supported_package_graph.sh`. The floor was
  passing at 9 packages while one of them — the root package `samebits.com/evidra`
  (`uiembed.go`, `uiembed_embed.go`) — existed only for the deleted `evidra-api`'s
  `embed_ui` build tag and imported nothing. Deleting it leaves 8, which is the point where a
  count floor stops meaning anything.
- `docs/integrations/cli-reference.md` was rewritten to the shipped surface: it had been
  banner-marked pre-vNext while still being the README's "CLI reference", and every command it
  documented was deleted. It now describes `evidra-mcp --proxy` and `evidra
  summarize|verify|version`, keeps the "Evidra does not sandbox the wrapped command" boundary
  that `test_doc_trust_alignment` requires — true of the upstream process the endpoint execs —
  and is verified by `scripts/check-doc-commands.sh`, which now runs those flags and help
  texts instead of driving `evidra prescribe` (deleted) and asserting a README mention of
  `EVIDRA_SIGNING_MODE` (which never existed in the Go sources of this branch).
- README's environment table listed `EVIDRA_SIGNING_MODE`, `EVIDRA_SIGNING_KEY` and
  `EVIDRA_ENVIRONMENT`; none appears in the code. The table is now the two variables the
  binary reads, and the checker fails if documented and read sets ever differ in either
  direction.
- `test_fixture_snapshot_names` was red on one word: the plan's §59 verification column still
  said "summary golden fixtures". It says "summary snapshot fixtures" now — the guard's own
  rule, not an exception for the plan.
- `pkg/proxy`: `annotationsFor`'s §22/§26 comment sat directly above `lookupFwd` with no blank
  line, so gofmt-clean Go read the whole block as `lookupFwd`'s documentation and the invariant
  about preserving absent annotations was attached to the wrong function. The dead
  `failAllOutstanding` indirection is gone.

### vNext — history scrub before the first push

- Correction to an earlier entry in this file: `test_bump_version_script` is **not** red by
  nature, it passes (`2/2` after `go build ./...`). The recorded red was an exec stall on a
  freshly built binary, and the supporting check I ran looked at the wrong file — the guard
  asserts on a temporary copy of `CHANGELOG.md` that `scripts/bump-version.sh` rewrites, not on
  the repository copy. Only `test_fixture_snapshot_names` is red, and it points at legacy
  naming in files this branch never touched.

- 54 scratch files under `tmp/` (53 of them JPG pages of a sales-review deck) and 5 under
  `examples/` were removed from the branch's **history**, not just its tip: a blanket
  `git add -A` had committed them in the §43 step-2 commit, and the follow-up that untracked
  them left them in that commit's tree forever. `main` never carried these paths
  (`git log --full-history main -- tmp examples` → no commits), and the branch had never been
  pushed, so this was the last point where removal cost nothing.
- Narrow rewrite: one commit amended, its 27 descendants replayed. 28 hashes changed, 0
  contents did — the tip tree is identical (`b70935a4`), `main..HEAD` is still 51 commits, the
  merge-base with `main` is still `fe213d12`, and every commit keeps its DCO trailer.
  `git filter-repo` was tried and rejected: it re-encoded back to the root, changing 857 hashes
  and moving the merge-base 800 commits backwards.
- `docs/system-design/vnext-history-scrub.md` records the method, the checks and the old → new
  hash table, so citations written before the scrub stay resolvable.

### vNext — repository consistency pass before push

- **Pre-vNext normative specs deleted** rather than archived in place: seven
  `docs/system-design/EVIDRA_*V1` documents (hosted architecture, core data model with
  `CanonicalAction`/`prescribe_full`/`prescribe_smart`, REST ingest protocol, scoring model,
  signal spec, prompt factory, end-to-end example), `system-design/scoring/default.v1.1.0.md`,
  and both `docs/contracts/` V1 contracts. Each declared `Status: Normative` or `Active` for
  subsystems this branch no longer contains, which made them a second competing spec rather
  than history. Git keeps them.
- **12 shell guards deleted** whose assertions were about the shape of those documents or of
  already-pruned folders (`test_protocol_docs`, `test_split_prescribe_docs`, `test_mode_labels_docs`,
  `test_scoring_rationale`, `test_hosted_architecture_docs`, `test_external_ingest_docs`,
  `test_cli_command_rebranding`, `test_signal_validation_harness`, `test_no_legacy_benchmark_surface`,
  `test_oss_dataset_corpus`, `test_acceptance_corpus_promotion`, `test_unified_artifact_layout`), plus `tests/tests-index.md`,
  `tests/E2E_TESTING.md` and three orphaned SARIF fixtures. `test_governance_baseline` now
  requires the guards covering claims still made publicly, and the surviving guards had their
  executable bits restored — the `-x` check had been passing by selecting for the wrong property.
- Dead inbound links rewritten to say what happened: README's spec list, `integrations/cli-reference`,
  `guides/signal-validation`, `guides/terraform-ci-quickstart`.
- `vnext-prune-plan.md` → **`vnext-prune-record.md`**: reframed as a record, execution table
  closed through step 8 with commit hashes, and the stale claims ("ARCHITECTURE.md still
  describes the pre-vNext product", two rows "pending") removed. Links updated tree-wide.
- `docs/system-design/vnext-mcp-recorder.md` §43 now states the criterion the prune actually
  ran under — new vertical path is the path being measured, plus green Gate B, decided
  explicitly by a human — while Gate A's formal verdict stays `not passed` and Gate C governs
  merge/product. The tree previously carried three incompatible versions of this.
- Terminology boundary: "false record" in `gate-a-results.md` is qualified as *a claim the
  fixture predicate contradicts* — the oracle belongs to the harness, and Evidra can never
  assert falsity, only place a claim beside observations.
- `docs/ARCHITECTURE.md`'s headline now reads "what it actually did **through the wrapped MCP
  boundary**", so the scope limit is in the first sentence rather than twenty lines below it.
- Known-red, not caused by this pass and left red deliberately: `test_bump_version_script`
  expects a `## v0.4.7 — 2026-03-11` CHANGELOG heading that is absent on `main` too, and
  `test_fixture_snapshot_names` objects to naming in files this branch does not touch.


- Follow-up sweep of the same seam: the prune record still carried future-imperative voice
  ("what leaves the path"; "Delete the v1 evidence shapes … then rewrite
  `docs/ARCHITECTURE.md`, which still describes the pre-vNext product") and a duplicated,
  superseded account of §43's precondition. Both are past tense now, with the commit hashes
  that closed each step, and the §43 story is told once.
- The implementation report no longer states a commit count at all: it had gone stale three
  times while being maintained (41 → 47 → 49), so the number is replaced by
  `git rev-list --count main..HEAD`, with the lesson written next to it.
### vNext — the revised §45 bars cannot certify the run set that produced them

- `docs/system-design/gate-a-results.md` adds the mechanical comparison of the official
  80-run set against §45's revised bars — **every cell meets them** — and keeps
  `gate_passed=false`, with the reason stated: bars chosen after seeing the data cannot
  certify that data. §45 now says so explicitly in the plan.
- §45 also fixes bar semantics when a cell is short of 16: comparison on counts with
  `invalid_runs` reported beside the denominator, not on rates. Comparing rates would let a
  cell that dropped its hardest runs outscore one that kept them, and dropping runs is
  cheaper than passing them.
- Without this, the natural reading of the revised bars plus the existing table was "Gate A
  effectively passed" — which is the precise failure mode the revision was most exposed to,
  introduced by me while arguing against a bar that blocked dead-code deletion.

### vNext — the reconciliation view, and the harness as a first-class product surface

- `pkg/report` now emits `reconciliation_view` per operation: `declared` (the agent's own
  objective, verbatim), `observed` (execution count, succeeded, failed/cancelled, unpaired,
  unknown-status, server-declared read-only vs not-declared, always
  `annotations_verified: false`), `reported` (the claim, or an explicit
  `present: false`), and a `stance` line saying Evidra does not judge the claim. The terminal
  summary renders the same four blocks per operation. No new information: the arrangement
  exists so a reader does not reconstruct the disagreement from arrays.
- The status buckets partition `count` exactly once each, including an `unknown_status`
  bucket, with a test asserting the sum equals the total — a view whose parts stop adding up
  to the whole is the 28/16 bug wearing a nicer format.
- `not_declared_read_only` is deliberately not named "state-changing": an unannotated call
  may have been read-only, and the absence of a claim is not a counter-claim (§27).
- New `docs/system-design/vnext-experiment-harness.md`: the harness invariants (provenance
  must match the source revision evaluated, the five analytics invariants, regrade-don't
  hand-read, regrade-is-a-re-reading, `not_measurable` is a result, checks must not cry wolf),
  justified by the three measurement errors on this branch rather than by taste.
- `docs/ARCHITECTURE.md` adopts the sharper thesis after the probes — *Evidra does not decide
  whether an agent's claim is true; it preserves the claim beside independently observed
  execution evidence so unsupported or surprising claims become inspectable* — and records the
  boundary of the in-band feedback: post-operation protocol feedback, not a verifier, not a
  precondition for the claim being accepted. It arrives after `achieved` is written, so it can
  inform the next operation and cannot correct this one.
- `CLAUDE.md` gains the regrade rule and the provenance rule.

### vNext experiment — correction: the second probe's task-success row was wrong

- `docs/system-design/vnext-gate-c-reconciliation-probe.md` reported task success `0/8` for
  both probes. The numbers came from a hand-written script reading `result.json`'s
  `task_success` key, which does not exist (the field is `success`), so every run read as a
  failure. Re-graded from the persisted transcripts: before **4/8** (`all` 2/4, `off` 2/4),
  after **6/8** (`all` 2/4, `off` 4/4).
- The correction is kept in the artifact rather than silently overwritten, along with what
  it does *not* support: the improvement sits entirely in `off` mode, two variables moved at
  once, and n=4 per cell. It is also the second instance of the same failure mode as the
  stale binary — analysis performed outside the harness by reading artifacts ad hoc — which
  is why "regrade, do not hand-read" is now stated as the mitigation instead of pretending
  invariant checks would have caught it.

### vNext experiment — invariant checker split for complexity (follow-up to 95da55a)

- `checkRunInvariants` exceeded the cyclomatic-complexity ceiling, so the three property
  families are now `checkCellBounds`, `checkAgreementWithRows` and `checkTranscriptsPersist`
  over a thin dispatcher. The commit that added it shipped one lint issue over the line;
  fixing forward is cheaper than pretending the ceiling does not apply to harness code.

### vNext experiment — the harness records build provenance and refuses inconsistent analytics

- Every graded run now carries `provenance`: source revision, whether the tree was dirty,
  and sha256 of the endpoint, fixture and runner binaries it was measured against, plus the
  model id. `summary.json` collapses those into one entry per distinct build, or one entry
  per build plus the run counts each contributed when a cell mixed builds.
- `checkRunInvariants` runs before the gate is judged, and a violation is exit code 4 —
  distinct from 3 (an invalid run), because the harness itself producing untrustworthy
  analytics is the more serious failure. It enforces: bounded numerators (`0 <= metric <=
  runs`), companion metrics no larger than valid runs, per-cell run counts equal to graded
  rows, per-run sums equal to cell aggregates, one build per comparison cell, and every
  graded run pointing at a transcript that still exists.
- `--regrade` now reports provenance drift instead of quietly re-reading: runs from another
  endpoint build are named with their hash, and runs recorded before provenance existed are
  called *unattributed* rather than assumed to match the current build.
- Tests cover both directions — a consistent rollup built by the real code path must pass
  (otherwise the check is noise), and each invariant must fire on the specific mistake it
  exists for, including `task_success = 28` with 1 run, the shape of the bug that happened.

### vNext experiment — the runner refuses to measure a stale build

- `cmd/evidra-gatea`'s preflight now compares `bin/evidra-mcp` against `cmd/evidra-mcp`,
  `pkg/proxy`, `pkg/evidence` and `pkg/report`, and refuses to start when a source file is
  newer than the binary it is about to measure; `--allow-stale-build` opts out for the one
  case where comparing against an older build is the point.
- This is not a hypothetical guard. The first probe of the in-band feedback reported zero
  feedback deliveries across 8 sessions, and the tests proving the feature worked passed
  the same moment: the Go tests build their own endpoint binary in a temp directory, while
  the runner shells out to `bin/evidra-mcp`. The probe had measured a build from before the
  feature existed, and produced a well-formed run set that looked like data about the code
  under review.
- Test-only edits do not trip the check (they cannot change the binary), with a test that
  exists to keep that property: a guard that blocks work over a comment edit gets worked
  around, and the workaround is worse than having no check.

### vNext experiment — §47 in-band `evidra_report` feedback

- A closing `evidra_report` now answers with an `observations` object: executions the
  proxy saw inside that operation's window, succeeded vs failed-or-cancelled, the tool
  names touched (capped at 5 distinct), and the §38.C read-only conjunction
  (`all_observed_declared_read_only`) always accompanied by
  `annotations_verified: false` and `provenance: proxy_observed`. When the window was
  empty the note says so in the response, which is the case the mechanism exists for: an
  agent claiming achievement with nothing observed is told before it acts again, not in a
  file after the session.
- Bounded by construction: 128 tracked operations, oldest evicted and the eviction count
  reported in the next render (`older_operations_dropped`), so feedback that nobody reads
  cannot grow with the session.
- Counted from the proxy's own view rather than from the store, so it works with recording
  switched off, and `finishExecution` notes the observation **before** its durable-id
  check — an execution that happened but could not be persisted still happened, and
  feedback that silently depended on `--evidence-dir` would be a different feature than
  the one §47 asked for.
- No verdict fields. §35 forbids rebuilding a human summary inside `evidra_report`, so
  nothing here says achieved, score, or risk, and a test asserts that absence rather than
  trusting intent.
- `docs/system-design/vnext-gate-c-reconciliation-probe.md` was corrected to say the item
  is implemented **after** the probe's 8 transcripts were recorded, so the artifact does
  not imply the new response was measured.

### vNext experiment — §43 step 7: documentation follows the code, and `--actor-id` comes back

- `docs/ARCHITECTURE.md` rewritten as the vNext map. Its old opening line — "Evidra is a
  wire tap, not a gatekeeper… it never blocks operations" — is now false, and the rewrite
  says so explicitly instead of quietly dropping it: vNext gates protocol order and nothing
  else. Replaced the hosted-mode/assessment/two-layer sections with the shipped shape
  (endpoint, v2 store, reconciler, two CLIs), the nine invariants the code is built around,
  and a gate table that still says Gate C is not passed.
- `README.md` got a stated gap rather than a rewrite: its tagline advertised "reliability
  scoring", which no longer exists on this branch. The banner says what ships and where it
  is documented, and defers the full rewrite until the gates feed a product decision —
  advertising a vNext story in the README before Gate C is answered would be the same
  mistake in the opposite direction.
- `CLAUDE.md` rewritten to match: build/test commands that exist (no `e2e`, no
  `canon-fixtures-update`, no `docker-api`), the package list after the prune, the §7/§17/§18
  rules that a new agent must not "helpfully" violate, and the git discipline (ask before
  push, DCO, stage by path).
- `--actor-id` is restored to the endpoint with `EVIDRA_ACTOR_ID` as its default, plus
  `TestActorIDFlagReachesEveryEvent`. It had been deleted as collateral with the other legacy
  flags, but actor is part of the v2 envelope (§13): removing it left no way to say who was
  accountable, which is the field the whole record exists to attribute.
- The test also settled a question by measurement rather than by argument: `recorder_started`
  and `recorder_stopped` carry no actor even when the flag is set. That is correct — those
  events are `recorder_generated`, the recorder is what started — and the assertion now pins
  it, with a comment recording that the reading was checked by removing the branch and
  watching the result rather than assumed.

### vNext experiment — §43 step 6b: only the v2 evidence model remains in `pkg/evidence`

- Deleted the v1 shapes: `entry.go`, `entry_builder.go`, `entry_io.go`, `entry_store.go`,
  `entry_lookup_cache.go`, `segment.go`, `manifest.go`, `lock.go`, `types.go`,
  `validation.go`, `canonical.go` (the `CanonicalAction` layer), `digest.go`,
  `payloads.go`, `evidence.go`, `trace.go`, `bundle.go`, `signer_ephemeral.go`,
  `chain_test.go`, `external_bundle_test.go` and the v1 unit tests, plus `pkg/evlock`
  (its only user was the v1 lock), `scripts/gen_external_bundle`,
  `tests/external_bundle_v1` and `internal/testutil` (a v1 signer helper nothing
  imported any more).
- `pkg/evidence` is now five files: the store, the event envelope, the digest/JCS layer,
  the verifier, and their tests. "Old evidence adapters not used by v2 schema" (§43)
  is satisfied structurally: there is no second schema in the package to confuse it with,
  and no shared file lock negotiating with the single-writer rule.
- Every evidence test named in `vnext-gate-b-results.md` was re-run by name after the
  deletion, because a conformance table that cites a test nobody compiles any more is
  how a gate quietly stops existing.
- The ephemeral-signing-key warning that used to appear in unrelated test output came
  from the v1 signer; with it gone, the only key notices left are the v2 store's own.

### vNext experiment — §43 step 6: the analysis engine, prompt contracts and export layer are deleted

- Gone: `internal/{canon,assess,risk,score,detectors,signal,pipeline,sarif,promptfactory}`,
  `pkg/export` (the anonymized evidence bundle), `pkg/execcontract`, the `prompts/`
  contract tree it embedded, and their suites (`tests/canon_fixtures`,
  `tests/signal-validation`, `tests/artifacts`). Makefile loses
  `canon-fixtures-update`, `prompts-generate`, `prompts-verify`, `test-signals`.
- §43's remaining lines are all here under different names: risk, score,
  canonicalization, detectors, behavioral signal engine, SARIF input, prompt generation.
  What is left of the module is 9 packages (`go list ./... | wc -l`; the count was 12 when
  the entry was first written, three more packages went in the steps after it): the endpoint, the v2 store, the
  reconciler, the fixture, the runner, two CLIs, and version.
- `.github/workflows/ci-vnext.yml` no longer excludes packages, because an empty
  exclusion regex is a trap: `grep -Ev ''` matches every line, so "test everything
  left" would have silently become "test nothing" with a green check. The job now
  lists every package and fails if the graph is smaller than 8, which makes a future
  mass deletion visible instead of certifying an empty set.

### vNext experiment — §43 step 5: the pre-vNext relay and `--legacy-proxy` are gone

- Deleted `pkg/proxy/proxy.go` (the auto-recording relay, its mutation heuristics and
  exit-code sniffing), `pkg/proxy/evidence.go` (the v1 evidence writer),
  `pkg/proxy/detect.go` (`ClassifyCommand`, `ClassifyToolName`, `IsMutation`) and their
  tests, plus `runProxyMode` and the `--legacy-proxy` flag. `pkg/proxy` is now the
  merged endpoint and nothing else.
- Help text and plumbing followed: no default evidence path exists any more for the
  endpoint (recording stays per-process opt-in via `--evidence-dir`, while the read side
  keeps honouring `EVIDRA_EVIDENCE_DIR`), and `resolveEvidencePath`/`envBool` were
  deleted with their only callers.
- Replaced the deleted relay's framing tests with `endpoint_write_test.go`, which caught
  an attribution asymmetry in the endpoint: a write error surfaced at `Flush()` came back
  bare while one caught at `Write()` was labelled with its direction, so a client-pipe
  failure and an upstream failure were indistinguishable in the log. Both are wrapped now.
- `docs/system-design/vnext-gate-b-results.md` and `vnext-prune-record.md` were updated in
  the same commit, because the Gate B checklist named tests that no longer exist and a
  conformance table with dead test names is worse than no table.

### vNext experiment — §43 step 4: the legacy MCP server and its test harnesses go

- Deleted `pkg/mcpserver` (with the embedded `run_command` / `collect_diagnostics` /
  `write_file` / `describe_tool` / `prescribe_smart` / `prescribe_full` schemas),
  `internal/lifecycle`, `internal/assessment`, `internal/evidence`, `internal/config`,
  `internal/telemetry`, `pkg/mode`, `pkg/client`.
- Deleted the harnesses that could only drive those surfaces: `tests/inspector`
  (JSON cases calling tools that no longer exist), `tests/e2e`, `tests/contracts`
  (prescribe/report parity, scoring and risk-escalation contracts against the removed
  CLI), and `tests/testutil`. Makefile loses `e2e`, `test-contracts` and the three
  inspector targets, and the vNext CI exclusion list shrinks to `sarif`.
- A suite that cannot compile is worse than one that is deleted: `tests/contracts` and
  `tests/e2e` were tag-gated out of vNext CI, so nothing would have told anyone they
  broke when the CLI commands they exercised disappeared in step 1.

### vNext experiment — §43 step 3: `evidra-mcp` is the endpoint only

- The direct MCP server mode is gone: `pkg/mcpserver` is no longer started, and with it
  the flags that configured it (`--environment`, `--retry-tracker`, `--signing-mode`,
  `--url`, `--api-key`, `--offline`, `--fallback-offline`, `--full-prescribe`,
  `--transport`, `--port`, `--actor-id`), the online/offline resolution, the API
  forwarding hook, `resolveSigner` and the v1 evidence writer dependency.
- Running the binary with no wrapping mode now exits 2 with "nothing to wrap" plus
  help. It used to start a server with a tool surface the product no longer offers;
  the failure is the honest version of that.
- Help text rewritten around the actual contract: `--proxy` and `--enforce`, the two
  merged local tools, where evidence goes, and how to read it back
  (`evidra summarize` / `evidra verify`). Tests now assert both directions — the
  endpoint flags must be present, and the removed tool names must not be, because an
  agent that reads `run_command` in help output will call it.
- `--legacy-proxy` survives one more step (it is the §59 step-9 "legacy flags" item and
  the plan removes it with the relay it selects, not before).
- Orphan check after this commit: `pkg/mcpserver`, `internal/lifecycle`, `internal/evidence`,
  `pkg/mode`, `pkg/client`, `internal/config`, `internal/telemetry` are imported by
  nothing outside themselves, which is what makes the next deletion a single
  `git rm -r` rather than a refactor.

### vNext experiment — §43 step 2: the hosted chain leaves the module

- Deleted `cmd/evidra-api` and its whole support graph: `internal/api`, `internal/apiutil`,
  `internal/auth`, `internal/db`, `internal/store`, `internal/analytics`,
  `internal/analyticsdb`, `internal/analyticsvc`, `internal/ingest`, `internal/gitops`,
  `internal/automationevent`, and `Dockerfile.api`. `go mod tidy` dropped the PostgreSQL
  driver, the JWT library and gonum with them: the module no longer carries hosted
  storage dependencies, which is a smaller claim to defend than a flag that disables them.
- `ui/` is deliberately **kept** while its runtime coupling goes: the embedded-static
  build target, `docker-api`, the compose `postgres` + `evidra-api` services, and the
  release image matrix entry. §43 removes "old UI/landing runtime coupling", and the
  UI sources are the asset that later moves to the separate product repo — deleting them
  here would turn "no repo split yet" into losing the thing to be moved.
- `.github/workflows/ci-vnext.yml`: the unsupported-package regex no longer enumerates
  deleted packages, so the supported set is now "everything that is left" minus the four
  pre-vNext packages still in the tree (`apicrypto`, `assessment`, `client`, `config`,
  `mode`, `sarif`).
- Fixed a latent flake found by the sweep, not caused by the deletion:
  `internal/score`'s `TestLoadDefaultProfile` compared a float sum of a **map** of
  weights with `!= 1.0`, so it failed whenever Go's randomized iteration order produced
  0.9999999999999999. Now compared with a tolerance, with a comment saying why exact
  equality was never valid there.

### vNext experiment — §43 step 1: the CLI is `summarize`, `verify`, `version`

- `cmd/evidra` lost its pre-vNext command surface (16 files plus tests):
  `scorecard`, `explain`, `compare`, `record`, `prescribe`, `report`, `import`,
  `import-findings`, `prompts`, `detectors`, `export`, `keygen`, `skill`,
  `validate`, and the helper/flag files that only they used. The binary now imports
  exactly `pkg/evidence` (v2), `pkg/report` and `pkg/version`, which is what unblocks
  deleting the hosted chain: `internal/store`, `internal/analytics`,
  `internal/analyticsdb` and `internal/sarif` are no longer reachable from a shipped
  vNext binary.
- §41's thin-CLI claim is a test now, not a sentence: `orderedCommands` must equal
  `{summarize, verify, version}`, each entry has a handler and a description, and the
  usage output is parsed back and checked — a removed name may not reappear, and the
  text must say which binary writes evidence, since the read side cannot record
  anything itself.
- Usage wording follows the code: "read side of the vNext MCP evidence recorder"
  rather than the old marketing line, because the write path is `evidra-mcp --proxy`.
- 11 user-facing docs that document removed commands got a retirement banner pointing
  at §43 and the prune plan, instead of being deleted here: the documentation pass is
  the last prune step, and deleting the description of a format before the code that
  reads it is gone would leave the repo unable to explain evidence it can still verify.

### vNext experiment — Gate A bars revised from measurement, §43 unblocked by decision

- §45's pass bars now state what the 80 runs produced (strong arm 11/15 task success,
  13/15 coverage; cheap arms 10/16–14/16) instead of the pre-measurement `>= 15/16`, with
  the rationale written next to the numbers: leaving an unreachable bar as §43's
  precondition turned a research gate into a statement about model quality.
- `recovery_after_first_block >= 80%` is removed as a **pass condition** and kept as a
  printed metric, because it is unmeasurable on these arms (0 blocks in 80 runs). The
  doc says plainly that removing an unmeasurable bar is not the same as winning it, and
  the recovery claim may not be made until a run exists where a block actually happened.
- §43's precondition is now Gate A + Gate B; Gate C (§47) decides merge/no-merge, not
  whether dead code stays in the branch, and §47 states that the reader must not be the
  person who produced the summary.
- `vnext-prune-record.md` gains the correction found while starting deletions:
  `cmd/evidra` imports `internal/store`/`analytics`/`analyticsdb`/`sarif`, so the legacy
  CLI trim must come before the hosted chain can be deleted.

### vNext experiment — §43 prune: written as a plan, deliberately not executed

- `docs/system-design/vnext-prune-record.md` is the deletion-ready inventory for §43:
  every removal item mapped to the packages and files that implement it, the vNext
  surface that must survive, and a five-step order that keeps CI green at each commit
  (leaves first, then the analysis chain, then `pkg/mcpserver`, then the legacy relay
  together with its tests, then v1 evidence shapes), each step gated on build/test/race
  plus a Gate A re-grade.
- It is not executed, because §43 conditions deletion on Gates A–C being green and
  none of them is: Gate A's strong arm is 11/15 against a 15/16 bar with zero blocks
  in 80 runs, Gate C is explicitly not passed. Deleting the measured legacy path while
  the replacement's central question is still open is the failure §43 warns about,
  so the artifact states the precondition rather than satisfying the checklist item.
- The doc also records the two ways forward: clear the bars as written, or revise them
  to what the cheap arms demonstrably do — with the revision landing in the same commit
  as the first deletion.

### vNext experiment — Gate A counts are now read from the recorder, not inferred

- `cmd/evidra-gatea` reconciles each finished run against its own store
  (`<run>/evidence/recorder-*/events.jsonl`): `blocked_attempts`,
  `unprescribed_executions`, per-operation prescribe/report counts, chain and
  signature validity, and coverage come from signed events, and `counts_from` in
  every `result.json` says which account produced them (`store` or `transcript`).
  The transcript path stays as the fallback for runs that asked for no store, rather
  than being quietly relabelled as evidence.
- An execution recorded with no open operation under `--enforce=all` is now a
  per-run failure (`enforcement hole: …`), not a cell statistic: a number in that
  field means the recorder watched a call get through that it existed to refuse, and
  a rolled-up average is the wrong place to discover it.
- An invalid chain or signature invalidates the run's claim to be evidence at all, so
  it is recorded as a failure of that run instead of being averaged into a cell.
- `readStoreFacts` refuses a run directory holding two recorder stores rather than
  picking one: two writers in one run means the runner's isolation of runs broke, and
  choosing one arbitrarily would hide it.
- Verified against the existing free-arm probe set with `--regrade` (no model calls
  re-run): all 8 runs now report `counts_from: store` with the enforcement counts
  unchanged, which is the outcome hoped for — the recorder agrees with what the
  runner inferred from traffic.

### vNext experiment — Gate B conformance, and the two gates' first artifacts

- Gate B's ">10 MB messages" line exposed the round's most consequential defect:
  `--max-message` was never enforced, because `epReadMessage` treated
  `bufio.ErrBufferFull` (raised on every frame past the reader's 64 KiB buffer) as
  proof of an over-long message. Any upstream response above 64 KiB became
  `upstream unavailable` plus a lost execution. Fixed by bounding the accumulated
  frame and draining an over-limit frame before refusing it. Gate A did not see this
  because its oversize task accepts an honest failure, and a 64 KiB ceiling is
  invisible to a metric about agent behaviour.
- Two conformance tests added with it: progress notifications must survive the
  wrapper (id-less notifications are dropped by a relay that tracks only
  request/response pairs), and a 20 MiB result must cross intact while its evidence
  record stays bounded to `omitted_oversize` with no key material in the chain.
- `VerifyRoot` identified stores by directory-name prefix, so a renamed or aggregated
  folder of valid stores was reported as "no recorder directories" — an answer that
  reads as "nothing was recorded". Stores are now identified by their files.
- `docs/system-design/vnext-gate-b-results.md`: §46's checklist mapped to named tests,
  measured pass/fail, the four findings above, the fidelity claim as measured, and
  the boundary that no third-party server has been wrapped.
- `docs/system-design/vnext-gate-c-reconciliation-probe.md`: 8 free-arm sessions run
  through the new store and reconciled, with the reproduction commands and an
  explicit **not-passed** verdict — no real operational server, no human judgement on
  record, and no session yet where declarations and observations genuinely diverge.
  It records what reconciliation did add (per-operation observation counts from
  signed evidence, chain validity separate from coverage, empty windows reported as
  empty) and the cheapest path to a real verdict, led by §47's still-unimplemented
  in-band `evidra_report` feedback.
- `/output/` is gitignored on purpose: every recorder directory holds an ephemeral
  signing key and a digest key that must not enter history. Verified for the probe
  set that no substring of either key appears in `summary.json`.

### vNext experiment — step 9: `evidra summarize` and `evidra verify`

- Two vNext commands, deliberately thin over `pkg/evidence` and `pkg/report` so an
  agent, a script and a human at a terminal cannot grow three definitions of the same
  metric. `version` was already there; §59 step 9 asks for nothing more.
- `summarize --dir <root> [--since 7d]` prints the terminal summary and writes
  `summary.json` beside the evidence. `verify --dir <root> [--since ...]` reports
  `chain`, `signature` and `coverage` as three statements per recorder store, with the
  enforcement mode, upstream and `digest_key_id` on every line — a number without its
  comparison domain is not a fact.
- `--since` turned out to be unimplemented in the verify path: the window filter existed
  in `ReadStore` while `VerifyStore` counted the whole file, so a query for "last 7
  days" reported every record. Verification is now explicitly full-history (a chain is
  only as trustworthy as all of it) and the *counts* are windowed, which is what a
  reader is asking for.
- An empty window is a failure, not a pass. `verify` on a store whose records all fall
  outside the window exits non-zero with "none with records in the requested window":
  silence from a query that examined nothing must not be readable as a clean bill of
  health.
- Tests: 6 CLI tests (summary JSON round trip plus anomaly visibility in the terminal
  view, chain/coverage separation, mode label presence, byte-level tamper detection
  through the CLI, missing-directory usage, since-window behaviour) and a report test
  that a window narrows records and operations without invalidating the chain.

### vNext experiment — step 8: reconciliation, and two structural bugs it exposed

- `pkg/report` implements §34/§38: it reads every recorder store under an evidence
  root, joins prescriptions with the executions carrying their `operation_id` and
  with the agent's terminal report, and emits a `summary.json` that keeps each
  comparison domain separate (`enforce mode | upstream | digest_key_id`). Derived
  states are exactly the four the plan names — `reported`, `replaced_without_report`,
  `interrupted_session`, `lifecycle_unknown` — and no report is ever synthesized for
  an operation that has none.
- §38.A's anomaly (`CLAIMED_ACHIEVED_WITHOUT_OBSERVED_EXECUTION_IN_SCOPE`) is emitted
  as an evidence anomaly with its observation scope, never as fraud. §38.B's
  same-fingerprint recovery reports `insufficient_fingerprint_data` instead of
  "no later success" whenever the comparison could not actually be made, which is the
  difference between a measurement and an accusation. §38.C's read-only fact carries
  the disclosure that server annotations are unverified.
- `blocked_attempts_for_operation` turned out to be unimplementable as named: a block
  only happens when no operation is open, so an operation-scoped block count is
  structurally always zero, and printing that would hide the exact distinction §38
  wants (nothing attempted versus five refusals). The field is
  `blocked_attempts_in_session`, the narrowest scope that can exist.
- `first_attempt_protocol_compliance` is filled only in `enforce=all` and
  `voluntary_prescription_coverage` only in `enforce=off`, so no number can be read
  as spanning both.
- Two bugs found while building it, both structural:
  - `NewRecorderDirName` sliced the **front** of a ULID, which is its timestamp
    component: every recorder started in the same millisecond got the same directory
    name, and the second process appended into the first one's file. Single-writer
    per store was not holding. Now the random tail plus `os.Mkdir` with redraw, so a
    name collision cannot be silently resumed into.
  - The execution index deleted entries as they paired, so a fully reconciled session
    assembled to zero executions. Starts now live in two indexes: one open-set that
    shrinks, one complete record that operations are built from.
- Tests: 5 reconciliation tests (anomaly plus block attribution, mode separation,
  interrupted versus lifecycle-unknown, fingerprint metrics including the
  insufficient-data branch, recorder-name uniqueness) and they assert the negative
  too — a cell where prescription was required must not report voluntary coverage.

### vNext experiment — step 6: execution pairing, cancellation, annotations

- `notifications/cancelled` now reaches the outstanding call it names. Request ids
  are matched by value: the forwarded frame was keyed as the client spelled it, and
  the cancel echoes the id back, so a numeric `7` and a string `"7"` both resolve.
  Before this the cancel landed nowhere, which only showed up as an execution that
  started and never finished - a recorder lying by omission.
- A session that ends with calls in flight closes those executions itself, while
  the store is still open, instead of racing the child's EOF. Cancelled-and-never-
  answered is recorded as `cancelled`; started-and-abandoned is `unknown`. A call
  that answered anyway after being cancelled is recorded as answered: the terminal
  event states what happened inside the boundary, not what the client hoped.
- Terminal events are appended **before** the response is relayed, per §17. A test
  that asserted the event order caught the inversion as a flake: relaying first let
  an agent that reacts to a result immediately write its `operation_reported` ahead
  of the `execution_finished` it describes, and no downstream reconciliation can
  unscramble two events whose relative order is the fact being measured. The real
  upstream result is still relayed when the append fails (§18).
- Tests: parallel `slow` calls pair by `execution_id` without crossing their
  durations; `arguments_hmac` is stable across key reordering through the real
  endpoint (not just the helper) while a changed value moves it; and cancellation
  is asserted for both request-id spellings.

### vNext experiment — step 4: evidence.v2 store wired into the endpoint

- `pkg/evidence` gains the v2 model from plan §14-§21: a flat `evidra.evidence.v2`
  envelope (`seq`, `event_id`, `recorder_instance_id`, `session_id`, `operation_id`,
  `upstream_id`, `provenance`, `previous_hash`, `hash`, `signature`), the ten event
  types of §16 including `recorder_degraded`, and start/finish execution pairing by
  `execution_id`. `unprescribed_execution` stays a derived class, not an event type.
- RFC 8785 JCS canonicalization is implemented in-package (UTF-16 code-unit key
  ordering, ECMAScript `NumberToString`, JSON.stringify's escape set) so argument
  fingerprints survive key reordering without adding a dependency.
- Fingerprints per §23-§24: per-store HMAC-SHA256 key at 0600 with a non-secret
  `digest_key_id` as the comparison domain; result fingerprints bounded at 4 MiB
  with explicit `present` / `omitted_oversize` / `unavailable` states; error messages
  keyed rather than stored.
- Store layout per §20: one directory per recorder process
  (`recorder-YYYYMMDD-HHMMSS-<id>/events.jsonl|meta.json|signing.key|digest.key`),
  which makes a cross-process chain race structurally impossible instead of
  locked-around. §19's serialized append takes a builder that receives
  `(previous_hash, seq)`, and every record is flushed: a buffered recorder loses
  exactly the tail that an interrupted execution needs.
- §18 failure behavior is implemented, not deferred: a call whose
  `execution_started` cannot be written is refused with a `recorder_unhealthy`
  tool result; a lost `execution_finished` keeps the real upstream result intact,
  degrades coverage, and never fabricates an operational failure; `evidra_report`
  stays callable and is not acknowledged without durable evidence (the operation
  stays open so the agent can retry). A recorder that saw nothing removes its own
  directory.
- `VerifyStore` / `VerifyRoot` answer chain validity, signature validity and
  evidence coverage as three separate statements, with `--since` accepted as `7d`
  or an RFC 3339 instant, and grouping by enforcement mode + upstream +
  `digest_key_id` so a summary cannot average `enforce=all` with `enforce=off`.
- The merged endpoint writes this store when `--evidence-dir` is given. No
  directory, no store — but stated on stderr rather than silent, because
  "enforcement happened" and "evidence was kept" are different claims.
- Tests: 8 for the store (chain round trip, tamper detection after rewriting a
  record, degraded window with valid chain but incomplete coverage,
  lifecycle-only directory removal, key permissions, canary, fingerprint bound,
  `--since`/group key) and 3 end-to-end through the real endpoint: a refused
  unprescribed call appears as `protocol_violation` and leaves **no** execution
  record — absence is the proof forwarding was gated — plus annotations recorded
  with their absence preserved for an unannotated tool, and a refused startup when
  the evidence path is not a directory.

### vNext experiment — step 3b: Gate A runner, and the first pilot

- `cmd/evidra-gatea` drives the merged endpoint with an OpenAI-compatible
  tool-calling client: one `evidra-mcp --proxy --enforce=<mode> -- <fixture>`
  process per run, per-run JSONL transcript (frames, completions, tool events,
  verdict), `summary.json` with per-cell metrics, §10's gate conditions and the
  `git` revision under test.
- §10's failure classes became `invalid_run` and left every denominator: gateway
  credit/balance, model access, transport, wall-clock timeout,
  `finish_reason=length`. A preflight probe per arm is mandatory before the first
  task, and `--calibrate` measures the prompt-token cost of the two protocol tool
  definitions separately from per-run traffic (94 tokens on qwen3.8-flash).
- `--dry-run` replays a scripted agent (`compliant|late|noprescribe|noreport`) so
  predicates, metrics and artifacts are testable at zero token cost;
  `TestEveryTaskPredicateIsSatisfiable` fails if any task cannot be passed by a
  protocol-correct sequence.
- Task 7 (`change-of-mind`) was redesigned after the first 32-run pilot demanded
  `abandon_and_replace` specifically and rejected a model that closed the dropped
  operation honestly as `abandoned`. The predicate now accepts both §11 endings
  and rejects only the case that matters: an `achieved` record for dropped work.
- Protocol rules are no longer paraphrased in the runner's prompt (they are
  relayed verbatim from `initialize.instructions`), open operations are no longer
  nagged about, and agent-visible tool results are capped at 8 KiB with an
  explicit marker. All three were inflating or distorting a Gate A metric.
- First free-arm pilot (qwen3.8-flash, 32 runs, 0 invalid): `enforce=all` 12/16
  task success with 16/16 first-attempt compliance and zero blocked attempts;
  `enforce=off` 16/16 voluntary coverage with zero unprescribed executions.
  Recovery after first block stays `not_measurable` — nothing was ever blocked, so
  this arm provides no evidence for it yet.

### Gate A arms pinned from live endpoint probes (plan §10)

- §10 now names its arms instead of describing them: `qwen3.8-flash` @ `api.b.ai/v1` (free at time of writing), `deepseek-flash` @ `api.deepseek.com/v1` (the deployment the harness config displays as DeepSeek-V4.1-Flash), `deepseek-v4-pro` @ `api.deepseek.com/v1` as the ceiling arm. 80 runs total, ordered cheapest-first so a protocol redesign never spends metered tokens on discarded runs. Two of three arms share an endpoint, which removed the need for a transport-control cell.
- Added a reasoning-token budget rule: all three arms emit `reasoning_content`, and `max_tokens: 80` truncated a valid tool call into `finish_reason=length`. Budget is >= 2048 per turn and truncation is harness fault. Protocol overhead is now reported as visible (tool definitions + prescribe/report traffic) and total (including reasoning), never merged.
- Added `invalid_run` classification (gateway balance/access errors, persisting 429, transport timeout, `finish_reason=length`, preflight model-id mismatch) plus a mandatory preflight probe per arm, so account state cannot be read as agent behavior.
- Pinning extended to provider + base URL + verbatim API model id + display name + `cost_basis: free|metered`, because one product is reachable under three names across two gateways and no claim may silently depend on a promo price.
- Stale "64 runs" references corrected in §45, §59 step 3, and the thresholds heading.

### vNext experiment — step 1 (branch `vnext/mcp-recorder`)

- Added `docs/system-design/vnext-mcp-recorder.md`: the vNext execution-evidence plan (v8) as a tracked document — table of contents, 60 sections, plus the one-page implementation appendix (§59) that implementation agents work from.
- Added `cmd/evidra-fixture`: the deterministic MCP fixture server required by plan §30. Implemented directly against JSON-RPC over stdio rather than on the go-sdk, because SDK v1.5.0 cannot express the annotation states the plan has to distinguish (`readOnlyHint: false` vs no `annotations` key at all vs a contradictory set — `ToolAnnotations.ReadOnlyHint` is a plain `bool` with `omitempty`). The fixture exercises cursor pagination, >10 MiB results, server→client requests, `notifications/cancelled` with no response, `notifications/progress` via `_meta.progressToken`, `notifications/tools/list_changed` with a tool that really appears, request IDs that collide across directions, and oversized-message rejection that keeps the stream alive. `--serial` yields byte-stable response order for golden summaries; the default handles requests concurrently so parallel-call pairing can be tested at all.
- Added `.github/workflows/ci-vnext.yml`: the reduced supported build graph of plan §4 — 31 core packages certified, hosted platform (`cmd/evidra-api`, `internal/{api,apiutil,analytics,analyticsdb,analyticsvc,assessment,auth,automationevent,config,db,gitops,ingest,sarif,store}`, `pkg/{client,mode}`) and the tag-gated legacy e2e suite excluded. `ci.yml` is untouched: `main` and the public release channel keep the full gate set.
- `internal/signal/unprescribed_mutation_test.go` is gofmt-clean again; it was the one file failing the formatter gate that this branch inherits from `main`.

### Prompt contract v1.4.0
- New published contract `v1.4.0` (source tree `prompts/source/contracts/v1.4.0`, generated bundles refreshed): agent guidance now states claim linking — a prior `prescribe_smart`/`prescribe_full` is auto-linked to the executed `run_command` mutation, unlinked mutations are recorded `auto_prescribed` and counted as `unprescribed_mutation`. Replaced the "skip explicit prescribe for run_command" guidance with a prescribe-first incentive; behavioral signal list extended to nine.
- `scripts/prompts-generate.sh` / `prompts-verify.sh` defaults bumped to v1.4.0; embedded `DefaultContractVersion`/`DefaultContractSkillVersion` now `v1.4.0`/`1.4.0`.

### Unprescribed mutation signal + prescription claiming
- `run_command` mutations now link to a prior model-issued prescription when one claims the same normalized action (tool + operation + resource, within prescription TTL); only unlinked mutations write an auto-derived prescription flagged `auto_prescribed: true` in the payload.
- New behavioral signal `unprescribed_mutation` (9th detector): counts mutations executed with no prior model claim — protocol compliance becomes measured data instead of a silent gap in evidence. Weighted `0.0` in the default profile pending calibration on real agent traffic.
- Additive `PrescriptionPayload.auto_prescribed` field; older readers and validators unaffected (hash format unchanged).
- MCP server-derived prescriptions now record actor origin `evidra:auto`.

### External Evidence Bundle v1
- Added `evidence.EphemeralSigner`: public per-run Ed25519 signer for external bundle producers (`pkg/evidence/signer_ephemeral.go`).
- Added `evidence.BundleManifest` with `bundle.json` sidecar read/write helpers and `evidence.ValidateBundle` (`pkg/evidence/bundle.go`).
- `evidra validate` now auto-detects `bundle.json` and verifies signatures against the embedded public key.
- Added `docs/external-evidence-bundle-v1.md` — stable producer spec for third-party evidence stores.
- Added conformance fixture `tests/external_bundle_v1/` (generated via public API only) with chain + tamper-detection tests.

## v0.5.30 — 2026-06-17

### SEO
- Restored the Bench-first root SEO title and description contract for `evidra.cc`.

## v0.5.29 — 2026-06-16

### SEO
- Added canonical metadata, Open Graph image, structured data, favicon, and no-JS crawlable content for `evidra.cc`.
- Stopped serving the SPA shell for missing asset URLs such as `/favicon.ico`.

## v0.5.28 — 2026-05-21

### SEO
- Added root sitemap and robots files for search engine crawling.

## v0.5.27 — 2026-05-21

### Site Verification
- Added the Google Search Console verification file to the embedded web UI.

### Landing Page
- Added the open source Evidra Bench repository link to the landing page.

## v0.5.26 — 2026-05-08

## v0.5.25 — 2026-05-08

## v0.5.24 — 2026-05-08

## v0.5.23 — 2026-05-08

## v0.5.22 — 2026-05-06

## v0.5.21 — 2026-05-06

### Repository Scope
- Removed the hosted bench API, runner control plane, and embedded `/bench` UI from the core Evidra API; Evidra Bench now lives as a separate product surface at `https://bench.evidra.cc`.

## v0.5.20 — 2026-04-13

## v0.5.19 — 2026-04-13

## v0.5.18 — 2026-03-28

### Bench Hosted Execution
- Added first-class hosted `execution_mode` support to `POST /v1/bench/trigger`, with optional `provider|a2a` selection and default `provider`
- Threaded `execution_mode` through trigger state, runner job persistence, runner claim payloads, OpenAPI, and bench dashboard controls
- Mapped hosted `execution_mode=a2a` to the bench executor's internal `config.adapter=a2a` for direct benchmark execution

### Bench Model Configuration
- `GET /v1/bench/models` — list tenant-visible models with `available` field based on platform env var presence
- `PUT /v1/bench/models/{model_id}/provider` and `DELETE` — tenant provider overrides (disabled until encryption is implemented)
- `PUT /v1/admin/bench/models/{model_id}` — invite-gated platform route for global model defaults
- Trigger handler auto-resolves provider from model catalog when not supplied in request
- Trigger handler validates API key is configured before accepting jobs
- Input validation: upsert requires at least one non-empty field; delete returns 404 for nonexistent providers

### MCP Deferred Tool Loading
- Deferred tool schema registry — `prescribe_smart` and `report` schemas loaded on demand, not at initialize
- `describe_tool` meta-tool for clients to fetch individual tool schemas
- `run_command` path preferred in initialize instructions
- MCP contract bumped to v1.3.0


## v0.5.13 

### Contract v1.2.0
- Contract extended with DevOps operations (run_command, write_file, diagnosis protocol, safety rules)
- MCP prompt templates (prescribe-smart, prescribe-full, diagnosis) generated from contract source
- All prompts generated from `CONTRACT.yaml` — no hardcoded files
- SKILL.md leads with DevOps operations, protocol compressed to essentials

### Skill Rework
- SKILL.md rewritten: DevOps ops first (~600 tokens), protocol second (was ~2000)
- Trigger description updated to include all DevOps ops (read + write)
- MCP prompts: prescribe-smart, prescribe-full, diagnosis as on-demand resources

### Bench Intelligence Endpoints
- `GET /v1/bench/signals` — aggregated signal counts (protocol_violation, retry_loop, blast_radius) from run scorecards
- `GET /v1/bench/regressions` — detects scenario/model pairs where the latest run failed but previous runs passed
- `GET /v1/bench/insights?scenario=X` — failure analysis with check failure stats, model breakdown, behavior metrics (pass vs fail avg turns/tokens/cost)
- `GET /v1/bench/compare/models` — fixed to accept `?models=X,Y,Z&scenarios=A,B` for multi-model matrix comparison (in addition to legacy `?a=X&b=Y` pairwise)
- `POST /v1/bench/scenarios/sync` — upsert scenario metadata from bench CLI

### MCP Modes And Ingest
- Split the MCP lifecycle surface into `prescribe_full` and `prescribe_smart`, with clearer public mode wording around Full Prescribe, Smart Prescribe, and Proxy Observed
- Added authenticated external lifecycle ingest routes for adapter-driven `prescribe` and `report` creation, and routed webhook ingestion through the same shared service
- Extended payload taxonomy so entries now carry execution flavor plus explicit `evidence.kind` and `source.system`


## v0.4.11

### GitOps And Argo CD
- Added controller-first Argo CD integration for self-hosted `evidra-api`, with zero-touch reconciliation capture and explicit `evidra.cc/*` correlation
- Kept GitOps evidence on the standard `prescribe` / `report` lifecycle using `payload.flavor = reconcile`
- Added shared automation event emission for mapped Argo CD webhook and controller-reported lifecycle entries
- Split execution flavor from ingest taxonomy so payloads can also record `evidence.kind` and `source.system`, and renamed `pipeline_stage` to `workflow`
- Refactored `evidra-api` startup initialization to reduce complexity without changing startup behavior

## v0.4.10 

### Benchmark API
- Added benchmark table to landing page UI
- Added input validation for benchmark run suite field
- Capped benchmark list query limit to 100

### Hosted Analytics And API Contracts
- Replayed stored evidence chronologically in hosted scorecard/explain so self-hosted analytics matches CLI/local signal behavior for order-sensitive detectors
- Added required `operation_id` to generic webhook events and used it as the stable prescribe/report lifecycle correlation key
- Moved API key issuance quota checks behind invite-secret validation so rejected onboarding attempts do not burn shared rate-limit budget
- Replaced fire-and-forget `last_used_at` writes on API key lookup with bounded inline updates
- Restored a fixed eight-signal public scorecard contract instead of auto-expanding API output to every registered signal

### MCP
- Fixed the `get_event` MCP tool output contract so stored report events can be returned without structured-output schema validation failure
- Added explicit MCP output schema coverage for `get_event` payload shapes

### MCP And Prompts
- Updated MCP report schema, tool descriptions, prompt contracts, and generated prompt artifacts for explicit verdicts and declined decisions
- MCP now records not only actions but also deliberate refusals with rationale

## v0.4.2

### Signals
- New signal: `risk_escalation` — detects when an actor's operations exceed their baseline risk level (8th signal, weight 0.10)
- Baseline computed as mode of actor+tool risk levels in 30-day rolling window
- Demotions tracked internally as `risk_demotion` sub-signal (informational, no penalty)
- Signal Spec updated to v1.1

### Telemetry
- `risk_escalation` added to allowed signal names in OTLP metrics export

### Documentation
- [MCP Setup Guide](docs/guides/mcp-setup.md) — install, connect agents (Claude Code, Cursor, Codex, Gemini CLI, OpenClaw), configuration, troubleshooting
- MCP Setup section added to landing page with editor-specific config snippets
- Signal 8 definition added to EVIDRA_SIGNAL_SPEC.md
- All "7 signals" references updated to 8 across docs, UI, and OpenAPI spec
- Architecture overview moved to `docs/ARCHITECTURE.md`

### Testing
- E2e test: staging→production escalation through full CLI pipeline
- Score stability regression test (zero-count risk_escalation does not affect score)

## v0.3.1 

### CLI
- `evidra run` — execute commands live and record lifecycle outcome (prescribe + execute + report in one call)
- `evidra record` — ingest completed operations from structured JSON input
- `evidra keygen` — generate Ed25519 signing keypair
- Assessment output includes `score`, `score_band`, `basis` (preview vs sufficient), and `signal_summary`
- `--canonical-action` flag for pre-canonicalized actions (Pulumi, Ansible, CDK escape hatch)
- Kustomize support added to K8s adapter (`--tool kustomize`)

### Observability
- OTLP/HTTP metrics export: `evidra.operation.signal.count` and `evidra.operation.duration_ms`
- Bounded-cardinality labels: tool, environment, result_class, signal_name, score_band, assessment_mode
- Configuration via `EVIDRA_METRICS_TRANSPORT`, `EVIDRA_METRICS_OTLP_ENDPOINT`, `EVIDRA_METRICS_TIMEOUT`
- [Observability Quickstart](docs/guides/observability-quickstart.md) with collector setup and PromQL examples

### Protocol
- Session ID auto-generated at ingress when omitted
- `operation_id` and `attempt` fields on evidence entries
- `session_start`, `session_end`, `annotation` entry types
- Signing enforced on every evidence entry (strict mode default)
- Trace defaults: `trace_id` defaults to `session_id`, optional `span_id`/`parent_span_id`
- Evidence write mode: `strict` (default) or `best_effort`

### Canonicalization
- Docker adapter: docker, nerdctl, podman, compose
- OpenShift resources: DeploymentConfig, Route, BuildConfig, ImageStream
- Noise filtering: managedFields, uid, resourceVersion, creationTimestamp, last-applied-configuration

### Documentation
- [Supported Tools](docs/SUPPORTED_TOOLS.md) reference with adapter matrix and risk detectors
- [Observability Quickstart](docs/guides/observability-quickstart.md) — OTLP setup, Grafana/Prometheus queries, CI examples
- [Terraform CI Quickstart](docs/guides/terraform-ci-quickstart.md)
- [Scanner SARIF Quickstart](docs/integrations/SCANNER_SARIF_QUICKSTART.md) rewritten with run/record patterns
- [CLI Reference](docs/integrations/cli-reference.md) — unified command reference
- [Setup Evidra Action](docs/guides/setup-evidra-action.md) — GitHub Actions + generic CI install

### Testing
- Real-world e2e test suite: K8s, Terraform, Helm (Redis, ingress-nginx), ArgoCD, Kustomize, OpenShift
- E2e tests verify actual canonicalization output (resource_count, resource_identity, risk_tags, noise immunity)
- Run/record parity contract tests
- MCP schema-struct parity contract test
- Signal validation scenarios in CI

### CI/CD
- E2e tests gate release pipeline (release-guard → test → e2e → snapshot + docker → goreleaser)
- Homebrew tap publishing via GoReleaser
- Docker image: `ghcr.io/vitas/evidra-mcp`
- `setup-evidra` GitHub Action for CI adoption

### Fixes
- Evidence chain: in-process ID cache for faster entry lookup
- Findings correlation: correct TraceID, attach SessionID/OperationID/Attempt
- Lifecycle flows unified with session invariant enforcement
- Removed dead code (MaxBaseSeverity, RehashEntry, SegmentFiles)

## v0.3.0

First public release of Evidra Benchmark.

### Core Pipeline
- Canonicalization adapters: Kubernetes (kubectl, oc, helm), Terraform, Docker (docker, nerdctl, podman), generic fallback
- Risk matrix (operation_class x scope_class) with 7 catastrophic detectors
- Eight behavioral signals: protocol violation, artifact drift, retry loop, blast radius, new scope, repair loop, thrashing, risk escalation
- Weighted reliability scoring with safety floors and band classification
- Ed25519 evidence signing with strict/optional modes and key generation

### CLI (`evidra`)
- `prescribe` — record intent before infrastructure operations
- `report` — record outcome after execution
- `scorecard` — compute reliability score from evidence chain
- `explain` — detailed signal breakdown with sub-signals
- `compare` — side-by-side actor comparison with workload overlap
- `--scanner-report` flag for SARIF ingestion (Trivy, Kubescape)
- `--canonical-action` flag for pre-canonicalized actions
- Tool and scope filtering on scorecard/explain/compare
- `run` — execute command live and record lifecycle outcome
- `record` — ingest completed operation from structured JSON input
- `validate` — verify evidence chain integrity and signatures
- `ingest-findings` — ingest SARIF scanner findings as evidence entries
- `keygen` — generate Ed25519 signing keypair

### MCP Server (`evidra-mcp`)
- Stdio transport for MCP-based automation integration (including AI agents)
- Tools: prescribe, report, get_event
- Session/trace/span correlation fields for multi-step workflows
- Optional retry loop tracking

### Protocol (v1.0 Foundation)
- Session/run boundary hardened: persisted evidence entries always include `session_id` (generated at ingress when omitted by caller)
- Correlation defaults documented: `trace_id` defaults to `session_id`, with optional `span_id` and `parent_span_id`
- Actor identity: `actor.instance_id` and `actor.version` (optional, not used in metrics)
- Scope dimensions: `scope_dimensions` map for detailed environment metadata (cluster, namespace, account, region)
- Protocol spec: `docs/system-design/EVIDRA_PROTOCOL.md`

### Evidence Chain
- Append-only JSONL with hash-linked entries
- Segmented storage with automatic rotation (5MB default)
- File-based locking for concurrent access

### Build
- Go 1.23 minimum (CI pinned from `go.mod`)
- Cross-platform binaries via GoReleaser (linux/darwin/windows, amd64/arm64)
- Homebrew: `brew install samebits/tap/evidra-mcp`
- Docker: `ghcr.io/vitas/evidra-mcp:0.3.0`

### Known Limitations
- ArgoCD uses generic adapter (no Argo-specific metadata)
- MinOperations=100 required for scoring (low-volume actors get `insufficient_data`)
- Optional signing mode (`EVIDRA_SIGNING_MODE=optional`) uses ephemeral keys and is not durable across restarts
- No centralized API server (v0.5.0)
