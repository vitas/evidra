# EVIDRA_ARCHITECTURE_INVARIANTS.md

This document defines **non‑negotiable invariants** for the Evidra refactor.  
If an implementation violates any invariant below, it is **architecturally incorrect** (even if it “works”).

Evidra category: **Reliability inspection for AI infrastructure automation**.

---

## 0) Scope of this document

These invariants apply to:

- MCP agent integration (prescribe/report)
- Evidence Store (append-only, hash-linked)
- Canonicalization Contract
- Signals Engine and Scoring
- Validator/Scanner integrations
- Multi‑tenant service mode

This document intentionally avoids implementation details and focuses on **what must remain true**.

---

## 1) Evidence is the single source of truth

**Invariant 1.1 — Evidence-first computation**  
All signals, scores, and explanations MUST be derived from recorded evidence entries.

- No “side channel” state is allowed to affect scoring (except configuration).
- If something is not in evidence, it is not inspectable.

**Invariant 1.2 — Replay determinism**  
Given the same evidence log + configuration + versions, the system MUST produce the same signals and scores.

---

## 2) Evidence Store is append-only and tamper-evident

**Invariant 2.1 — Append-only**  
Evidence entries MUST NOT be mutated or deleted. Corrections are represented as new entries.

**Invariant 2.2 — Tamper-evident chain**  
Evidence storage MUST be hash-linked (or equivalent) such that:
- insertion/reordering/modification is detectable during verification
- verification is possible offline

**Invariant 2.3 — Receipts (optional) are also evidence**  
If the server issues receipts/acks/signatures, they MUST be written as evidence entries (e.g., `type=receipt`) and MUST NOT create a parallel evidence model.

---

## 3) Inspector protocol defines inspection lifecycle

**Invariant 3.1 — prescribe/report are the protocol**  
Inspection lifecycle is defined by **prescribe** (intent evidence) and **report** (outcome evidence).

**Invariant 3.2 — Lifecycle states are explicit**  
Sessions MUST be representable in the following lifecycle model:

- `PRESCRIBED`
- `REPORTED`
- `CLOSED`
- `EXPIRED` (prescribed without a report within TTL)
- `UNPRESCRIBED` (report without a preceding prescribe)

**Invariant 3.3 — TTL is data**  
TTL MUST be a field of the prescription (e.g., `ttl_ms`).  
A “default TTL” is allowed, but MUST be materialized into the stored prescription data (to keep replay deterministic).

---

## 4) Identity and correlation are not optional

**Invariant 4.1 — tenant_id is always present (service mode)**  
Every evidence entry MUST belong to exactly one tenant.

**Invariant 4.2 — trace_id is the primary correlation key**  
Every prescribe/report MUST include a `trace_id`, which represents an inspection session/task.

**Invariant 4.3 — Correlation fields are optional but standardized**  
If available, the following fields MUST follow stable semantics and naming:

- `repo`
- `work_item_key` (Jira/issue/ticket id)
- `commit_sha`
- `env`
- `target` (cluster/account/namespace)

Missing fields MUST be represented as `unknown`/empty with no ambiguity.

---

## 5) Actor identity is first-class and auditable

**Invariant 5.1 — actor is mandatory for prescribe/report**  
Every prescribe/report MUST include an `actor` structure at minimum:

- `actor.type` (e.g., `ai_agent`, `ci`, `human`, `unknown`)
- `actor.id` (stable identifier)
- `actor.provenance` (e.g., `mcp`, `oidc`, `git`, `manual`)

**Invariant 5.2 — Identity confidence is explicit**  
The system MUST track and expose identity confidence.  
Example: MCP identity > CI OIDC identity > Git author heuristic.

---

## 6) Canonicalization is mandatory for inspection semantics

**Invariant 6.1 — Canonicalization Contract is normative**  
Canonicalization MUST follow the versioned contract (e.g., `CANONICALIZATION_CONTRACT_V1`).

**Invariant 6.2 — intent_digest and artifact_digest are distinct**  
- `intent_digest`: hash of canonical semantic representation
- `artifact_digest`: hash of raw artifact bytes (or verified equivalent)

They MUST NOT be treated as interchangeable.

**Invariant 6.3 — Canonicalization failures are evidence**  
Parsing/canonicalization failures MUST produce evidence entries (e.g., `type=canonicalization_failure`) rather than silent drops.

---

## 7) Trust and confidence are explicit outputs

**Invariant 7.1 — Self-asserted vs verified is explicit**  
Reports MUST carry (or allow deriving) a trust mode, e.g.:

- `self_asserted`
- `verified_by_server`
- `verified_by_reference`

**Invariant 7.2 — Score always includes confidence**  
Every score/band output MUST include a confidence indicator influenced by:
- evidence completeness (missing report, etc.)
- actor identity confidence
- canonicalization trust mode
- protocol violations

---

## 8) Signals are behavioral, not policy rules

**Invariant 8.1 — Signals describe automation behavior**  
Signals MUST model behaviors such as drift, retries, scope expansion, protocol violations, blast radius.

Evidra MUST NOT evolve into a policy engine or scanner rule system.

**Invariant 8.2 — Signals are derived from evidence**  
Signals MUST be computable from evidence + configuration, with no hidden inputs.

**Invariant 8.3 — Explainability is mandatory**  
For any score, the system MUST produce an explanation including:
- top contributing signals
- supporting evidence references

---

## 9) Scoring is stable, simple, and non-magical

**Invariant 9.1 — Scoring model is versioned**  
Score computation MUST be versioned and replayable.

**Invariant 9.2 — Bands are stable**  
The mapping to bands (GREEN/YELLOW/RED or A/B/C) MUST be deterministic given score+config.

**Invariant 9.3 — Safety floors / ceilings (minimal anti-gaming)**  
At minimum, the system MUST support:
- score ceilings for untrusted evidence
- floor/override behavior for catastrophic risk signals (if configured)

---

## 10) Validators are external; Evidra integrates but does not replace

**Invariant 10.1 — Evidra does not run validators (v1)**  
Evidra MUST NOT become a runner/orchestrator of scanners in core architecture.

**Invariant 10.2 — Validators produce findings; Evidra produces signals**  
External tools produce **findings**. Evidra records findings as evidence and may transform them into **behavioral signals**.

**Invariant 10.3 — Findings schema is normalized**  
Validator outputs MUST be normalized into a stable “finding” schema, minimally:

- `tool`
- `rule_id`
- `severity`
- `resource`
- `message`
- `artifact_digest` (linking key)

---

## 11) Multi-tenancy is strict and end-to-end

**Invariant 11.1 — Tenant isolation for storage and queries**  
Tenants MUST NOT be able to read, explain, or benchmark across boundaries.

**Invariant 11.2 — Tenant context is derived from auth**  
API keys map to a tenant_id via auth middleware; business logic MUST NOT “guess” tenant context.

---

## 12) Versioning is visible everywhere

**Invariant 12.1 — Every record and output carries versions**  
At minimum, outputs MUST include:

- `spec_version`
- `canonical_version`
- `adapter_version` (where applicable)
- `scoring_version`

This is mandatory for benchmark reproducibility and supportability.

---

## 13) Deployment modes share the same model

**Invariant 13.1 — Local and service mode share evidence model**  
Local single-node mode and self-hosted service mode MUST use the same evidence schema and semantics.

---

## 14) Non-goals (must remain non-goals)

To protect scope and adoption, the following are explicitly non-goals for v1:

- becoming a full policy engine (OPA/Kyverno competitor)
- becoming a scanner/validator (Checkov/Trivy competitor)
- becoming a CI/CD orchestrator
- storing raw secrets by default (must be optional/controlled)
- perfect cryptographic attestation of all claims (v1 focuses on practical trust levels)

---

## 15) Implementation checklist (for the refactor)

Before refactor is considered “architecturally complete”, ensure:

1. Evidence schema is finalized and versioned.
2. prescribe/report protocol includes TTL and lifecycle semantics.
3. All evidence entries include tenant_id (service mode) and trace_id (protocol events).
4. Canonicalization contract is enforced + golden corpus tests exist.
5. Signal computation is replay deterministic.
6. Score includes band + confidence + explanation.
7. Validator findings normalization exists (adapter layer).
8. Version fields appear in outputs and stored evidence.

---

## 16) Quick reference: what to refactor first

Recommended order for the new codebase:

1) Evidence schema + store (append-only, verification)  
2) Inspector protocol (prescribe/report, TTL, lifecycle)  
3) Canonicalization adapters + golden corpus  
4) Signals engine (v1 set)  
5) Scoring + explain  
6) Integration adapters (validators)  
7) Service API + multi-tenant auth

