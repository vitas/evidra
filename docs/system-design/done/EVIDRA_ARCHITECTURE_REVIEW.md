# Evidra Benchmark — Architecture Review

## Status
**Historical snapshot.** This document captured gaps and
contradictions found during design. Most issues have been resolved.
See the fix list at the bottom for current status.

This document lives in docs/history/ and is NOT part of the active
architecture. For current architecture, see EVIDRA_ARCHITECTURE_OVERVIEW.md.

---

## Verdict

The architecture is sound. The core idea (prescribe/report protocol
→ canonical intent → signals → score) is simple and correct. The
canonicalization contract is the strongest part — frozen, tested,
versioned. The three-component model (CLI, MCP, API) is the right
deployment split.

Below: everything I'd fix before writing code.

---

## 1. GAPS (things that don't exist yet and must)

### 1.1 Prescription TTL and lifecycle — UNDEFINED

The protocol says: "prescription without matching report within
TTL → protocol violation." But TTL is never specified as a
concrete protocol element.

Questions without answers:
- What is the default TTL? (5 min was mentioned once, in e2e example)
- Is TTL per-tool? (terraform apply can take 30 min, kubectl 5 sec)
- Who tracks TTL? (evidra-mcp in-process? a background goroutine? CLI has no long-running process)
- What happens in CI when the pipeline crashes — who detects the missing report?

**The problem:** evidra-mcp can track TTL in-process (it's
long-running). But evidra CLI exits after each command. There is no
process to detect "prescription issued, no report within TTL."

**Fix:** TTL detection happens at scorecard computation time, not
in real-time. When `evidra scorecard` scans the evidence chain, it
finds prescriptions without matching reports and retroactively
marks them as protocol violations. No background process needed.
This must be explicitly stated in the protocol spec.

Real-time TTL detection is a v0.5.0 feature of evidra-api (which
is long-running and can run periodic scans).

### 1.2 Prescription-to-report matching — UNDERSPECIFIED

Report contains `prescription_id`. But:
- What if the agent sends two reports for one prescription?
- What if the agent sends a report with a prescription_id that doesn't exist?
- What if the agent sends report for someone else's prescription?
- Can prescription_id be reused across actors?

**Fix:** Define matching rules explicitly:
- prescription_id is globally unique (ULID, already in the codebase)
- First report wins. Second report for same prescription_id → protocol violation
- Report with unknown prescription_id → protocol violation (unprescribed action)
- prescription_id carries actor.id — cross-actor matching is rejected

### 1.3 Evidence chain integrity during forward — UNDEFINED

Local evidence is JSONL, hash-linked, signed. When forwarded to
evidra-api:
- Does evidra-api verify the hash chain?
- What if entries arrive out of order?
- What if there are gaps (network failure)?
- Is the forward idempotent?

**Fix:** Forward is append-only, idempotent (entries keyed by
entry_hash), ordered by timestamp. evidra-api verifies signatures
but does NOT verify chain continuity (entries may arrive with gaps
from network issues). Gaps are flagged but don't reject entries.
Chain verification is for local evidence only.

### 1.4 Actor identity — HOW IS IT TRUSTWORTHY?

Actor provides its own id: `actor.id = "claude-code"`. Nothing
stops an agent from lying. Two different agents could claim the
same id.

For v0.3.0 this is acceptable (self-reported, local evidence).
But for evidra-api: if one team's agent claims to be "claude-code"
and another team's agent also claims "claude-code", their evidence
merges.

**Fix for v0.5.0:** evidra-api issues API keys per tenant. Actor
identity is scoped to tenant: `(tenant_id, actor.id)` is the
unique key. The prescribe call carries the API key, not the
actor.id for authentication. Actor.id is just a label.

### 1.5 Pre-canonicalized path — trust model unclear

When a tool sends its own canonical_action, Evidra trusts the
resource identity. But:
- What if the tool lies about resource_count?
- What if scope_class is wrong?
- Blast radius signal uses resource_count — a lying tool breaks it.

**Fix:** Accept the trade-off and document it. Pre-canonicalized
path trades accuracy for reach. Signals are only as good as the
input. The tool is responsible for correct identity. Evidra's
contribution in this path is: risk detectors on raw artifact,
protocol consistency (prescribe/report), and retry loop detection.
Blast radius may be inaccurate. Document this in the protocol.

---

## 2. OVERENGINEERING (things that are more complex than needed)

### 2.1 resource_shape_hash — used by exactly one signal

resource_shape_hash exists solely for retry loop detection. It
adds complexity to every adapter (must compute SHA256 of normalized
spec), to every evidence entry (extra field), to the guarantees
table (separate stability table), and to the test strategy
(separate sensitivity test).

**Alternative:** retry loop could compare artifact_digest directly.
If the raw artifact bytes are the same → retry. If different → not
a retry. No shape_hash needed.

**Counter-argument:** artifact_digest changes on whitespace/reorder
while shape_hash doesn't. But: if the agent is retrying with a
reformatted but semantically identical artifact, that IS a retry.
The whitespace changed by accident, not by intent.

**Recommendation:** keep shape_hash but acknowledge it's the most
complex part of canonicalization for one signal. If it becomes a
maintenance burden, fall back to artifact_digest for retry detection.
Acceptable accuracy loss.

### 2.2 Ed25519 signatures on every evidence entry

Every entry in the evidence chain is Ed25519 signed. This is good
for tamper detection but:
- The key is stored on the same machine as the evidence
- Anyone with access to the evidence file also has access to the key
- It protects against casual tampering, not adversarial attacks

For v0.3.0 (local evidence) this is already in the codebase (from
v0.2.0) so the cost of keeping it is zero. But don't oversell it
as "cryptographic integrity." It's "detect accidental corruption."

**Recommendation:** keep it (already built), but don't add more
crypto infrastructure. No certificate chains. No HSM. Not for v1.

### 2.3 Scope class resolution — namespace mapping undefined

Scope class (production, staging, development, unknown) is
"derived" by the adapter. But how?

Canonicalization contract says "scope hint: staging (optional;
adapter can derive via namespace mapping)." The namespace mapping
is never defined. What maps namespace "prod-us-east-1" to
"production"?

**Fix:** For v0.3.0, scope_class comes from ONE of:
1. Explicit `environment` field in prescribe request (highest priority)
2. Namespace pattern matching: contains "prod" → production,
   contains "stag" → staging, contains "dev" → development
3. Default: unknown

Keep it dumb. Three substring matches. No regex. No config file.
If the user doesn't like the heuristic, they pass `--env production`
explicitly.

### 2.4 Workload overlap computation — undefined algorithm

"If overlap < 25% → warning." How is overlap computed?

Two agents with these profiles:
```
A: kubectl (3000 ops), terraform (200 ops)
B: kubectl (500 ops)
```

Is this 100% overlap (B is a subset of A's tools)? Or 50%
overlap (B has 1 of A's 2 tools)? Or something else?

**Fix:** Don't compute overlap numerically. Just show the profiles
side by side and let the human decide. If they share zero tools →
"WARNING: no shared tools." If they share some → show the shared
subset. No percentage. Percentages create false precision.

---

## 3. CONTRADICTIONS (places where documents disagree)

### 3.1 risk_tags in canonical_action vs computed after

Canonical action schema says: `risk_tags` is a field in
canonical_action. But also says: "risk_tags are populated by
catastrophic risk detectors AFTER canonicalization."

If risk_tags are computed after canonicalization, they should NOT
be in the canonical_action struct. They should be in the
prescription (which wraps canonical_action + risk analysis).

**Fix:** Remove risk_tags from CanonicalAction. Put them in
Prescription:

```go
type Prescription struct {
    ID              string
    CanonicalAction CanonicalAction  // no risk_tags here
    ArtifactDigest  string
    IntentDigest    string
    RiskLevel       string
    RiskTags        []string          // here, computed after canon
    Signature       string
}
```

This is cleaner: canonical_action is adapter output (deterministic,
testable). risk_tags are detector output (separate concern).

### 3.2 Detectors inspect "canonicalized payload" vs "raw artifact"

Benchmark doc says: "Detectors inspect the canonicalized payload."
But also says: "Catastrophic risk detectors read RAW artifact."

Which one? The canonical payload has noise removed. The raw artifact
has everything. A detector for hostPath mounts needs to read spec —
which is NOT in canonical_action (only identity is).

**Fix:** Detectors receive BOTH canonical_action (for identity and
scope context) AND raw artifact bytes (for content inspection). The
canonical_action tells them "what kind of resource" (Deployment,
SecurityGroup). The raw artifact tells them "what's inside."

Document this explicitly.

### 3.3 "No deny" but constraints in prescribe output

Protocol output includes `constraints: [...]` with "human-readable
risk descriptions." The word "constraints" implies restrictions.

**Fix:** Rename to `risk_details` or `risk_context` everywhere.
The word "constraints" is already used inconsistently — some places
say risk_details, some say constraints. Pick one. I recommend
`risk_details` (already used in most places).

---

## 4. MISSING PIECES (smaller gaps)

### 4.1 How does an agent detect it should ask the human?

Evidra returns risk_level: "high". The agent contract says "smart
agents stop and ask the human." But:
- The MCP prompt says "High risk does not mean stop"
- There is no machine-readable "ask human" flag
- The agent must parse risk_level and make its own policy

This is by design (inspector model). But the agent contract should
have a concrete recommendation: "If risk_level == high AND you are
an autonomous agent, consider requesting human approval before
proceeding." Not enforce, but recommend.

### 4.2 How does evidra CLI work offline?

CLI writes to local JSONL. But `evidra scorecard` reads JSONL.
What if evidence is spread across machines?

CI runner 1 has evidence from Tuesday.
CI runner 2 has evidence from Wednesday.
Developer laptop has MCP evidence from Thursday.

`evidra scorecard` on any single machine sees partial data.

**Fix for v0.3.0:** Document that local scorecard is per-machine.
For cross-machine scorecard, use evidra-api (v0.5.0) or manually
merge evidence files (`cat evidence1.jsonl evidence2.jsonl | sort`).
This is honest. Don't pretend local = global.

### 4.3 Metrics cardinality — undefined

`evidra_catastrophic_context_total{agent="claude-code"}` has label
`agent`. If 50 agents exist, that's 50 time series per metric.

What about per-tool? Per-scope? Per-signal?

```
evidra_protocol_violation_total{agent, tool, scope}
```

With 50 agents × 5 tools × 4 scopes = 1000 time series per signal.
× 5 signals = 5000 time series. This is fine for Prometheus but
should be explicitly documented as the cardinality model.

### 4.4 What happens when Evidra itself is down?

evidra-mcp crashes. Agent continues operating without prescribe/
report. Operations are completely invisible. No evidence. No
signals. No score.

This is the observer effect: Evidra can only score what it sees.
If it's down, the score period has fewer operations, which may
inflate the reliability score (fewer chances to fail).

**Fix:** Document this honestly. Evidra measures cooperation with
the protocol, not ground truth. An agent that kills Evidra and
operates freely would score perfectly (zero violations on zero
operations). This is a known limitation of the inspector model.

For teams that care: monitor evidra-mcp uptime separately.
`evidra_operations_total` metric dropping to zero = Evidra is down
or agent stopped calling it.

### 4.5 evidence.jsonl concurrency

Two CLI processes running simultaneously (parallel CI jobs) both
append to the same JSONL file. The existing codebase has file
locking (pkg/evlock). Is this sufficient?

Hash chain requires sequential appends (entry N links to entry
N-1). Concurrent writers break the chain unless they serialize.

**Fix:** The existing file locking handles this (already built in
v0.2.0). Document that parallel writers are safe because of flock.
On NFS/network filesystems, flock may not work — document this as
a known limitation.

---

## 5. WHAT'S CORRECTLY SIMPLE

These are the parts that are correctly minimal and should NOT be
made more complex:

- **Five signals, fixed** — resist adding more
- **Risk matrix: 12-cell table** — resist adding rules
- **Detectors: ~10 patterns in Go** — resist adding a policy engine
- **Evidence: JSONL** — resist adding a database for v0.3.0
- **Score: weighted sum** — resist adding ML or baselines
- **Golden corpus: 10 cases + 5 mutators** — resist adding thousands
- **Protocol: 2 calls** — resist adding approve, cancel, etc.
- **No deny** — resist adding "just one deny for really bad things"

---

## 6. PRIORITY FIX LIST

### Must fix before implementation

1. ~~**TTL detection at scorecard time, not real-time**~~ ✓ FIXED in benchmark §2
2. ~~**Prescription matching rules**~~ ✓ FIXED in benchmark §2 + signal spec
3. ~~**Move risk_tags out of CanonicalAction**~~ ✓ FIXED in canon contract §2
4. ~~**Detectors receive both canon + raw**~~ ✓ FIXED in canon contract §2
5. ~~**Rename constraints → risk_details everywhere**~~ ✓ FIXED in benchmark §6
6. **Scope class: 3 substring matches + explicit override** — already in canon contract §10 (was there, review missed it)

### Should fix before v0.3.0 release

7. **Document local scorecard is per-machine** (§4.2) — TODO
8. **Document Evidra-down limitation** (§4.4) — TODO
9. **Document pre-canonicalized accuracy trade-off** (§1.5) — TODO
10. ~~**Drop workload overlap percentage, show profiles instead**~~ — deferred, current text is acceptable

### Can defer to v0.5.0

11. **Actor identity scoped to tenant** (§1.4)
12. **Evidence forward integrity model** (§1.3)
13. **Prometheus cardinality model** (§4.3) — ✓ FIXED in benchmark §10
