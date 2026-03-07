# Evidra Experiments Go CLI Design

Date: 2026-03-07
Status: approved for implementation

## Goal

Replace experiment shell/python runner utilities with a single Go CLI binary (`evidra-exp`) that supports:
- artifact benchmark runs
- execution benchmark runs
- adapter parity for `claude`, `bifrost`, and `mcp-kubectl`
- deterministic output layout and schemas used by existing experiment workflows

This is a clean break: old script entry points are removed after migration.

## Command Surface

- `evidra-exp artifact run`
- `evidra-exp execution run`

Shared concepts:
- `--out-dir`
- `--clean-out-dir`
- `--repeats`
- `--timeout-seconds`
- model/provider/prompt metadata
- adapter selection via `--agent`

## Architecture

Packages under `internal/experiments`:
- `runner`: run loops, summary counters, result writing
- `adapters`: agent implementations (`claude`, `bifrost`, `mcp-kubectl`, `dry-run`)
- `io`: case/scenario loading, path resolution, safe out-dir clean
- `types`: result structs for `evidra.result.v1` and `evidra.exec-result.v1`

## Adapter Interface

Artifact adapter:
- input: artifact path, expected path, model/provider/prompt metadata
- output: risk level/tags, optional raw stream

Execution adapter:
- input: scenario fields (`tool`, `operation`, `artifact`, `execute_cmd`) and metadata
- output: prescribe/report booleans, command exit code, risk level/tags, ids

## Compatibility Policy

- Keep schema versions unchanged:
  - `evidra.result.v1`
  - `evidra.exec-result.v1`
- Keep file layout unchanged:
  - `<run_dir>/agent_stdout.log`
  - `<run_dir>/agent_stderr.log`
  - `<run_dir>/agent_output.json`
  - `<run_dir>/agent_raw_stream.jsonl`
  - `<run_dir>/result.json`
  - `<out_dir>/summary.jsonl`

## Error Model

- Statuses: `success`, `failure`, `timeout`, `dry_run`
- Timeout enforced by Go context timeout
- Safe clean for `--clean-out-dir` (reject empty, `/`, `.`, `..`)

## Migration

1. Add `cmd/evidra-exp` and internal implementation.
2. Port tests to call `evidra-exp`.
3. Update docs and Makefile commands.
4. Remove legacy shell/python experiment runners.
