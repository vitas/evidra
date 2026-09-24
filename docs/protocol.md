# Prescribe and Report Protocol

This page is the detailed agent-facing contract for the current Evidra MCP
endpoint. It describes what a client sends, how one operation moves through the
endpoint, and what the resulting evidence can establish. For startup flags and
a runnable fixture, see [Getting Started](getting-started.md). For the wire
record and cryptographic limits, see the [evidence format](evidence-format.md).

## Boundary and vocabulary

Evidra wraps one child stdio MCP server. The client sees the child's tools plus
two local tools named `evidra_prescribe` and `evidra_report`. Other upstream
tool names, schemas, and results are relayed without being renamed.

| Term | Meaning |
|---|---|
| Session | One client-facing endpoint process and its in-memory protocol state. |
| Operation | One agent-declared objective, identified by an `operation_id`. |
| Prescription | The agent's declaration of intended work before upstream calls. |
| Execution | One upstream `tools/call` observed by the proxy. Several executions may belong to one operation. |
| Report | The agent's terminal account of an operation. |
| Recorder root | Optional directory passed to `--evidence-dir`; each process creates one recorder directory beneath it. |

The endpoint enforces **order**, not the meaning or safety of an action. It does
not inspect an objective to decide whether a tool may run. It also does not
decide whether a tool is harmless from its name, arguments, or annotations.

## Where an agent receives the rules

The MCP `initialize` result contains Evidra's protocol instructions followed,
when present, by the upstream server's own instructions under a visible source
heading. The first `tools/list` page includes the two local tools with their
descriptions and input schemas. A client should make those instructions and
descriptions available to the agent. Evidra does not need an MCP prompt
capability or an installed skill for the protocol to function.

The repository also offers an optional [agent skill](../skills/evidra/SKILL.md)
and [portable instruction file](../prompts/evidra-agent.md) for clients that
support them. They restate the current lifecycle; they do not add tools or
change enforcement.

## The normal lifecycle

~~~text
closed
  │ evidra_prescribe(objective)
  ▼
open ── upstream tools/call ──► open
  │                              │
  │ evidra_report                │ more calls may follow
  ▼                              ▼
closed                         open
~~~

One endpoint process has at most one open operation. An operation can contain
several upstream calls, including investigation, a change, and verification.
Choose an objective that describes one coherent piece of work. Prescribe before
the first upstream tool call, even when that first call only reads information.

A complete operation has this order:

1. Call `evidra_prescribe` with a non-empty objective.
2. Keep the returned `operation_id`.
3. Call the wrapped server's tools as needed.
4. Call `evidra_report` with that ID and an honest status and outcome.

The report is needed after failure, cancellation, or a deliberate decision to
stop as well as after success. Starting a prescription and then ending the
session without a report leaves an open declaration in the evidence. No
upstream tool result automatically closes it.

### One complete MCP exchange

These examples show `tools/call` request parameters and the JSON object encoded
as text in each Evidra tool response. The outer MCP result has a `content`
array and `isError` flag.

~~~json
{"name":"evidra_prescribe","arguments":{
  "objective":"Read the fixture service status",
  "expected_outcome":"The fixture reports status ok"
}}
~~~

~~~json
{"operation_id":"EV-...","state":"open","recorded_at":"...","expected_outcome":"The fixture reports status ok","note":"Use evidra_report when this operation is finished."}
~~~

The agent then calls the upstream fixture tool:

~~~json
{"name":"get_status","arguments":{}}
~~~

After reading the actual result:

~~~json
{"name":"evidra_report","arguments":{
  "operation_id":"EV-...",
  "status":"completed",
  "outcome":"achieved",
  "summary":"The fixture returned status ok."
}}
~~~

The report acknowledgement has `ok: true`, `state: "reported"`, the
`operation_id`, and an `observations` object. Its counts cover upstream
responses completed by the time of the report; they are not a verdict about
the external system. A zero count can mean no call was made or that a call
is still in flight, so wait for relevant calls before reporting.

## `evidra_prescribe`

The advertised input schema is:

| Field | Type | Required | Meaning |
|---|---|---|---|
| `objective` | string | Yes | Intended result, in the agent's own words. Whitespace-only text is rejected. |
| `expected_outcome` | string | No | Observable state that would mean success. It is recorded as a claim, not checked automatically. |
| `continue_current` | boolean | No | Reuse the open operation instead of creating another. |
| `abandon_and_replace` | boolean | No | Record replacement of the open operation and open a new one. |

When no operation is open, a successful call returns a new `operation_id`,
`state: "open"`, and `recorded_at`. The agent must wait for that acknowledgement
before making an upstream call. With recording enabled, the prescription is
appended before the acknowledgement.

A second prescription while one is open returns `operation_already_open`
unless the agent chooses one of these paths:

- `continue_current: true` returns the current ID and original objective. It
  creates no second prescription event. The request still needs a non-empty
  `objective` because that is required for every prescription call.
- `abandon_and_replace: true` records that the old operation was replaced
  without a report, then opens a new operation with a new ID and objective.
  This is for genuinely changed work, not a way to hide a failed attempt.

The two choices are mutually exclusive. To close an operation normally, call
`evidra_report` and then prescribe the next one.

## Upstream tool calls and enforcement

The enforcement setting is selected when starting `evidra-mcp`. The table
assumes the recorder is healthy; a recording failure can refuse further
upstream calls in either mode.

| Mode | Behavior without an open operation |
|---|---|
| `--enforce=all`, the default | Refuse every upstream `tools/call` before forwarding it. The refusal is an MCP tool result with `isError: true`. |
| `--enforce=off` | Forward the call and observe it with no operation ID. A later summary classifies such execution as unprescribed. |

The rule covers **all** upstream tools, even tools annotated read-only. The two
local Evidra tools remain callable so the agent can open or close an operation.
Other supported MCP traffic is relayed according to the
[architecture](architecture.md); this rule specifically concerns upstream
`tools/call` requests.

In `all` mode, a blocked result has a JSON text body like:

~~~json
{
  "error":"no_open_operation",
  "tool":"get_status",
  "required_action":"evidra_prescribe",
  "instruction":"Call evidra_prescribe with the objective you intend to achieve, then repeat this tool call. Every upstream tool of this server requires an open operation, including tools the server declares read-only."
}
~~~

That call did not reach the upstream. If the task still calls for it, prescribe
first and then repeat it. With recording enabled, Evidra attempts a
`protocol_violation` event for the blocked attempt. A refusal is not evidence
that the upstream tool ran.

## `evidra_report`

| Field | Type | Required | Meaning |
|---|---|---|---|
| `operation_id` | string | Yes | ID returned by the prescription being closed. |
| `status` | enum | Yes | `completed`, `failed`, `cancelled`, or `abandoned`. |
| `outcome` | enum | Yes | `achieved`, `not_achieved`, or `unknown`. |
| `summary` | string | No | Short account of what happened. |

Status describes how the work ended. Outcome describes whether the intended
result was reached. They are separate agent claims. For example, a completed
investigation may have `outcome: "unknown"` if the external state cannot be
confirmed. A successful tool response alone does not justify `achieved`.

| Status | Use when |
|---|---|
| `completed` | The intended work reached its normal end, regardless of whether the desired outcome was confirmed. |
| `failed` | The attempt ended because it could not complete the intended work. |
| `cancelled` | Work was interrupted or cancelled before its normal end. |
| `abandoned` | The agent intentionally stopped pursuing this operation. |

For example, a tool error while trying to restart a service may warrant
`status: "failed"` and `outcome: "not_achieved"` if the service demonstrably
did not restart. If its external state is unknown, use `outcome: "unknown"`.
The endpoint records these values but does not infer or verify them.

A valid report for the open ID appends `operation_reported`, closes the
operation, and returns `state: "reported"`. The response's `observations`
include:

- `executions_observed`, `succeeded`, and `failed_or_cancelled` counts;
- up to five `tools_observed` names when executions occurred;
- `all_observed_declared_read_only: true` only when every observed call was
  annotated read-only by the upstream; an unannotated call prevents that
  statement. One explicit false may produce
  `all_observed_declared_read_only: false` even when other calls lack the
  annotation. Either value is accompanied by `annotations_verified: false`;
- `provenance: "proxy_observed"` and an explanatory note.

These are in-memory proxy counters and are available even without
`--evidence-dir`. They do not prove a real-world outcome. If no response
completed before the report, the acknowledgement says so; it does not rule
out an in-flight call. Counters for sufficiently old operations may be
evicted under the endpoint's bounded tracking;
`older_operations_dropped` reports that condition.
The `failed_or_cancelled` bucket counts completed responses that were not
classified as success; its name is not a claim about external state.

If no operation is open and the last successfully reported ID is reported
again, the endpoint returns `state: "already_reported"` without appending
another report. This handles a retry of the final report; it does not create
another operation. Once a new operation is open, a report for the old ID is
instead an `operation_id_mismatch`.

### Examples of changed or unsuccessful work

If a wrapped tool returns an error, the operation stays open. The agent can
inspect or retry within the same objective, then report what actually happened.
A tool error does not write a report on the agent's behalf:

~~~text
evidra_prescribe("Restore checkout availability") → operation ID A
restart(...) → upstream tool error
get_status(...) → checkout still unavailable
evidra_report(A, failed, not_achieved, "Restart failed; status remained unavailable.")
~~~

If the task changes while A is open, the agent may report A as abandoned and
then prescribe B. An explicit replacement is also available when a new
objective must start immediately:

~~~json
{"name":"evidra_prescribe","arguments":{
  "objective":"Investigate the payment service instead",
  "abandon_and_replace":true
}}
~~~

The replacement returns a new ID. It records A as replaced without an
`operation_reported` event; it does not silently rewrite A's objective.
Use `continue_current: true`, with a non-empty `objective`, when the work
still belongs to A.

With `--enforce=off`, an upstream call made before any prescription is
forwarded with an empty operation ID. It remains an observed execution, but
there is no declared objective to attach to it. A later prescription does not
retroactively place that earlier call under its operation.

## State and error recovery

Protocol failures from the local tools are returned as MCP tool results with
`isError: true`. Their text content is JSON with an `error` field. Inspect that
field rather than treating every error as a transport failure.

| Error | When it appears | Agent action |
|---|---|---|
| `objective_required` | Prescription objective is blank. | Supply a real objective. |
| `operation_already_open` | A second prescription has no choice flag. | Report the current operation, continue it, or explicitly replace it. |
| `conflicting_choices` | Both continuation and replacement are requested. | Select one. |
| `invalid_status` or `invalid_outcome` | Report enum is unsupported. | Use an advertised enum value. |
| `operation_id_mismatch` | Report ID differs from the open ID. | Use the open operation ID returned in the error. |
| `no_open_operation` | An upstream call is blocked, or a report has no matching open operation. | Prescribe before retrying an upstream call; do not invent a report. |
| `recorder_unhealthy` | Required evidence could not be written. | Do not assume a refused upstream call happened. Repair storage and start a new endpoint process before retrying upstream work. |

A prescription whose evidence cannot be appended is not acknowledged. A report
whose evidence cannot be appended leaves the operation open so it can be
retried if storage becomes writable again. A refused upstream execution whose
start could not be recorded is not forwarded. If the upstream already
responded but the terminal evidence write fails, Evidra still relays the real
response and marks or logs the recording gap; changing it to a fabricated
tool failure could induce an unsafe retry.

After a recording failure, this endpoint process keeps refusing upstream
calls even if the path becomes writable again. A report may still persist and
close the current operation, but upstream work requires a new endpoint
process and a new prescription. Inspect the old recorder with `evidra verify`
and `evidra summarize` to understand its coverage.

When replacing an open operation, wait for a successful acknowledgement before
using the new ID. If either replacement event fails to append, the endpoint
leaves no operation open. The event stream may contain a replacement without
the new prescription if the second append failed. Repair storage, restart the
endpoint, and prescribe anew; do not act under the unacknowledged ID.

## Evidence sequence and reconciliation

With `--evidence-dir`, the endpoint writes one signed event stream per process.
A typical operation produces:

~~~text
operation_prescribed        agent_declared
execution_started          proxy_observed
execution_finished         proxy_observed
operation_reported         agent_declared
~~~

For each forwarded tool call, `execution_started` is appended before the
request is sent upstream. When a response arrives, `execution_finished` is
normally appended before the response reaches the client. Multiple calls can
be in flight; their results can arrive out of order. The operation and
execution IDs, not adjacency in the file, pair the records.

The endpoint can accept a report while an upstream call is still in flight.
The later finish event can therefore appear after `operation_reported`.
Clients should ordinarily wait for relevant calls to settle before reporting;
readers must use IDs rather than assume the report is the final event.

`evidra summarize --dir <root>` reconciles the three source layers:

| Layer | What it can say |
|---|---|
| Declared | The objective, expected outcome, replacement, and report the agent supplied. |
| Observed | Which upstream calls crossed this endpoint, their start/finish boundaries and response classification. |
| Recorder-generated | Process lifecycle, blocked attempts, and known degraded recording windows. |

An unprescribed execution in observe-only mode is a read-time classification,
not a separate stored event type. A missing report or unmatched execution
start is likewise a reconciliation finding. For the exact event fields,
fingerprints, signatures, and limits of verification, read the
[evidence format](evidence-format.md).

## Process and trust limits

Protocol state is in memory for one endpoint process. Restarting the endpoint
does not reopen an operation from an earlier process; a new session starts
closed. Recording is opt-in with `--evidence-dir`. Without it, order enforcement
and the report's observation counters still work, but no event stream is saved.

Evidra observes only MCP traffic routed through its endpoint. It does not see
calls made to the upstream by another route, inspect external state, validate
upstream tool annotations, or certify that the agent's report is true. It does
not sandbox the upstream command. A valid evidence chain establishes local
record consistency under the recorder's key, subject to the limitations in
the [trust model](evidence-format.md).

## Using the protocol with an agent

The short [skill](../skills/evidra/SKILL.md) and
[portable instructions](../prompts/evidra-agent.md) are optional client-side
guidance. They can improve adoption when a client does not expose the
endpoint's `initialize.instructions` to its model. The runtime rules above
still apply if neither is installed. Use the
[Getting Started guide](getting-started.md) to configure a client and walk
through one recorded operation.
