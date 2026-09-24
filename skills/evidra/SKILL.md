---
name: evidra
description: Use when an MCP task uses a server that exposes evidra_prescribe and evidra_report, including read-only investigation and operational changes through that server.
---

# Evidra MCP operation protocol

Evidra wraps one MCP server. Its local tools are `evidra_prescribe` and
`evidra_report`; the other tools come from the wrapped server. Follow the
server's current `initialize.instructions` and tool schemas when available.

## Work under one operation

1. Before **any** wrapped-server tool call, including a read-only call, use
   `evidra_prescribe` with a concrete `objective`. Add `expected_outcome` when
   there is an observable success condition.
2. Wait for a successful response and keep its `operation_id`. If the call
   fails with `recorder_unhealthy`, do not proceed as if an operation opened.
3. Use the wrapped tools for the intended work. Several calls may share one
   operation when they serve the same objective.
4. After the calls settle, use `evidra_report` with that ID, a `status`, an
   `outcome`, and a brief `summary`. Report failure, cancellation, and
   abandonment too. Do not claim `achieved` solely because a tool returned
   success.

Report statuses: `completed`, `failed`, `cancelled`, `abandoned`.
Outcomes: `achieved`, `not_achieved`, `unknown`.

## When an operation is already open

Finish and report it before starting unrelated work. If the new call belongs
to the same objective, supply a non-empty `objective` with
`continue_current: true` and keep the returned ID.
If the objective genuinely changed, use `abandon_and_replace: true` with the
new objective, wait for the new ID, and recognize that the old operation will
be recorded as replaced without a report. The two choice flags cannot be used
together.

If a wrapped tool is refused with `no_open_operation`, its action was not
forwarded. Prescribe, then retry only if the task still requires that call.
If recording fails, never assume a refused action happened. Repair storage and
start a new endpoint process before retrying upstream work; prescribe again
in that new session. A failed report leaves its operation open and may be
retried in the current session after storage recovers. If replacement failed,
no operation remains open in that session.

Keep declarations and reports factual. Evidra observes traffic at its MCP
boundary; it does not verify external state or the truth of an agent claim.
