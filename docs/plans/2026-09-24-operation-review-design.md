# Operation Review Design

**Status:** Approved by the request to build the proposed one-operation review.

## Purpose

Give an operator a short, source-aware review of one recorded operation. The review should help the operator decide what to check next without treating an agent claim or MCP success as proof of an external outcome.

## Interface

`evidra summarize --dir ROOT --operation ID` writes a Markdown review to stdout. Without `--operation`, `summarize` keeps its current terminal and `summary.json` behavior. The operation review reads the full recorder history; combining it with `--since` is rejected. Unknown or duplicate IDs fail with a clear message.

## Content

The review names the recorder, upstream, enforcement mode, chain validity, signature validity, and known coverage. It presents the agent's objective and expected outcome, observed tool calls and terminal statuses, and the agent's final claim in separate sections. It lists reconciliation findings and a next check based on missing reports, unpaired calls, observed errors, or claims of success with no observed execution. Every review reminds the reader that external state needs independent verification.

## Boundaries

The renderer uses the existing `pkg/report` summary and does not read or reveal raw MCP arguments or results. It never infers that a successful MCP response achieved the real-world objective. Agent text is escaped before insertion into Markdown. A valid local chain and signature do not establish organizational identity or protect against replacement of the whole recorder directory.

## Verification

Test rendering for a success claim without execution, an operation with observed calls, an ambiguous ID, and untrusted agent text. Test CLI dispatch against a real local recorder. Run Go tests, repository guards, and `make lint`.
