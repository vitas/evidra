# Signal Validation

- Status: Guide
- Version: current
- Canonical for: signal-validation harness usage
- Audience: public

Evidra ships a deterministic signal-validation harness for checking whether the
behavioral signal engine still differentiates meaningful operation patterns.

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

## How It Works

- uses only the local `evidra` CLI plus `jq`
- writes evidence into a temporary local store
- runs scripted prescribe/report sequences
- validates observed signals and score bands against `expected-bands.json`

The expectation windows in `expected-bands.json` are calibration snapshots
derived from the active scoring model and profile, not an independent scoring
spec. For the score pipeline itself, see
[`EVIDRA_SCORING_MODEL_V1.md`](../system-design/EVIDRA_SCORING_MODEL_V1.md).

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
