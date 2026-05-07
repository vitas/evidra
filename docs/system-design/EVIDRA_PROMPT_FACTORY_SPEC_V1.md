# EVIDRA Prompt Factory Spec (Single Source of Truth)

**Status:** Active (`v1.3.0`)
**Date:** 2026-03-26
**Scope:** Prompt contract source, folder structure, generation targets, versioning, CI gates

---

## 1. Problem

Evidra has multiple prompt surfaces:
- MCP server prompts (`prompts/mcpserver/*`)
- MCP prompt references (`prompts/mcp/*`)
- Skill definitions (`prompts/skill/*`)
- Framework-specific integrations (future)

If these prompts evolve independently, protocol behavior drifts and validation results become non-comparable.

We need one canonical prompt contract artifact and deterministic generation to all target surfaces.

---

## 2. Design Goals

1. One source of truth for prompt behavior and contract semantics.
2. Deterministic generation for different agents, runtimes, and protocols.
3. Shared contract version across generated outputs.
4. Explicit traceability: every generated prompt maps to one source contract artifact.
5. Backward compatibility with existing active prompt file paths.

---

## 3. Non-Goals

1. This spec does not change wire protocol fields.
2. This spec does not require immediate migration of all existing prompts in one PR.
3. This spec does not force one "universal wording" for every target; only shared semantics are mandatory.

---

## 4. Canonical Prompt Source Model

Canonical source is a versioned contract artifact:

- Contract semantics (normative rules/invariants)
- Command classification (mutate vs read-only)
- Required failure-path rules
- Target template bindings

Generated prompts are artifacts, not hand-authored source.

---

## 5. Folder Structure

```text
prompts/
  source/                                  # single source of truth
    contracts/
      v1.0.1/                              # legacy baseline
        ...
      v1.1.0/
        ...
      v1.2.0/
        ...
      v1.3.0/                              # active contract
        CONTRACT.yaml                      # canonical rules and invariants
        CLASSIFICATION.yaml                # mutate/read-only command taxonomy
        CHANGELOG.md
        templates/
          mcp/
            initialize.tmpl
            report.tmpl
            get_event.tmpl
            agent_contract.tmpl
            run_command.tmpl
            prescribe_full.tmpl            # full artifact prescribe
            prescribe_smart.tmpl           # lightweight intent prescribe
            prompt_prescribe_full.tmpl
            prompt_prescribe_smart.tmpl
            prompt_diagnosis.tmpl
          skill/
            SKILL.tmpl
            SKILL_SMART.tmpl
            SKILL_FULL.tmpl

  generated/                               # generated artifacts (never manual source)
    v1.0.1/                                # legacy
      ...
    v1.3.0/                                # active
      mcpserver/
        initialize/instructions.txt
        tools/run_command_description.txt
        tools/prescribe_full_description.txt
        tools/prescribe_smart_description.txt
        tools/report_description.txt
        tools/get_event_description.txt
        resources/content/agent_contract_v1.md
      mcp/
        prompt_prescribe_full.md
        prompt_prescribe_smart.md
        prompt_diagnosis.md
      skill/
        SKILL.md
        SKILL_SMART.md
        SKILL_FULL.md

  manifests/
    v1.0.1.json                            # legacy manifest
    v1.1.0.json
    v1.2.0.json
    v1.3.0.json                            # active manifest

  mcpserver/                               # active MCP server location (compat)
  mcp/                                     # active prompt reference location (compat)
  skill/                                   # active skill definitions (compat)
```

Compatibility rule:
- Prompt readers continue reading current active paths (`prompts/mcpserver/*`, `prompts/mcp/*`, `prompts/skill/*`).
- Generation pipeline MUST materialize outputs into those active paths.
- Some render specs are optional across contract versions. Legacy bundles may
  generate fewer active files, such as `tools/prescribe_description.txt` in
  pre-split versions.

---

## 6. Generation Contract

Inputs:
1. `CONTRACT.yaml`
2. `CLASSIFICATION.yaml`
3. target templates
4. profile (optional)

Outputs:
1. Generated prompt files for each target surface.
2. Manifest with file hashes.
3. Metadata with `contract_version`.

Determinism requirements:
1. Stable ordering of sections.
2. Stable newline formatting.
3. No timestamps embedded in generated prompt text.
4. Same input must produce byte-identical output.

---

## 7. Versioning and Semantics

1. Contract version uses semver (`vMAJOR.MINOR.PATCH`).
2. Every generated prompt starts with a contract header:
   - text: `# contract: vX.Y.Z`
   - markdown: `<!-- contract: vX.Y.Z -->`
3. `actor.skill_version` must map to this contract version.

Version bump rules:
1. Patch: wording clarifications, no new required behavior.
2. Minor: new guidance that may affect behavior but not wire schema.
3. Major: required behavior change or protocol contract change.

---

## 8. Target Mapping (Minimum)

### MCP

Generated files:
- `initialize/instructions.txt`
- `tools/run_command_description.txt`
- `tools/prescribe_full_description.txt`
- `tools/prescribe_smart_description.txt`
- `tools/report_description.txt`
- `tools/get_event_description.txt`
- `resources/content/agent_contract_v1.md`

Must preserve:
- mutate/read-only boundary
- prescribe_full vs prescribe_smart selection guidance
- prescribe/report invariants
- retry and failure-path rules
- `actor.skill_version` guidance

### MCP Prompt References

Generated files:
- `prompt_prescribe_full.md`
- `prompt_prescribe_smart.md`
- `prompt_diagnosis.md`

Must preserve:
- the same contract header and version traceability as MCP server artifacts
- direct examples for explicit prescribe/report use
- diagnosis workflow guidance aligned with `run_command`-first instructions

### Skill Definitions

Generated files:
- `SKILL.md` (default installable skill)
- `SKILL_SMART.md` (smart/default skill variant)
- `SKILL_FULL.md` (full-prescribe skill variant)

Must preserve:
- same core invariants as MCP
- tool selection guidance (full vs smart)

### Future targets

Templates may be added for:
- OpenAI tool runtimes
- LangChain/LangGraph wrappers
- REST/hosted instruction formats

---

## 9. CI and Validation Gates

Required checks:
1. `generate` produces no diff on committed outputs.
2. All active prompt files have valid contract header.
3. Contract version in generated files is consistent across targets.
4. Hash manifest matches generated files.
5. Prompt text does not contradict `docs/system-design/EVIDRA_PROTOCOL_V1.md`.

Recommended command contract:

```bash
make prompts-generate
make prompts-verify
```

---

## 10. Implementation State

Implemented:
1. Source contract tree under `prompts/source/contracts/` (`v1.0.1`, `v1.1.0`, `v1.2.0`, `v1.3.0`).
2. Deterministic generator + verifier (`evidra prompts generate|verify`).
3. Make wrappers (`make prompts-generate`, `make prompts-verify`).
4. CI/release drift enforcement via `make prompts-verify`.
5. Prescribe split: `prescribe_full` (raw artifact) and `prescribe_smart` (target intent).
6. `run_command`-first MCP guidance with separate generated MCP server and MCP prompt-reference surfaces.
7. Skill target surface with `SKILL.md`, `SKILL_SMART.md`, and `SKILL_FULL.md` generation.
8. Runtime embed layer (`prompts/embed.go`) with `DefaultContractVersion = v1.3.0` and skill_version derivation.
9. Optional render specs — generator gracefully handles missing optional templates across contract versions.

Planned next:
1. Additional target templates (LangChain/LangGraph/REST-hosted variants).
2. Optional contract profiles/overrides when needed.
3. Contributor guardrails for non-editable generated paths.

---

## 11. Ownership

Single source contract ownership:
- Evidra maintainers owning protocol + prompt behavior.

Review requirement for contract changes:
1. protocol correctness review
2. cross-run comparability review
