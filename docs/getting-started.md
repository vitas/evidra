# Getting Started

This guide builds Evidra from source, wraps the included deterministic MCP
fixture, completes one recorded operation, and reads the resulting evidence.

> The relaunched Core line is currently an unreleased preview built from main.

## Prerequisites

- Go 1.26 or later
- GNU Make
- An MCP client that can start a local stdio server

The first run uses `cmd/evidra-fixture`, which is part of this repository. It
keeps the setup reproducible and does not require credentials or an external
service.

## Build from source

From the repository root:

```bash
make build
go build -o bin/evidra-fixture ./cmd/evidra-fixture
```

This produces the read-side CLI at `bin/evidra`, the endpoint at
`bin/evidra-mcp`, and the test upstream at `bin/evidra-fixture`.

## Wrap the included fixture

Create a recorder root, then start the merged endpoint:

```bash
mkdir -p evidence

./bin/evidra-mcp \
  --proxy \
  --evidence-dir ./evidence \
  --server-name fixture \
  -- \
  ./bin/evidra-fixture
```

The process speaks newline-delimited MCP JSON-RPC over stdin and stdout. It is
not a shell prompt or interactive demo; an MCP client normally starts and owns
this process.

`./evidence` is the **recorder root**. Every endpoint process creates one dated
**recorder directory** below it. A recorder directory contains that process's
metadata, event chain, signing key, and digest key. Read-side commands consume
the root and inspect its immediate recorder directories.

## Configure an MCP client

Use absolute paths because most MCP clients do not start servers in the
repository directory. Replace every `/absolute/path/to/evidra` placeholder with
the absolute checkout path:

```json
{
  "mcpServers": {
    "evidra-fixture": {
      "command": "/absolute/path/to/evidra/bin/evidra-mcp",
      "args": [
        "--proxy",
        "--evidence-dir",
        "/absolute/path/to/evidra/evidence",
        "--server-name",
        "fixture",
        "--",
        "/absolute/path/to/evidra/bin/evidra-fixture"
      ],
      "env": {
        "EVIDRA_ACTOR_ID": "local-evaluator"
      }
    }
  }
}
```

The argument order matters: Evidra flags precede `--`; the upstream executable
and all of its arguments follow it.

## Complete one operation

After the client connects, its tool list contains the fixture tools plus two
local Evidra tools. Complete this sequence through the MCP client:

1. Call `evidra_prescribe` with an objective such as `Read the fixture service
   status` and expected outcome `The fixture reports status ok`.
2. Save the returned `operation_id`.
3. Call the upstream `get_status` tool with `{}`.
4. Call `evidra_report` once with that `operation_id`, a status from
   `completed`, `failed`, `cancelled`, or `abandoned`, and an outcome from
   `achieved`, `not_achieved`, or `unknown`.

`evidra_prescribe` records the agent's intended work before operational calls.
`evidra_report` closes that operation with the agent's account of what happened.
The report response also contains narrow counts observed by the proxy; those
counts are not a verdict about the external outcome.

Only one operation can be open in an endpoint process. A second prescription
must either continue the current operation or explicitly abandon and replace it.

## Verify and summarize

After the MCP client stops the endpoint, read the recorder root:

```bash
./bin/evidra verify --dir ./evidence
./bin/evidra summarize --dir ./evidence
```

`verify` reports chain validity, signature validity, and known evidence coverage
separately for each recorder directory. `summarize` writes
`./evidence/summary.json` and prints the declared, observed, and reported layers.

A successful upstream response is not proof of the external outcome. In this
first run the included fixture supplies a deterministic result; with a real
upstream, Evidra only knows what crossed its MCP boundary.

## Wrap a real upstream

Replace the fixture command after `--` with an upstream stdio MCP server and its
arguments:

```bash
./bin/evidra-mcp \
  --proxy \
  --evidence-dir /absolute/path/to/evidence \
  --server-name my-upstream \
  -- \
  /absolute/path/to/upstream --upstream-option value
```

Review that command before using it. Evidra launches the child with the current
process's working directory, environment, and operating-system authority. It is
an observation and protocol-enforcement boundary, not a sandbox.

## Configuration

The most important endpoint settings are:

| Setting | Effect |
|---|---|
| `--enforce all` | Default. Refuse upstream tool calls when no operation is open. |
| `--enforce off` | Observe without protocol refusals. Recording still occurs when `--evidence-dir` is present. |
| `--evidence-dir <root>` | Enable writes and create one recorder directory for this process. There is no write-side default. |
| `--actor-id <id>` | Attribute events to an actor ID. Defaults to `EVIDRA_ACTOR_ID`; if neither is set, the actor ID remains unset. |
| `--server-name <label>` | Label the wrapped upstream in metadata and client-facing server information. |
| `--max-message <size>` | Bound one JSON-RPC frame; default `64MiB`. |
| `--advertise-passthrough` | Advertise relayed prompt, resource, and completion capabilities outside the tested profile. |

`EVIDRA_EVIDENCE_DIR` is read only by `evidra summarize` and `evidra verify` as
their default input root. The endpoint never uses it for writing; pass
`--evidence-dir` explicitly.

## Troubleshooting

**The endpoint says “nothing to wrap.”** Include `--proxy`, then `--`, then a
real upstream executable.

**The endpoint starts but no evidence remains.** Pass `--evidence-dir`. A process
that records only start/stop lifecycle noise removes its empty recorder directory.

**No recorder directories are found.** Point `--dir` at the recorder root, not
at `events.jsonl` or another file. Confirm that at least one substantive operation
was recorded.

**The endpoint refuses to start because of a reserved tool.** Upstreams may not
advertise `evidra_prescribe` or `evidra_report`; Evidra will not rename either
side of that collision.

**An upstream tool call is refused.** With `--enforce all`, call
`evidra_prescribe` first. If the response reports `recorder_unhealthy`, recover
the evidence storage before retrying the operational call.

## Next steps

- Read the complete [CLI reference](cli-reference.md).
- Review the runtime boundaries in [Architecture](architecture.md).
- Understand keys, fingerprints, and limitations in the
  [evidence trust model](evidence-format.md).
- Check what has and has not been proven in [Validation status](validation.md).
