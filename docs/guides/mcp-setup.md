# Evidra MCP Setup

- Status: Guide
- Version: current
- Canonical for: MCP setup and local operation
- Audience: public

Evidra MCP is a DevOps MCP server with built-in flight recorder and reliability scoring.

It is a flight recorder for AI agents that touch infrastructure.
The agent reports voluntarily; Evidra observes, scores, and explains.

It gives AI agents `run_command` for kubectl, helm, terraform, and aws with token-efficient
smart output. Every mutation is automatically recorded in an append-only evidence chain —
no extra agent code needed. By default the direct surface includes `run_command`,
`collect_diagnostics`, `write_file`, `prescribe_smart`, `report`, and `get_event`.
Start the server with `--full-prescribe` when you also want `prescribe_full` for
artifact-aware explicit control over evidence recording.

---

## Quick Start

### 1. Install

```bash
# Option A: Homebrew (recommended)
brew install samebits/tap/evidra

# Option B: Go
go install samebits.com/evidra/cmd/evidra-mcp@latest

# Option C: From source
git clone https://github.com/vitas/evidra.git
cd evidra && go build -o evidra-mcp ./cmd/evidra-mcp

# Option D: Docker
docker pull ghcr.io/vitas/evidra-mcp:latest
```

### 2. Connect to your agent

**Claude Code:**
```bash
claude mcp add evidra -- evidra-mcp --signing-mode optional
```

**Cursor / Claude Desktop / Windsurf (JSON config):**
```json
{
  "mcpServers": {
    "evidra": {
      "command": "evidra-mcp",
      "args": ["--signing-mode", "optional"]
    }
  }
}
```

Config file locations:
- **Claude Desktop:** `~/Library/Application Support/Claude/claude_desktop_config.json` (macOS), `~/.config/Claude/claude_desktop_config.json` (Linux)
- **Cursor:** `.cursor/mcp.json` in your project
- **Windsurf:** `~/.codeium/windsurf/mcp_config.json`

**Codex:**
```toml
# In ~/.codex/config.toml
[mcp_servers.evidra]
command = "evidra-mcp"
args = ["--signing-mode", "optional"]
```

**Gemini CLI:**
```json
// In ~/.gemini/settings.json under mcpServers:
{
  "evidra": {
    "command": "evidra-mcp",
    "args": ["--signing-mode", "optional"]
  }
}
```

**OpenClaw:**
```yaml
# In openclaw.yaml:
agents:
  - id: main
    model: anthropic/claude-sonnet-4-5
    mcp_servers:
      - name: evidra
        command: evidra-mcp
        args: ["--signing-mode", "optional"]
```

### 2.5 Install the Evidra skill (Claude Code)

The MCP server gives agents the tools. The skill teaches them when and how to use them — agents with the skill installed achieve 100% protocol compliance.

```bash
evidra skill install
```

This writes the skill to `~/.claude/skills/evidra/SKILL.md`. For project-scoped installation: `evidra skill install --scope project`.

Full guide: [Skill Setup](skill-setup.md)

### 3. Test

Ask your agent: *"What tools do you have from Evidra?"*

You should see seven default tools: `run_command`, `collect_diagnostics`, `write_file`, `describe_tool`, `prescribe_smart`, `report`, and `get_event`. Start `evidra-mcp` with `--full-prescribe` when you also want `prescribe_full`.

Try: *"Apply this deployment to staging"* — the agent should usually call `run_command` for the direct path. Use `describe_tool`, then `prescribe_smart` / `report`, only when you want the explicit protocol.

---

## DevOps Server Mode (new)

evidra-mcp provides DevOps tooling with built-in evidence recording.
The default direct surface is 7 tools; `--full-prescribe` adds `prescribe_full` as an optional eighth tool.

| Tool | Description |
|---|---|
| `run_command` | Execute kubectl, helm, terraform, aws with smart output |
| `collect_diagnostics` | Run one bundled Kubernetes diagnosis pass for a workload |
| `write_file` | Write config or manifest files under the workspace or temp directories |
| `describe_tool` | Show the full schema for deferred protocol tools when you need explicit control |
| `prescribe_smart` | Record intent (lightweight); full schema is loaded on demand via `describe_tool` |
| `report` | Record outcome; full schema is loaded on demand via `describe_tool` |
| `get_event` | Look up evidence |

Enable `--full-prescribe` when you also want `prescribe_full` for artifact-aware explicit intent capture.

### Smart output

`run_command` returns token-efficient summaries instead of raw JSON:

```
# Raw kubectl output (~2,000 tokens):
{"apiVersion":"apps/v1","metadata":{"managedFields":[...],...},...}

# Smart output (~80 tokens):
deployment/web (demo): 0/2 ready | image: nginx:99.99 | Available=False
```

### Auto-evidence

Mutations are automatically recorded — no skill prompt needed:

```bash
# Agent calls run_command("kubectl apply -f fix.yaml")
# evidra-mcp automatically: prescribe → execute → report
# Agent just sees the result
```

Read-only commands (`get`, `describe`, `logs`) pass through with no evidence overhead.

### AgentGateway integration

```yaml
# agentgateway config.yaml — one server, everything included
servers:
  - name: evidra
    url: http://evidra-mcp:3001/mcp
    transport: streamable-http
```

One MCP connection for both infrastructure commands and evidence recording.

---

## How It Works

By default Evidra exposes seven MCP tools, plus optional `prescribe_full` when the server is started with `--full-prescribe`:

**`run_command`** — Execute infrastructure commands (kubectl, helm, terraform, aws) with token-efficient smart output. Mutations are auto-recorded as evidence. Command allowlist prevents dangerous operations.

**`collect_diagnostics`** — Run a fixed read-only Kubernetes diagnosis sequence for one workload. It gathers pods, describe output, recent events, and recent logs when a failing pod needs more context, then returns one compact summary plus machine-readable findings and the commands it executed.

**`write_file`** — Write files under the current workspace or temp directories. Use it for agent-authored YAML, Terraform snippets, or scripts without exposing system directories. Evidra blocks system paths and `~/.ssh`.

**`describe_tool`** — Return the full schema for deferred protocol tools. Most agents do not need it. Use it when you want explicit `prescribe_smart` / `report` control instead of the default `run_command` auto-evidence path.

**`prescribe_full`** — Record intent BEFORE an infrastructure mutation when artifact bytes are available. It records the artifact digest, returns a `prescription_id`, and supports artifact drift detection. Risk context is only present when supplied as external assessment enrichment.

**`prescribe_smart`** — Record intent BEFORE an infrastructure mutation when you know the target operation and resource but do not have artifact bytes. It returns a `prescription_id` and stores the target as declared intent. In the default tool surface its schema is deferred; call `describe_tool` first if you want the full explicit schema.

**`report`** — Record the terminal verdict for the prescription. Executed operations report `success`, `failure`, or `error` with an exit code. Intentional refusals report `declined` with a short operational reason. In the default tool surface its schema is deferred; call `describe_tool` first if you want the full explicit schema.

**`get_event`** — Retrieve a previous evidence record by event ID for debugging or audit.

When using `run_command`, evidence is recorded automatically. That is the default path and the recommended path for cheaper models. When using `describe_tool` plus `prescribe_*` / `report` directly, the agent controls the evidence flow explicitly. Both approaches produce the same evidence chain.

### The workflow

```
You → Agent: "Deploy nginx to production"
       Agent → Evidra: prescribe_full(kubectl, apply, artifact=deployment.yaml, env=production)
       Evidra → Agent: ok=true, prescription_id=rx-01JQ...
       Agent → executes kubectl apply -f deployment.yaml
       Agent → Evidra: report(prescription_id=rx-01JQ..., verdict=success, exit_code=0)
       Evidra → Agent: ok=true, report_id=rep-01JQ..., score_band=excellent, signal_summary={...}
Agent → You: "Deployed successfully. Current score band: excellent."
```

On failure:
```
You → Agent: "Apply the config change"
       Agent → Evidra: prescribe_full(kubectl, apply, artifact=config.yaml, env=staging)
       Evidra → Agent: ok=true, prescription_id=rx-01JR...
       Agent → executes kubectl apply -f config.yaml → fails (exit 1)
       Agent → Evidra: report(prescription_id=rx-01JR..., verdict=failure, exit_code=1)
       Evidra → Agent: ok=true, report_id=rep-01JR..., score_band=..., signal_summary={...}
Agent → You: "Apply failed (exit 1). Recorded for reliability tracking."
```

On deliberate refusal:
```
You → Agent: "Apply this privileged manifest to production"
       Agent → Evidra: prescribe_full(kubectl, apply, artifact=privileged.yaml, env=production)
       Agent runs scanner/policy: risk=critical
       Agent → Evidra: report(prescription_id=rx-01JS..., verdict=declined, decision_context={trigger:"risk_threshold_exceeded", reason:"external assessment marked risk critical"})
       Evidra → Agent: ok=true, report_id=rep-01JS..., verdict=declined
Agent → You: "I declined to apply it because the external assessment marked the risk critical."
```

---

## Evidence Modes

Evidra-mcp has four evidence modes:

**Full Prescribe** — the agent calls `prescribe_full` and `report` explicitly and sends `raw_artifact`. This records artifact identity and supports artifact drift detection.

**Smart Prescribe** — the agent calls `prescribe_smart` and `report` explicitly, sending a lightweight target shape such as `tool`, `operation`, `resource`, and optional `namespace`. This keeps the same evidence chain with lower token cost, but does not support artifact drift detection unless the caller supplies an artifact digest.

**Run-Command Evidence** — `run_command` records every mutation without agent cooperation. When a prior model-issued prescription claims the same normalized action (tool, operation, and target resource within the prescription TTL), the automatic report links to that claim instead of writing a second prescription. Mutations with no prior claim get a server-derived prescription flagged `auto_prescribed`, which the experimental `unprescribed_mutation` signal counts — the chain is always complete, and protocol compliance becomes measured data instead of a silent gap.

**Proxy Observed** — evidra-mcp wraps another MCP server and auto-records evidence for infrastructure mutations. The agent doesn't need to know about evidra. Zero extra tokens, zero agent changes.

### When to use each mode

Use Full Prescribe when:
- You want the agent to actively participate in explicit intent recording
- You need declined verdicts (agent refuses dangerous operations)
- You want artifact-level drift detection
- You have the full manifest/plan content and a capable model

Use Smart Prescribe when:
- You still want explicit prescribe/report participation from the agent
- Your model struggles with full-artifact prescribe payloads
- You can describe the target resource and namespace but do not want to send the full artifact
- You can accept no artifact drift detection unless you provide an artifact digest

Use Proxy Observed when:
- You already have an infrastructure MCP server (kubectl, helm, terraform tools)
- You want to add reliability monitoring without changing agent behavior
- Your model can't follow the prescribe/report protocol
- You want the fastest possible onboarding (one config line change)

### Proxy Observed setup

Wrap your existing MCP server command with `evidra-mcp --proxy --`:

```json
{
  "mcpServers": {
    "infra": {
      "command": "evidra-mcp",
      "args": ["--proxy", "--evidence-dir", "~/.evidra/evidence", "--", "your-mcp-server", "--your-flags"]
    }
  }
}
```

The proxy intercepts `run_command` tool calls and generic mutation-shaped MCP tool names it can classify heuristically, then auto-records prescribe/report evidence. Read-only or unclassified tool calls pass through unrecorded.

### What Proxy Observed records

For each detected mutation:
```json
{"type":"prescribe","prescription_id":"proxy-...","tool":"kubectl","operation":"apply","command":"kubectl apply -f fix.yaml","timestamp":"..."}
{"type":"report","prescription_id":"proxy-...","exit_code":0,"verdict":"success","timestamp":"..."}
```

This evidence feeds the same scorecard engine, behavioral signals, and reliability scoring as Full Prescribe and Smart Prescribe.

### Limitations of Proxy Observed

- No risk assessment — the proxy infers tool/operation from the command, but doesn't analyze artifacts
- No declined verdicts — the proxy can't know when the agent chose not to act
- No artifact drift detection — no YAML manifest is captured for hash comparison
- Command-level only — the proxy sees the command string, not the intent

For explicit agent participation, use Full Prescribe or Smart Prescribe with the evidra skill.
For the richest protocol compliance, use Full Prescribe with the evidra skill. For lower-cost explicit recording, use Smart Prescribe.

---

## What Evidra Measures

Evidra detects 9 behavioral signals from the evidence chain:

| Signal | What it detects |
|--------|----------------|
| **protocol_violation** | Missing prescribe or report, duplicate reports |
| **artifact_drift** | Artifact content changed between prescribe and execution |
| **retry_loop** | Same operation retried multiple times |
| **blast_radius** | Operations affecting many resources or critical scopes |
| **new_scope** | Actor operating in an environment they haven't used before |
| **repair_loop** | Delete-then-recreate patterns indicating instability |
| **thrashing** | Rapid apply/delete cycles on the same resources |
| **risk_escalation** | Actor's operations exceed their baseline risk level |
| **unprescribed_mutation** | Mutation executed with no prior model claim; the server recorded it automatically (experimental, weight 0) |

These signals feed into a weighted reliability score (0–100) with score bands (`excellent`, `good`, `fair`, `poor`). Sufficiency is reported separately via the response basis.

---

## Agent Instructions

This section explains how your AI agent should use Evidra. Claude Code with MCP auto-discovers tools. For other agents, include these instructions in your system prompt.

### When to prescribe/report

**Always prescribe + report for mutations:** `kubectl apply/delete/create/patch`, `terraform apply/destroy/import`, `helm install/upgrade/uninstall/rollback`, `docker run/build/push`.

For GitOps systems such as Argo CD, direct MCP interaction is usually the
upstream intent-registration step, not the execution boundary. In explicit
GitOps mode the agent or CI should register intent first and annotate the Argo
`Application`; the controller integration records the reconcile outcome later.

**Skip for read-only:** `get`, `describe`, `list`, `plan`, `show`, `diff`, `status`, `logs`, `top`.

### Protocol rules

1. If `prescribe_full` is available and you have artifact bytes, use it. Otherwise call `prescribe_smart` BEFORE execution — do not execute until it returns `ok=true` with a `prescription_id`.
2. Call `report` with an explicit `verdict` for every prescription.
3. For `success`, `failure`, or `error`, include the exit code.
4. For `declined`, do not include an exit code. Include `decision_context.trigger` and `decision_context.reason`.
5. Every prescribe must have exactly one report. Never skip the report, even on failure or refusal.
6. On retry, call the same prescribe tool again for each attempt (new prescription per attempt).
7. If unsure whether a command mutates state, call `prescribe_smart`.
8. Do not use prescribe/report for non-infrastructure tasks (coding, analysis, documentation).

### How to call prescribe

Smart Prescribe:

```json
{
  "tool": "kubectl",
  "operation": "apply",
  "resource": "deployment/web",
  "namespace": "default",
  "actor": {
    "type": "agent",
    "id": "claude",
    "origin": "mcp",
    "skill_version": "1.1.0"
  }
}
```

Full Prescribe:

```json
{
  "tool": "kubectl",
  "operation": "apply",
  "raw_artifact": "<contents of the YAML manifest>",
  "actor": {
    "type": "agent",
    "id": "claude",
    "origin": "mcp",
    "skill_version": "1.1.0"
  },
  "environment": "production",
  "session_id": "optional-session-id",
  "scope_dimensions": {
    "cluster": "prod-us-east",
    "namespace": "default"
  }
}
```

**Smart Prescribe required fields:** `tool`, `operation`, `resource`, and `actor`.

**Full Prescribe required fields:** `tool`, `operation`, `raw_artifact`, and `actor`.

**Full Prescribe (`raw_artifact`) examples:**
- **kubectl apply -f manifest.yaml** — read the file, pass full YAML content
- **terraform apply** — pass the plan JSON or HCL content
- **helm upgrade** — pass the values override YAML
- **kubectl delete** — pass resource type and name as artifact

**Smart Prescribe (`resource` / `namespace`) examples:**
- **kubectl apply deployment/web** — send `resource: "deployment/web"` and `namespace`
- **kubectl delete configmap/app** — send `resource: "configmap/app"` and `namespace`
- **helm upgrade release/chart** — send `resource: "release/my-app"` when chart bytes are not available in context

### How to call report

```json
{
  "prescription_id": "rx-01JQ...",
  "verdict": "success",
  "exit_code": 0,
  "actor": {
    "type": "agent",
    "id": "claude",
    "origin": "mcp"
  }
}
```

Declined example:

```json
{
  "prescription_id": "rx-01JS...",
  "verdict": "declined",
  "decision_context": {
    "trigger": "risk_threshold_exceeded",
    "reason": "external assessment marked risk critical"
  },
  "actor": {
    "type": "agent",
    "id": "claude",
    "origin": "mcp"
  }
}
```

### Handling responses

**`prescribe_full` or `prescribe_smart` returns `ok=true`:** Proceed with execution. If an external assessment was supplied, note `effective_risk` and the `risk_inputs` panel for context.

**report returns an assessment snapshot:** Informational. Note `score_band`, `signal_summary`, `basis`, and `confidence`, then continue — Evidra observes, it does not block.

**Error (`ok=false` with error field):** Something went wrong with evidence recording. Log it and continue — Evidra is non-blocking by design.

---

## Agent System Prompt (Codex / Gemini / Other Agents)

For agents without automatic MCP tool discovery, add this to your system prompt:

```
## Evidra MCP Operating Contract

You have access to MCP tools: "run_command", "collect_diagnostics", "write_file", "prescribe_smart", "report", and "get_event". If the server was started with "--full-prescribe", you also have "prescribe_full".

Before every infrastructure mutation (apply, delete, create, patch, upgrade,
destroy, import), call "prescribe_full" when artifact bytes are available.
Call "prescribe_smart" when you only know the target tool, operation, and
resource context. Wait for ok=true before executing.

After each prescription, call "report" with an explicit verdict.
For success/failure/error, include exit_code.
If you intentionally refuse to execute, call "report" with verdict=declined
plus decision_context.trigger and decision_context.reason.

Skip for read-only commands: get, describe, list, plan, show, diff, status.

On retry, call the same prescribe tool again for each new attempt.

Evidra observes and measures — it does not block operations.
```

---

## Configuration

### Evidence storage

Default: `~/.evidra/evidence`

Override:
```bash
evidra-mcp --evidence-dir /var/lib/evidra/evidence
# or
export EVIDRA_EVIDENCE_DIR=/var/lib/evidra/evidence
```

### Environment

Label the environment for evidence context:
```bash
evidra-mcp --environment production
```

Values: `production`, `staging`, `development`. This is recorded as context and may be used by external assessment tools.

### Signing mode

Evidence entries are signed with Ed25519 keys. Two modes:

```bash
# Strict (default) — requires a signing key
evidra-mcp --signing-mode strict
export EVIDRA_SIGNING_KEY_PATH=~/.evidra/keys/private.pem

# Optional — uses ephemeral keys if no key configured (good for development)
evidra-mcp --signing-mode optional
```

Generate a keypair with:
```bash
evidra keygen --output-dir ~/.evidra/keys
```

### Retry tracking

Enable in-memory retry loop detection:
```bash
evidra-mcp --retry-tracker
# or
export EVIDRA_RETRY_TRACKER=true
```

### Connection modes

```bash
# Offline (default) — all evidence stored locally
evidra-mcp

# Online — forward evidence to API server
evidra-mcp --url https://your-api.example.com --api-key YOUR_KEY

# Online with offline fallback
evidra-mcp --url https://your-api.example.com --api-key YOUR_KEY --fallback-offline

# Force offline — skip API even if EVIDRA_URL is set
evidra-mcp --offline
```

### Environment variables

| Variable | Description |
|---|---|
| `EVIDRA_EVIDENCE_DIR` | Evidence store directory (default: `~/.evidra/evidence`) |
| `EVIDRA_ENVIRONMENT` | Environment label (`production`, `staging`, `development`) |
| `EVIDRA_RETRY_TRACKER` | Enable retry tracking (`true`/`false`) |
| `EVIDRA_SIGNING_MODE` | `strict` (default) or `optional` |
| `EVIDRA_SIGNING_KEY` | Base64-encoded Ed25519 private key |
| `EVIDRA_SIGNING_KEY_PATH` | Path to PEM Ed25519 private key |
| `EVIDRA_EVIDENCE_WRITE_MODE` | `strict` (default) or `best_effort` |
| `EVIDRA_URL` | API endpoint (enables online mode) |
| `EVIDRA_API_KEY` | Bearer token for API authentication |
| `EVIDRA_FALLBACK` | `closed` (default) or `offline` |

---

## Self-Hosted API Setup

Run the full stack with Docker Compose:

```bash
curl -O https://raw.githubusercontent.com/vitas/evidra/main/docker-compose.yml

export EVIDRA_API_KEY=my-secret-key
docker compose up -d

# Verify
curl http://localhost:8080/healthz
```

Then configure the MCP server to forward evidence:

**Claude Code:**
```bash
claude mcp add evidra -- evidra-mcp \
  --signing-mode optional \
  --url http://localhost:8080 \
  --api-key my-secret-key \
  --fallback-offline
```

**JSON config (Cursor / Claude Desktop / Windsurf):**
```json
{
  "mcpServers": {
    "evidra": {
      "command": "evidra-mcp",
      "args": ["--signing-mode", "optional"],
      "env": {
        "EVIDRA_URL": "http://localhost:8080",
        "EVIDRA_API_KEY": "my-secret-key",
        "EVIDRA_ENVIRONMENT": "production",
        "EVIDRA_FALLBACK": "offline"
      }
    }
  }
}
```

Self-hosted `/v1/evidence/scorecard` and `/v1/evidence/explain` are available for tenant-wide analytics over stored evidence. Use CLI/MCP when you want local-first workflows or immediate command-side assessment. See [Self-Hosted Experimental Status](self-hosted-setup.md).

---

## Troubleshooting

**Agent doesn't call prescribe:**
- Verify MCP connection: ask the agent "what tools do you have?"
- If tools not listed: check MCP config, restart agent
- If listed but not called: add explicit instructions (see Agent System Prompt section)

**prescribe returns error:**
- Check evidence directory is writable: `ls -la ~/.evidra/evidence/`
- If signing mode is strict: configure a signing key or use `--signing-mode optional`
- Run standalone to verify: `evidra-mcp --signing-mode optional --help`

**No evidence recorded:**
- Default store: `~/.evidra/evidence`
- Check `EVIDRA_EVIDENCE_DIR` override
- Verify disk space — evidence files are append-only

**Retry signals not detected:**
- Enable retry tracking: `--retry-tracker` or `EVIDRA_RETRY_TRACKER=true`
- Retry detection requires in-memory state (per-process, resets on restart)

**Scorecard shows insufficient data:**
- Minimum 100 operations required for scoring
- Check with: `evidra scorecard --evidence-dir ~/.evidra/evidence`
