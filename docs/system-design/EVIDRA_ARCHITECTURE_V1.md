# Evidra Architecture

- Status: Normative
- Version: v1.0
- Canonical for: system overview and component boundaries
- Audience: public

**One page. The complete system.**
**Consolidated source:** delivered implementation notes from removed `V1_IMPLEMENTATION_NOTES.md` are preserved in this document.

---

## Protocol Vocabulary

Evidra keeps one lifecycle vocabulary across imperative commands, workflows,
and reconciliation systems:

- `prescribe` = intent registered before execution
- `report` = outcome recorded after execution

Context is carried by payload metadata, not by inventing new primary lifecycle
entry types.

- Request-side ingest taxonomy: `flavor` = execution shape (`imperative`,
  `reconcile`, `workflow`), `evidence.kind` = acquisition mode (`declared`,
  `observed`, `translated`), `source.system` = producing adapter or upstream
  system
- Persisted entries expose the same context as `payload.flavor`,
  `payload.evidence.kind`, and `payload.source.system`

The self-hosted API exposes the same taxonomy through raw `/v1/evidence/forward`
and `/v1/evidence/batch`, plus typed `/v1/evidence/ingest/prescribe` and
`/v1/evidence/ingest/report` lifecycle ingest. Webhook routes are compatibility
wrappers over that shared ingest service, not a second evidence lane.

The signal engine and scorecard remain flavor-agnostic in v1. They operate on
the same prescribe/report pairs regardless of whether the execution came from an
AI agent, a workflow runner, or a controller such as Argo CD.

## Pipeline

```
  ┌──────────────────────────── RECORDER (write path) ──────────────────────────┐
  │                                                                             │
  │   OBSERVATION             OPTIONAL ENRICHMENT              EVIDENCE        │
  │                                                                             │
  │   MCP direct ────┐      ┌────────────────────────┐                         │
  │   MCP proxy ─────┤      │ declared intent        │                         │
  │   CLI ───────────┼──▸   │ + artifact digest      │                         │
  │   Webhooks ──────┤      │ + canonical_action?    │                         │
  │   CI/import ─────┘      │ + assessment?          │                         │
  │                         └───────────┬────────────┘                         │
  │                                     │                                      │
  │                                     ▼                                      │
  │   ┌──────────────────┐       ┌──────────────────────────────┐              │
  │   │ report           │       │ prescribe entry              │              │
  │   │ verdict          │       │ intent                       │              │
  │   │ exit_code        │─────▸ │ assessment.status            │              │
  │   │ decision_context │       │ optional risk_inputs[]       │              │
  │   └──────────────────┘       └──────────────┬───────────────┘              │
  │                                             │                               │
  │                                     sign → chain → store                    │
  │                                             │                               │
  └─────────────────────────────────────────────┼───────────────────────────────┘
                                                │
                                                ▼
  ┌──────────────────────── INTELLIGENCE (read path) ───────────────────────────┐
  │                                                                             │
  │   SIGNALS ENGINE                          SCORING                           │
  │                                                                             │
  │   evidence    ┌─ retry_loop           signal counts    ┌─────────────┐     │
  │   sequence    ├─ protocol_violation   + rates           │ SCORECARD   │     │
  │   ──────────▸ ├─ artifact_drift       ──────────────▸  │ weighted    │     │
  │               ├─ blast_radius                          │ penalty     │     │
  │               ├─ new_scope                             │ ──▸ 0-100   │     │
  │               ├─ repair_loop                           │ ──▸ band    │     │
  │               ├─ thrashing                             └─────────────┘     │
  │               └─ risk_escalation                                           │
  │                                                                             │
  └─────────────────────────────────────────────────────────────────────────────┘
                                                │
                             ┌──────────────────┼──────────────────┐
                             ▼                  ▼                  ▼
                     ┌──────────────┐   ┌──────────────┐   ┌──────────────────┐
                     │ CLI          │   │ MCP Server   │   │ Self-hosted API  │
                     │ scorecard    │   │ prescribe    │   │ /v1/evidence/    │
                     │ explain      │   │ report       │   │   scorecard      │
                     │ record       │   │ get_event    │   │   explain        │
                     │ compare      │   │              │   │                  │
                     └──────────────┘   └──────────────┘   └──────────────────┘
```

The recorder write path is intentionally small: normalize caller-declared
intent, attach optional caller-supplied enrichments, sign, and store. It does
not require a built-in canonicalizer or risk assessor.

`canonical_action` is optional identity enrichment for callers that already
have a stable normalized shape. `assessment` is optional risk enrichment from an
external scanner, policy engine, or gateway. If neither is provided, the
prescribe entry remains valid and analytics fall back to declared intent.

---

## Layers

| # | Layer | Input | Output | What It Does |
|---|-------|-------|--------|-------------|
| 1 | **Evidence Chain** | prescribe + report entries | Signed JSONL segments | Tamper-evident append-only log of all operations |
| 2 | **Optional Enrichment** | caller-supplied canonical_action / assessment | Stored payload fields | External systems can add normalized identity and risk context without becoming core dependencies |
| 3 | **Signals Engine** | Evidence entries (sequence) | signal counts + rates | Detects behavioral patterns across operation sequences |
| 4 | **Scorecard** | Signal counts + rates | score (0-100) + band | Weighted penalty model → reliability metric |

**Layers 1-2** happen at record time. **Layers 3-4** evaluate at scorecard time
(post-hoc). Legacy built-in canonicalizer and risk components may remain as
optional tooling during migration, but they are not on the core write path.

---

## Three Event Vocabularies

```
RESOURCE RISK (from detectors, per-artifact)
  k8s.privileged_container    k8s.hostpath_mount       k8s.docker_socket
  aws.s3_public_access        aws.iam_wildcard_policy   aws.rds_public
  gcp.storage_public          azure.nsg_open            docker.privileged
  ... (40+ at launch)

OPERATION RISK (from detectors, per-action)
  ops.mass_delete             ops.namespace_delete      ops.kube_system

BEHAVIOR SIGNALS (from signals engine, per-session)
  retry_loop                  protocol_violation        artifact_drift
  blast_radius                new_scope                 risk_escalation
  repair_loop (+)             thrashing (-)
```

Resource/operation tags = what the code looks like (static).
Behavior signals = how the automation operates (dynamic).
**Signals are the product. Tags are the vocabulary.**

Architecture principle: **graph-ready, graph-free.** Signals work on `[]Entry` sequences using intent_digest + artifact_digest + verdict + optional exit_code. No graph data structure needed. Intent Graph can be added later as optimization, but current signals don't require it.

---

## Data Flow Example

```
1. Agent calls: evidra prescribe --tool kubectl --operation apply --artifact deployment.yaml

2. Core builds declared intent:
     tool=kubectl, operation=apply, target=deployment.yaml,
     artifact_digest=sha256:abc...

3. Optional external scanner/policy may supply assessment:
     assessment.status=provided, effective_risk=critical
     or assessment.status=not_provided when no scanner ran

4. Evidence entry written:
     type=prescribe, intent={...}, assessment={status:not_provided},
     prescription_id=01HXY...

5. Agent executes kubectl apply → fails (exit_code=1)

6. Agent calls: evidra report --prescription 01HXY... --verdict failure --exit-code 1

7. Evidence entry written:
     type=report, prescription_id=01HXY..., verdict=failure, exit_code=1

9. Agent retries same operation (same artifact, same prescribe, exit_code=1) × 2 more

10. Signals engine (at scorecard time):
      retry_loop: count=3 (same intent_digest, 3 failures)
      protocol_violation: count=0 (all prescribes have reports)
      artifact_drift: count=0 (artifact unchanged)

11. Scorecard:
      score = 100 - (retry_loop_penalty × 3)
      score = 62, band = fair

12. Output:
      "Your agent scored 62 (fair). 3 retry loops detected on a critical
       privileged container deployment. Consider: why is the agent retrying
       without changing the artifact?"
```

---

## Component Inventory

### Implemented (current)

| Component | Location | Status |
|-----------|----------|--------|
| Evidence chain | `pkg/evidence/` | Stable |
| 8 signal detectors | `internal/signal/` | Stable |
| Scorecard + explain | `internal/score/` | Stable |
| MCP server | `pkg/mcpserver/` | Stable |
| Optional canonical action schema | `pkg/evidence/` | Stable |
| Legacy companion canonicalizers/detectors | companion packages under `internal/` | Migration |
| Ed25519 signing | `pkg/evidence/` | Stable |
| Hash chain | `pkg/evidence/` | Stable |

### v1.0 (delivered)

| Component | Document | Status |
|-----------|----------|--------|
| Detector architecture (registry, metadata, producer chain) | V1_ARCHITECTURE (this doc) | Delivered |
| Docker adapter + Docker detectors | V1_ARCHITECTURE (this doc) | Delivered |
| Signal validation harness (A-I scenarios) | V1_ARCHITECTURE (this doc) | Delivered (calibrated separately from production sufficiency threshold) |
| Self-hosted API | [self-hosted-setup.md](../guides/self-hosted-setup.md) | Delivered for evidence ingestion, browsing, and tenant-wide scorecard/explain |
| Signal validation | `tests/signal-validation/` scripts | Running in CI/manual flows |

CLI and MCP are the primary local analytics entry points in v1. Self-hosted also
exposes tenant-wide `/v1/evidence/scorecard` and `/v1/evidence/explain` over
centralized stored evidence using the same signal and scoring path.

## Self-Hosted Mode

Self-hosted mode keeps the same evidence semantics as local CLI and MCP workflows. What changes is ingress and storage, not the scoring model.

- **Forwarded evidence:** CLI and MCP can append evidence locally or forward the same signed entries to `evidra-api` for centralized storage.
- **Controller-first GitOps ingress:** Argo CD can contribute controller-observed reconciliation evidence in self-hosted mode. Webhooks remain supported, but they are not the only GitOps path.
- **Centralized store:** Self-hosted evidence is persisted in Postgres so teams can browse and replay tenant-wide evidence instead of reading per-machine JSONL chains.
- **Shared analytics path:** Self-hosted `scorecard` and `explain` load stored evidence and run the same signal detectors and scoring engine as local analysis.
- **deliberate refusal:** A deny decision is still explicit evidence, not a side channel. The terminal record remains `report(verdict=declined, decision_context)`, so local and self-hosted analytics interpret it the same way.

```text
CLI / MCP ---> signed evidence entries ---> evidra-api ---> Postgres
    |                                                 |
    | local JSONL                                     | tenant-wide replay
    v                                                 v
local scorecard/explain                  self-hosted scorecard/explain

GitOps controllers / webhooks ---> mapped or controller-observed evidence ---^
```

### v1.x (designed, not started)

| Component | Status / notes |
|-----------|----------------|
| Community contribution + percentiles | Planned. No checked-in design doc yet. |
| Agent experiment (multi-model) | Not planned in this snapshot. |
| Fault injection CI job | Planned. No checked-in design doc yet. |
| Scanner mapping lifecycle (Trivy/Checkov/Kubescape) | Planned. Current notes live in this document; no dedicated checked-in design doc yet. |

### v1.1+ (designed, not started — requires signal validation first)

| Component | Description |
|-----------|-------------|
| Intent Graph | Model operations as directed graph (nodes=intents, edges=transitions). Enables: repair_loop detection (`A→B→C→success`), thrashing detection (`A→B→C→A`). Lives inside Signals Engine, no changes to adapters or detectors. |
| Repair bonus | Positive scoring for successful recovery chains. Requires Intent Graph. |
| External scanner mappings | Trivy/Checkov/Kubescape rule → tag mappings (YAML config, loaded at startup via TagProducer) |

---

## Consolidated Implementation Notes

This section preserves the useful implementation notes that previously lived in `V1_IMPLEMENTATION_NOTES.md`.

### Detector Architecture (delivered snapshot)

Package layout:

```text
internal/detectors/
  registry.go
  producer.go
  producers.go
  native_producer.go
  sarif_producer.go
  all/all.go
  k8s/*.go
  terraform/aws/*.go
  terraform/helpers.go
  docker/*.go
  ops/*.go
```

Core model:

- `Detector` is self-registering (`init()` + `Register`).
- One detector pattern lives in one file.
- `TagMetadata` is required for every detector and exported via registry calls.
- `RunAll` provides native deterministic tags.
- `TagProducer` is the extension boundary for non-native sources.
- `ProduceAll` merges producers with de-duplication.

Vocabulary separation:

- resource risk (detectors on artifact content)
- operation risk (detectors on canonical action context)
- behavior signals (signal engine on evidence sequences)

Detectors emit resource/operation risks only; behavioral signals are computed later from prescribe/report sequences.

### Delivered Detector Scope

Current deterministic detector set is 20 tags:

- K8s: privileged, host namespace escape, hostPath, docker socket, run as root, dangerous capabilities, cluster-admin binding, writable rootfs
- Ops: mass delete, kube-system mutation, namespace delete
- Terraform/AWS: wildcard IAM (strict + broad), S3 public access, security group open, RDS public, EBS unencrypted
- Docker/Compose: privileged, host network, socket mount

CLI verification:

```bash
evidra detectors list
```

### Signal + Scoring Rules (delivered snapshot)

Signal pipeline includes 8 behavior signals:

- `protocol_violation`
- `artifact_drift`
- `retry_loop`
- `blast_radius`
- `new_scope`
- `repair_loop`
- `thrashing`
- `risk_escalation`

Score model additions:

- `repair_loop` bonus (negative weight, reduces penalty)
- `thrashing` penalty (positive weight, increases penalty)
- `signal_profiles` map (`none|low|medium|high`) for each signal

Scoring confidence/min-operations behavior remains unchanged (`MinOperations=100`) for production scorecard evaluation. The signal-validation harness calibrates its declared bands with `SCORECARD_MIN_OPERATIONS=1`, so its comparable output does not depend on the production sufficiency threshold.

### Validation Gate

Operational validation scripts:

- `tests/signal-validation/helpers.sh`
- `tests/signal-validation/validate-signals-engine.sh`

The sequence harness covers A-I scenarios, including explicit repair, thrashing, artifact drift, and risk escalation.
Score comparison between scenarios is meaningful only when the harness coverage is complete and the declared calibration threshold is met; compare against the signal-validation bands rather than the production sufficiency threshold.

### Remaining Scope (not delivered in this snapshot)

- Hosted LLM-generated explanation or analysis layers are not part of the
  delivered v1 surface in this repo.
- External scanner mappings are migration-era companion code; production scanners should supply assessment as external enrichment.
- Intent graph is not required for the currently delivered signal set.

---

## Interfaces (stable contracts)

### Adapter Interface

```go
type Adapter interface {
    Name() string
    CanHandle(tool string) bool
    Canonicalize(tool, operation, environment string, rawArtifact []byte) (CanonResult, error)
}
```

### Detector Interface

```go
type Detector interface {
    Tag() string
    BaseSeverity() string
    Detect(action canon.CanonicalAction, raw []byte) bool
    Metadata() TagMetadata
}
```

### TagProducer Interface

```go
// TagProducer is the universal interface for anything that generates risk tags.
// Native detectors are one implementation. External scanners are another.
// The signals engine never knows which producer generated a tag.
type TagProducer interface {
    Name() string
    ProduceTags(action canon.CanonicalAction, raw []byte) []string
}

// Implementations:
//   NativeProducer  — wraps all registered Detector instances
//   SARIFProducer   — maps scanner ruleId → Evidra tag via YAML config
```

### Assessor Interface

```go
// Assessor produces risk inputs from a canonical action and raw artifact.
// Implementations: MatrixAssessor, DetectorAssessor, SARIFAssessor.
type Assessor interface {
    Name() string
    Assess(ctx context.Context, action canon.CanonicalAction, raw []byte) ([]RiskInput, error)
}
```

### Signal Detector Interface

```go
type SignalDetector interface {
    Name() string
    Detect(entries []Entry) []Signal
}
```

### Evidence Entry (wire format)

```json
{
  "entry_id": "01HXY...",
  "type": "prescribe|report|finding|signal",
  "session_id": "sess_01HXY...",
  "timestamp": "2026-03-10T14:00:00Z",
  "actor": { "type": "ai", "id": "claude-code", "provenance": "anthropic" },
  "payload": { ... },
  "artifact_digest": "sha256:...",
  "intent_digest": "sha256:...",
  "prev_hash": "sha256:...",
  "signature": "ed25519:..."
}
```

### Scorecard Output

```json
{
  "score": 97.5,
  "band": "good",
  "sufficient": true,
  "signals": {
    "retry_loop": { "count": 3, "rate": 0.15 },
    "protocol_violation": { "count": 0, "rate": 0.0 },
    "artifact_drift": { "count": 0, "rate": 0.0 },
    "thrashing": { "count": 0, "rate": 0.0 },
    "blast_radius": { "count": 0, "rate": 0.0 },
    "risk_escalation": { "count": 0, "rate": 0.0 },
    "new_scope": { "count": 1, "rate": 0.05 },
    "repair_loop": { "count": 0, "rate": 0.0 }
  },
  "total_operations": 20,
  "risk_summary": {
    "tags_detected": ["k8s.privileged_container", "k8s.run_as_root"],
    "max_risk_level": "critical"
  }
}
```

---

## Architecture Principle: Recorder + Intelligence

Evidra separates two concerns:

**Recorder** (write path, real-time):
- Ingest evidence entries from any source
- Normalize declared intent and optional external enrichment
- Sign entries with Ed25519, chain via previous_hash
- Store in JSONL (local) or Postgres (self-hosted)

**Intelligence** (read path, post-hoc):
- Signal detection across evidence sequences
- Scoring (weighted penalty → 0-100 reliability metric)
- Analytics (scorecards, explain, trends)

The recorder is on the hot path — it must be fast. The intelligence layer
operates on stored evidence and can be async.

## Observation Modes

Evidra connects to infrastructure automation through three patterns. All
produce the same evidence entries and feed the same intelligence pipeline.

| Mode | How Evidra connects | Prescribe? | Assessment? |
|------|-------------------|-----------|------------|
| **MCP direct** | Agent calls `prescribe_full`/`prescribe_smart` + `report` via MCP tools | Yes | Optional external |
| **MCP proxy** | `evidra-mcp --proxy` wraps upstream MCP server on stdio, intercepts `tools/call` | Implicit | Observed only |
| **OTLP bridge** | Reads AgentGateway OTLP traces, translates to prescribe/report ingest | Implicit | Observed only |
| **Ext-authz** (future) | Gateway supplies assessment before forwarding tool call | Yes | External |

MCP direct gives the richest evidence (intent + optional artifact digest before
execution). Proxy and bridge are passive taps — they record what happened
without the agent knowing. Ext-authz can attach external assessment while the
agent remains unchanged.

## Access Points

| Interface | Consumer | Protocol |
|-----------|----------|----------|
| **CLI** (`evidra record/import/scorecard/explain`) | CI pipelines, bash scripts, human operators | Shell + JSONL evidence files |
| **MCP Server** (`evidra-mcp`) | AI agents (Claude Code, Cursor, custom) | JSON-RPC over stdio (`prescribe_full`, `prescribe_smart`, `report`, `get_event`) |
| **Self-hosted API** | Forwarded evidence, webhook sources, self-hosted analytics consumers | HTTP + JSON |
| **OTLP bridge** (separate repo) | AgentGateway telemetry | gRPC/HTTP OTLP → Evidra ingest |

All share the same evidence model and analytics path. Same detectors, same signals, same scorecard. Different entry points and storage boundaries.

---

## What Evidra Is NOT

| Not This | Why | What Instead |
|----------|-----|-------------|
| Security scanner | Trivy/Checkov already exist | Operational reliability measurement |
| AI agent | Does not make decisions | Records and measures decisions |
| Policy engine | Does not block operations | Scores operations after the fact |
| Testing framework | Does not test code correctness | Measures operational behavior |
| Monitoring tool | Does not watch runtime metrics | Analyzes operation evidence chains |

**Evidra is a flight recorder + reliability score for infrastructure automation.**
**Evidra learns from patterns, not from your infrastructure.**

It answers one question: **"Is this automation operating reliably?"**

Not "is the config correct?" (scanner). Not "should we allow this?" (policy). Not "what happened at runtime?" (monitoring). But: **"Over this sequence of operations, how reliably did the automation follow protocol, avoid drift, avoid loops, and control blast radius?"**
