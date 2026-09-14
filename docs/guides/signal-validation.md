# Signal Validation
> **Pre-vNext surface.** This guide documents the signal engine and the scoring profile, both removed by the §43 prune. It is kept for reference until the documentation pass after Gate C; nothing here describes the MCP evidence recorder.

- Status: Guide
- Version: frozen at removal (see the banner above)
- Canonical for: signal-validation harness usage
- Audience: public

Evidra ships a deterministic signal-validation harness for checking whether the
behavioral signal engine still differentiates meaningful operation patterns.
This is the retained signal/scoring harness used for local calibration and CI.

## What It Covers

The harness exercises these behavioral patterns:

- clean operation history
- retry loops
- protocol violations
- blast radius spikes
- new scope activity
- repair loops
- thrashing
- artifact drift
- risk escalation
- unprescribed mutations (server-derived prescriptions)

## How It Works

- uses only the local `evidra` CLI plus `jq`
- writes evidence into a temporary local store
- writes run artifacts under `/tmp/evidra-signal-validation-results/<timestamp>/`
- runs scripted prescribe/report sequences
- validates observed signals and score bands against `expected-bands.json`

Sequences that require operation class, scope, or resource count provide those
fields as explicit `--canonical-action` enrichment. The harness does not rely on
the core prescribe path to canonicalize artifacts or infer risk.

The expectation windows in `expected-bands.json` are calibration snapshots
derived from the active scoring model and profile, not an independent scoring
spec. For the score pipeline itself, see
`EVIDRA_SCORING_MODEL_V1.md`, removed in the vNext prune and readable in Git history.

The harness is intentionally deterministic:

- CLI failures are fatal
- response payloads must be valid JSON
- protocol-violation timing is backdated in evidence instead of relying on wall-clock drift
- expected sequence coverage is checked explicitly

## Run

```bash
make test-signals
```

## Files

- [`tests/signal-validation/helpers.sh`](../../tests/signal-validation/helpers.sh)
- [`tests/signal-validation/validate-signals-engine.sh`](../../tests/signal-validation/validate-signals-engine.sh)
- [`tests/signal-validation/expected-bands.json`](../../tests/signal-validation/expected-bands.json)
- [`tests/signal-validation/README.md`](../../tests/signal-validation/README.md)

## What To Look For

- each sequence should trigger its required signal pattern
- score ranges should stay within the declared expectation windows
- relative comparisons such as repair scoring better than raw retry should hold

If those properties drift, either the signal engine changed intentionally and
the expectations need recalibration, or a regression was introduced.
