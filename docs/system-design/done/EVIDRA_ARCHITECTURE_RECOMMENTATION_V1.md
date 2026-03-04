# PO Architecture Review — Triage & Response

## How to read this

Each point from the PO review is classified:

- **DONE** — already implemented in code or docs
- **GAP** — real gap, needs work, with priority
- **DEFER** — valid concern, but deliberately out of scope for v0.3.0
- **DISAGREE** — explained why we chose differently

---

## §2 Architecture gaps from ARCHITECTURE_REVIEW.md

### 2.1 Prescription TTL & lifecycle
**Status: DONE (90%)**

TTL detection exists in code (`signal/protocol_violation.go:DetectUnreported`).
Default 10 min. Runs at scorecard time, not real-time. Sub-signals
classify stalled_operation vs crash_before_report.

State machine PRESCRIBED → REPORTED exists implicitly (prescription
entry + report entry in evidence chain). No explicit state field
because evidence is append-only — you reconstruct state by scanning.

| Case | Behavior | Status |
|------|----------|--------|
| Late report (after TTL) | Scorecard flags as unreported_prescription | DONE |
| Missing report | Detected by DetectUnreported at scorecard time | DONE |
| Double report | Detected as duplicate_report protocol violation | DONE |
| Report without prescription | Detected as unprescribed_action | DONE |

**GAP:** No explicit state machine diagram in docs. TTL default
should be 10 min in code (currently correct) but configurable via
`--ttl` flag on scorecard command.

**Action:** P2 — add state diagram to Inspector Model doc. Low
priority because behavior is correct and tested.

### 2.2 Prescription-to-report matching
**Status: DONE**

Four matching rules implemented in `signal/protocol_violation.go`:
1. prescription_id is ULID (globally unique)
2. First report wins, second = duplicate_report violation
3. Unknown prescription_id = unprescribed_action
4. Cross-actor report = cross_actor_report violation

Relationship is strictly 1:1 (one prescription, one report).
No many-to-one or one-to-many — by design.

Batched apply (e.g. terraform apply with 10 resources) = one
prescription with resource_count=10, one report. Not 10 reports.

**GAP:** No run_id / session_id. Not needed for v0.3.0 — the
prescription_id IS the correlation key. Session tracking is a
v0.5.0 concern when evidra-api aggregates across machines.

**Action:** None for v0.3.0.

### 2.3 Integrity during forward
**Status: DEFER to v0.5.0**

Forward integrity is defined in architecture docs but not
implemented. Forwarder code exists (pkg/evidence/forwarder.go)
with cursor-based state, but it's append-only push without
server receipts.

PO suggests batch signing instead of per-entry signing. We
already have per-entry Ed25519 signing (internal/evidence/signer.go).
This is overkill but already built — no reason to rip it out.

**Action:** v0.5.0 — add server receipt entry when evidra-api
receives forwarded batch. Don't change signing model.

### 2.4 Actor identity
**Status: PARTIALLY DONE — GAP exists**

Current actor model:
```go
type Actor struct {
    Type   string `json:"type"`   // "agent", "ci", "human"
    ID     string `json:"id"`     // stable id
    Origin string `json:"origin"` // "mcp", "cli", "api"
}
```

Plus actor_meta (optional map: agent_version, model_id, prompt_id).

**MISSING:** auth_context, provenance source. PO is right that
enterprise needs "who says this actor IS this actor?"

But for v0.3.0 this is acceptable:
- CLI: actor is whoever runs the command (CI runner, human)
- MCP: actor is the MCP client (Claude Code, Cursor)
- Both: self-reported, not verified

**Action:**
- v0.3.0: document "actor identity is self-reported, not verified"
  as a known limitation. Add to threat model.
- v0.5.0: evidra-api issues API keys per tenant, actor identity
  scoped to (tenant_id, actor.id). Add auth_context field.

### 2.5 Pre-canonicalized path
**Status: DONE — but needs trust marker**

Pre-canonicalized path exists: PrescribeInput has optional
`canonical_action` field. If provided, adapter is skipped.

**GAP:** No trust marker. PO is right — pre-canonicalized entries
should be marked `canonicalization_source=external` so scorecards
can show "X% of data is self-reported by tools."

**Action:** P1 for v0.3.0 — add `canon_source` field to evidence
entry ("adapter" or "external"). Small change, high value for
credibility.

---

## §3 Canonicalization Contract

### 3.1 Artifact digest vs Intent digest
**Status: DONE**

Two separate digests exist in code:
- `artifact_digest`: SHA256 of raw bytes (what was actually provided)
- `intent_digest`: SHA256 of canonical JSON (what the operation means)

Both stored in CanonResult and written to evidence. Signal detector
(artifact_drift) compares prescription.artifact_digest with
report.artifact_digest.

### 3.2 Frozen noise list for K8s
**Status: DONE**

`internal/canon/noise.go` has explicit frozen lists:
- k8sNoiseFields (uid, resourceVersion, generation, etc.)
- k8sNoiseAnnotationPrefixes (kubectl.kubernetes.io/, etc.)

Comment in code: "Frozen — adding new noise fields requires a
new canon version."

### 3.3 Terraform unknown values
**Status: GAP**

Code uses hashicorp/terraform-json which handles unknown values
in plan JSON, but there's no explicit stable marker for unknowns.

**Action:** P2 — add `has_unknowns: true` flag to CanonResult
when terraform plan contains unknown values. Replace unknowns
with stable marker `"<unknown>"` in shape_hash computation.
Not blocking for v0.3.0 (digests are already stable for same
plan output).

### 3.4 Parse failures as first-class events
**Status: PARTIALLY DONE**

Parse errors are returned in MCP response:
```go
if cr.ParseError != nil {
    return PrescribeOutput{OK: false, Error: &ErrInfo{Code: "parse_error", ...}}
```

But parse failures are NOT written to evidence chain. PO is right —
silence kills trust.

**Action:** P1 for v0.3.0 — write evidence entry on parse failure
with type=canonicalization_failure, raw_digest (always computable),
adapter version, error message. ~30 lines of code.

### 3.5 Golden corpus structure & maintenance
**Status: DONE**

10 golden cases (6 K8s + 4 Terraform) with frozen digests.
Test runner compares output against golden files. Update requires
EVIDRA_UPDATE_GOLDEN=1 + version bump.

PR that changes canonical output fails golden tests automatically.

---

## §4 Signals & Scoring

### 4.1 Baselines / learning mode
**Status: DISAGREE for v0.3.0**

PO suggests learning mode with baselines, per-scope statistics,
confidence levels. We explicitly chose against this:

> "No ML. No baselines. No adaptive thresholds. Every signal is
> a simple predicate."  — Signal Spec

Rationale:
- Baselines add statefulness (need history store)
- Confidence adds complexity to score interpretation
- Learning mode delays time-to-value
- Simple counters are debuggable; ML-based scores are not

Instead: minimum 100 operations before scoring (MinOperations
constant). Below that = "insufficient_data" band. This is the
cold start solution — not learning, just minimum sample size.

**Action:** None. This is a deliberate design decision. Document
it as a non-goal.

### 4.2 Score formula
**Status: DONE**

```go
penalty = Σ(weight_i × rate_i)
score = 100 × (1 - penalty)
```

Bands: excellent (≥99), good (≥95), fair (≥90), poor (<90).
Weights: protocol_violation 0.35, artifact_drift 0.30,
retry_loop 0.20, blast_radius 0.10, new_scope 0.05.

**GAP:** PO asks for "top-3 signals that dropped score" in output.
This is the `evidra explain` command — see §11.2.

### 4.3 Anti-Goodhart
**Status: PARTIALLY ADDRESSED**

Architecture review addendum exists (docs/system-design/backlog/).
Protocol_violation has highest weight (0.35) — you can't improve
score by skipping prescribe calls, because missing calls ARE the
signal.

**GAP:** No explicit "safety floor" where safety signals prevent
score from exceeding a threshold. E.g. if protocol_violation_rate
> 10%, score MUST NOT exceed 90 regardless of other signals.

**Action:** P2 — add safety floor in score.go. Simple:
```go
if rates["protocol_violation"] > 0.10 { score = min(score, 90) }
if rates["artifact_drift"] > 0.05 { score = min(score, 85) }
```
~10 lines.

---

## §5 Telemetry Plane

### 5.1 Label provenance
**Status: DEFER**

Valid concern but adds significant complexity. Every label gets a
source field (user-supplied/inferred/verified). For v0.3.0, all
labels are user-supplied via CLI flags or MCP input.

**Action:** v0.5.0 — add label_source when evidra-api can verify
labels against OIDC claims or K8s service accounts.

### 5.2 Intent fingerprinting
**Status: DONE (via operation_class)**

Operation classes (mutate/destroy/read/plan) are the v0.3.0
version of intent classification. PO wants 3-5 intent classes
(deploy/scale/security/infra-change) — this is reasonable as
a v0.4.0 enhancement on top of operation_class.

**Action:** v0.4.0 — extend operation_class or add intent_class
field derived from canonical_action.

---

## §6 Inspector Model

### 6.1 Strict but friendly protocol
**Status: DONE**

Protocol violations are tracked as signals. MCP prompts and agent
contract say "always prescribe first, always report after." SDK
(MCP server) makes it easy to do the right thing.

### 6.2 Minimal required fields
**Status: GAP — needs explicit table**

Fields exist in code but no explicit MUST/SHOULD/MAY table.

**Action:** P1 — add to prescribe schema:

| Field | Level | Source |
|-------|-------|--------|
| tool | MUST | caller |
| operation | MUST | caller |
| raw_artifact | MUST | caller |
| actor.type | MUST | caller |
| actor.id | MUST | caller |
| actor.origin | MUST | caller |
| environment | SHOULD | caller or flag |
| actor_meta | MAY | caller |
| canonical_action | MAY | self-aware tools only |
| scanner_report | MAY | SARIF from scanner |

Add this table to agent contract doc and prescribe schema
description.

---

## §7 Benchmark: fair & reproducible

### 7.1 Reproducibility
**Status: GAP**

No run metadata captured. PO is right — every scorecard should
record versions.

**Action:** P1 — add to scorecard output:
```json
{
  "meta": {
    "canon_version": "k8s/v1",
    "spec_version": "1.0",
    "evidra_version": "0.3.0",
    "generated_at": "2025-03-04T...",
    "evidence_path": "/home/user/.evidra/evidence",
    "total_entries": 847
  },
  "score": { ... }
}
```

### 7.2 Same-conditions comparison
**Status: DONE (in docs)**

Workload overlap computation exists (score/compare.go). Benchmark
doc §5 says "only compare agents doing the same work."

### 7.3 Scorecard breakdown
**Status: GAP**

Score is a single number. No breakdown by tool/scope/signal.

**Action:** P2 for v0.3.0 — add `breakdown` field to Scorecard:
```go
type Breakdown struct {
    ByTool  map[string]SubScore `json:"by_tool"`
    ByScope map[string]SubScore `json:"by_scope"`
}
```

---

## §8 Integration: don't spread thin

### 8.1 Pick one killer integration
**Status: AGREE**

PO is right. v0.3.0 launch should focus:
**Terraform + Checkov** as the primary story.

Rationale:
- Terraform is 80% of IaC market
- Checkov is most popular scanner (SARIF output)
- "terraform plan → checkov → evidra prescribe → terraform apply → evidra report → evidra scorecard"
- One complete flow, one blog post, one GH Action example

K8s adapter ships too (it's already built) but marketing focus is
Terraform.

### 8.2 Normalized scanner report
**Status: GAP — needs SARIF parser**

Decided: SARIF is the canonical scanner format. One parser covers
Checkov, Trivy, tfsec, KICS, Terrascan, Snyk.

**Action:** P1 for v0.3.0. See codebase review GAP 6.

---

## §9 Security: threat model

### 9.1 Deployment hardening checklist
**Status: GAP**

Threat model doc exists but not visible in README.

**Action:** P2 — add "Security" section to README with:
- Evidence is append-only, treat as sensitive
- Ed25519 keys: generate per deployment, rotate annually
- Forward evidence over TLS
- Actor identity is self-reported in v0.3.0
- Non-goal: Evidra does not prevent compromised agents from lying

### 9.2 Honest non-goals
**Status: DONE (in architecture docs)**

Architecture overview and Signal Spec both document what Evidra
is NOT (not a policy engine, not a security scanner, not enforcement).

**Action:** P2 — consolidate non-goals into README Security section.

---

## §10 What to remove / defer

### Ed25519 per entry → batch signing?
**Status: DISAGREE**

Already built, tested, working (signer.go + signer_test.go, 16 test
functions). Ripping it out adds risk, saves nothing. Batch signing
would be for v0.5.0 evidence forwarding, not a replacement.

### resource_shape_hash
**Status: KEEP**

Used by retry_loop detector (same intent + same shape = retry,
different shape = new version). Already built. PO concern about
single-signal use is valid, but removing it would break retry_loop
which is the third-most-weighted signal.

### Fuzz/crash safety
**Status: AGREE — P1 not P0**

Not blocking v0.3.0. Test strategy doc already has fuzz plan.

---

## §11 OSS adoption

### 11.1 Hello World in 5 minutes
**Status: GAP — CRITICAL for launch**

No "getting started" flow exists. User can't go from zero to
scorecard in 5 minutes.

**Action:** P0 for v0.3.0 — create:
```bash
# Install
brew install evidra-io/tap/evidra

# Prescribe (before terraform apply)
evidra prescribe --tool terraform --op apply \
  --artifact tfplan.json --actor-id "me"

# Apply
terraform apply tfplan

# Report
evidra report --prescription $PRESCRIPTION_ID --exit-code $?

# See your score
evidra scorecard
```

This requires CLI prescribe/report to write evidence (codebase
review GAP 5).

### 11.2 `evidra explain` command
**Status: GAP — HIGH VALUE**

PO is right — this is a killer feature. Shows score + why + evidence.

**Action:** P1 for v0.3.0 — add `evidra explain` subcommand:
```
$ evidra explain

Reliability Score: 96.5 (good)

Top signals:
  1. protocol_violation: 3 events (rate: 0.35%)
     → 2x stalled_operation, 1x crash_before_report
  2. artifact_drift: 1 event (rate: 0.12%)
  3. retry_loop: 0 events

Period: last 30 days (847 operations)
Versions: spec=1.0, canon=k8s/v1, evidra=0.3.0
```

~100 lines on top of existing scorecard logic.

### 11.3 Version in every output
**Status: PARTIALLY DONE**

MCP prescribe output includes `canon_version`. Missing: spec_version,
evidra_version.

**Action:** P1 — add to all output formats:
```go
type OutputMeta struct {
    EvidraVersion string `json:"evidra_version"`
    SpecVersion   string `json:"spec_version"`
    CanonVersion  string `json:"canon_version"`
}
```

---

## Priority Summary

### P0 — must ship with v0.3.0

| Item | Section | Effort | Dep |
|------|---------|--------|-----|
| Unify evidence types (remove OPA remnants) | Codebase review GAP 1 | 2 days | — |
| Wire CLI to evidence store | Codebase review GAP 5 | 2 days | GAP 1 |
| Fix signal parameters (thresholds, scope_key) | Codebase review GAP 7-9 | 0.5 day | — |
| Fix ScopeClass (env-based) | Codebase review GAP 3 | 0.5 day | — |
| Hello World getting started flow | §11.1 | 1 day | CLI wired |
| `evidra explain` command | §11.2 | 1 day | CLI wired |

**Total P0: ~7 days**

### P1 — should ship with v0.3.0

| Item | Section | Effort |
|------|---------|--------|
| Parse failures as evidence entries | §3.4 | 0.5 day |
| canon_source field (adapter/external) | §2.5 | 0.5 day |
| Required fields table in docs | §6.2 | 0.5 day |
| SARIF scanner integration | §8.2 | 3 days |
| Scorecard run metadata | §7.1 | 0.5 day |
| Version in all outputs | §11.3 | 0.5 day |

**Total P1: ~5.5 days**

### P2 — nice to have for v0.3.0

| Item | Section | Effort |
|------|---------|--------|
| Safety floor in scoring | §4.3 | 0.5 day |
| Scorecard breakdown by tool/scope | §7.3 | 1 day |
| TF unknown values marker | §3.3 | 0.5 day |
| State diagram in Inspector Model doc | §2.1 | 0.5 day |
| Security section in README | §9.1 | 0.5 day |

**Total P2: ~3 days**

### DEFERRED to v0.5.0+

| Item | Section | Reason |
|------|---------|--------|
| Actor auth_context / OIDC | §2.4 | Requires evidra-api |
| Forward integrity + server receipts | §2.3 | Requires evidra-api |
| Label provenance | §5.1 | Requires verification source |
| Intent fingerprinting beyond operation_class | §5.2 | Enhancement, not blocking |
| Baselines / learning mode | §4.1 | Deliberate architectural decision: no ML |

### EXPLICITLY REJECTED

| Item | Section | Reason |
|------|---------|--------|
| Learning mode / baselines | §4.1 | Violates "no ML, no adaptive thresholds" principle |
| Confidence on signals | §4.2 | Adds interpretation complexity, reduces trust |
| Batch signing replacing per-entry | §10 | Already built, no benefit from ripping out |
| Remove resource_shape_hash | §10 | Needed for retry_loop, already built |