# Experiments

This folder is for running and storing real-agent benchmark experiment outputs.

## What to use

- Runner script: `scripts/run-agent-experiments.sh`
- Execution runner script: `scripts/run-agent-execution-experiments.sh`
- Bifrost OpenAI-compatible agent command wrapper: `scripts/agent-cmd-bifrost.sh`
- Claude headless agent command wrapper: `scripts/agent-cmd-claude.sh`
- MCP + kubectl execution wrapper: `scripts/agent-cmd-mcp-kubectl.sh`
- Matrix definition: `docs/experimental/EXPERIMENT_MATRIX.md`
- Result schema: `docs/experimental/RESULT_SCHEMA.json`
- Execution result schema: `docs/experimental/EXECUTION_RESULT_SCHEMA.json`
- Experiment prompt contract: `prompts/experiments/runtime/system_instructions.txt`
- Prompt source contract: `prompts/source/contracts/v1.0.1/`
- Prompt source-of-truth spec: `docs/system-design/EVIDRA_PROMPT_FACTORY_SPEC.md`

Prompt editing policy:
- Edit only `prompts/source/contracts/<version>/...`
- Regenerate active prompts with `make prompts-generate`
- Verify no drift with `make prompts-verify`

## Schema Differences

| Schema | Used by | Focus | Core object | Schema version |
|---|---|---|---|---|
| `docs/experimental/RESULT_SCHEMA.json` | `scripts/run-agent-experiments.sh` | Artifact-only risk classification quality | `case` | `evidra.result.v1` |
| `docs/experimental/EXECUTION_RESULT_SCHEMA.json` | `scripts/run-agent-execution-experiments.sh` | Real execution behavior (MCP + command execution) | `scenario` + `agent_result` | `evidra.exec-result.v1` |

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
  --provider bifrost \
  --prompt-version v1 \
  --repeats 3 \
  --timeout-seconds 300 \
  --agent-cmd 'bash scripts/agent-cmd-bifrost.sh'
```

If you use a custom harness, replace `--agent-cmd` with your real command string.

Bifrost run (prompted, contract-versioned):

```bash
export EVIDRA_BIFROST_BASE_URL="http://localhost:8080/openai"
# optional Bifrost headers:
# export EVIDRA_BIFROST_VK="vk_..."
# export EVIDRA_BIFROST_AUTH_BEARER="..."

bash scripts/run-agent-experiments.sh \
  --model-id anthropic/claude-3-5-haiku \
  --provider bifrost \
  --mode local-mcp \
  --prompt-file prompts/experiments/runtime/system_instructions.txt \
  --repeats 3 \
  --timeout-seconds 300 \
  --agent-cmd 'bash scripts/agent-cmd-bifrost.sh'
```

Claude headless run (chat subscription path, no Anthropic API credits required):

```bash
# prerequisite: claude CLI installed and logged in
bash scripts/run-agent-experiments.sh \
  --model-id claude/haiku \
  --provider claude \
  --mode local-mcp \
  --prompt-file prompts/experiments/runtime/system_instructions.txt \
  --repeats 3 \
  --timeout-seconds 300 \
  --agent-cmd 'bash scripts/agent-cmd-claude.sh'
```

The Claude wrapper maps `claude/<alias>` to CLI `--model <alias>` (for example `claude/haiku`, `claude/sonnet`, `claude/opus`).

Notes:
- If `--prompt-version` is omitted, the runner uses prompt file `# contract: ...` header.
- `EVIDRA_PROMPT_FILE`, `EVIDRA_PROMPT_VERSION`, and `EVIDRA_PROMPT_CONTRACT_VERSION`
  are exported to each agent run.
- `EVIDRA_AGENT_RAW_STREAM` is exported per run and can be used to persist raw model/MCP output.

## Execution-Mode Runs (MCP + Real kubectl)

This mode is for real behavior validation (prescribe -> execute -> report), not artifact-only classification.

```bash
# prerequisites:
# - kube context points to a test cluster
# - npx + MCP inspector available
# - evidra-mcp built (or buildable via go)

bash scripts/run-agent-execution-experiments.sh \
  --model-id execution/mcp-kubectl \
  --provider local \
  --mode local-mcp \
  --repeats 1 \
  --timeout-seconds 600 \
  --agent-cmd 'bash scripts/agent-cmd-mcp-kubectl.sh'
```

To reuse the same output directory safely, add `--clean-out-dir` (it removes existing files inside `--out-dir` before the run).

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
- `agent_raw_stream.jsonl`
- `result.json`

A run index is written to `summary.jsonl` in the timestamp folder.
