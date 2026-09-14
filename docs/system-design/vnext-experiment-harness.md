# The experiment harness is part of the evidence product

Status: standing invariants for `cmd/evidra-gatea` and its artifacts. Written after three
measurement errors on this branch, all of which produced plausible data rather than failures.

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

**5. `not_measurable` is a result.** If the predicate that would produce a number never
occurs, the artifact says so and does not borrow a nearby number as a substitute. Standing
examples: recovery-after-first-block with zero blocks (Gate A), and the in-band feedback's
behavioural effect, whose trigger sentence fired zero times across 8 sessions.

**6. Deleting a check is cheaper than bypassing one, so checks must not cry wolf.**
Test-only source edits do not trip the freshness guard, with a test asserting that. A guard
that blocks routine work gets habitually opted out of, and an opt-out everyone passes is
worse than no guard, because it manufactures false confidence in the ones that matter.

## What this rules out

No "quick" one-off analysis scripts that write numbers into docs. No comparing two run sets
without first checking their recorded builds match. No reporting a metric whose denominator
was not written in the same artifact. No treating an executed-zero mechanism's null result
as evidence about that mechanism.
