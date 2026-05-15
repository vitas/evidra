# ext-audit System Design

How `ext-audit` fits the MCP ecosystem and what it means for Evidra.

## The Extension

`ext-audit` is a minimal, neutral audit event for MCP tool calls:

```json
{
  "method": "audit/toolCall",
  "params": {
    "timestamp": "...",
    "sessionId": "...",
    "sequence": 7,
    "tool": "apply_yaml",
    "argumentsDigest": "sha256:...",
    "status": "success",
    "durationMs": 1250,
    "actor": { "type": "agent", "id": "claude-code" },
    "trace": { "traceId": "...", "spanId": "..." },
    "metadata": {}
  }
}
```

Facts only. No classification, no risk, no scoring. Any audit consumer
can ingest it.

## What the Standard Provides

```
MCP Server
    │
    │ audit/toolCall notification (on MCP transport)
    │
    ▼
Any consumer:
    ├── Compliance logger (SOC2, immutable log)
    ├── SIEM (Datadog, Splunk, via metadata + trace correlation)
    ├── Reliability scorer (Evidra)
    └── Cost tracker (token counts in metadata)
```

One event format. No proxies, no bridges, no collectors needed for
basic audit trail. The MCP transport carries it.

## What Evidra Adds on Top

The standard gives facts. Evidra gives judgment.

```
ext-audit event (facts)
    │
    ▼
Evidra consumes via POST /v1/evidence/ingest/audit
    │
    ├── Record: declared intent + optional external canonical/assessment fields
    ├── Chain: cryptographically linked evidence entries
    │
    ▼
Evidence store
    │
    ├── Signal detection: retry_loop, blast_radius, protocol_violation...
    ├── Scoring: weighted penalty → 0-100 reliability metric
    ├── Benchmarking: run comparison, leaderboards
    └── Analytics: scorecards, explain, trends
```

### Mapping

| ext-audit field | Evidra derives |
|----------------|----------------|
| `tool` | `canonical_action.tool` + inferred `operation_class` |
| `argumentsDigest` | `artifact_digest` |
| `status` | `report.verdict` (success/failure/error) |
| `durationMs` | stored in scope_dimensions |
| `sessionId` | `session_id` |
| `sequence` | ordering for signal detection |
| `actor` | `actor` |
| `trace` | `trace_id`, `span_id` |
| `metadata.operationClass` | `canonical_action.operation_class` (if provided) |
| `metadata.scopeClass` | `canonical_action.scope_class` (if provided) |
| `metadata.riskLevel` | `effective_risk` (if provided) |

When `metadata` contains classification fields, Evidra uses them
directly. When absent, Evidra infers from the tool name using its
own canonicalization.

## What Evidra Drops

If `ext-audit` is adopted:

| Component | Status |
|-----------|--------|
| **OTLP bridge** (evidra-agentgateway-bridge) | Deprecated — events come on MCP transport |
| **OTel Collector** (gRPC→HTTP converter) | Deprecated — no OTLP path needed |
| **MCP proxy interception** | Fallback only — for servers without ext-audit |
| **Custom OTLP span mapping** | Deprecated — standard event format |

Evidra becomes a pure consumer + intelligence layer. Fewer components,
fewer repos, simpler deployment.

## What Evidra Keeps

| Capability | Why ext-audit doesn't replace it |
|-----------|--------------------------------|
| **Cryptographic evidence chain** | ext-audit events are ephemeral notifications; Evidra chains them with hashes and signatures |
| **Behavioral signal detection** | ext-audit provides facts; pattern detection requires an engine |
| **Reliability scoring** | No standard score — that's the intelligence layer |
| **Benchmarking / comparison** | ext-audit doesn't define run comparison or leaderboards |
| **Optional assessment enrichment** | Risk assessment is consumer logic, not audit format |
| **Prescribe (pre-flight intent)** | ext-audit is post-execution; Evidra's MCP tools offer pre-flight intent recording |

## How Evidra's MCP Tools Coexist with ext-audit

Evidra offers two modes that serve different needs:

**ext-audit (passive):** Server emits events. Evidra consumes them.
Agent doesn't know Evidra exists. Post-execution only.

**Evidra MCP tools (active):** Agent calls `prescribe_smart` or
`prescribe_full` before execution, gets a prescription ID, then calls `report`
after. Pre-flight + post-flight. Richer evidence but requires agent awareness.

Both feed the same evidence chain and scoring pipeline. Teams choose
based on how much agent integration they want:

```
Zero integration:   ext-audit events → Evidra (passive)
Light integration:  ext-audit + metadata hints → Evidra (enriched passive)
Full integration:   Evidra MCP tools → Evidra (active prescribe/report)
```

## Migration Path

### Phase 1: Propose ext-audit

- Submit SEP to MCP community
- Build prototype emitter as MCP SDK middleware
- Evidra consumes ext-audit events alongside existing paths

### Phase 2: Adoption

- Major MCP servers add ext-audit capability
- AgentGateway forwards ext-audit events natively
- Evidra implements `POST /v1/evidence/ingest/audit`

### Phase 3: Simplify

- Deprecate OTLP bridge (events come on MCP transport)
- Deprecate OTel Collector requirement
- MCP proxy becomes fallback-only for legacy servers

### Phase 4: Evidra as Metadata Convention

- Publish recommended `metadata` fields for reliability tools:
  `operationClass`, `scopeClass`, `resourceIdentity`
- Servers that want richer Evidra analysis include these in metadata
- Servers that don't care omit them — base audit still works

## Open Questions for MCP Community

1. **Notification method name:** `audit/toolCall` follows MCP's
   namespace convention. Alternative: `notifications/audit`.

2. **Should there be an `audit/session` event?** Session start/end
   boundaries would help consumers that process batches. Or is
   `sessionId` on each event sufficient?

3. **Should `argumentsDigest` be required or optional?** Required
   enables duplicate detection universally. Optional keeps the event
   even simpler.

4. **Should servers include `tools/call` result summary in the event?**
   Current proposal omits tool output (privacy). A `resultDigest`
   field could enable drift detection without exposing content.

5. **How does this interact with the proposed Triggers extension?**
   If MCP adds server-to-client event delivery, audit events could
   use that mechanism instead of bare notifications.
