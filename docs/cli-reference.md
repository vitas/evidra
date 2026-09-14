# CLI Reference

Evidra builds two user-facing binaries. `evidra-mcp` launches and wraps one MCP
upstream. `evidra` reads recorder roots produced by endpoint processes.

## evidra-mcp

### Usage

```text
evidra-mcp --proxy [flags] -- <upstream-command> [args...]
evidra-mcp --version
evidra-mcp --help
```

The endpoint initializes one child stdio MCP server, merges
`evidra_prescribe` and `evidra_report` into its tool list, and relays the
supported MCP profile.

### Flags

| Flag | Meaning |
|---|---|
| `--proxy` | Select the merged endpoint mode. There is no standalone server mode. |
| `--evidence-dir <root>` | Enable recording under this root. One recorder directory is created per process. If omitted, protocol enforcement remains active but no evidence is written. |
| `--server-name <label>` | Set the wrapper's upstream label in evidence and client-facing server information. If omitted, the runtime display label is derived from the executable name while evidence `server_name` remains empty. |
| `--enforce <all\|off>` | `all` is the default and refuses upstream calls outside an open operation. `off` observes calls without those refusals. |
| `--advertise-passthrough` | Advertise relayed prompt, resource, and completion capabilities that are outside the tested default profile. |
| `--max-message <size>` | Set the largest JSON-RPC frame accepted in either direction. Default: `64MiB`. Plain bytes and `k`, `kb`, `KiB`, `m`, `mb`, `MiB`, `g`, `gb`, and `GiB` spellings are accepted. |
| `--actor-id <id>` | Set the actor ID on operation, execution, and violation events. The default is `EVIDRA_ACTOR_ID`; without either value the actor ID is empty. Recorder lifecycle events have no actor. |
| `--version` | Print build information and exit. |
| `--help` | Print endpoint usage and exit. |

All endpoint flags must precede `--`. Everything after `--` is passed as the
upstream command and arguments.

### Exit behavior

`evidra-mcp` exits `0` after help, version output, or a clean endpoint shutdown.
It exits `2` for command-line flag parsing errors or when no wrapping mode is
selected. It exits `1` when proxy setup or runtime fails, including a missing
upstream command, invalid message size, invalid enforcement mode, evidence-store
startup failure, upstream startup failure, or endpoint transport failure.

Operational protocol refusals are normally returned as MCP tool results with
`isError: true`; they do not by themselves terminate the endpoint process.

## evidra

With no arguments or with `help`, `--help`, or `-h`, `evidra` prints its command
list. Unknown commands exit `2`.

### summarize

```text
evidra summarize --dir <recorder-root> [--since <duration-or-time>]
```

Reads the recorder directories immediately below the root, prints reconciliation
findings, and writes `<recorder-root>/summary.json` with mode/upstream/digest-key
comparison domains kept separate.

| Flag | Meaning |
|---|---|
| `--dir <root>` | Recorder root to read. Defaults to `EVIDRA_EVIDENCE_DIR`. Required if that variable is unset. |
| `--since <value>` | Include events at or after a relative duration such as `7d` or `36h`, or an RFC 3339 timestamp. |

The command exits `0` after writing a summary, `2` for invalid flags, an invalid
`--since`, or a missing input root, and `1` for read, reconciliation, encoding,
or output-write failures.

### verify

```text
evidra verify --dir <recorder-root> [--since <duration-or-time>]
```

Reports record counts, sequence range, chain validity, signature validity,
coverage, enforcement mode, upstream ID, digest-key ID, and findings per recorder.

It exits `0` when at least one recorder has records in the requested view and all
reported chain/signature checks pass. It exits `2` for invalid flags, an invalid
`--since`, or a missing input root. It exits `1` for unreadable stores, failed
cryptographic checks, no recorder directories, or a view containing no records.

For a whole-store integrity decision, omit `--since`. A time-filtered invocation
does not establish the integrity of records before its boundary.

### version

```text
evidra version
evidra --version
```

Prints build information and exits `0`.

## Environment variables

| Variable | Consumer | Meaning |
|---|---|---|
| `EVIDRA_ACTOR_ID` | `evidra-mcp` | Default value for `--actor-id`. |
| `EVIDRA_EVIDENCE_DIR` | `evidra summarize`, `evidra verify` | Default read-side recorder root when `--dir` is omitted. |

There is no environment-variable default for write-side storage. Recording starts
only when `evidra-mcp` receives `--evidence-dir`.

## Trust boundary

Evidra does not sandbox the wrapped command. The upstream command is launched
with the same operating-system authority available to `evidra-mcp`, using the
current working directory and inherited environment. Choosing an upstream has
the same execution trust implications as launching that command directly.

The endpoint enforces prescription order, not command safety, resource policy,
or semantic risk. Chain validity, signature validity, and evidence coverage are
separate conclusions. A successful upstream response is not proof of the
external outcome: it only shows what the proxy received at the MCP boundary.
