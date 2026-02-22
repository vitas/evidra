# Argo Ingest Logic (v0.1)

## Overview

Argo CD exposes application snapshots and partial history. It does not provide a full old-vs-new event stream for every transition.
Evidra v0.1 ingests Argo data using a Kubernetes Informer watch pattern on Application resources, combined with checkpoint deduplication to produce stable investigation evidence while limiting duplicate timeline noise.

## Architecture

The Argo collector uses a **Kubernetes dynamic informer** (`client-go`) watching `argoproj.io/v1alpha1/applications` as the primary ingest path. When no Kubernetes config is available (no in-cluster service account and no kubeconfig), the collector falls back to polling via the Argo CD API.

Both paths share the same checkpoint deduplication logic.

## What we ingest from Argo

- Application destination and scope:
  - destination cluster/server
  - destination namespace
  - application identity
- OperationState:
  - initiated by (user or automated)
  - startedAt / finishedAt
  - terminal phase/result
- Health:
  - current health status
  - observed timestamp based on `status.reconciledAt` (fallback: informer sync time)
- History:
  - history id
  - revision
  - deployed timestamp
  - used for backfill when needed

## Event types (v0.1)

- `argo.sync.started` — OperationState indicates running/terminating sync.
- `argo.sync.finished` — Terminal OperationState only (`Succeeded`, `Failed`, `Error`). Primary completion signal.
- `argo.health.changed` — Observed health state at reconciled/informer sync time.
- `argo.deployment.recorded` — History record observed from Argo history. Backfill/supplemental signal, not primary terminal completion.

## Checkpointing and deduplication

Checkpoint state stores per-app cursors:
- last history id/time
- last start operation key
- last terminal operation key
- last observed health

Checkpoint data is mutable cursor state, not evidence. It prevents duplicate ingest writes across informer watch cycles and restarts. The dedup identity is unified across both the informer and polling fallback paths.

Terminal OperationState is the single authority for `argo.sync.finished`. History-derived `argo.deployment.recorded` is supplemental/backfill only.

## Failure modes

- Stream disconnect: informer reconnects automatically; polling fallback can cover gaps.
- Missed updates: checkpoint comparison detects and backfills gaps on reconnect.
- Event re-ordering: deterministic IDs + checkpoint guards prevent duplicate evidence records.
- Argo restart / transient API errors: polling fallback restores ingest continuity.

## Operational controls

- `EVIDRA_ARGO_COLLECTOR_ENABLED=true` — enable the collector
- `EVIDRA_ARGO_API_URL` — Argo CD API endpoint (required even with dynamic client; used for fallback)
- `EVIDRA_ARGO_API_TOKEN` — Argo CD API token (for fallback polling path)
- `EVIDRA_ARGO_COLLECTOR_INTERVAL` — polling interval for fallback path (default `30s`)
- `EVIDRA_ARGO_CHECKPOINT_FILE` — checkpoint file path (default `./var/argo_checkpoint.json`)
- Kubernetes config: in-cluster service account (preferred) or `KUBECONFIG` / `~/.kube/config`

## Known limitations

- Extremely rapid or flapping transitions between informer syncs might be compressed into single states.
- Health transitions are sampled from the Argo snapshot state broadcasted by the Kubernetes API.

Mitigations:
- Change summary includes health-at-start and health-after-deploy.
- Post-deploy degradation is reported as observational signal only.
- Evidra does not claim causality from timeline ordering.

## Design rationale

Argo CD is the upstream operational authority. Evidra focuses on normalization, correlation, timeline construction, and export — not on duplicating Argo control-plane capabilities.

The Kubernetes informer was chosen over poll-only because:
- Better freshness for active investigations.
- Lower chance of missing short health transitions.
- Better investigator experience during live incidents.

Polling fallback is retained because watch-only is fragile under stream disconnects and restart gaps. Audit-grade evidence cannot rely on transient stream events alone.

The watch is scoped exclusively to `argoproj.io/v1alpha1/applications` — it remains Argo CD-first, not a generic cluster event collector.
