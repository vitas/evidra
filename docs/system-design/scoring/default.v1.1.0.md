# Default Scoring Profile

- Status: Normative
- Version: v1.1.0
- Canonical for: default scoring weights, ceilings, and caps
- Audience: public

This document explains the active default scoring profile used by Evidra on the
`v1.1.0` scoring line.

For the scoring pipeline itself in plain language, see
[`EVIDRA_SCORING_MODEL_V1.md`](../EVIDRA_SCORING_MODEL_V1.md).

## Profile Identity

- profile id: `default.v1.1.0`
- spec version: `v1.1.0`
- scoring version: `v1.1.0`

## What Is Normative

The following values are normative because they are part of the runtime scoring
profile loaded by the product:

- signal weights
- `min_operations`
- score caps
- confidence ceilings
- score bands
- signal profile thresholds

If the profile file changes, score semantics change.

## What Is Heuristic

The chosen values are heuristic. They are project-authored calibration choices
based on Evidra's current product intent, not universal laws of automation
reliability.

In particular:

- weights reflect Evidra's ranking of behavioral signals
- score caps are safety rails to keep severe protocol drift from appearing
  artificially healthy
- confidence rules express observability trust and data quality, not workload
  danger directly

## Weight Rationale

- `protocol_violation = 0.30`
  Highest weight because broken prescribe/report semantics directly reduce trust
  in the evidence chain and can invalidate downstream interpretation.
- `artifact_drift = 0.25`
  Near-highest weight because intent and outcome diverging at the artifact level
  usually means the operator or agent did not execute what was originally
  described.
- `retry_loop = 0.15`
  High enough to matter because repeated failed retries are a strong signal of
  unstable automation behavior, but lower than protocol or artifact integrity
  failures.
- `thrashing = 0.10`
  Penalized materially because many distinct failed intents without success
  indicate poor adaptation, but still below direct integrity failures.
- `blast_radius = 0.10`
  Important, but scoped lower because large destructive actions are not
  inherently unreliable if they are deliberate and controlled.
- `risk_escalation = 0.10`
  Penalized moderately because moving above the actor's baseline risk suggests
  degraded operational discipline.
- `new_scope = 0.05`
  Small penalty because scope expansion can be legitimate exploration; it
  should be visible without dominating the score.
- `repair_loop = -0.05`
  A small bonus because a successful repair after failure is evidence of
  recovery. The bonus is intentionally limited so repair does not erase serious
  preceding failures.

The default profile is normalized: the net sum of all weights, including the
`repair_loop` bonus, is exactly `1.0`.

## Score Cap Rationale

- `artifact_drift > 5% => score <= 85`
  Repeated drift between prescribed and reported artifacts is severe enough that
  the score should not remain in a high-trust band.

There is no separate default score cap for `protocol_violation`. The stricter
`85` confidence ceiling below is the canonical guardrail for that condition.

## Confidence Rationale

- `protocol_violation > 10% => low confidence / ceiling 85`
  If the protocol itself is unreliable, the score is less trustworthy.
- `external_pct > 50% => medium confidence / ceiling 95`
  Heavy dependence on externally canonicalized data reduces confidence because
  Evidra cannot fully verify the canon source itself.
- otherwise `high confidence / ceiling 100`

## Signal Profile Thresholds

- `0` => `none`
- `< 0.02` => `low`
- `< 0.10` => `medium`
- `>= 0.10` => `high`

These thresholds are qualitative labels for human interpretation. They are
heuristic presentation tiers, not a separate scoring formula.
