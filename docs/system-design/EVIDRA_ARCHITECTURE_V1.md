# Evidra Architecture

- Status: Normative
- Version: v1.0
- Canonical for: system overview and component boundaries
- Audience: public

**One page. The complete system.**
**Consolidated source:** delivered implementation notes from removed `V1_IMPLEMENTATION_NOTES.md` are preserved in this document.

---

## Pipeline

```
                    ┌─────────────────────────────────────────────────────────────┐
                    │                                                             │
   Artifact         │   ADAPTERS              DETECTORS           SIGNALS ENGINE  │
   (YAML/JSON/HCL)  │                                                             │
        │           │   ┌──────────┐          ┌──────────────┐                    │
        ▼           │   │ K8s      │    ┌────▸│ k8s/         │                    │
   ┌──────────┐     │   │ Terraform│    │     │  privileged  │                    │
   │ prescribe│────▸│   │ Docker   │────┘     │  hostpath    │──▸ risk_tags       │
   │          │     │   │ Generic  │          │  docker_sock │                    │
   └──────────┘     │   └──────────┘          │  run_as_root │                    │
        │           │        │                │  ...         │                    │
        │           │        ▼                ├──────────────┤                    │
        │           │   CanonicalAction       │ terraform/   │                    │
        │           │   + ArtifactDigest      │  aws/        │                    │
        │           │   + IntentDigest        │    s3_public │──▸ risk_tags       │
        │           │                         │    iam_wild  │                    │
        │           │                         │  gcp/        │                    │
        │           │                         │  azure/      │                    │
        │           │                         ├──────────────┤                    │
        │           │                         │ ops/         │                    │
        │           │                         │  mass_delete │──▸ risk_tags       │
        │           │                         │  kube_system │                    │
        │           │                         ├──────────────┤                    │
        │           │                         │ docker/      │                    │
        │           │                         │  privileged  │──▸ risk_tags       │
        │           │                         └──────────────┘                    │
        │           │                                │                            │
        │           │                    risk_tags + canonical_action             │
        │           │                                │                            │
        │           │                         ┌──────▼──────┐                     │
        │           │                         │ Risk Matrix │                     │
        │           │                         │             │                     │
        │           │                         │ base_sev    │                     │
        │           │                         │ × op_class  │──▸ risk_level       │
        │           │                         │ × scope     │                     │
        │           │                         └─────────────┘                     │
        │           │                                                             │
        ▼           │                                                             │
   ┌─────────┐      │                                                             │
   │ EVIDENCE│◀─────│── prescribe entry (risk_tags, risk_level, digests)          │
   │ CHAIN   │      │                                                             │
   │ (JSONL) │      │                                                             │
   │         │◀─────│── report entry (verdict, exit_code?, decision_context?, artifact_digest) │
   └─────────┘      │                                                             │
        │           │                                                             │
        │           │         ┌────────────────────────────────────┐              │
        ▼           │         │ SIGNALS ENGINE                     │              │
   evidence         │         │                                    │              │
   entries ────────▸│         │  ┌──────────────────┐              │              │
                    │         │  │ retry_loop       │  same intent │              │
                    │         │  │                  │  repeated    │              │
                    │         │  │                  │  after fail  │──▸ signal    │
                    │         │  └──────────────────┘              │              │
                    │         │  ┌──────────────────┐              │              │
                    │         │  │ protocol_violat  │  prescribe   │              │
                    │         │  │                  │  without     │              │
                    │         │  │                  │  report      │──▸ signal    │
                    │         │  └──────────────────┘              │              │
                    │         │  ┌──────────────────┐              │              │
                    │         │  │ artifact_drift   │  digest at   │              │
                    │         │  │                  │  report ≠    │              │
                    │         │  │                  │  prescribe   │──▸ signal    │
                    │         │  └──────────────────┘              │              │
                    │         │  ┌──────────────────┐              │              │
                    │         │  │ blast_radius     │  destroy     │              │
                    │         │  │                  │  many        │              │
                    │         │  │                  │  resources   │──▸ signal    │
                    │         │  └──────────────────┘              │              │
                    │         │  ┌──────────────────┐              │              │
                    │         │  │ new_scope        │  new tool/   │              │
                    │         │  │                  │  env combo   │              │
                    │         │  │                  │  first seen  │──▸ signal    │
                    │         │  └──────────────────┘              │              │
                    │         │  ┌──────────────────┐              │              │
                    │         │  │ repair_loop      │  delete then │              │
                    │         │  │                  │  recreate    │              │
                    │         │  │                  │ same resource│──▸ signal    │
                    │         │  └──────────────────┘              │              │
                    │         │  ┌──────────────────┐              │              │
                    │         │  │ thrashing        │  rapid apply │              │
                    │         │  │                  │  /delete     │              │
                    │         │  │                  │  cycles      │──▸ signal    │
                    │         │  └──────────────────┘              │              │
                    │         │  ┌──────────────────┐              │              │
                    │         │  │ risk_escalation  │  risk level  │              │
                    │         │  │                  │  exceeds     │              │
                    │         │  │                  │  baseline    │──▸ signal    │
                    │         │  └──────────────────┘              │              │
                    │         └────────────────────────────────────┘              │
                    │                        │                                    │
                    │              signal counts + rates                          │
                    │                        │                                    │
                    │                 ┌──────▼──────┐                             │
                    │                 │ SCORECARD   │                             │
                    │                 │             │                             │
                    │                 │ weighted    │                             │
                    │                 │ penalty     │──▸ score (0-100)            │
                    │                 │ model       │──▸ band (excellent/good/    │
                    │                 │             │         fair/poor)          │
                    │                 └─────────────┘                             │
                    │                                                             │
                    └─────────────────────────────────────────────────────────────┘
                                             │
                              ┌──────────────┼──────────────┐
                              ▼              ▼              ▼
                         ┌────────┐    ┌──────────┐   ┌──────────┐
                         │  CLI   │    │ MCP      │   │ REST API │
                         │        │    │ Server   │   │          │
                         │scorecard│   │prescribe │   │/assess   │
                         │explain │    │report    │   │/scorecard│
                         │validate│    │get_event │   │+ LLM     │
                         └────────┘    └──────────┘   └──────────┘
                                            │              │
                                       AI Agents      LLM Layer
                                       (Claude Code,  (tag discovery,
                                        Cursor, etc)   explanation)
```

---

## Layers

| # | Layer | Input | Output | What It Does |
|---|-------|-------|--------|-------------|
| 1 | **Adapters** | Raw artifact + tool name | CanonicalAction + digests | Normalizes YAML/JSON/HCL into structured representation |
| 2 | **Detectors** | CanonicalAction + raw bytes | risk_tags[] | Pattern-matches misconfigs. One file, one tag, self-registering |
| 3 | **Risk Matrix** | risk_tags + op_class + scope | risk_level | Computes severity from base_severity × context |
| 4 | **Evidence Chain** | prescribe + report entries | Signed JSONL segments | Tamper-evident append-only log of all operations |
| 5 | **Signals Engine** | Evidence entries (sequence) | signal counts + rates | Detects behavioral patterns across operation sequences |
| 6 | **Scorecard** | Signal counts + rates | score (0-100) + band | Weighted penalty model → reliability metric |

**Layers 1-3** fire at prescribe time (per operation, instant).
**Layers 4-5** accumulate over a session (sequence of operations).
**Layer 6** evaluates at session end.

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

2. K8s adapter parses YAML → CanonicalAction:
     tool=kubectl, operation=apply, op_class=mutate, scope=staging
     resource_identity=[{kind:Deployment, name:web-app, ns:staging}]
     resource_count=1, artifact_digest=sha256:abc...

3. Detectors scan raw YAML:
     k8s.privileged_container → fires (privileged: true)
     k8s.run_as_root → fires (runAsNonRoot absent)

4. Risk matrix:
     base_severity = max(critical, medium) = critical
     context = mutate × staging → no elevation
     risk_level = critical

5. Evidence entry written:
     type=prescribe, risk_tags=[k8s.privileged_container, k8s.run_as_root],
     risk_level=critical, prescription_id=01HXY...

6. Agent executes kubectl apply → fails (exit_code=1)

7. Agent calls: evidra report --prescription 01HXY... --verdict failure --exit-code 1

8. Evidence entry written:
     type=report, prescription_id=01HXY..., verdict=failure, exit_code=1

9. Agent retries same operation (same artifact, same prescribe, exit_code=1) × 2 more

10. Signals engine (at scorecard time):
      retry_loop: count=3 (same intent_digest, 3 failures)
      protocol_violation: count=0 (all prescribes have reports)
      artifact_drift: count=0 (artifact unchanged)

11. Scorecard:
      score = 100 - (retry_loop_penalty × 3) - (risk_tag_penalties)
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
| K8s adapter | `internal/canon/k8s.go` | Stable |
| Terraform adapter | `internal/canon/terraform.go` | Stable |
| Generic adapter | `internal/canon/generic.go` | Stable |
| 20 risk detectors | `internal/detectors/` | Stable |
| Risk matrix | `internal/risk/matrix.go` | Stable |
| Evidence chain | `pkg/evidence/` | Stable |
| 8 signal detectors | `internal/signal/` | Stable |
| Scorecard + explain | `internal/score/` | Stable |
| TagProducer chain | `internal/detectors/{producer.go,producers.go}` | Stable |
| MCP server | `pkg/mcpserver/` | Stable |
| Ed25519 signing | `pkg/evidence/` | Stable |
| Hash chain | `pkg/evidence/` | Stable |

### v1.0 (in progress)

| Component | Document | Status |
|-----------|----------|--------|
| Detector architecture (registry, metadata, producer chain) | V1_ARCHITECTURE (this doc) | Delivered |
| Docker adapter + Docker detectors | V1_ARCHITECTURE (this doc) | Delivered |
| Signal validation harness (A-G scenarios) | V1_ARCHITECTURE (this doc) | Delivered (score sufficiency still gated by operation count) |
| Self-hosted API | [self-hosted-setup.md](../guides/self-hosted-setup.md) | Delivered for evidence ingestion, browsing, and tenant-wide scorecard/explain |
| Signal validation | `tests/signal-validation/` scripts | Running in CI/manual flows |

CLI and MCP are the primary analytics entry points in v1. Self-hosted also exposes tenant-wide `/v1/evidence/scorecard` and `/v1/evidence/explain` over centralized stored evidence using the same signal and scoring path.

## Self-Hosted Mode

Self-hosted mode keeps the same evidence semantics as local CLI and MCP workflows. What changes is ingress and storage, not the scoring model.

- **Forwarded evidence:** CLI and MCP can append evidence locally or forward the same signed entries to `evidra-api` for centralized storage.
- **Webhook ingress:** ArgoCD and generic webhook sources can submit events to the API, which maps them into the same prescribe/report-oriented evidence model used by local workflows.
- **Centralized store:** Hosted evidence is persisted in Postgres so teams can browse and replay tenant-wide evidence instead of reading per-machine JSONL chains.
- **Shared analytics path:** Hosted `scorecard` and `explain` load stored evidence and run the same signal detectors and scoring engine as local analysis.
- **deliberate refusal:** A deny decision is still explicit evidence, not a side channel. The terminal record remains `report(verdict=declined, decision_context)`, so local and hosted analytics interpret it the same way.

```text
CLI / MCP ---> signed evidence entries ---> evidra-api ---> Postgres
    |                                                 |
    | local JSONL                                     | tenant-wide replay
    v                                                 v
local scorecard/explain                     hosted scorecard/explain

ArgoCD / generic webhooks ---> mapped evidence entries ---^
```

### v1.x (designed, not started)

| Component | Document |
|-----------|----------|
| Community contribution + percentiles | COMMUNITY_BENCHMARK_DESIGN |
| Benchmark dataset (corpus + cases) | DATASET_ARCHITECTURE |
| Agent experiment (multi-model) | EXPERIMENT_DESIGN |
| Fault injection CI job | FAULT_INJECTION_RUNBOOK |
| Scanner mapping lifecycle (Trivy/Checkov/Kubescape) | V1_ARCHITECTURE + LLM_RISK_PREDICTION_CONTRACT |

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

Scoring confidence/min-operations behavior remains unchanged (`MinOperations=100`).

### Validation Gate

Operational validation scripts:

- `tests/signal-validation/helpers.sh`
- `tests/signal-validation/validate-signals-engine.sh`

The sequence harness covers A-G scenarios, including explicit repair and thrashing.
Score comparison between scenarios is meaningful only when operation count reaches scorecard sufficiency (`MinOperations`).

### Remaining Scope (not delivered in this snapshot)

- REST API + hosted LLM integration remains a separate dependency stream.
- External scanner mappings are scaffolded via `TagProducer` and SARIF producer, but need production mapping/config lifecycle.
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

## Access Points

| Interface | Consumer | Protocol |
|-----------|----------|----------|
| **CLI** (`evidra record/import/scorecard`) | CI pipelines, bash scripts, human operators | Shell + JSONL evidence files |
| **MCP Server** (`evidra-mcp`) | AI agents (Claude Code, Cursor, custom) | JSON-RPC over stdio |
| **Self-hosted API** | Forwarded evidence, webhook sources, hosted analytics consumers | HTTP + JSON |

All three share the same evidence model and analytics path. Same detectors, same signals, same scorecard. Different entry points and storage boundaries.

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
