# Signal Engine Validation

**Zero dependencies beyond `evidra` binary and `jq`.**
No cluster. No LLM. No API keys. No external data.

Detailed guide (purpose/how it works/run/interpretation):
- [`docs/experimental/SIGNAL_VALIDATION_GUIDE.md`](../../docs/experimental/SIGNAL_VALIDATION_GUIDE.md)

Related parity/UX tests for `run` + `record`:
- CLI tests: `go test ./cmd/evidra -run 'RunAndRecord_|RunOutput_' -count=1`
- E2E parity (tagged): `go test -tags=e2e ./tests/e2e -run RunRecordParity -count=1`

## Run

```bash
cd /path/to/evidra
make test-signals
```

## What It Does

Creates scripted operation sequences (A-H).
Each sequence triggers a specific behavioral signal.
No real infrastructure — just `evidra prescribe` / `evidra report` against local evidence files.

| Sequence | Operations | Behavioral Pattern | Expected Signal |
|----------|-----------|-------------------|----------------|
| A | 20 clean prescribe/report pairs | Normal operation | No signals, score 99-100 |
| B | 5 identical failures + 5 clean | Agent stuck retrying | retry_loop ≥ 3 |
| C | 5 clean + 5 orphaned prescriptions + 5 clean | Agent forgets to report | protocol_violation ≥ 3 |
| D | 1 mass delete (15 resources) + 9 clean | Disproportionate impact | blast_radius ≥ 1 |
| E | 5 kubectl + 5 helm + 5 terraform | Agent switching tools | new_scope ≥ 2 |
| F | Fail, change artifact, succeed (+ clean ops) | Agent adapts strategy | repair_loop ≥ 1 |
| G | 5 different failed intents (+ clean ops) | Agent thrashing | thrashing ≥ 1 |
| H | Report digest differs from prescribed digest (+ clean ops) | Artifact changed between prescribe/report | artifact_drift ≥ 1 |
| I | Low-risk baseline then critical operations (+ clean ops) | Agent escalates risk level beyond baseline | risk_escalation ≥ 1 |

## Success Criteria

Distinct score/signal profiles across A-I = **signal engine produces meaningful differentiation**.
Validation now includes repair/thrashing, artifact drift, and risk escalation.

```
A (clean)    → 99-100 excellent
B (retry)    → 75-85  poor
C (protocol) → 85-90  poor
D (blast)    → 95-99  good
E (scope)    → 98-100 excellent
F (repair)   → 75-85  adapted (should score higher than B)
G (thrash)   → 70-80  unstable (should score lower than B)
H (drift)    → 84-86  poor
I (escalation) → 85-95  good
```

Note: B and F share the same score band (`75-85`), but the validation gate also enforces `F_repair > B_retry`.

If all sequences score the same → signal engine bug.
If scores are inverted → weight calibration needed.

## Files

```
tests/signal-validation/
  expected-bands.json            # Score/signal assertions and comparisons
  helpers.sh                     # Shared functions
  validate-signals-engine.sh     # Main validation script
  README.md                      # This file
```

## Related Planning Docs

- [`2026-03-07-parallel-execution-implementation-plan.md`](../../docs/plans/done/archive/2026-03-07-parallel-execution-implementation-plan.md)
- [`V1_ARCHITECTURE.md`](../../docs/system-design/V1_ARCHITECTURE.md)

## After Running

Evidence chains are preserved in `/tmp/evidra-signal-validation/evidence-*/`.
Run artifacts are written to `experiments/results/signals/<timestamp>/`.
Inspect manually:

```bash
# See raw signals
evidra explain --evidence-dir /tmp/evidra-signal-validation/evidence-XXXXX --ttl 1s | jq .

# See scorecard
evidra scorecard --evidence-dir /tmp/evidra-signal-validation/evidence-XXXXX | jq .

# See raw evidence entries
cat /tmp/evidra-signal-validation/evidence-XXXXX/segments/*.jsonl | jq .
```
