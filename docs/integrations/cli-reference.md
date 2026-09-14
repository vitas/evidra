# CLI reference

- Status: Reference
- Version: current (vNext)
- Spec: [`docs/system-design/vnext-mcp-recorder.md`](../system-design/vnext-mcp-recorder.md) §7, §20, §41

Evidra ships two binaries, and both are listed here. Anything not in this file does not exist
in this build. The pre-vNext command surface (`prescribe`, `report`, `record`, `import`,
`export`, `scorecard`, `explain`, `compare`, `validate`, `detectors`, `prompts`, `skill`,
`keygen`, `import-findings`) and the self-hosted API server were removed by the
[§43 prune](../system-design/vnext-prune-record.md); `scripts/check-doc-commands.sh` runs the
commands documented below, so a document that outlives its command fails CI.

## `evidra-mcp` — the endpoint that enforces and records

```bash
evidra-mcp --proxy [flags] -- <upstream-command> [args...]
```

One process wraps one upstream stdio MCP server. It merges two local tools into the upstream
tool list — `evidra_prescribe` (open an operation) and `evidra_report` (close it) — refuses
`tools/call` while no operation is open under `--enforce=all`, and appends a signed event
chain to its recorder directory.

```bash
evidra-mcp --proxy --evidence-dir /tmp/evidence -- ./bin/evidra-fixture
```

| Flag | Meaning |
|---|---|
| `--proxy` | Wrap the upstream command as the merged endpoint. Required. |
| `--enforce <all\|off>` | `all` (default): refuse calls outside an open operation. `off`: observe and record, never refuse. Protocol order only — argument content, tool names and annotations never gate (§7). |
| `--evidence-dir <dir>` | Recorder root; one dated directory per recorder process (§20). Omit it to enforce without writing evidence. |
| `--server-name <label>` | `serverInfo.name` and the label stored in evidence. |
| `--advertise-passthrough` | Advertise relayed prompts/resources/completions that sit outside the supported profile. Not advertised by default (§44). |
| `--max-message <size>` | Largest single JSON-RPC message accepted in either direction (default 64 MiB). |
| `--actor-id <id>` | Actor recorded as accountable on substantive events. Default `$EVIDRA_ACTOR_ID`, else the recorder's fallback identity. Lifecycle events stay attributed to the recorder. |
| `--version`, `--help` | Version / usage. |

**Evidra does not sandbox the wrapped command.** `-- <upstream-command>` is executed as a
child of the recorder with the recorder's own privileges, environment and working directory:
the endpoint observes and records what the upstream does, it does not confine it. Treat the
upstream command line as part of the trust boundary: whoever can choose it can run anything
the recorder itself could run, with the same trust model as direct shell execution.

There is **no default evidence location for the endpoint.** Recording is a per-process choice
made with `--evidence-dir`, so a test or dogfood run cannot silently append to a home
directory nothing pointed at; the endpoint says so on stderr when it runs without a store.

Arguments leave the process only as `HMAC(digest_key, JCS(args))`; results are hashed up to
4 MiB and larger ones become `omitted_oversize` (§23, §24). Raw upstream payloads and key
material never enter the chain or `summary.json`.

## `evidra` — the read side

```
COMMANDS:
  summarize    Reconcile vNext MCP evidence: declared vs observed vs reported
  verify       Verify vNext evidence chains, signatures and coverage per recorder
  version      Print version information
```

`summarize` and `verify` take the same two flags:

| Flag | Meaning |
|---|---|
| `-dir <dir>` | Evidence root holding recorder directories, or one recorder directory. Default: `$EVIDRA_EVIDENCE_DIR`. |
| `-since <instant>` | Only events recorded after this instant: `7d`, `36h`, or RFC 3339. |

`EVIDRA_EVIDENCE_DIR` applies to the read side only — never to the endpoint.

```bash
evidra summarize --dir /tmp/evidence      # declared / observed / reported per operation
evidra verify --dir /tmp/evidence --since 7d
evidra version
```

`summarize` prints, per operation, what the agent **declared**, what the proxy **observed**
through the wrapped boundary, and what the agent **reported**, then the reconciliation stance.
It does not decide whether a claim is true; it puts the layers side by side so an unsupported
one is inspectable. `verify` reports chain validity, signature validity and coverage as three
separate statements — a valid chain can still be incomplete (§34).

A fourth `evidra` command is a plan change, not a registry entry (§41).
