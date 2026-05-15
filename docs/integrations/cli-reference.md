# Evidra CLI Reference

- Status: Reference
- Version: current
- Canonical for: CLI commands and flags
- Audience: public

This document is the user-facing command and flag reference for all binaries in this repository:
- `evidra` (core CLI)
- `evidra-mcp` (MCP server)

For architecture and protocol semantics, see:
- [Integration Protocol](../system-design/EVIDRA_PROTOCOL_V1.md)
- [Record/Import Contract](../contracts/EVIDRA_RUN_RECORD_CONTRACT_V1.md)
- [Core Data Model](../system-design/EVIDRA_CORE_DATA_MODEL_V1.md)

## 1) `evidra` (core CLI)

### Command Groups

| Command | Purpose |
|---|---|
| `scorecard` | Generate reliability scorecard for an actor/session/window |
| `explain` | Show signal-level explanation for scorecard |
| `compare` | Compare actors and workload overlap |
| `record` | Execute wrapped command and record lifecycle outcome |
| `import` | Ingest completed operation from structured JSON input |
| `prescribe` | Record pre-execution intent/risk |
| `report` | Record post-execution outcome |
| `validate` | Validate evidence chain/signatures |
| `import-findings` | Ingest SARIF findings as evidence entries |
| `prompts` | Prompt artifact generation/verification |
| `keygen` | Generate Ed25519 keypair |
| `skill` | Install Evidra skill for AI agent protocol compliance |
| `version` | Print version |

### `evidra scorecard` Flags

| Flag | Description |
|---|---|
| `--actor` | Actor ID filter |
| `--period` | Time period filter (`30d` default) |
| `--evidence-dir` | Evidence directory override |
| `--ttl` | TTL for unreported prescription detection (`10m0s` default) |
| `--tool` | Tool filter |
| `--scope` | Scope-class filter |
| `--session-id` | Session ID filter |
| `--min-operations` | Override score sufficiency threshold |
| `--pretty` | Render human-readable ASCII output instead of JSON |

`scorecard` JSON output includes `days_observed`, which is the number of distinct UTC calendar days with matching prescription activity inside the selected window.

### `evidra explain` Flags

| Flag | Description |
|---|---|
| `--actor` | Actor ID filter |
| `--period` | Time period filter (`30d` default) |
| `--evidence-dir` | Evidence directory override |
| `--ttl` | TTL for unreported prescription detection (`10m0s` default) |
| `--tool` | Tool filter |
| `--scope` | Scope-class filter |
| `--session-id` | Session ID filter |

### `evidra compare` Flags

| Flag | Description |
|---|---|
| `--actors` | Comma-separated actor IDs (required for meaningful output; expects at least 2) |
| `--period` | Time period filter (`30d` default) |
| `--evidence-dir` | Evidence directory override |
| `--tool` | Tool filter |
| `--scope` | Scope-class filter |
| `--session-id` | Session ID filter |

### `evidra prescribe` Flags

| Flag | Description |
|---|---|
| `-f`, `--artifact` | Artifact file path (YAML/JSON) |
| `--tool` | Tool name (for example `kubectl`, `terraform`) |
| `--operation` | Operation name (`apply` default) |
| `--environment` | Environment label |
| `--findings` | SARIF findings path (repeatable); writes finding evidence, does not alter risk without an explicit assessment |
| `--evidence-dir` | Evidence directory override |
| `--actor` | Actor ID |
| `--canonical-action` | Optional external canonical identity JSON; Evidra records it but does not run adapters or assessment |
| `--session-id` | Session boundary ID (generated if omitted) |
| `--operation-id` | Operation identifier |
| `--attempt` | Retry attempt counter |
| `--signing-key` | Base64 Ed25519 private key |
| `--signing-key-path` | PEM Ed25519 private key path |
| `--signing-mode` | `strict` (default) or `optional` |
| `--url` | Evidra API URL for evidence forwarding |
| `--api-key` | API key for online mode |
| `--offline` | Force offline mode |
| `--fallback-offline` | Fall back to offline mode on API failure |
| `--timeout` | API request timeout |

### `evidra report` Flags

| Flag | Description |
|---|---|
| `--prescription` | Prescription event ID |
| `--verdict` | Required terminal verdict: `success`, `failure`, `error`, or `declined` |
| `--exit-code` | Command exit code (required for `success`/`failure`/`error`, forbidden for `declined`) |
| `--decline-trigger` | Required trigger string for `--verdict declined` |
| `--decline-reason` | Required short operational reason for `--verdict declined` |
| `--evidence-dir` | Evidence directory override |
| `--actor` | Actor ID |
| `--artifact-digest` | Artifact digest for correlation |
| `--external-refs` | External references JSON array |
| `--session-id` | Session boundary ID |
| `--operation-id` | Operation identifier |
| `--signing-key` | Base64 Ed25519 private key |
| `--signing-key-path` | PEM Ed25519 private key path |
| `--signing-mode` | `strict` (default) or `optional` |
| `--url` | Evidra API URL for evidence forwarding |
| `--api-key` | API key for online mode |
| `--offline` | Force offline mode |
| `--fallback-offline` | Fall back to offline mode on API failure |
| `--timeout` | API request timeout |

### `evidra record` Flags

`record` requires `--` before the wrapped command:

```bash
evidra record -f deploy.yaml -- kubectl apply -f deploy.yaml
```

Expanded form is equivalent when you want to attach more metadata:

```bash
evidra record \
  -f deploy.yaml \
  --environment staging \
  --actor ci-gha \
  -- kubectl apply -f deploy.yaml
```

Security boundary: Evidra does not sandbox the wrapped command. Treat it with
the same trust model as direct shell execution. Evidra records and analyzes
evidence around the command; it does not contain or block it.

| Flag | Description |
|---|---|
| `-f`, `--artifact` | Artifact file path (YAML/JSON) |
| `--tool` | Tool name override (optional when inferred from wrapped command) |
| `--operation` | Operation override (optional when inferred from wrapped command) |
| `--environment` | Environment label |
| `--findings` | SARIF findings path (repeatable) |
| `--evidence-dir` | Evidence directory override |
| `--actor` | Actor ID |
| `--canonical-action` | Pre-canonicalized JSON action |
| `--session-id` | Session boundary ID (generated if omitted) |
| `--operation-id` | Operation identifier |
| `--attempt` | Retry attempt counter |
| `--signing-key` | Base64 Ed25519 private key |
| `--signing-key-path` | PEM Ed25519 private key path |
| `--signing-mode` | `strict` (default) or `optional` |

`record` infers `tool` from the wrapped command's first word for `kubectl`, `oc`, `helm`, `terraform`, `docker`, `argocd`, `kustomize`, and `pulumi`. It infers `operation` only from supported command patterns. Shell wrappers such as `sh -c` require explicit `--tool` and `--operation`.

### `evidra import` Flags

| Flag | Description |
|---|---|
| `--input` | Path to import JSON file (`-` for stdin) |
| `--evidence-dir` | Evidence directory override |
| `--signing-key` | Base64 Ed25519 private key |
| `--signing-key-path` | PEM Ed25519 private key path |
| `--signing-mode` | `strict` (default) or `optional` |
| `--url` | Evidra API URL for evidence forwarding |
| `--api-key` | API key for online mode |
| `--offline` | Force offline mode |
| `--fallback-offline` | Fall back to offline mode on API failure |
| `--timeout` | API request timeout |

### Assessment Snapshot Output

`evidra record` and `evidra import` return the same immediate analytics fields:

- `risk_inputs`
- `effective_risk`
- `score`
- `score_band`
- `signal_summary`
- `basis`
- `confidence`

`risk_inputs` and `effective_risk` are empty unless an external assessment is
provided by the caller. Score and signal fields are still computed from the
evidence chain.

The legacy score-band alias is not part of the v1 output contract.

`evidra report` returns an immediate session assessment snapshot:

- `prescription_id`
- `verdict`
- `exit_code`
- `decision_context` (when `verdict=declined`)
- `score`
- `score_band`
- `signal_summary`
- `basis`
- `confidence`

### `evidra validate` Flags

| Flag | Description |
|---|---|
| `--evidence-dir` | Evidence directory override |
| `--public-key` | Ed25519 public key PEM (enables signature verification) |

### `evidra import-findings` Flags

| Flag | Description |
|---|---|
| `--sarif` | SARIF report path |
| `--artifact` | Artifact path used for digest linking |
| `--tool-version` | Tool version override for all ingested findings |
| `--evidence-dir` | Evidence directory override |
| `--actor` | Actor ID |
| `--session-id` | Session boundary ID |
| `--signing-key` | Base64 Ed25519 private key |
| `--signing-key-path` | PEM Ed25519 private key path |
| `--signing-mode` | `strict` (default) or `optional` |

### `evidra prompts` Subcommands and Flags

| Subcommand | Flags |
|---|---|
| `prompts generate` | `--contract` (default `v1.3.0`), `--root` (default `.`), `--write-active` (default `true`), `--write-generated` (default `true`), `--write-manifest` (default `true`) |
| `prompts verify` | `--contract` (default `v1.3.0`), `--root` (default `.`) |

### `evidra skill install` Flags

| Flag | Description |
|---|---|
| `--target` | Target platform: `claude` (default: `claude`) |
| `--scope` | Installation scope: `global` (default) or `project` |
| `--project-dir` | Project directory for `--scope project` (default: `.`) |
| `--full-prescribe` | Install the full-prescribe skill variant |

Global installs to `~/.claude/skills/evidra/SKILL.md`. Project installs to `.claude/skills/evidra/SKILL.md` in the specified directory.

### Developer Commands

These commands are functional but not yet part of the stable public API.

#### `evidra detectors list`

Legacy/companion surface for the built-in detector registry. Core
prescribe/report does not depend on these detectors.

| Flag | Description |
|---|---|
| `--stable-only` | Show only stable (non-experimental) detectors |

Output: JSON with `count` and `items` array of detector metadata (tag, description, severity, stability).

## 2) `evidra-mcp` (MCP server)

### Flags

| Flag | Description |
|---|---|
| `--evidence-dir` | Evidence chain storage path |
| `--environment` | Environment label |
| `--retry-tracker` | Enable retry-loop tracking |
| `--signing-mode` | `strict` (default) or `optional` |
| `--version` | Print version and exit |
| `--help` | Print help and exit |

### Environment Variables

| Variable | Description |
|---|---|
| `EVIDRA_EVIDENCE_DIR` | Default evidence directory |
| `EVIDRA_ENVIRONMENT` | Default environment label |
| `EVIDRA_RETRY_TRACKER` | Retry tracker toggle (`true/false`) |
| `EVIDRA_EVIDENCE_WRITE_MODE` | Evidence write mode (`strict` or `best_effort`) |
| `EVIDRA_SIGNING_MODE` | Signing mode (`strict` or `optional`) |
| `EVIDRA_SIGNING_KEY` | Base64 Ed25519 private key |
| `EVIDRA_SIGNING_KEY_PATH` | PEM Ed25519 private key path |

### MCP Tools

`prescribe_full`, `prescribe_smart`, `report`, `get_event`
