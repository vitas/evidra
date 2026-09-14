# The experiment harness is part of the evidence product

Status: standing invariants for `cmd/evidra-gatea` and its artifacts. Written after three
measurement errors on this branch, all of which produced plausible data rather than failures —
and extended once more when the harness gained a fourth comparison (`mode none`, the no-Evidra
baseline), which surfaced two more.

## Why this document exists

The harness decides what the product's central claim is allowed to mean. It is not test
scaffolding: Gate A's numbers, Gate C's probe sets, and every "the agent claimed achieved
without observing anything" statement in the docs come out of this code. Which means it needs
the same property the recorder sells — an artifact must state what produced it, or it cannot
be challenged later.

Three incidents on this branch, in order of how much they cost:

| What happened | Why it was dangerous | What enforces it now |
|---|---|---|
| An 8-session probe of the in-band feedback reported zero deliveries. The runner had used `bin/evidra-mcp` built before the feature existed; the Go tests build their own endpoint binary, so the tests proving the feature worked passed at the same time. | The run set was complete, well-formed, and about a different program. Nothing in the artifact hinted at it. | `freshness.go`: preflight refuses a binary older than any of its source trees; `--allow-stale-build` for deliberate comparisons. |
| A cell metric printed **28 against a denominator of 16**. Per-run values were summed into a numerator nobody bounded. | It survived review because the number looked like a plausible count and no line of code could be wrong. | `provenance.go`: `checkCellBounds` — every bounded metric must lie in `[0, runs]`. |
| A comparison table reported task success **0/8 in both arms**. A throwaway script read `result.json`'s `task_success` key; the field is `success`, so every run read as `nil` → failure. Real numbers: 4/8 before, 6/8 after. | Hand analysis outside the harness is invisible to every guard the harness has. | The correction is kept in the artifact; the rule below. |
| `applyStoreFacts` appended an *enforcement hole* ("N executions reached upstream with no open operation") to `res.Failures`, and the next line assigned `res.Failures = task.evaluate(tr)` — overwriting it. Same for an invalid chain or signature. | The one clause that turns a recorder defect into a per-run failure could not fail a run. Silent since it was written. | `finalizeRun` is the single grading path (live and `--regrade`); a regression test asserts a store-reported hole now fails the row. No recorded verdict changed: across every archived run set (`output/gatea/*`, 166 rows) no row has an unprescribed execution or an invalid chain, so the clause never had anything to report. |
| `runResult.duration_ms` was declared, marshalled, printed in nothing — and never assigned. Through the whole official Gate A set it read `0`. | A zero in a field that exists looks like a measurement of zero. The baseline comparison needs duration, so it had to be fixed before it could be quoted. | Assigned in `runOne`; the rollup's `duration_ms_total`/`mean_duration_ms` are invariant-checked against the rows, which is what makes "declared" and "measured" the same statement. |

## Invariants

**1. Experiment binary provenance must match the product source revision being evaluated.**
Every graded run records: source revision, whether the tree was dirty, sha256 of the endpoint
binary, sha256 of the fixture, sha256 of the runner, model id. `summary.json` collapses this
to one entry per distinct build and the number of runs each contributed. A comparison cell
whose runs come from two builds is an invariant violation, not a subtlety — averaging two
programs yields a number that describes neither.

**2. Analytics must satisfy arithmetic that nobody has to notice.** Checked before the gate
is judged, violation → exit code 4 (distinct from 3, which means "a run was invalid"):

- `0 <= metric <= runs` for every bounded numerator, and the companion metric
  (`protocol_only_success`) can never exceed valid runs;
- per-cell run counts equal the graded rows routed to that cell;
- per-run sums equal cell aggregates for task success, terminal report coverage and blocked
  attempts — the rollup may not recompute something slightly differently from the row loop;
- the same for the operational columns: `duration_ms_total`, `upstream_calls`,
  `upstream_errors`, `turns_total`, and both token totals are summed from the valid rows, and a
  mean (`mean_turns`, `mean_duration_ms`) must equal its total over its own denominator — a
  ratio is only evidence if it is the ratio of the two numbers printed beside it;
- the `none`/`off` comparison block must equal the cells it names, per column, including its
  delta; a summary that hand-builds a comparison table is where a comparison starts drifting
  from the measurement it claims to summarise;
- one build per cell;
- every graded run points at a transcript that still exists (an invalid run does not: there is
  no verdict to protect).

**3. Regrade; do not hand-read artifacts.** `--regrade` recomputes verdicts from persisted
transcripts and stores, writes them back, and re-runs the same invariant checks. Reading
`result.json` from an ad-hoc script is how the 0/8 table happened. Any number quoted in a
document should trace to a regrade run or to `evidra summarize` output.

**4. A regrade is a re-reading, not a re-measurement.** `--regrade` prints provenance drift:
runs measured against another endpoint build are named by hash, and runs recorded before
provenance existed are labelled *unattributed* rather than assumed to be current.

**5. `not_measurable` is a result — and it is not the same statement as `n/a`.** If the
predicate that would produce a number never occurs, the artifact says so and does not borrow a
nearby number as a substitute. Standing examples: recovery-after-first-block with zero blocks
(Gate A), and the in-band feedback's behavioural effect, whose trigger sentence fired zero times
across 8 sessions. `n/a` is stronger: the metric has no referent in that cell at all, which is
how a baseline cell reports protocol compliance — not "never blocked" (an observation) but
"nothing was enforcing" (a property of the arm).

**6. Freshness is required of what participated.** `freshnessChecks(modes, …)` takes the mode
set as an argument. A run set containing only `none` must not be blocked by a stale
`bin/evidra-mcp`, because no such process is started — and it must still be blocked by a stale
fixture or a stale runner, because both are used. The inverse trap is equally real: a
freshness rule that depends on the endpoint cannot be used to measure whether the endpoint
helps.

**7. A metric needs a referent.** `none` gives the model no `evidra_prescribe`, no
`evidra_report`, no instructions and no store, so eleven §10 metrics have no value in a `none`
cell. They are rendered `"n/a"` (`cellMetrics.MarshalJSON`), not `0`, and not `"0/8"`: a zero
in `terminal_report_coverage` is a claim that the agent never closed a record, which is a
statement about behaviour that was unobservable in that cell. The invariants then require the
underlying counters to be *structurally* zero — a `none` cell carrying protocol traffic, or a
`none` row whose provenance names an endpoint binary, or a `none` run that produced an evidence
store, fails the rollup. Compliance-shaped success is not merely unreported for a baseline:
`first_attempt_protocol_compliance` is not tallied at all when no protocol existed to attempt.

**8. A baseline certifies nothing.** `judgeGate` evaluates `enforce=all` cells; a `none` cell
can never appear in a gate condition, and `summary.json` carries `gate_applicability` saying so
in words. A run set of only baseline cells has zero conditions and `gate_passed=false`. The
comparison it supports (`none` vs `off`, same arm, same task, same run count) is descriptive:
no harm threshold was declared before the data, so none is computed after it. `off` vs `all`
remains the enforcement question; `none` vs `off` is the question of whether Evidra's presence
costs anything at all.

**9. Deleting a check is cheaper than bypassing one, so checks must not cry wolf.**
Test-only source edits do not trip the freshness guard, with a test asserting that. A guard
that blocks routine work gets habitually opted out of, and an opt-out everyone passes is
worse than no guard, because it manufactures false confidence in the ones that matter.

## The no-Evidra baseline arm (`mode none`)

`none` is a harness mode, not a product mode: there is no `evidra-mcp --enforce=none`, the
endpoint refuses the value, and `validateModes` rejects anything outside `none,off,all` rather
than planning zero runs. It exists because `off` and `all` share every property that could make
an agent worse at its work — the wrapper, the merged tool list, the two protocol tools, the
server instructions, the proxy transport — so the pair can measure enforcement but cannot
measure presence.

| | `none` | `off` | `all` |
|---|---|---|---|
| process topology | agent → fixture | agent → `evidra-mcp --proxy` → fixture | same, `--enforce=all` |
| prompt | `baseline_goal` | `goal` | `goal` |
| tools offered | fixture's own, from its `tools/list` | merged (fixture + 2 protocol) | merged |
| `initialize.instructions` | the fixture's | Evidra's | Evidra's |
| evidence dir / store | none | per run | per run |
| graded clauses | operational only | operational + protocol | operational + protocol |
| protocol metrics | `"n/a"` | measured | measured |
| `execution_path` | `direct_fixture` | `evidra_proxy` | `evidra_proxy` |
| `endpoint_binary_sha256` | `""` (not hashed) | hashed | hashed |
| `counts_from` | `no_store` | `store` or `transcript` | `store` or `transcript` |
| participates in Gate A | no | no | yes |

Operational clauses are: expected upstream tools with argument predicates, ordering pairs, and
no stray upstream calls — all readable from calls the fixture itself received. Protocol clauses
are the terminal report and, for `change-of-mind`, replacement/abandonment state. The split is
one function (`finalizeRun`) shared by the live path and `--regrade`, so a verdict is not
reproducible only in the process that produced it.

Wording is authored, not edited at runtime: each task carries a `baseline_goal` in
`tasks.json`, and `validateBaselineSpecs` refuses a task set where one is missing, identical to
the wrapped goal, or contains operation vocabulary (`record`, `prescri`, `report`, `evidra`,
`abandon`). Deriving the baseline prompt by deleting phrases would produce a task no reader can
check against the wrapped one.

Three known asymmetries, stated rather than smoothed over:

- **The word "Evidra" is not absent from the baseline.** The fixture introduces itself as
  "Fixture MCP server for Evidra vNext conformance tests". Rewriting the upstream's self
  description to make a control arm cleaner would change what both arms see, so the baseline
  model sees the project name once, in the server's own sentence.
- **`unannotated-and-contradictory` measures something different per mode.** Wrapped, it asks
  whether an ambiguous annotation changes enforcement or prescription behaviour. In `none` the
  annotations are still there but nothing enforces or records, so the only comparable quantity
  is "used each of the two tools exactly once". Read the two cells as different experiments
  that share a fixture, not as one measurement with a missing term.
- **`oversize-result`'s wrapped cost includes the recorder's bounding**, so its tokens and
  duration are not purely "Evidra's protocol surface"; part of that delta is §24's work, which
  is present in both `off` and `all` but in neither baseline.

Run it with: `./bin/evidra-gatea --arms-only qwen38-flash,mimo-v25,ds-v4-pro --modes-only none
--runs 2 --out output/gatea/no-evidra-baseline` (48 runs; `off` numbers come from the existing
official set, same source revision required for any comparison — see invariant 4).

## What this rules out

No treating a baseline cell as protocol evidence, and no protocol number in it to treat.
No "quick" one-off analysis scripts that write numbers into docs. No comparing two run sets
without first checking their recorded builds match. No reporting a metric whose denominator
was not written in the same artifact. No treating an executed-zero mechanism's null result
as evidence about that mechanism.
