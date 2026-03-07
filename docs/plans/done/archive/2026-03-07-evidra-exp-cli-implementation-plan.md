# Evidra Experiments Go CLI Implementation Plan

Date: 2026-03-07

## Scope (v1)

- New binary: `evidra-exp`
- Commands:
  - `artifact run`
  - `execution run`
- Adapters:
  - `claude`
  - `bifrost`
  - `mcp-kubectl`
  - `dry-run`
- Remove script/python runner stack.

## Steps

1. Add command surface and argument parsing with tests.
2. Implement shared run context and output writer.
3. Implement artifact runner and evaluation metrics.
4. Implement execution runner and evaluation metrics.
5. Implement adapters in Go with contract-compatible outputs.
6. Migrate shell tests to call `evidra-exp`.
7. Update docs and Makefile targets.
8. Remove legacy experiment scripts and Python adapters.
9. Run verification (unit + shell + targeted make commands).

## Risks

- `mcp-kubectl` adapter depends on local MCP inspector/CLI setup.
- Claude stream parsing must handle multi-event payloads.
- Bifrost headers/URL compatibility must match existing behavior.

## Acceptance

- Existing experiment shell tests pass against `evidra-exp`.
- Output schemas and paths remain unchanged.
- No Python dependency remains for experiment runners.
