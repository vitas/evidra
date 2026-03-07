# Signal Engine Validation

**Zero dependencies beyond `evidra` binary and `jq`.**
No cluster. No LLM. No API keys. No external data.

## Run

```bash
cd /path/to/evidra-benchmark
make build
export PATH="$PWD/bin:$PATH"

bash tests/signal-validation/validate-signals-engine.sh
```

## What It Does

Creates 5 scripted operation sequences (100 total operations).
Each sequence triggers a specific behavioral signal.
No real infrastructure — just `evidra prescribe` / `evidra report` against local evidence files.

| Sequence | Operations | Behavioral Pattern | Expected Signal |
|----------|-----------|-------------------|----------------|
| A | 20 clean prescribe/report pairs | Normal operation | No signals, score 95+ |
| B | 5 identical failures + 5 clean | Agent stuck retrying | retry_loop ≥ 3 |
| C | 5 clean + 5 orphaned prescriptions + 5 clean | Agent forgets to report | protocol_violation ≥ 3 |
| D | 1 mass delete (15 resources) + 9 clean | Disproportionate impact | blast_radius ≥ 1 |
| E | 5 kubectl + 5 helm + 5 terraform | Agent switching tools | new_scope ≥ 2 |
| F | Prescribe artifact X, report with different artifact | Intent ≠ execution | artifact_drift ≥ 1 |

## Success Criteria

Six different scores across six sequences = **signal engine produces meaningful differentiation**.
All five behavioral signals have validation coverage.

```
A (clean)    → 90-100  excellent    ← baseline, agent is reliable
B (retry)    → 50-70   fair         ← agent stuck, medium penalty
C (protocol) → 40-65   poor-fair    ← agent breaking contract, high penalty
D (blast)    → 60-80   fair-good    ← one bad op in mostly clean session
E (scope)    → 80-95   good         ← tool switching is informational, low penalty
F (drift)    → TBD     TBD          ← artifact drift, set after first run
```

If all sequences score the same → signal engine bug.
If scores are inverted → weight calibration needed.

## Files

```
tests/signal-validation/
  helpers.sh                     # Shared functions
  validate-signals-engine.sh     # Main validation script
  README.md                      # This file
```

## Related Planning Docs

- [`PARALLEL_EXECUTION_PLAN.md`](../../docs/plans/PARALLEL_EXECUTION_PLAN.md)
- [`2026-03-07-parallel-execution-implementation-plan.md`](../../docs/plans/2026-03-07-parallel-execution-implementation-plan.md)

## After Running

Evidence chains are preserved in `/tmp/evidra-signal-validation/evidence-*/`.
Inspect manually:

```bash
# See raw signals
evidra explain --evidence-dir /tmp/evidra-signal-validation/evidence-XXXXX --ttl 1s | jq .

# See scorecard
evidra scorecard --evidence-dir /tmp/evidra-signal-validation/evidence-XXXXX | jq .

# See raw evidence entries
cat /tmp/evidra-signal-validation/evidence-XXXXX/segments/*.jsonl | jq .
```
