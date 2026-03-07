# Evidra v1 Architecture

**One page. The complete system.**

---

## Pipeline

```
                    ┌─────────────────────────────────────────────────────────────┐
                    │                                                             │
   Artifact         │   ADAPTERS              DETECTORS           SIGNALS ENGINE  │
   (YAML/JSON/HCL)  │                                                             │
        │           │   ┌──────────┐          ┌──────────────┐                    │
        ▼           │   │ K8s      │    ┌────▸│ k8s/         │                    │
   ┌─────────┐      │   │ Terraform│    │     │  privileged  │                    │
   │ prescribe│─────▸│   │ Docker   │────┘     │  hostpath    │──▸ risk_tags       │
   │         │      │   │ Generic  │          │  docker_sock │                    │
   └─────────┘      │   └──────────┘          │  run_as_root │                    │
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
        │           │                    risk_tags + canonical_action              │
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
   │         │◀─────│── report entry (exit_code, artifact_digest)                 │
   └─────────┘      │                                                             │
        │           │                                                             │
        │           │         ┌────────────────────────────────────┐              │
        ▼           │         │ SIGNALS ENGINE                     │              │
   evidence         │         │                                    │              │
   entries ────────▸│         │  ┌─────────────────┐               │              │
                    │         │  │ retry_loop       │  same intent │              │
                    │         │  │                  │  repeated    │              │
                    │         │  │                  │  after fail  │──▸ signal    │
                    │         │  └─────────────────┘               │              │
                    │         │  ┌─────────────────┐               │              │
                    │         │  │ protocol_violat  │  prescribe   │              │
                    │         │  │                  │  without     │              │
                    │         │  │                  │  report      │──▸ signal    │
                    │         │  └─────────────────┘               │              │
                    │         │  ┌─────────────────┐               │              │
                    │         │  │ artifact_drift   │  digest at   │              │
                    │         │  │                  │  report ≠    │              │
                    │         │  │                  │  prescribe   │──▸ signal    │
                    │         │  └─────────────────┘               │              │
                    │         │  ┌─────────────────┐               │              │
                    │         │  │ blast_radius     │  destroy     │              │
                    │         │  │                  │  many        │              │
                    │         │  │                  │  resources   │──▸ signal    │
                    │         │  └─────────────────┘               │              │
                    │         │  ┌─────────────────┐               │              │
                    │         │  │ new_scope        │  new tool/   │              │
                    │         │  │                  │  env combo   │              │
                    │         │  │                  │  first seen  │──▸ signal    │
                    │         │  └─────────────────┘               │              │
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
                    │                 │             │         fair/poor)           │
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
  blast_radius                new_scope
  repair_loop (+)             thrashing (-)
```

Resource/operation tags = what the code looks like (static).
Behavior signals = how the automation operates (dynamic).
**Signals are the product. Tags are the vocabulary.**

Architecture principle: **graph-ready, graph-free.** Signals work on `[]Entry` sequences using intent_digest + artifact_digest + exit_code. No graph data structure needed. Intent Graph can be added later as optimization, but current signals don't require it.

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

7. Agent calls: evidra report --prescription 01HXY... --exit-code 1

8. Evidence entry written:
     type=report, prescription_id=01HXY..., exit_code=1

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
| 7 signal detectors | `internal/signal/` | Stable |
| Scorecard + explain | `internal/score/` | Stable |
| TagProducer chain | `internal/detectors/{producer.go,producers.go}` | Stable |
| MCP server | `pkg/mcpserver/` | Stable |
| Ed25519 signing | `pkg/evidence/` | Stable |
| Hash chain | `pkg/evidence/` | Stable |

### v1.0 (in progress)

| Component | Document | Status |
|-----------|----------|--------|
| Detector architecture (registry, metadata, producer chain) | V1_IMPLEMENTATION_NOTES | Delivered |
| Docker adapter + Docker detectors | V1_IMPLEMENTATION_NOTES | Delivered |
| Signal validation harness (A-G scenarios) | V1_IMPLEMENTATION_NOTES | Delivered (score sufficiency still gated by operation count) |
| REST API + LLM augmentation | [2026-03-07-parallel-execution-implementation-plan.md](../plans/2026-03-07-parallel-execution-implementation-plan.md) | In progress |
| LLM tag discovery | LLM_RISK_PREDICTION_CONTRACT | Architecture done |
| MCP contract prompts | MCP_CONTRACT_PROMPTS | Ready to commit |
| Signal validation | `tests/signal-validation/` scripts | Running in CI/manual flows |

### v1.x (designed, not started)

| Component | Document |
|-----------|----------|
| Community contribution + percentiles | COMMUNITY_BENCHMARK_DESIGN |
| Benchmark dataset (corpus + cases) | DATASET_ARCHITECTURE |
| Agent experiment (multi-model) | EXPERIMENT_DESIGN |
| Fault injection CI job | FAULT_INJECTION_RUNBOOK |
| Scanner mapping lifecycle (Trivy/Checkov/Kubescape) | V1_IMPLEMENTATION_NOTES |

### v1.1+ (designed, not started — requires signal validation first)

| Component | Description |
|-----------|-------------|
| Intent Graph | Model operations as directed graph (nodes=intents, edges=transitions). Enables: repair_loop detection (`A→B→C→success`), thrashing detection (`A→B→C→A`). Lives inside Signals Engine, no changes to adapters or detectors. |
| Repair bonus | Positive scoring for successful recovery chains. Requires Intent Graph. |
| External scanner mappings | Trivy/Checkov/Kubescape rule → tag mappings (YAML config, loaded at startup via TagProducer) |

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
  "score": 62.4,
  "band": "fair",
  "sufficient": true,
  "signals": {
    "retry_loop": { "count": 3, "rate": 0.15, "weight": 0.20 },
    "protocol_violation": { "count": 0, "rate": 0.0, "weight": 0.35 },
    "artifact_drift": { "count": 0, "rate": 0.0, "weight": 0.30 },
    "blast_radius": { "count": 0, "rate": 0.0, "weight": 0.10 },
    "new_scope": { "count": 1, "rate": 0.05, "weight": 0.05 }
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
| **CLI** (`evidra prescribe/report/scorecard`) | CI pipelines, bash scripts, human operators | Shell + JSONL evidence files |
| **MCP Server** (`evidra-mcp`) | AI agents (Claude Code, Cursor, custom) | JSON-RPC over stdio |
| **REST API** (`/api/v1/assess`) | Web UI, integrations, LLM augmentation layer | HTTP + JSON |

All three share the same engine. Same detectors, same signals, same scorecard. Different entry points.

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
