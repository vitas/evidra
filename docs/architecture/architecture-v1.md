# Architecture v1

Evidra v1 is an Argo CD-first investigation and evidence layer. It derives lifecycle evidence from Argo CD operation history, revision metadata, and sync/health transitions, stores immutable records, and serves timeline/export APIs. The unified data model leverages the CNCF CloudEvents framework native integration.

## Scope
- Primary source: Argo CD operation/history and revision metadata.
- Supporting correlation: Argo Application annotations.
- Out of scope in v1: generic multi-provider ingestion, direct cluster-wide collectors as primary source, governance workflow execution.

## Component view
```text
Argo CD API/events -> Argo collector -> normalize/correlate -> append-only store
                                              |                  |
                                              +-> query API      +-> export builder
                                                           |
                                                     Explorer UI
```

## Core rules
- Change timeline is built from real lifecycle transitions.
- Event records are append-only.
- Event structure maps directly to the CNCF CloudEvents SDK for cross-system standard interoperability.
- Deterministic identifiers are required for Change and export references.
- UI is read-only; operational control stays in Argo CD.
