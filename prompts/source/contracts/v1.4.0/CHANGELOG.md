# Prompt Contract Changelog (`v1.4.0`)

- `v1.4.0` (2026-07-29): Aligned agent guidance with claim linking: `run_command` evidence auto-links to a prior model-issued prescription, unlinked mutations are recorded with `auto_prescribed=true` and counted by the experimental `unprescribed_mutation` signal (Signal Spec v1.2.0). Replaced the "skip explicit prescribe for run_command" bullet with prescribe-first incentive; added the ninth signal to the behavioral list.
- `v1.3.0` (2026-03-26): Shifted MCP initialize and runtime guidance to a `run_command`-first workflow, added `describe_tool` as the explicit-control discovery path, and kept `prescribe_full` opt-in.
- `v1.2.0` (2026-03-24): Unified contract: DevOps operations, MCP prompt templates, and evidence protocol under single source of truth. Added diagnosis flowchart and risk tag reference to contract.
- `v1.1.0` (2026-03-18): Split direct prescribe into `prescribe_full` and `prescribe_smart` with separate MCP prompt artifacts and shared report semantics.
- `v1.0.1` (2026-03-06): Canonicalized MCP + runtime harness prompt semantics under source templates with invariant/checklist hardening.
- `v1.0.0` (2026-03-06): Initial prompt contract for prescribe/report guidance.
