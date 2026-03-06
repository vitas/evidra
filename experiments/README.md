# Experiments

This folder is for running and storing real-agent benchmark experiment outputs.

## What to use

- Runner script: `scripts/run-agent-experiments.sh`
- Matrix definition: `docs/system-design/EXPERIMENT_MATRIX.md`
- Result schema: `docs/system-design/RESULT_SCHEMA.json`

## Quick Start

Dry run (sanity check):

```bash
bash scripts/run-agent-experiments.sh \
  --model-id test/model \
  --dry-run \
  --repeats 1 \
  --max-cases 1
```

Real run (agent command must write JSON to `$EVIDRA_AGENT_OUTPUT`):

```bash
bash scripts/run-agent-experiments.sh \
  --model-id anthropic/claude-3-5-haiku \
  --provider anthropic \
  --prompt-version v1 \
  --repeats 3 \
  --timeout-seconds 300 \
  --agent-cmd '...your harness command...'
```

## Expected Agent Output JSON

```json
{
  "predicted_risk_level": "critical",
  "predicted_risk_details": ["k8s.privileged_container"]
}
```

## Output Layout

By default, results are written to `experiments/results/<timestamp>/`.

Each run contains:
- `agent_stdout.log`
- `agent_stderr.log`
- `agent_output.json`
- `result.json`

A run index is written to `summary.jsonl` in the timestamp folder.
