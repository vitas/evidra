# Signal Validation Guide

This guide explains the deterministic signal-validation harness in `tests/signal-validation`:
- what it validates
- how the scripted sequences work
- how to run it
- how to interpret pass/fail and scores

## Purpose

Signal validation verifies that Evidra's detector + scoring pipeline differentiates behavior patterns in a predictable way.

It is intentionally offline and deterministic:
- no Kubernetes cluster
- no LLM calls
- no API keys
- only local evidence generated via `evidra prescribe` / `evidra report`

Primary goal: detect regressions in detector behavior, score calibration, and relative ordering (for example, `F_repair > B_retry`, `G_thrash < B_retry`).

## What It Runs

The harness executes 8 scripted sequences (`A`..`H`) in isolated sessions.

| Sequence | Pattern | Expected primary signal |
|---|---|---|
| `A_clean` | clean prescribe/report pairs | none |
| `B_retry` | repeated same failed intent/artifact | `retry_loop >= 3` |
| `C_protocol` | orphan prescriptions (no report) | `protocol_violation >= 3` |
| `D_blast` | one mass delete operation | `blast_radius >= 1` |
| `E_scope` | new tools/scopes introduced | `new_scope >= 2` |
| `F_repair` | fail -> change artifact -> succeed | `repair_loop >= 1` |
| `G_thrash` | multiple distinct failed intents | `thrashing >= 1` |
| `H_drift` | report digest != prescribed digest | `artifact_drift >= 1` |

Thresholds, score bands, and cross-sequence comparisons are defined in:
- `tests/signal-validation/expected-bands.json`

## How It Works Internally

1. Validate runtime prerequisites (`evidra`, `jq`).
2. Create a workspace under `/tmp/evidra-signal-validation` (guarded to avoid unsafe deletion).
3. For each sequence:
   - create a new evidence directory + session id
   - emit scripted `prescribe`/`report` operations
   - capture live `explain`/`scorecard` output
4. Recompute per-sequence artifacts and persist:
   - `sequence-<label>-scorecard.json`
   - `sequence-<label>-explain.json`
   - one row in `summary.jsonl`
5. Evaluate assertions from `expected-bands.json`:
   - expected band
   - score range `[score_min, score_max]`
   - required signal minimum counts
   - comparison rules (for example `F_repair > B_retry`)
6. Write final aggregate:
   - `summary.json` with `pass`, `failures`, and all sequence rows
7. Exit non-zero if any assertion fails.

## How To Run

### Standard path (recommended)

```bash
make test-signals
```

`make test-signals` builds `bin/evidra` if missing, prepends `bin/` to `PATH`, and runs the harness script.

### Direct script invocation (advanced)

```bash
PATH="$PWD/bin:$PATH" \
SCORECARD_MIN_OPERATIONS=1 \
WORKSPACE=/tmp/evidra-signal-validation \
EXPECTED_BANDS_FILE=tests/signal-validation/expected-bands.json \
RESULTS_DIR=experiments/results/signals/manual-$(date -u +%Y%m%dT%H%M%SZ) \
bash tests/signal-validation/validate-signals-engine.sh
```

## Output Layout

Workspace evidence (kept for inspection):
- `/tmp/evidra-signal-validation/evidence-*/segments/*.jsonl`

Run artifacts:
- `experiments/results/signals/<timestamp>/summary.jsonl`
- `experiments/results/signals/<timestamp>/summary.json`
- `experiments/results/signals/<timestamp>/sequence-A_clean-scorecard.json`
- `experiments/results/signals/<timestamp>/sequence-A_clean-explain.json`
- ... same for all sequences `B`..`H`

## How To Interpret Results

### 1) Pass/fail gate

Start with `summary.json`:
- `pass: true` means all configured assertions passed.
- `failures > 0` means at least one band/range/signal/comparison assertion failed.

### 2) Detector correctness

Use per-sequence `...-explain.json` files to confirm the expected signal fired with enough count.

### 3) Score calibration

Use per-sequence `...-scorecard.json` to ensure each sequence remains in its expected band/range.

### 4) Relative-order sanity

Comparison checks are high-value calibration guards:
- `F_repair > B_retry`
- `G_thrash < B_retry`

If these invert, weights or detector logic likely drifted.

## Quick Inspection Commands

Overall gate result:

```bash
jq '{pass, failures, generated_at, run_stamp, min_operations}' \
  experiments/results/signals/<stamp>/summary.json
```

Per-sequence snapshot:

```bash
jq '.sequences[] | {label, score, band, signals}' \
  experiments/results/signals/<stamp>/summary.json
```

Signal counts for one sequence:

```bash
jq '.signals | map(select(.count > 0))' \
  experiments/results/signals/<stamp>/sequence-F_repair-explain.json
```

## Common Pitfalls

- Missing `evidra` in `PATH` (use `make test-signals` to avoid this).
- Running with a `WORKSPACE` outside `/tmp/evidra-signal-validation*` (blocked by safety guard).
- Forgetting that `SCORECARD_MIN_OPERATIONS` changes score behavior; keep it consistent when comparing runs.
- Editing detector weights/signals without updating `expected-bands.json` accordingly.
