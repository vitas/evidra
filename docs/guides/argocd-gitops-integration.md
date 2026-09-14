# Argo CD GitOps Integration
> **Pre-vNext surface.** The GitOps controller integration and the hosted API this guide drives were removed by the §43 prune. Kept for reference until the documentation pass after Gate C; it does not describe the MCP evidence recorder.

- Status: Guide
- Version: frozen at removal (see the banner above)
- Canonical for: controller-first Argo CD integration guidance
- Audience: public

This guide describes the v1 Argo CD integration model for Evidra.

## Summary

Evidra treats Argo CD as a GitOps execution source, not as a CLI-first adapter.

- `prescribe` = intent registered before execution
- `report` = outcome recorded after execution
- `payload.flavor = reconcile` marks reconciliation-style execution
- `payload.evidence.kind = translated` marks controller events mapped into the
  standard Evidra lifecycle
- `payload.source.system = argocd` identifies the producing control plane

The same prescribe/report pair, signal detection, and scorecard logic apply in
both imperative and reconciliation flows.

## Two Modes

### Zero-touch mode

Use this when you want the fastest rollout and can accept best-effort
correlation.

- Evidra watches Argo `Application` objects in-cluster
- reconcile start becomes a translated `prescribe`
- reconcile completion becomes a translated `report`
- correlation is best-effort using application identity, revision, cluster,
  namespace, and timing

Advantages:

- no Git integration required
- no CI changes required
- immediate visibility into reconciliation and observed outcome

Tradeoff:

- exact desired render is not guaranteed in zero-touch mode

### Explicit traceability mode

Use this when you want the strongest link between upstream automation and Argo
reconciliation.

- CI or an upstream agent registers intent first with `prescribe`, `record`, or `import`
- the same pipeline annotates the Argo `Application`
- Evidra observes reconciliation and emits the terminal `report` linked to the
  existing prescription

Advantages:

- strongest agent/session attribution
- explicit prescription linkage
- clearer audit chain across CI, controller, and cluster outcome

Tradeoff:

- requires annotation injection into the `Application`

## Recommended Annotations

Use annotations, not labels, for trace identifiers:

```yaml
metadata:
  annotations:
    evidra.cc/prescription-id: "01ABC..."
    evidra.cc/session-id: "sess_123"
    evidra.cc/trace-id: "trace_123"
    evidra.cc/agent-id: "gha-deploy"
    evidra.cc/run-id: "run_456"
    evidra.cc/tenant-id: "tenant_123"
```

Explicit mode is enabled when at least `evidra.cc/prescription-id` or
`evidra.cc/session-id` is present.

## Self-Hosted Setup

Enable the controller in `evidra-api`:

```bash
export EVIDRA_ARGOCD_CONTROLLER_ENABLED=true
export EVIDRA_ARGOCD_APPLICATION_NAMESPACE=argocd
export EVIDRA_ARGOCD_TENANT_ID=default
export EVIDRA_KUBECONFIG=                         # optional for local development
```

Recommended deployment model:

- run `evidra-api` in-cluster
- let it watch the Argo `Application` namespace
- keep Git provider access optional
- treat direct `argocd` CLI capture as fallback, not the main product path

## Field Placement

- `scope_dimensions` = stable execution context such as cluster, namespace,
  application, application namespace, project, revision, correlation mode
- `external_refs` = foreign identifiers such as Argo application ID, Argo
  operation ID, Argo revision, CI run ID
- sync phase and health are supplemental status, not primary filter dimensions

## What V1 Guarantees

- controller-observed reconciliation evidence
- same prescribe/report protocol as other Evidra integrations
- same signal detection and scoring behavior
- no required Git integration

V1 does not guarantee exact desired render in zero-touch mode. When customers
need artifact-level fidelity, they should explicitly provide rendered manifests
through the Kubernetes path or use explicit traceability mode.
