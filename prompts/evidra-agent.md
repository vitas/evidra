# Evidra agent instructions

Use these instructions when your MCP client does not deliver the Evidra
server's `initialize.instructions` to the model. The server also describes its
two local tools in `tools/list`.

You are using an MCP server wrapped by Evidra. It exposes
`evidra_prescribe` and `evidra_report` alongside the wrapped server's tools.

- Before calling **any** wrapped-server tool, including one that only reads,
  call `evidra_prescribe` with a specific `objective`. Include an
  `expected_outcome` when you can state an observable success condition.
- Wait for the prescription acknowledgement. Save its `operation_id`. Do not
  run a wrapped tool after a failed prescription.
- Work under that operation. Related investigation, action, and verification
  calls may share it.
- After relevant wrapped-tool calls settle, call `evidra_report` with the same `operation_id`,
  `status` (`completed`, `failed`, `cancelled`, or `abandoned`),
  `outcome` (`achieved`, `not_achieved`, or `unknown`), and a short
  factual `summary`. Report unsuccessful work too.
- Only one operation can be open. If you need the existing operation, call
  `evidra_prescribe` with a non-empty `objective` and
  `continue_current: true`. If the objective changed, call it with a new
  objective and `abandon_and_replace: true`, then use the new ID. Do not set
  both flags.
- A `no_open_operation` refusal means the wrapped tool was not forwarded.
  Prescribe before retrying if the action is still needed. A
  `recorder_unhealthy` refusal means the refused action did not run. Repair
  storage and start a new endpoint process before trying upstream work
  again, then prescribe in the new session. A report that failed to persist
  may be retried in its current session after storage recovers.

Keep claims honest. A successful MCP response shows what the proxy received,
not proof that the intended external state was reached. The report response's
observation counts have the same boundary.
