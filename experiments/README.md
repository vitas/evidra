# Experiments

This folder is for running and storing real-agent benchmark experiment outputs.

## What to use

- Go CLI: `evidra-exp` (`go run ./cmd/evidra-exp ...` or `bin/evidra-exp` after build)
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
| `docs/experimental/RESULT_SCHEMA.json` | `evidra-exp artifact run` | Artifact-only risk classification quality | `case` | `evidra.result.v1` |
| `docs/experimental/EXECUTION_RESULT_SCHEMA.json` | `evidra-exp execution run` | Real execution behavior (MCP + command execution) | `scenario` + `agent_result` | `evidra.exec-result.v1` |

## Quick Start

Dry run (sanity check):

```bash
go run ./cmd/evidra-exp artifact run \
  --model-id test/model \
  --agent dry-run \
  --repeats 1 \
  --max-cases 1
```

Real run with Bifrost adapter:

```bash
go run ./cmd/evidra-exp artifact run \
  --model-id anthropic/claude-3-5-haiku \
  --provider bifrost \
  --agent bifrost \
  --prompt-version v1 \
  --repeats 3 \
  --timeout-seconds 300
```

Bifrost run (prompted, contract-versioned):

```bash
export EVIDRA_BIFROST_BASE_URL="http://localhost:8080/openai"
# optional Bifrost headers:
# export EVIDRA_BIFROST_VK="vk_..."
# export EVIDRA_BIFROST_AUTH_BEARER="..."

go run ./cmd/evidra-exp artifact run \
  --model-id anthropic/claude-3-5-haiku \
  --provider bifrost \
  --agent bifrost \
  --mode local-mcp \
  --prompt-file prompts/experiments/runtime/system_instructions.txt \
  --repeats 3 \
  --timeout-seconds 300
```

Claude headless run (chat subscription path, no Anthropic API credits required):

```bash
# prerequisite: claude CLI installed and logged in
go run ./cmd/evidra-exp artifact run \
  --model-id claude/haiku \
  --provider claude \
  --agent claude \
  --mode local-mcp \
  --prompt-file prompts/experiments/runtime/system_instructions.txt \
  --repeats 3 \
  --timeout-seconds 300
```

The Claude adapter maps `claude/<alias>` to CLI `--model <alias>` (for example `claude/haiku`, `claude/sonnet`, `claude/opus`).

Notes:
- If `--prompt-version` is omitted, the runner uses prompt file `# contract: ...` header.

## Execution-Mode Runs (MCP + Real kubectl)

This mode is for real behavior validation (prescribe -> execute -> report), not artifact-only classification.

```bash
# prerequisites:
# - kube context points to a test cluster
# - npx + MCP inspector available
# - evidra-mcp built (or buildable via go)

go run ./cmd/evidra-exp execution run \
  --model-id execution/mcp-kubectl \
  --provider local \
  --agent mcp-kubectl \
  --mode local-mcp \
  --repeats 1 \
  --timeout-seconds 600
```

To reuse the same output directory safely, add `--clean-out-dir` (it removes existing files inside `--out-dir` before the run).

## Output Layout

By default, results are written to `experiments/results/<timestamp>/`.

Each run contains:
- `agent_stdout.log`
- `agent_stderr.log`
- `agent_output.json`
- `agent_raw_stream.jsonl`
- `result.json`

A run index is written to `summary.jsonl` in the timestamp folder.
