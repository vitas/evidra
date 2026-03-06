# Experiment Matrix (Real Agent Behavior)

Date: 2026-03-06

This matrix defines a minimal but rigorous path to measure real agent behavior on Evidra benchmark cases.

## Scope

Use this order:
1. Contract sanity on limited dataset (current 10 cases)
2. Pilot behavior runs (repeatability + variance)
3. Expanded dataset runs (30-50 curated real-derived cases)
4. Stress/fault-injection runs

Do not compare agents publicly from Phase 1-2 results.

## Fixed Controls (Must Stay Constant per Matrix Run)

- `model_id` and provider
- prompt template + `prompt_version`
- tool access policy
- timeout / retry policy
- dataset label (`limited-contract-baseline` for current baseline)
- run mode (`local-mcp`, `local-rest`, etc.)

## Phase Matrix

| Phase | Goal | Dataset | Models | Repeats | Gate |
|---|---|---|---|---|---|
| P0 | Harness correctness | 10 baseline cases | 1 model | 1 | `benchmark-validate` + `benchmark-check-contracts` pass |
| P1 | Behavior variance pilot | 10 baseline cases | 1-2 models | 3 | no protocol break in harness, stable result schema output |
| P2 | Comparative baseline | 30-50 curated cases | 3-5 models | 3 | >=95% runs produce valid result files |
| P3 | Adversarial behavior | +fault scenarios | 3-5 models | 3-5 | signal/failure patterns reproducible |

## Recommended Initial Matrix (Now)

- Cases: all current `tests/benchmark/cases/*`
- Models: one production target + one cheaper control model
- Repeats: `3`
- Timeout: `300s`
- Mode: `local-mcp` (or `local-rest` if MCP inspector unavailable)

## Required Outputs per Run

Each run must produce:
- `agent_stdout.log`
- `agent_stderr.log`
- `agent_output.json` (raw model/harness output)
- `result.json` (normalized to `RESULT_SCHEMA.json`)

## Suggested Evaluation Metrics

- run success/failure/timeout rates
- risk-level match rate (`predicted_risk_level` vs expected)
- risk-details precision/recall/F1
- protocol-violation counts
- duration and cost (if available)

## Execution

Use:

```bash
bash scripts/run-agent-experiments.sh \
  --model-id anthropic/claude-3-5-haiku \
  --provider anthropic \
  --prompt-version v1 \
  --repeats 3 \
  --timeout-seconds 300 \
  --agent-cmd 'jq -n --arg lvl "unknown" --argjson tags "[]" "{predicted_risk_level:$lvl,predicted_risk_details:$tags}" > "$EVIDRA_AGENT_OUTPUT"'
```

Then aggregate from `experiments/results/<run_stamp>/summary.jsonl`.
