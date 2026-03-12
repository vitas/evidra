# Evidra Scoring Model

- Status: Normative
- Version: v1.0
- Canonical for: how Evidra converts signal counts into score, band, and confidence
- Audience: public

## Purpose

This document explains the scoring pipeline in plain language.

It answers:

- what a scorecard starts from
- how counts become rates
- how rates become penalty contributions
- how penalty becomes score, band, and confidence
- why some outcomes that look like "magic numbers" are actually direct results of the active profile

Exact default weights, caps, and thresholds remain canonical in:

- [`scoring/default.v1.1.0.md`](./scoring/default.v1.1.0.md)

Signal detection logic remains canonical in:

- [`EVIDRA_SIGNAL_SPEC_V1.md`](./EVIDRA_SIGNAL_SPEC_V1.md)

## Inputs To A Scorecard

Evidra does not score single events in isolation. A scorecard starts from a
window of evidence entries and computes:

- total operations
- total prescriptions
- total reports
- count of each behavioral signal

Each signal is then converted into a rate. The denominator depends on the
signal:

- `protocol_violation` uses total operations
- `artifact_drift` uses reports with matching prescriptions
- `retry_loop`, `blast_radius`, `new_scope`, `repair_loop`, `thrashing`, and `risk_escalation` use total prescriptions

This denominator choice matters. A signal count is never interpreted without
its window size.

## Scoring Flow

### 1. Compute Signal Rates

For each signal:

```text
rate = signal_count / signal_denominator
```

Examples:

- `retry_loop = 5` over `10` prescriptions -> `retry_rate = 0.5`
- `artifact_drift = 1` over `10` reports -> `drift_rate = 0.1`

### 2. Multiply Rates By Weights

Each signal rate contributes to the total penalty:

```text
penalty_contribution = weight(signal) × rate(signal)
```

Positive weights increase the penalty.
Negative weights reduce it.

Under the default profile, `repair_loop` is intentionally negative. It is a
small recovery bonus, not a failure penalty.

### 3. Sum The Penalty

```text
penalty = Σ(weight_i × rate_i)
```

The default profile is normalized. The net sum of all default weights,
including the negative `repair_loop` bonus, equals `1.0`.

### 4. Convert Penalty Into Raw Score

```text
raw_score = 100 × (1 - penalty)
```

Then clamp to the valid score range:

```text
score ∈ [0, 100]
```

This means:

- lower penalty -> higher score
- higher penalty -> lower score
- negative penalty from recovery behavior can lift the raw score above `100`, but the final score is clamped back to `100`

### 5. Apply Score Caps

Some conditions impose hard ceilings even if the raw weighted score is still
high.

In the active default profile:

- `artifact_drift > 5%` -> `score <= 85`

This is why a sequence can have a mathematically high raw score but still land
in a low band.

### 6. Apply Confidence Ceiling

Confidence is not another penalty term. It is a trust ceiling on the score.

In the active default profile:

- `protocol_violation > 10%` -> low confidence and `score <= 85`
- `external_pct > 50%` -> medium confidence and `score <= 95`
- otherwise confidence is high and there is no score ceiling beyond `100`

### 7. Map Score To Band

Under the active default profile:

- `99-100` -> `excellent`
- `95-<99` -> `good`
- `90-<95` -> `fair`
- `<90` -> `poor`

Band mapping happens after penalty, caps, and confidence ceilings are applied.

## Worked Examples

These examples use the active default profile `default.v1.1.0`.

### Example A: Retry Loop Alone

Sequence:

- `retry_loop = 5`
- `total_prescriptions = 10`

Calculation:

```text
retry_rate = 5 / 10 = 0.5
penalty = 0.15 × 0.5 = 0.075
raw_score = 100 × (1 - 0.075) = 92.5
```

No score cap applies.

Final result:

- `score = 92.5`
- `band = fair`

This is why a retry-heavy sequence is not automatically `poor`. The score is
driven by rate and weight, not by the label alone.

### Example B: Repair Loop

Sequence:

- `repair_loop = 1`
- `total_prescriptions = 10`

Calculation:

```text
repair_rate = 1 / 10 = 0.1
penalty = -0.05 × 0.1 = -0.005
raw_score = 100 × (1 - (-0.005)) = 100.5
clamped_score = 100
```

Final result:

- `score = 100`
- `band = excellent`

This is intentional. `repair_loop` means the actor failed, changed the artifact,
and then recovered successfully. The default model treats that as weak positive
evidence, not as unreliability.

### Example C: Thrashing Plus Retry

Sequence:

- `retry_loop = 5`
- `thrashing = 3`
- `total_prescriptions = 10`

Calculation:

```text
retry contribution = 0.15 × (5 / 10) = 0.075
thrashing contribution = 0.10 × (3 / 10) = 0.03
penalty = 0.105
raw_score = 100 × (1 - 0.105) = 89.5
```

Final result:

- `score = 89.5`
- `band = poor`

This sequence falls below the `90` fair/poor threshold because the two failure
patterns compound.

### Example D: Artifact Drift Cap

Sequence:

- `artifact_drift = 1`
- `total_reports = 10`

Weighted score first:

```text
drift_rate = 1 / 10 = 0.1
penalty = 0.25 × 0.1 = 0.025
raw_score = 97.5
```

But `artifact_drift > 5%`, so the default score cap applies:

```text
final_score = min(97.5, 85) = 85
```

Final result:

- `score = 85`
- `band = poor`

This is a good example of a non-obvious result that is not arbitrary: the cap
is explicit policy in the scoring profile.

## Why The Validation Harness Uses Score Windows

The signal-validation harness checks score windows in
[`tests/signal-validation/expected-bands.json`](../../tests/signal-validation/expected-bands.json).

Those values are not an independent source of truth. They are calibration
snapshots derived from:

- the active scoring profile
- the scripted sequence design
- the current score-band thresholds

If the scoring profile changes intentionally, the harness expectations should be
recalibrated to match.

## What Is Stable vs. What Can Change

Stable on the `v1` scoring-model line:

- scoring flow structure
- count -> rate -> weighted penalty -> score pipeline
- the existence of caps, confidence ceilings, and bands

Potentially changeable across profile or version updates:

- exact weights
- exact cap thresholds
- exact confidence rules
- exact band cutoffs

That is why the explainer doc and the profile doc are separate:

- this doc explains the model
- the profile doc defines the active numbers
