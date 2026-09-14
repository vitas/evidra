# Architecture

## System boundary

Evidra is an MCP endpoint placed in front of one child stdio MCP server. It sees
the MCP messages that cross that boundary, adds two local lifecycle tools, and
optionally appends evidence to a local recorder directory.

```text
client stdin/stdout
       ↕
evidra-mcp endpoint
  ├─ local: evidra_prescribe, evidra_report
  ├─ evidence: signed JSONL recorder directory
  └─ child stdio: one upstream MCP server
```

Evidra does not inspect the external system behind the upstream. It cannot infer
that a successful MCP response caused the intended real-world state.

## Components

- `evidra-mcp` owns client framing, the upstream child process, protocol
  composition, operation state, enforcement, and the recorder connection.
- The local `evidra_prescribe` tool opens an operation from agent-provided
  objective text. `evidra_report` closes it with the agent's terminal claim.
- The proxy observer pairs upstream tool-call starts and finishes by JSON-RPC
  request ID and operation ID.
- `pkg/evidence` writes and verifies one signed event chain per endpoint process.
- `pkg/report` reconciles declared, observed, and reported records and writes
  `summary.json`.
- `evidra` is the read-side CLI for verification and reconciliation.

`cmd/evidra-fixture` and `cmd/evidra-gatea` are validation tools, not the runtime
product path.

## MCP composition

At startup the endpoint launches the child, initializes it using MCP version
`2025-06-18`, and fetches every page of its tool list. An upstream that declares
`evidra_prescribe` or `evidra_report` is rejected because local and upstream tool
names are never rewritten.

The client-facing `tools/list` places the two local tools on the first upstream
page. Upstream names, arguments, results, cursors, annotations, notifications,
and request IDs remain wire data. `ping` and `initialize` are answered by the
endpoint; other supported traffic is relayed.

By default the endpoint advertises only the profile covered by conformance tests:
tools, plus upstream logging when present. Prompts, resources, and completions
can be advertised with `--advertise-passthrough`, but remain outside the tested
default profile. Server-to-client requests and notifications are relayed.

Client-visible protocol versions are `2025-06-18`, `2025-03-26`, and
`2024-11-05`. Unknown requested versions negotiate to `2025-06-18`.

## Operation state machine

One endpoint process owns one MCP session and at most one open operation:

```text
closed
  │ evidra_prescribe
  v
open ── upstream tool calls ──> open
  │
  ├─ evidra_report ──> closed (reported)
  └─ abandon_and_replace ──> open (new operation)
```

A second prescription while open returns `operation_already_open`. The agent may
set `continue_current=true` to reuse the existing ID, or
`abandon_and_replace=true` to mark the old operation replaced and open a new one.
Those choices are mutually exclusive.

Reports require the current `operation_id`, one of four statuses (`completed`,
`failed`, `cancelled`, `abandoned`), and one of three outcomes (`achieved`,
`not_achieved`, `unknown`). A duplicate report for the most recently closed
operation is acknowledged idempotently without creating another event.

With `--enforce=all`, an upstream `tools/call` outside an open operation is not
forwarded. The client receives a recoverable error result and, when recording is
enabled, a `protocol_violation` event is attempted. `--enforce=off` forwards and
observes the call with an empty operation ID. Neither mode interprets tool names,
arguments, or annotations as risk.

## Evidence write ordering

For a forwarded operational call, ordering is deliberate:

1. Canonicalize and fingerprint arguments.
2. Append `execution_started`.
3. Forward the original request to the child.
4. Receive and classify the child's response.
5. Append `execution_finished`.
6. Relay the original response to the client.

The start record therefore exists before an action can reach the upstream. The
finish record normally exists before a client can react to the response. On
shutdown, unanswered calls receive terminal evidence with `unknown` or
`cancelled` status before outstanding clients are failed.

Prescriptions are appended before an operation is acknowledged. Reports are
appended before closure is acknowledged. Recorder lifecycle records bracket the
process when recording is enabled.

## Storage-failure behavior

Failure handling depends on what is already known to have happened:

- If the requested recorder root cannot open, the endpoint does not start.
- If `execution_started` cannot be appended, the operational call is refused and
  is not forwarded.
- If `execution_finished` cannot be appended after the upstream responded, the
  real response is still relayed. Evidra does not fabricate a tool failure that
  could cause an unsafe retry.
- If a report cannot be appended, the operation remains open so the agent can
  retry the report.
- When the store recovers, it attempts a `recorder_degraded` event describing
  the known failed-write window. Coverage is reported separately from chain and
  signature validity.
- Without `--evidence-dir`, an explicit no-op recorder is used: enforcement and
  in-process observation continue, but nothing is persisted.

The JSONL writer flushes every event to the process-visible file. It does not
claim persistence across abrupt host or storage failure; see the
[evidence trust model](evidence-format.md).

## Concurrency boundary

Client and upstream streams have separate serialized writers. One goroutine reads
the client and one reads the upstream; the child may answer multiple tool calls
out of order. Pending requests pair responses by direction and ID. The evidence
store serializes sequence assignment, hashing, signing, append, and tail updates
with one mutex, which is the single-writer boundary for a recorder directory.

Operation state and request maps are protected independently from the store.
The current endpoint can accept a report while upstream executions remain in
flight; event IDs and operation IDs remain the authoritative pairing mechanism,
and consumers must not assume a report is always the final event for an operation.

## Supported and unsupported scope

Supported runtime scope:

- one local endpoint process and one child stdio upstream;
- MCP tools and the documented relayed message behavior;
- protocol-order enforcement (`all`) or observation without refusals (`off`);
- local recorder roots with one writer per recorder directory;
- offline chain/signature/coverage verification and reconciliation.

Not supported or not claimed:

- HTTP/SSE transports or multi-upstream routing;
- a hosted storage or analysis service;
- a built-in catalogue of operational tools;
- semantic command safety, policy evaluation, or sandboxing;
- validation of upstream-provided annotations;
- observation of work that bypasses this endpoint;
- independent proof of external-world outcomes or organizational identity.

## Package map

| Path | Responsibility |
|---|---|
| `cmd/evidra-mcp` | Endpoint flags, process lifecycle, and stdio entry point. |
| `cmd/evidra` | Read-only `summarize`, `verify`, and `version` commands. |
| `pkg/proxy` | MCP framing, composition, operation state, enforcement, observations, and recorder adapter. |
| `pkg/evidence` | Event schema, JCS, fingerprints, store, keys, and verification. |
| `pkg/report` | Per-operation reconciliation and `summary.json`. |
| `pkg/version` | Build-version metadata shared by binaries. |
| `cmd/evidra-fixture` | Deterministic MCP conformance upstream. |
| `cmd/evidra-gatea` | Model/protocol experiment and regrade harness. |
