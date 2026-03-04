# Evidra Codebase Review — Architecture × Code Alignment

## Verdict

The code is **real**. 7005 lines of Go, 114 tests, working MCP
server, working CLI, two adapters, five signal detectors, score
computation, evidence chain with hash-linking and Ed25519 signing.
This is not a prototype — it's a functioning v0.3.0 candidate
with specific gaps to close.

---

## What's done right

**Type system matches the architecture.** CanonicalAction, Prescription,
Report are clean structs. risk_tags correctly NOT in CanonicalAction
(in Prescription wrapper). Adapter interface matches the contract
(Name, CanHandle, Canonicalize). SelectAdapter fallback to GenericAdapter.

**Signal detectors are pure functions.** All five detectors take
[]Entry and return SignalResult. No side effects, no I/O. Protocol
violation handles all four sub-signals (unreported, unprescribed,
duplicate, cross-actor). TTL detection at scorecard time, not real-time.
Stalled vs crash classification exists.

**Canonicalization is production-quality.** K8s adapter: multi-doc
YAML split, identity extraction, sorted by (apiVersion, kind,
namespace, name), noise removal, shape_hash. Terraform adapter:
parses terraform-json Plan, extracts ResourceChanges, sorts by
type+name, computes shape_hash from structured entries.

**Evidence chain works.** Segmented storage with manifest, hash-linking,
Ed25519 signing, file locking (Unix flock), forwarder with cursor
state. This is real operational code.

**MCP server is correct.** Prescribe + Report + GetEvent tools.
Embedded JSON schemas. Pre-canonicalized path (optional canonical_action).
Retry tracker. Evidence writing on both prescribe and report.

**Tests are comprehensive.** 114 test functions across canon (18),
evidence (24 payload + 16 signer + 12 builder), risk (24),
signal (20), score (7), MCP server (5). Golden corpus with
10 cases (6 K8s + 4 Terraform) with frozen digests.

**OPA fully removed.** Zero references to OPA, Rego, policy engine,
deny, validate tool. Clean migration.

---

## GAPS — must fix before v0.3.0

### GAP 1: Dual evidence type system (CRITICAL)

Two separate `EvidenceRecord` structs:

```
internal/evidence/types.go → EvidenceRecord (with ServerID, TenantID, SigningPayload)
pkg/evidence/types.go      → EvidenceRecord (with PolicyRef, BundleRevision, ProfileName)
```

Plus `pkg/evidence/types.go` has `type Record = EvidenceRecord`
aliasing its own version.

The internal version has OPA-era fields (PolicyRef, BundleRevision).
The pkg version has OPA-era fields too (PolicyRef, BundleRevision,
ProfileName).

**Neither matches the benchmark architecture.** A benchmark evidence
record should be:

```go
type EvidenceEntry struct {
    EventID        string          `json:"event_id"`
    Timestamp      time.Time       `json:"ts"`
    EntryType      string          `json:"type"` // "prescription" | "report"
    ActorID        string          `json:"actor_id"`
    Tool           string          `json:"tool"`
    Operation      string          `json:"operation"`
    CanonicalAction *CanonicalAction `json:"canonical_action,omitempty"`
    ArtifactDigest string          `json:"artifact_digest"`
    IntentDigest   string          `json:"intent_digest,omitempty"`
    RiskLevel      string          `json:"risk_level,omitempty"`
    RiskTags       []string        `json:"risk_tags,omitempty"`
    PrescriptionID string          `json:"prescription_id,omitempty"` // reports only
    ExitCode       *int            `json:"exit_code,omitempty"`       // reports only
    PreviousHash   string          `json:"previous_hash"`
    Hash           string          `json:"hash"`
    Signature      string          `json:"signature"`
}
```

**Fix:** Unify into one type. Remove PolicyRef, BundleRevision,
ProfileName, ServerID, TenantID, PolicyDecision. These are OPA
remnants.

### GAP 2: PolicyDecision struct still exists

```go
type PolicyDecision struct {
    Allow     bool     `json:"allow"`
    RiskLevel string   `json:"risk_level"`
    Reason    string   `json:"reason"`
    ...
}
```

Used in MCP server prescribe handler. This is an OPA concept
(allow/deny decision). In the benchmark architecture, prescribe
returns risk_level and risk_tags — there is no "decision" and
no "allow" field.

**Fix:** Remove PolicyDecision. Put risk_level and risk_tags
directly in the evidence entry.

### GAP 3: ScopeClass doesn't match the contract

The contract defines scope_class as:
```
production | staging | development | unknown
```

Code returns:
```go
func ScopeClass(resources []ResourceID) string {
    if len(resources) == 0 { return "unknown" }
    if len(resources) == 1 { return "single" }
    if len(namespaces) <= 1 { return "namespace" }
    return "cluster"
}
```

"single", "namespace", "cluster" are NOT scope classes. They're
resource scope (blast radius concern). Scope class should be
derived from environment/namespace substring matching:

```
contains "prod" → production
contains "stag" → staging
contains "dev"  → development
else            → unknown
```

**Fix:** Rename current ScopeClass to ResourceScope. Add real
ScopeClass from environment field or namespace substring matching.

### GAP 4: CLI scorecard is a stub

```go
func cmdScorecard(args []string, stdout, stderr io.Writer) int {
    // Demo with empty entries
    results := signal.AllSignals(nil)
    sc := score.Compute(results, 0)
    ...
}
```

Scorecard doesn't read evidence. It runs signals on nil entries
and prints an empty scorecard. The score computation logic works
(tested), but the CLI can't actually load evidence entries and
run signals against them.

**Fix:** Add evidence → signal.Entry conversion. Read evidence
from --evidence-dir, convert to []signal.Entry, run AllSignals,
compute score.

### GAP 5: CLI prescribe/report don't write evidence

```go
func cmdPrescribe(args []string, stdout, stderr io.Writer) int {
    // read flags...
    cr := canon.Canonicalize(tool, operation, raw)
    riskTags := risk.RunAll(cr.CanonicalAction, raw)
    riskLevel := risk.RiskLevel(cr.CanonicalAction.OperationClass, cr.CanonicalAction.ScopeClass)
    // output JSON to stdout
    // NO evidence writing
}
```

MCP server writes evidence. CLI doesn't. This means `evidra
prescribe` + `evidra report` + `evidra scorecard` don't form a
working pipeline yet.

**Fix:** Add evidence writing to CLI prescribe/report, same as
MCP server does.

### GAP 6: No scanner_report / SARIF integration

Per the integration roadmap, `--scanner-report` accepting SARIF
is a v0.3.0 deliverable. Not implemented yet.

**Fix:** Add SARIF parser, --scanner-report flag to CLI prescribe,
scanner_report field to MCP prescribe schema.

---

## GAPS — should fix before v0.3.0

### GAP 7: Blast radius thresholds don't match spec

Code: `BlastRadiusDestructive = 10`, `BlastRadiusMutating = 50`

Signal Spec says: default threshold 5 resources for destructive,
mutating not flagged.

**Fix:** Change to `BlastRadiusDestructive = 5`, remove mutating
threshold (spec says "Only fires on destructive operations").

### GAP 8: New scope key is too narrow

Code uses `(tool, operation_class)`:
```go
type scopeKey struct{ tool, opClass string }
```

Signal Spec defines scope_key as `(actor.id, tool, operation_class, scope_class)`.
Missing actor_id and scope_class means new_scope fires wrong.

**Fix:** Add actor_id and scope_class to scopeKey.

### GAP 9: Retry loop window default mismatch

Code: `DefaultRetryWindow = 10 * time.Minute`
Signal Spec: `window = 30 min`

**Fix:** Change to 30 minutes.

### GAP 10: No evidence → signal.Entry bridge

signal.Entry has fields (EventID, Timestamp, Tool, IsPrescription,
IsReport, PrescriptionID, ArtifactDigest, IntentDigest, ShapeHash,
etc.) but there's no function to convert evidence records to
signal entries. The MCP server and CLI need this to run signals
at report time.

**Fix:** Add `func EntryFromRecord(r evidence.Record) signal.Entry`.

---

## OPA Remnants to clean

| Location | Remnant | Fix |
|----------|---------|-----|
| pkg/evidence/types.go | PolicyDecision, PolicyRef, BundleRevision | Remove |
| internal/evidence/types.go | PolicyRef, BundleRevision, ServerID, TenantID | Remove |
| internal/evidence/decision.go | Entire file | Remove |
| internal/evidence/payload.go | writeField policy_ref, bundle_revision | Remove |
| internal/evidence/builder.go | PolicyRef, BundleRevision assignment | Remove |
| pkg/mcpserver/server.go | PolicyDecision{Allow: true, ...} | Replace with direct risk_level |

---

## What's well-engineered

| Component | Lines | Quality | Notes |
|-----------|-------|---------|-------|
| internal/canon/ | 580 | Excellent | Both adapters correct, noise removal solid |
| internal/signal/ | 470 | Good | All 5 detectors implemented, pure functions |
| internal/risk/ | 410 | Good | 7 detectors, correct interface (action + raw bytes) |
| internal/score/ | 130 | Good | Weighted sum, bands, Jaccard overlap |
| pkg/mcpserver/ | 690 | Good | Working MCP server, retry tracker |
| pkg/evidence/ | 850 | Functional | Segmented storage works but types need cleanup |
| tests/ | 2100 | Comprehensive | 114 tests, 10 golden cases |
| cmd/ | 360 | Partial | MCP works, CLI is stub-ish |

---

## Project naming

`evidra-benchmark` is correct. Here's why:

| Option | Assessment |
|--------|------------|
| `evidra-benchmark` | Accurate. The product IS a benchmark. Matches "Evidra Agent Reliability Benchmark." |
| `evidra` | Too broad. Sounds like a company/platform, not a tool. |
| `evidra-signals` | Too narrow. Signals are one component, not the product. |
| `evidra-spec` | Sounds like a spec doc, not running code. |
| `evidra-telemetry` | Confusing. Implies it collects metrics (Prometheus territory). |

**Keep `evidra-benchmark`.** The module path `samebits.com/evidra-benchmark`
is fine. The binaries are `evidra` (CLI) and `evidra-mcp` (server) —
users see those, not the repo name.

One change: `server.json` has `"name": "io.github.vitas/evidra-benchmark"`.
For OSS release, decide: `io.github.evidra-io/evidra-benchmark` or
keep personal namespace. Not blocking for v0.3.0.

---

## Implementation readiness scorecard

| Area | Ready? | Blocking? | Work |
|------|--------|-----------|------|
| Canonicalization (K8s, TF, Generic) | YES | — | Done |
| Signal detectors (5/5) | 90% | Minor | Fix thresholds, scope_key |
| Risk detectors (7) | YES | — | Done |
| Score computation | YES | — | Done |
| MCP server (prescribe/report) | YES | — | Done |
| Evidence chain (append, hash, sign) | YES | — | Done |
| Evidence types (unified) | NO | **BLOCKING** | ~2 days |
| CLI pipeline (prescribe→report→scorecard) | NO | **BLOCKING** | ~2 days |
| SARIF scanner integration | NO | Nice-to-have | ~3 days |
| Golden corpus | YES | — | 10 cases |
| Docker images | YES | — | Both Dockerfiles exist |
| server.json / MCP registry | YES | — | Done |

**Estimated work to v0.3.0-rc1: ~5-7 days for one developer.**

Two blocking items: unify evidence types (remove OPA remnants)
and wire CLI to evidence store. Everything else is threshold
adjustments and new features.

---

## Recommended fix order

1. **Unify evidence types** — single EvidenceEntry struct, remove
   PolicyDecision, PolicyRef, BundleRevision. This touches the
   most files but is purely mechanical.

2. **Wire CLI to evidence** — prescribe/report write to evidence,
   scorecard reads from evidence. This completes the local pipeline.

3. **Fix signal parameters** — blast radius threshold (5 not 10),
   retry window (30 min not 10), scope_key (add actor_id + scope_class).

4. **Fix ScopeClass** — environment-based (prod/staging/dev/unknown)
   not resource-count-based.

5. **Add evidence→signal.Entry bridge** — converter function that
   both CLI and MCP server use.

6. **SARIF integration** — parser + --scanner-report flag. Can be
   v0.3.1 if time is tight.
