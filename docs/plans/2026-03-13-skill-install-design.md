# Skill Install Feature Design

## Problem

The Evidra skill (SKILL.md) proved 100% protocol compliance in testing — agents follow the prescribe/report protocol perfectly when guided by the skill text. But customers have no way to install the skill from the Evidra binary. They must manually copy the file from the repository into their Claude Code skills directory. There is also no documentation explaining why the skill matters or how it complements the MCP server.

## Decision

Add `evidra skill install` as a new CLI command that writes the embedded SKILL.md to the appropriate skills directory. Update all public-facing documentation and the landing page to explain the skill's role and guide customers through installation.

## CLI Command

```
evidra skill install [--target claude] [--scope global|project]
```

### Flags

| Flag | Default | Description |
|------|---------|-------------|
| `--target` | `claude` | Target platform. Currently only `claude` is supported. |
| `--scope` | `global` | Installation scope: `global` writes to `~/.claude/skills/evidra/SKILL.md`, `project` writes to `.claude/skills/evidra/SKILL.md` in current directory. |

### Behavior

1. Read embedded `SKILL.md` from binary (via `//go:embed`).
2. Determine target path based on `--target` + `--scope`.
3. Create directory if needed.
4. Write file.
5. Print confirmation with path and contract version.
6. If file already exists, overwrite and print "updated" instead of "installed".

### Output

```
Evidra skill installed: ~/.claude/skills/evidra/SKILL.md
Contract version: v1.0.1
Target: claude (global)

The skill guides AI agents to follow the Evidra prescribe/report protocol
with 100% compliance for infrastructure mutations.
```

### Future Subcommands (Not In Scope)

- `evidra skill list` — list installed skills
- `evidra skill update` — update to latest version
- `evidra skill uninstall` — remove installed skill

## Embedding

Add `skill/SKILL.md` to the `//go:embed` directive in `prompts/embed.go`. Add a `ReadSkill()` function that returns the embedded content. The `evidra skill install` handler calls `prompts.ReadSkill()` and writes the result to disk. No external file dependencies.

Binary size impact: ~12KB — negligible.

## Documentation Updates

### New: `docs/guides/skill-setup.md`

Dedicated guide covering:
- What is the Evidra skill
- Why install it (MCP alone = agent may skip steps; MCP + skill = 100% compliance)
- Install command with options (global vs project)
- Verification steps
- How skill and MCP server work together

### Update: `docs/integrations/cli-reference.md`

- Add `skill` to the Command Groups table
- Add `evidra skill install` flags section

### Update: `docs/guides/mcp-setup.md`

Add a section after "2. Connect to your editor" with the skill install one-liner and a note about compliance improvement. Brief, links to the full guide.

### Update: `README.md`

- Add `evidra skill install` to "Fastest Path For DevOps" after Install
- Add skill setup guide link to Docs Map
- Mention that MCP + skill = 100% protocol compliance

### Update: `ui/src/pages/Landing.tsx`

- Add a step in the `McpSetup` component between "2. Connect to your editor" and "3. Verify" — "Install the Evidra skill" with `evidra skill install` code block. Only visible when `claude-code` tab is selected.
- Add a guide card to the GUIDES array pointing to `docs/guides/skill-setup.md`.

## Key Message

**The MCP server gives agents the tools. The skill teaches them when and how to use them.**

- MCP server alone: agents have prescribe/report tools available but may skip steps, forget to report failures, or prescribe read-only operations. Compliance varies by model and context.
- MCP server + skill: protocol rules, invariants, classification tables, and decision flowchart are embedded directly in the agent's context. Testing showed 100% protocol compliance with the skill vs inconsistent behavior without it.

This message appears proportionally across all updated docs:
- `skill-setup.md` — full explanation
- `mcp-setup.md` — one paragraph with link
- `README.md` — one sentence
- Landing page — brief callout

## Sequencing

### Commit 1: Embed skill + CLI command

- Add `skill/SKILL.md` to `//go:embed` in `prompts/embed.go`
- Add `ReadSkill()` function
- Add `cmd/evidra/skill.go` with install handler
- Register `skill` command in `command_registry.go`
- Add tests
- `make build && make test`

### Commit 2: Documentation

- Create `docs/guides/skill-setup.md`
- Update `docs/integrations/cli-reference.md`
- Update `docs/guides/mcp-setup.md`
- Update `README.md`

### Commit 3: Landing page

- Update `ui/src/pages/Landing.tsx`
