# Evidra

[![CI](https://github.com/vitas/evidra/actions/workflows/ci.yml/badge.svg?branch=main)](https://github.com/vitas/evidra/actions/workflows/ci.yml)
[![Release Pipeline](https://github.com/vitas/evidra/actions/workflows/release.yml/badge.svg?event=push)](https://github.com/vitas/evidra/actions/workflows/release.yml)
[![License](https://img.shields.io/badge/License-Apache_2.0-blue.svg)](https://opensource.org/licenses/Apache-2.0)

**Flight recorder and reliability scoring for infrastructure automation**

Evidra records intent, outcome, and refusal for every infrastructure mutation — across MCP agents, CI pipelines, A2A agents, and scripts. The append-only evidence chain enables risk assessment, behavioral signal detection, and reliability scoring.

CLI and MCP are the authoritative analytics surfaces today.

**Two ways to use it:**

| | What | How |
|---|---|---|
| **DevOps MCP Server** | All-in-one: kubectl/helm/terraform/aws with smart output + auto-evidence | `evidra-mcp` as your agent's MCP server |
| **Flight Recorder** | Add evidence to any existing workflow — no MCP required | `evidra record`, `evidra import`, webhooks, or proxy mode |

## Quick Start — MCP Server

```json
{
  "mcpServers": {
    "evidra": {
      "command": "evidra-mcp",
      "args": ["--evidence-dir", "~/.evidra/evidence"]
    }
  }
}
```

Your agent gets seven default DevOps tools: `run_command`, `collect_diagnostics`, `write_file`, `describe_tool`, `prescribe_smart`, `report`, and `get_event`. The normal path is still `run_command` with automatic evidence recording for mutations. Use `describe_tool` only when you want the full explicit-control schema for `prescribe_smart` or `report`. Add `--full-prescribe` when you also want artifact-aware `prescribe_full`.

## Quick Start — CLI (No MCP)

```bash
# Wrap any command — evidence recorded automatically
evidra record -f deploy.yaml -- kubectl apply -f deploy.yaml

# Import from CI pipelines
evidra import --input record.json

# View reliability scorecard
evidra scorecard --period 30d
```

Works with any agent framework, CI system, or script. No MCP required.

Security boundary: Evidra does not sandbox the wrapped command. Treat it with the same trust model as direct shell execution.

```bash
# Install
brew install samebits/tap/evidra
```

## What Your Agent Gets

### Smart output — fewer tokens, same information

```
Agent: run_command("kubectl get deployment web -n bench")

# Without evidra-mcp (raw JSON): ~2,400 tokens
{"apiVersion":"apps/v1","metadata":{"managedFields":[...],...},"spec":{...},"status":{...}}

# With evidra-mcp (smart output): ~40 tokens
deployment/web (bench): 0/2 ready | image: nginx:99.99 | Available=False
```

### Auto-evidence for mutations — zero agent code

```
Agent: run_command("kubectl apply -f fix.yaml")
  → evidra auto-prescribes (intent recorded)
  → kubectl executes
  → evidra auto-reports (outcome recorded)
  → smart output returned to agent
```

Read-only commands (`get`, `describe`, `logs`) execute directly — no overhead.

### Skills — tested on real infrastructure

Install the [Evidra skill](docs/guides/skill-setup.md) to give your agent
operational discipline: diagnosis before fix, safety boundaries, domain-specific
patterns. Skills are tested on 62 real scenarios via [infra-bench](https://lab.evidra.cc)
before shipping — skills that hurt performance don't ship.

### 7 default tools, plus optional Full Prescribe

| Tool | Description |
|---|---|
| `run_command` | Execute kubectl, helm, terraform, aws — with smart output |
| `collect_diagnostics` | Gather pods, describe output, events, and recent logs for one workload |
| `write_file` | Write config or manifest files under the current workspace or temp directories |
| `describe_tool` | Show the full schema for deferred protocol tools when you want explicit control |
| `prescribe_smart` | Smart Prescribe with deferred schema loading; use `describe_tool` first when needed |
| `report` | Record outcome; full explicit schema available via `describe_tool` |
| `get_event` | Look up evidence |

Enable `--full-prescribe` to add **Full Prescribe** when your agent has artifact bytes and you want artifact-aware explicit intent capture.

Most agents only need `run_command`. Use `collect_diagnostics` when the model would otherwise spend multiple turns on `get` / `describe` / `events` / `logs`. Use `write_file` for agent-authored manifests or Terraform snippets without leaving the MCP surface. Use `describe_tool` only when you deliberately want the explicit `prescribe_smart` / `report` flow instead of the default auto-evidence path.

## Why Not Just kubectl-mcp-server?

| | kubectl-mcp-server | evidra-mcp |
|---|---|---|
| Tools | 270 specialized | 7 default tools + optional Full Prescribe |
| Output | Raw JSON (~2400 tokens) | Smart summary (~40 tokens) |
| Evidence | None | Auto prescribe/report for mutations |
| Security | Open | Command allowlist + blocked subcommands |
| Skills | None | Bench-tested, installable |
| Scoring | None | Reliability scorecards + behavioral signals |

## For Platform Teams

### Self-hosted analytics

```bash
docker compose up --build -d
```

Centralize evidence across agents, pipelines, and controllers:
- Which agents retry the same operation?
- Which scenarios cause the most failures?
- How does model X compare to model Y on real infrastructure?

### CI/CD integration

```bash
# Wrap any command — CLI records prescribe/execute/report
evidra record -f deploy.yaml -- kubectl apply -f deploy.yaml

# Import completed operations
evidra import --input record.json

# View reliability scorecard
evidra scorecard --period 30d
```

References: [Self-hosted setup](docs/guides/self-hosted-setup.md) · [CLI reference](docs/integrations/cli-reference.md) · [API reference](docs/api-reference.md)

## For Agent Benchmarking

Test which skills and tools actually improve your agent. 62 real scenarios
on real Kubernetes clusters.

```bash
# Baseline — no skill
infra-bench certify --track cka --model sonnet --provider bifrost

# With role skill
infra-bench certify --track cka --model sonnet --role k8s-admin

# Result: skills help L1 (75% fewer turns) but break L2 diagnosis
```

Bench repo: [evidra-infra-bench](https://github.com/vitas/evidra-infra-bench) |
Dashboard: [lab.evidra.cc/bench](https://lab.evidra.cc/bench)

## Intelligence Layer

From the evidence chain, Evidra computes:

- **Risk assessment** — pluggable pipeline with multiple assessors
- **Behavioral signals** — protocol violations, retry loops, blast radius, drift detection
- **Reliability scorecards** — 0-100 score with band and confidence

Eight behavioral signals documented in the [Signal specification](docs/system-design/EVIDRA_SIGNAL_SPEC_V1.md).

## Explicit Protocol (Advanced)

For agents that want full control over evidence recording:

```text
prescribe_smart / prescribe_full  →  canonicalize artifact → assess risk → record intent
execute    →  run the command (or decline to act)
report     →  record verdict, exit code, or refusal reason
```

Three evidence modes:

| Mode | How | Agent awareness |
|---|---|---|
| **Proxy Observed** | Auto prescribe/report via observed mutation-style tool calls | None needed |
| **Smart Prescribe** | Agent calls `prescribe_smart` + `report` | Minimal (~30 tokens) |
| **Full Prescribe** | Agent calls `prescribe_full` with artifact | Full artifact (~300 tokens) |

Most users should use Proxy Observed or the default DevOps surface. Smart Prescribe and Full Prescribe are for teams
that want agents to see risk assessments before executing.

## Proxy Mode — Wrap Mutation-Oriented MCP Servers

Add evidence to an existing MCP server — zero agent changes:

```json
{
  "mcpServers": {
    "infra": {
      "command": "evidra-mcp",
      "args": ["--proxy", "--", "npx", "-y", "@anthropic/mcp-server-kubernetes"]
    }
  }
}
```

The proxy records evidence when it sees `run_command` or other mutation-shaped MCP tool calls it can classify heuristically. Unclassified or read-only tool calls pass through without evidence.

## Docs

- [MCP Setup Guide](docs/guides/mcp-setup.md)
- [Skill Setup Guide](docs/guides/skill-setup.md)
- [CLI Reference](docs/integrations/cli-reference.md)
- [API Reference](docs/api-reference.md)
- [Architecture](docs/system-design/EVIDRA_ARCHITECTURE_V1.md)
- [Protocol Specification](docs/system-design/EVIDRA_PROTOCOL_V1.md)
- [Supported Tools](docs/supported-tools.md)

## Development

```bash
make build
make test
make lint
make test-mcp-inspector    # MCP protocol compliance tests
```

### Environment Variables

| Variable | Description |
|---|---|
| `EVIDRA_EVIDENCE_DIR` | Evidence storage path (default: `~/.evidra/evidence`) |
| `EVIDRA_SIGNING_MODE` | `strict` (default) or `optional` (dev mode) |
| `EVIDRA_SIGNING_KEY` | Base64 Ed25519 signing key |
| `EVIDRA_ENVIRONMENT` | Environment label (production, staging) |

## License

Licensed under the [Apache License 2.0](LICENSE).
