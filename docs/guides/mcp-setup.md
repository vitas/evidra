# Evidra MCP Setup

- Status: Guide
- Version: current
- Canonical for: MCP setup and local operation
- Audience: public

Evidra MCP is a flight recorder for AI agents that touch infrastructure.
It records intent, explicit decisions, and outcomes for each reported operation, computes
behavioral signals, and produces reliability assessments over an append-only
evidence chain.

The MCP server (`evidra-mcp`) lets any MCP-capable AI agent report to Evidra out of the box.

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

You should see three tools: `prescribe`, `report`, and `get_event`.

Try: *"Apply this deployment to staging"* — the agent should call `prescribe` before executing and `report` after.

---

## How It Works

Evidra exposes three MCP tools:

**`prescribe`** — Record intent BEFORE an infrastructure mutation. Analyzes the artifact, computes risk level, and returns a `prescription_id`. The agent must not execute until prescribe returns `ok=true`.

**`report`** — Record the terminal verdict for the prescription. Executed operations report `success`, `failure`, or `error` with an exit code. Intentional refusals report `declined` with a short operational reason.

**`get_event`** — Retrieve a previous evidence record by event ID for debugging or audit.

The agent reports voluntarily; Evidra observes, scores, and explains. It does
not intercept commands, block execution, or enforce policy.

### The workflow

```
You → Agent: "Deploy nginx to production"
       Agent → Evidra: prescribe(kubectl, apply, artifact=deployment.yaml, env=production)
       Evidra → Agent: ok=true, prescription_id=rx-01JQ..., risk_level=high
       Agent → executes kubectl apply -f deployment.yaml
       Agent → Evidra: report(prescription_id=rx-01JQ..., verdict=success, exit_code=0)
       Evidra → Agent: ok=true, report_id=rep-01JQ..., score_band=excellent, signal_summary={...}
Agent → You: "Deployed successfully. Risk level: high. Current score band: excellent."
```

On failure:
```
You → Agent: "Apply the config change"
       Agent → Evidra: prescribe(kubectl, apply, artifact=config.yaml, env=staging)
       Evidra → Agent: ok=true, prescription_id=rx-01JR...
       Agent → executes kubectl apply -f config.yaml → fails (exit 1)
       Agent → Evidra: report(prescription_id=rx-01JR..., verdict=failure, exit_code=1)
       Evidra → Agent: ok=true, report_id=rep-01JR..., score_band=..., signal_summary={...}
Agent → You: "Apply failed (exit 1). Recorded for reliability tracking."
```

On deliberate refusal:
```
You → Agent: "Apply this privileged manifest to production"
       Agent → Evidra: prescribe(kubectl, apply, artifact=privileged.yaml, env=production)
       Evidra → Agent: ok=true, prescription_id=rx-01JS..., effective_risk=critical, risk_inputs=[...]
       Agent → Evidra: report(prescription_id=rx-01JS..., verdict=declined, decision_context={trigger:"risk_threshold_exceeded", reason:"effective_risk=critical and blast_radius covers production namespace"})
       Evidra → Agent: ok=true, report_id=rep-01JS..., verdict=declined
Agent → You: "I declined to apply it because the assessed risk was critical and the blast radius reached production."
```

---

## What Evidra Measures

Evidra detects 8 behavioral signals from the evidence chain:

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

These signals feed into a weighted reliability score (0–100) with score bands (`excellent`, `good`, `fair`, `poor`). Sufficiency is reported separately via the response basis.

---

## Agent Instructions

This section explains how your AI agent should use Evidra. Claude Code with MCP auto-discovers tools. For other agents, include these instructions in your system prompt.

### When to prescribe/report

**Always prescribe + report for mutations:** `kubectl apply/delete/create/patch`, `terraform apply/destroy/import`, `helm install/upgrade/uninstall/rollback`, `docker run/build/push`, `argocd app sync`.

**Skip for read-only:** `get`, `describe`, `list`, `plan`, `show`, `diff`, `status`, `logs`, `top`.

### Protocol rules

1. Call `prescribe` BEFORE execution — do not execute until it returns `ok=true` with a `prescription_id`.
2. Call `report` with an explicit `verdict` for every prescription.
3. For `success`, `failure`, or `error`, include the exit code.
4. For `declined`, do not include an exit code. Include `decision_context.trigger` and `decision_context.reason`.
5. Every prescribe must have exactly one report. Never skip the report, even on failure or refusal.
6. On retry, call `prescribe` again for each attempt (new prescription per attempt).
7. If unsure whether a command mutates state, call `prescribe`.
8. Do not use prescribe/report for non-infrastructure tasks (coding, analysis, documentation).

### How to call prescribe

```json
{
  "tool": "kubectl",
  "operation": "apply",
  "raw_artifact": "<contents of the YAML manifest>",
  "actor": {
    "type": "agent",
    "id": "claude",
    "origin": "mcp",
    "skill_version": "1.0.1"
  },
  "environment": "production",
  "session_id": "optional-session-id",
  "scope_dimensions": {
    "cluster": "prod-us-east",
    "namespace": "default"
  }
}
```

**Required fields:** `tool`, `operation`, `raw_artifact`, `actor`.

**Filling raw_artifact from context:**
- **kubectl apply -f manifest.yaml** — read the file, pass full YAML content
- **terraform apply** — pass the plan JSON or HCL content
- **helm upgrade** — pass the values override YAML
- **kubectl delete** — pass resource type and name as artifact

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
    "reason": "risk_level=critical and blast_radius covers production namespace"
  },
  "actor": {
    "type": "agent",
    "id": "claude",
    "origin": "mcp"
  }
}
```

### Handling responses

**prescribe returns `ok=true`:** Proceed with execution. Note `effective_risk` and the `risk_inputs` panel for context.

**report returns an assessment snapshot:** Informational. Note `score_band`, `signal_summary`, `basis`, and `confidence`, then continue — Evidra observes, it does not block.

**Error (`ok=false` with error field):** Something went wrong with evidence recording. Log it and continue — Evidra is non-blocking by design.

---

## Agent System Prompt (Codex / Gemini / Other Agents)

For agents without automatic MCP tool discovery, add this to your system prompt:

```
## Infrastructure Reliability Benchmark (Evidra)

You have access to MCP tools: "prescribe", "report", and "get_event".

Before every infrastructure mutation (apply, delete, create, patch, upgrade,
destroy, import), call "prescribe" with tool name, operation, and the raw
artifact content. Wait for ok=true before executing.

After each prescription, call "report" with an explicit verdict.
For success/failure/error, include exit_code.
If you intentionally refuse to execute, call "report" with verdict=declined
plus decision_context.trigger and decision_context.reason.

Skip for read-only commands: get, describe, list, plan, show, diff, status.

On retry, call prescribe again for each new attempt.

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

Label the environment for risk classification:
```bash
evidra-mcp --environment production
```

Values: `production`, `staging`, `development`. Affects risk matrix scoring — production operations carry higher risk levels.

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
