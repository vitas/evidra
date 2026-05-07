# Evidra Skill Setup

- Status: Guide
- Version: current
- Canonical for: Skill installation and usage
- Audience: public

The Evidra skill teaches AI agents DevOps operational discipline and the evidence recording protocol. When installed, agents diagnose before fixing, follow safety boundaries, and record evidence for infrastructure mutations via the prescribe/report protocol.

---

## Why Install the Skill

The Evidra MCP server gives AI agents `run_command` (with smart output and auto-evidence) plus `prescribe_smart`, `prescribe_full`, `report`, and `get_event` tools. The installed skill variant adds two things:

1. **DevOps discipline** — diagnose before fixing, verify after patching, don't over-scope changes
2. **Protocol guidance** — when to use the explicit prescribe/report flow for the chosen skill mode, and how to handle retries and failures

Without the skill, agents use `run_command` and get auto-evidence for free (proxy mode). With the skill, agents also learn operational patterns that improve their infrastructure decision-making.

**The MCP server gives agents the tools. The skill teaches them how to think about infrastructure.**

---

## Install

```bash
# Install the default smart skill globally (recommended — works across all projects)
evidra skill install

# Install the full-prescribe skill variant
evidra skill install --full-prescribe

# Install for a specific project only
evidra skill install --scope project
```

### Flags

| Flag | Default | Description |
|------|---------|-------------|
| `--target` | `claude` | Target platform. Currently supports `claude` (Claude Code). |
| `--scope` | `global` | `global` installs to `~/.claude/skills/evidra/SKILL.md`. `project` installs to `.claude/skills/evidra/SKILL.md` in the current directory. |
| `--project-dir` | `.` | Project directory when using `--scope project`. |
| `--full-prescribe` | `false` | Install the full-prescribe skill variant instead of the default smart skill. |

### Global vs Project

- **Global** (`~/.claude/skills/evidra/SKILL.md`): The skill is available in every Claude Code session on this machine. Recommended for individual use.
- **Project** (`.claude/skills/evidra/SKILL.md`): The skill is scoped to one repository. Useful when you want to commit the skill alongside your project so all team members get it automatically.

---

## Verify

After installing, start a new Claude Code session and ask:

> "What skills do you have?"

You should see `evidra` in the list. Then ask:

> "Apply this deployment to staging" (with a YAML manifest)

The agent should call `prescribe_full` or `prescribe_smart` before executing `kubectl apply` and `report` after.

---

## How Skill and MCP Server Work Together

```
MCP Server (evidra-mcp)          Skill (SKILL.md)
─────────────────────────        ─────────────────────────
Provides tools:                  Provides protocol knowledge:
  • prescribe_full                 • When to call prescribe_full
  • prescribe_smart                • When to call prescribe_smart
  • report                         • What fields are required
  • get_event                      • How to handle failures
                                   • Classification tables
Observes and scores              • Decision flowchart
  • Risk assessment                • Retry rules
  • Signal detection
  • Reliability scoring

         ┌──────────────────────┐
         │  Both are needed for │
         │  100% compliance     │
         └──────────────────────┘
```

The MCP server handles evidence recording, risk analysis, and scoring. The skill handles agent behavior — ensuring the agent calls the right tool at the right time with the right inputs.

Default install (`evidra skill install`) writes the smart skill:

- send `tool`, `operation`, `resource`, and optional `namespace` when the target is known
- keep `actor.type`, `actor.id`, `actor.origin`, and `actor.skill_version` present
- fall back to `prescribe_full` with `raw_artifact` when you want native detector coverage and artifact drift detection

Full install (`evidra skill install --full-prescribe`) writes the full-prescribe skill:

- prefer `prescribe_full` when artifact bytes are available and the server was started with `--full-prescribe`
- fall back to `prescribe_smart` only when the target is known but bytes are not available

---

## Update

When you update Evidra (`brew upgrade evidra` or download a new release), run `evidra skill install` again to get the latest skill version. The command overwrites the existing file.

---

## Supported Platforms

| Platform | Status | Target flag |
|----------|--------|-------------|
| Claude Code | Supported | `--target claude` (default) |
| Cursor | Planned | — |
| Codex | Planned | — |
| Windsurf | Planned | — |
