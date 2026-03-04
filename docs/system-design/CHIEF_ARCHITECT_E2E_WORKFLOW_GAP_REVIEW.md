# Chief Architect E2E Workflow Simulation Review

Date: 2026-03-04  
Primary source: `docs/system-design/EVIDRA_END_TO_END_EXAMPLE_v2.md`  
Simulation evidence root: `/tmp/evidra-e2e-sim.AtybXV`

## Executive Summary

The current codebase covers most of the E2E flow mechanically (prescribe, report, evidence write, signal detection, scorecard, compare), but it does not yet fully align with the system design contract from a logic and consistency perspective.

Main outcome:
- Workflow execution coverage: mostly present.
- Architectural/logic consistency: partial.
- Highest impact gaps are in protocol-violation counting, ID semantics, and adapter completeness.

## What Was Simulated

Executed flows:
- Part 1 MCP-style Kubernetes flow using CLI plus MCP lifecycle tests.
- Part 2 CI Terraform flow (`prescribe` + `report`).
- Part 3 scorecard and cross-actor compare.
- Failure cases:
  - crash before report
  - stalled operation
  - artifact drift
  - retry loop
  - unprescribed action
  - adapter parse failure
  - two actors with different workload overlap
- Appendix paths:
  - helm
  - pre-canonicalized (`--canonical-action`)
  - argocd

## Coverage Matrix

| E2E Workflow Item | Status | Evidence | Notes |
|---|---|---|---|
| Part 1 prescribe/report (K8s) | Covered | `out/part1_prescribe.json`, `out/part1_report.json`, `out/part1_mcp_tests.txt` | Core lifecycle works. |
| K8s canonicalization + digests | Covered | `out/part1_prescribe.json` | Returns `canon_version: k8s/v1`, intent/artifact digest. |
| Risk detector integration | Covered | `out/part1_prescribe.json`, `out/part2_prescribe.json` | Tags and risk elevation are active. |
| Evidence chain write | Covered | `evidence_part1/segments/*.jsonl` | Append-only entries generated. |
| Part 2 Terraform CI flow | Covered | `out/part2_prescribe.json`, `out/part2_report.json` | Works with valid Terraform JSON plan format. |
| Part 3 compare output | Covered | `out/part3_compare.json` | Workload overlap computed. |
| Crash before report | Partially covered | `out/fail_crash_explain.json` | Detected, but double-counted with `unreported_prescription`. |
| Stalled operation | Partially covered | `out/fail_stall_explain.json` | Detected, but double-counted with `unreported_prescription`. |
| Artifact drift | Covered | `out/fail_drift_explain.json` | Drift signal fired correctly. |
| Retry loop | Covered | `out/fail_retry_explain.json` | Retry loop fired with threshold behavior. |
| Unprescribed action | Partially covered | `out/fail_unprescribed_cli_exit_code.txt`, `out/fail_unprescribed_detector_test.txt` | Detector exists, but normal CLI/MCP ingress rejects unknown prescription before event creation. |
| Adapter parse failure | Covered | `out/fail_parse_prescribe_stdout.json`, `out/fail_parse_evidence_grep.txt` | `canonicalization_failure` entry written. |
| Helm appendix flow | Partially covered | `out/app_helm_prescribe.json` | Uses k8s adapter, but `operation_class` is `unknown` for `upgrade`. |
| ArgoCD appendix flow | Missing vs design intent | `out/app_argocd_prescribe.json` | Falls back to `generic/v1`, no Argo-specific behavior/metadata. |
| Pre-canonicalized flow | Covered (partial contract fit) | `out/app_external_prescribe.json` | `--canonical-action` works. Version marker differs from MCP path (`external` vs `external/v1`). |

## Logic Gaps (Prioritized)

1. **Protocol violation is double-counted**  
   - Symptom: same entry repeated in protocol signal; rates can exceed 1.0 (`fail_stall_explain.json`, `fail_crash_explain.json`).  
   - Cause: `signal.AllSignals()` already includes unreported prescriptions; CLI then adds `DetectUnreported()` on top.  
   - Code: `cmd/evidra/main.go` (`cmdScorecard`, `cmdExplain`), `internal/signal/protocol_violation.go`.

2. **Prescription ID semantics are inconsistent**  
   - Symptom: returned `prescription_id` is `entry_id`, but payload stores a different ID (`payload.prescription_id == trace_id`).  
   - Evidence: JSONL entries in `evidence_part1/segments/evidence-000001.jsonl`.  
   - Impact: confusing linkage, weak contract clarity.  
   - Code: `cmd/evidra/main.go` (`cmdPrescribe`), `pkg/mcpserver/server.go` (`Prescribe`).

3. **Unprescribed action cannot be produced through normal ingress**  
   - Symptom: CLI rejects unknown prescription (`report` exits with code 1).  
   - Impact: failure mode exists in design/spec, but operational path suppresses it unless data is injected externally.  
   - Code: `cmd/evidra/main.go` (`cmdReport`), `pkg/mcpserver/server.go` (`Report`).

4. **TTL defaults are inconsistent across modules**  
   - Symptom: CLI default `--ttl` is 5m; signal package default is 10m.  
   - Impact: different results by entrypoint and integration path.  
   - Code: `cmd/evidra/main.go`, `internal/signal/types.go`.

5. **Helm operation mapping is incomplete**  
   - Symptom: `helm upgrade` returns `operation_class: unknown` and inflated risk (`high`).  
   - Impact: incorrect risk matrix application and noisy scoring for Helm users.  
   - Code: `internal/canon/types.go` (`k8sOperationClass`).

6. **ArgoCD adapter path from design is not implemented**  
   - Symptom: `tool=argocd` falls to `generic/v1`; no Argo metadata support.  
   - Impact: appendix scenario cannot be represented as designed.  
   - Code: `internal/canon/generic.go`, adapter selection in `internal/canon/types.go`.

7. **Small-window scoring is operationally weak**  
   - Symptom: frequent `insufficient_data` (`score=-1`) due `MinOperations=100`.  
   - Impact: E2E and real small-team windows are hard to evaluate.  
   - Code: `internal/score/score.go`.

8. **Trace ID lifecycle in MCP service is coarse**  
   - Symptom: one `traceID` initialized on service creation and reused.  
   - Impact: weak per-operation correlation semantics.  
   - Code: `pkg/mcpserver/server.go` (`BenchmarkService.traceID`).

## Recommendations

1. **Fix protocol-violation aggregation first (P0)**  
   - Make one source of truth for unreported prescriptions.  
   - Keep sub-signal classification, but remove duplicate counting.

2. **Unify prescription identity (P0)**  
   - Define `prescription_id == entry_id` everywhere, including payload.  
   - Keep `trace_id` separate for correlation only.

3. **Define explicit policy for unprescribed reports (P1)**  
   - Option A: accept and record as protocol violation.  
   - Option B: reject but emit an explicit evidence event for attempted unprescribed action.

4. **Centralize TTL config (P1)**  
   - One default constant used by CLI, MCP, and signal logic.

5. **Complete adapter alignment (P1)**  
   - Helm op-class mapping (`upgrade/install` -> mutate, `uninstall` -> destroy).  
   - ArgoCD adapter or at least structured interim adapter with metadata.

6. **Improve score usability for low-volume actors (P2)**  
   - Add confidence layer in output and/or configurable `min-operations`.  
   - Keep statistical guardrails but avoid hard non-actionable outputs.

7. **Normalize canonical source/version semantics (P2)**  
   - Align `external` vs `external/v1` between CLI and MCP.

## Final Assessment

The architecture and codebase can execute the majority of the E2E workflow, but the current logic contract is not fully coherent yet.  
Before treating this as production-grade benchmark behavior, resolve the P0/P1 gaps above, then rerun this same simulation suite as a release gate.
