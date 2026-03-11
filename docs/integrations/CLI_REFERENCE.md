# Evidra CLI Reference

This document is the user-facing command and flag reference for all binaries in this repository:
- `evidra` (core CLI)
- `evidra-mcp` (MCP server)

For architecture and protocol semantics, see:
- [Integration Protocol](../system-design/EVIDRA_PROTOCOL.md)
- [Benchmark CLI Design](../system-design/internal/EVIDRA_BENCHMARK_CLI.md)
- [Prompt Factory Spec](../system-design/internal/EVIDRA_PROMPT_FACTORY_SPEC.md)

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
| `--scanner-report` | SARIF report path for finding ingestion |
| `--evidence-dir` | Evidence directory override |
| `--actor` | Actor ID |
| `--canonical-action` | Pre-canonicalized JSON action (bypasses adapter) |
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

### `evidra record` Flags

`record` requires `--` before the wrapped command:

```bash
evidra record -f deploy.yaml -- kubectl apply -f deploy.yaml
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

`evidra record` and `evidra import` return the same immediate assessment fields:

- `risk_level`
- `score`
- `score_band`
- `signal_summary`
- `basis`
- `confidence`

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
| `prompts generate` | `--contract` (default `v1.0.1`), `--root` (default `.`), `--write-active` (default `true`), `--write-generated` (default `true`), `--write-manifest` (default `true`) |
| `prompts verify` | `--contract` (default `v1.0.1`), `--root` (default `.`) |

### Developer Commands

These commands are functional but not yet part of the stable public API.

#### `evidra detectors list`

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

`prescribe`, `report`, `get_event`

## 3) `evidra-exp` (experiments)

See also [Experiments README](../../experiments/README.md) for run modes and output schema.
For execution flow and result interpretation, see [Artifact Runner Guide](../experimental/ARTIFACT_RUNNER_GUIDE.md).

### Top-Level Commands

| Command | Purpose |
|---|---|
| `artifact run` | Artifact-only classification experiments |
| `artifact baseline` | Multi-model artifact baseline with aggregate summary |
| `execution run` | Execution-mode experiments (MCP + command outcome) |
| `version` | Print version |
| `help` | Print usage |

### `evidra-exp artifact run` Flags

| Flag | Description |
|---|---|
| `--model-id` | Required model ID |
| `--provider` | Provider label (`unknown` default) |
| `--prompt-version` | Prompt version label |
| `--prompt-file` | Prompt file path (default `prompts/experiments/runtime/system_instructions.txt`) |
| `--temperature` | Temperature override |
| `--mode` | Execution mode label (`custom` default) |
| `--repeats` | Repeats per case (`3` default) |
| `--timeout-seconds` | Per-run timeout in seconds (`300` default) |
| `--case-filter` | Regex filter for case IDs |
| `--max-cases` | Maximum number of selected cases |
| `--cases-dir` | Cases directory (default `tests/benchmark/cases`) |
| `--out-dir` | Output directory (default timestamp under `experiments/results`) |
| `--clean-out-dir` | Clear output directory before run |
| `--delay-between-runs` | Sleep between runs (duration, for example `2s`, `500ms`) |
| `--agent` | Adapter: `claude`, `bifrost`, `dry-run` |
| `--dry-run` | Skip real adapter execution |

### `evidra-exp artifact baseline` Flags

| Flag | Description |
|---|---|
| `--model-ids` | Required comma-separated model IDs |
| `--provider` | Provider label (`unknown` default) |
| `--prompt-version` | Prompt version label |
| `--prompt-file` | Prompt file path (default `prompts/experiments/runtime/system_instructions.txt`) |
| `--temperature` | Temperature override |
| `--mode` | Execution mode label (`custom` default) |
| `--repeats` | Repeats per case (`3` default) |
| `--timeout-seconds` | Per-run timeout in seconds (`300` default) |
| `--case-filter` | Regex filter for case IDs |
| `--max-cases` | Maximum number of selected cases |
| `--cases-dir` | Cases directory (default `tests/benchmark/cases`) |
| `--out-dir` | Output directory (default timestamp under `experiments/results/llm`) |
| `--clean-out-dir` | Clear output directory before run |
| `--delay-between-runs` | Sleep between runs (duration, for example `2s`, `500ms`) |
| `--agent` | Adapter: `claude`, `bifrost`, `dry-run` |
| `--dry-run` | Skip real adapter execution |

### `evidra-exp execution run` Flags

| Flag | Description |
|---|---|
| `--model-id` | Required model ID |
| `--provider` | Provider label (`unknown` default) |
| `--prompt-version` | Prompt version label |
| `--prompt-file` | Prompt file path (default `prompts/experiments/runtime/system_instructions.txt`) |
| `--scenarios-dir` | Scenario directory (default `tests/experiments/execution-scenarios`) |
| `--mode` | Execution mode label (`local-mcp` default) |
| `--repeats` | Repeats per scenario (`1` default) |
| `--timeout-seconds` | Per-run timeout in seconds (`600` default) |
| `--scenario-filter` | Regex filter for scenario IDs |
| `--max-scenarios` | Maximum selected scenarios |
| `--out-dir` | Output directory (default timestamp under `experiments/results`) |
| `--clean-out-dir` | Clear output directory before run |
| `--delay-between-runs` | Sleep between runs (duration, for example `2s`, `500ms`) |
| `--agent` | Adapter: `mcp-kubectl`, `dry-run` |
| `--dry-run` | Skip real adapter execution |
