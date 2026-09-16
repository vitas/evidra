# AgentGateway Upstream Handoff and Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Preserve the completed AgentGateway fixes, explain their product and architectural intent, and prepare two focused upstream pull requests when human-authorship requirements can be met.

**Architecture:** AgentGateway owns gateway-specific MCP lifecycle and telemetry behavior. Evidra consumes standard ExtMCP or OTLP outputs and does not add Evidra-specific persistence, claims, or protocol fields to AgentGateway.

**Tech Stack:** Rust, MCP JSON-RPC, ExtMCP gRPC/protobuf, OpenTelemetry, Go-generated API bindings, GitHub pull requests.

---

## Current status

Implementation is complete and pushed to `vitas/agentgateway`. No pull request has been opened against `agentgateway/agentgateway`, and no release or tag has been created.

| Change | Fork branch | Head |
|---|---|---|
| MCP tool-error telemetry | [`fix/mcp-tool-error-otel`](https://github.com/agentgateway/agentgateway/compare/main...vitas:fix/mcp-tool-error-otel) | `9ae3e015` |
| ExtMCP terminal lifecycle | [`fix/extmcp-terminal-lifecycle`](https://github.com/agentgateway/agentgateway/compare/main...vitas:fix/extmcp-terminal-lifecycle) | `92c08e3f` |

The branches are independent and based on upstream `main` at `1d1c219b`.

## What was fixed

### 1. MCP tool errors in OpenTelemetry

MCP tool calls may return HTTP `200` while the JSON-RPC result contains `isError: true`. Previously such calls produced successful spans, hiding application-level tool failures.

The telemetry branch now:

- detects `CallToolResult.isError=true` from the captured MCP result;
- marks the span as an error;
- adds `error.type=tool_error`;
- preserves the existing precedence of gateway errors and HTTP `5xx` failures;
- includes a regression test for successful and failed tool results.

This is a generic AgentGateway observability fix. It contains no Evidra-specific behavior.

### 2. Terminal outcomes in ExtMCP

The existing ExtMCP response hook observed normal responses but could not distinguish tool errors, JSON-RPC errors, cancellations, or downstream disconnects.

The ExtMCP branch adds an optional terminal reason:

- `COMPLETED` — normal result;
- `ERROR` — JSON-RPC error;
- `TOOL_ERROR` — tool result with `isError=true`;
- `CANCELLATION_REQUESTED` — the cancellation notification was forwarded;
- `DISCONNECTED` — the downstream response stream ended before a result.

`CANCELLATION_REQUESTED` deliberately does not claim that upstream execution stopped. Terminal events without a response body are observational: ExtMCP mutations or rejections are ignored.

A request-scoped registry atomically claims the terminal event, so a response, cancellation, and disconnect race can emit at most one hook. Existing method, backend, and metadata context are preserved; raw error and cancellation payloads are not added.

## Verification already completed

- full AgentGateway `make test` passed;
- full AgentGateway `make lint` passed;
- focused OTLP regression test passed on its standalone branch;
- standalone OTLP branch passed `make lint`;
- ExtMCP cancellation, disconnect, JSON-RPC error, and terminal-classification tests passed;
- generated Go API consumer check passed with `go test ./controller/pkg/agentgateway/ir`;
- both branch histories pass `git diff --check` and contain DCO `Signed-off-by` trailers.

## Product and repository decision

Keep these changes in the AgentGateway project rather than copying gateway internals into Evidra.

- AgentGateway should expose accurate, vendor-neutral lifecycle and telemetry signals.
- Evidra should translate those signals into its evidence model in the separate adapter or bridge.
- AgentGateway must not write Evidra declarations or treat observations as agent claims.
- ExtMCP is the richer integration path; OTLP remains the lower-coupling fallback.

No new Evidra landing deployment is required for these upstream fixes.

## Why the upstream PRs are paused

AgentGateway's Generative AI Policy requires PR descriptions, documentation, and human-facing comments to be written by a human. It also requires disclosure of non-trivial AI assistance.

The branches may remain in the fork, but an upstream PR must not falsely check the human-authorship statement. The two internal files under `docs/plans/` should normally be omitted from the ExtMCP PR; they remain useful as local implementation records.

## Upstream preparation plan

### Task 1: Human ownership review

**Files:** No changes required.

1. Review both diffs and confirm the behavior can be explained and defended during maintainer review.
2. Confirm the terminal semantics, especially that cancellation means “forwarded request,” not “confirmed cancellation.”
3. Decide whether the ExtMCP API shape is acceptable as an additive optional field.

### Task 2: Clean the ExtMCP contribution surface

**Files:**

- Remove from the upstream branch: `docs/plans/2026-09-16-extmcp-terminal-lifecycle-design.md`
- Remove from the upstream branch: `docs/plans/2026-09-16-extmcp-terminal-lifecycle.md`
- Human-review and rewrite comments in: `crates/protos/proto/ext_mcp.proto`
- Human-review and rewrite comments in: `crates/agentgateway/src/mcp/handler.rs`

Keep the implementation and tests unchanged unless review finds a technical defect. Commit the cleanup with DCO sign-off.

### Task 3: Write the PR descriptions manually

Create two short descriptions in the contributor's own words. Each should state the problem, behavioral change, and tests run. Disclose non-trivial AI assistance as required by the Code of Conduct, and only check the human-authorship item after its conditions are true.

### Task 4: Refresh before submission

Run in each branch:

```bash
git fetch origin main
git rebase origin/main
make lint
make test
git diff --check origin/main...HEAD
```

Expected result: both branches are current, lint and tests pass, and the diffs contain no whitespace errors. Do not force-push without explicit approval; if rebasing changes published history, request approval first.

### Task 5: Open two upstream PRs

Open the smaller telemetry PR first. Open the ExtMCP PR separately so maintainers can discuss the public protocol independently from telemetry classification.

Targets:

1. `vitas:fix/mcp-tool-error-otel` → `agentgateway/agentgateway:main`
2. `vitas:fix/extmcp-terminal-lifecycle` → `agentgateway/agentgateway:main`

Do not create a release or tag. Keep both worktrees until CI and maintainer review finish.

## Definition of done

- both PR descriptions and all new human-facing text satisfy upstream authorship policy;
- the branches are rebased without unrelated changes;
- `make lint` and `make test` pass on each final PR head;
- upstream CI is green;
- maintainer feedback is resolved without adding Evidra-specific coupling;
- the PR URLs and final decisions are recorded in the Evidra integration notes.
