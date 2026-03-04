
# Evidra Threat Model
Status: Draft v0.1

This document defines the threat model for Evidra as an inspector system:
a flight recorder and reliability benchmark for infrastructure automation.

It clarifies:
- what threats are in scope
- attacker models
- security goals (what Evidra guarantees)
- non-goals (what Evidra does not guarantee)
- mitigations and residual risk

---

# 1. System Overview (Security-Relevant)

Evidra operates as an inspector, not an enforcer.

Core components:
- Protocol: prescribe() and report()
- Domain adapters + canonicalization
- Evidence chain (append-only, hash-linked, signed)
- Signal detection + scorecard computation
- Optional external references (audit/CI pointers)

Evidra does not execute infrastructure actions and does not block them.

---

# 2. Assets to Protect

A1. Evidence Integrity
The evidence chain must be tamper-evident and attributable.

A2. Benchmark Trustworthiness
Scores and signal rates should reflect actual recorded behavior and remain comparable across time and versions.

A3. Canonicalization Stability
The mapping from raw artifacts to canonical actions must be deterministic and versioned to prevent silent drift.

A4. Actor Attribution
Protocol entries should correctly attribute actions to the right actor (agent/CI/script identity) at least to the level supported by deployment.

---

# 3. Trust Boundaries

Primary trust boundary:
- External actor (agent/CI/script) is outside trust.
- Evidra core is inside trust (binary + configs + signing keys).
- Evidence storage is partially trusted (tamper-evidence assumed, not secrecy).

Optional external systems:
- CI platforms, audit logs, CloudTrail are treated as corroborating inputs but not authoritative for protocol verdicts unless explicitly configured.

---

# 4. Attacker Models

M1. Buggy Agent (Most Common)
Capabilities:
- can omit prescribe/report
- can retry loops
- can drift behavior

M2. Untrusted / Malicious Agent
Capabilities:
- can fabricate reports
- can reuse digests
- can spam events to distort metrics
- can avoid scope labeling

M3. Insider (Operator / Dev)
Capabilities:
- delete or edit evidence logs
- rewrite history
- disable detectors

M4. Infrastructure Toolchain Noise
Not an attacker, but a source of risk.
Tools produce non-deterministic outputs (ordering, defaults, timestamps).

M5. Supply Chain / Build Compromise
Attacker compromises Evidra binary or adapter code.

---

# 5. Security Goals (What Evidra Guarantees)

G1. Tamper-Evident Evidence
If evidence is altered after the fact, it can be detected.
Mechanisms:
- hash-linked append-only log
- signed entries (key in trusted boundary)

G2. Deterministic Canonicalization (within a version)
For a given canonicalization version, semantically identical artifacts should map to identical canonical output.
Mechanisms:
- canonicalization contract
- golden corpus tests
- versioned canonicalization

G3. Honest Protocol Verdicts (relative to received events)
Given the events received, protocol verdicts (compliant/deviation/unreported) are computed deterministically.

G4. Low-Noise Catastrophic Risk Context (optional)
Risk detectors can flag high-risk patterns in artifacts without attempting full compliance coverage.

---

# 6. Non-Goals (What Evidra Does NOT Guarantee)

NG1. Prevention
Evidra does not prevent or block destructive operations.

NG2. Truth of External Reality
Evidra does not guarantee that a reported action actually happened in the infrastructure unless external corroboration is configured and trusted.

NG3. Complete Compliance Coverage
Evidra does not implement or maintain policies for all cases.

NG4. Confidentiality of Artifacts by Default
Evidence chains are designed for integrity and auditability.
Confidentiality requires explicit deployment controls (encryption, access control).

---

# 7. Threats and Mitigations

T1. Evidence Log Tampering
Threat:
- delete/modify JSONL entries to hide incidents
Mitigations:
- hash-linked chain detects missing/modified entries
- signatures prevent silent rewriting
- optional remote append-only storage
Residual risk:
- attacker can delete the entire log if storage is not protected; ship logs off-host or replicate.

---

T2. Protocol Theater (Agent fakes compliance)
Threat:
- agent calls prescribe/report but lies about results
Mitigations:
- track effectiveness metrics (completion, latency, escalation)
- allow report.external_refs[] to attach corroborating pointers (CI/audit ids)
- expose corroboration coverage if correlators exist
Residual risk:
- without corroboration, a malicious agent can fabricate reports.

---

T3. Score Gaming (Goodhart’s Law)
Threat:
- optimize for score instead of operational outcomes
Mitigations:
- separate Safety Score and Effectiveness Score
- add stalled-task detection (backlog)
- penalize excessive escalation
Residual risk:
- gaming evolves; keep scoring transparent and adjustable.

---

T4. Canonicalization Drift
Threat:
- intent digests change due to formatting/order/tool version or adapter change
Mitigations:
- canonicalization contract
- frozen noise lists (k8s/v1)
- golden corpus tests
- explicit canonicalization_version in protocol entries
- scorecard warns on mixed versions
Residual risk:
- new tools/CRDs require adapter updates; version bumps expected.

---

T5. Cardinality Explosion / Telemetry DoS
Threat:
- high-cardinality labels or event spam
Mitigations:
- keep Prometheus metrics low-cardinality (avoid model_id/prompt_id labels)
- rate limit ingestion / per-actor quotas
- store rich metadata in evidence log, not metric labels
Residual risk:
- extreme spam can fill disks; mitigate with retention + quotas.

---

T6. Key Compromise (Signing key stolen)
Threat:
- attacker can forge valid signed entries
Mitigations:
- store keys in OS keyring/HSM where available
- rotate keys periodically
- include key_id and rotation metadata
- optional remote signing service
Residual risk:
- key compromise undermines integrity until detected; rotation limits exposure.

---

T7. Supply Chain Compromise
Threat:
- compromised binary/adapters produce incorrect canonicalization or scoring
Mitigations:
- reproducible builds
- signed releases
- pinned dependencies
- adapter corpus tests in CI
Residual risk:
- cannot be fully eliminated; focus on verification.

---

# 8. Deployment Hardening Checklist

- Run Evidra with least privilege (no infra credentials required).
- Write evidence logs to append-only storage if possible.
- Ship evidence logs off-host to prevent deletion.
- Protect signing keys (file perms, keyring, or HSM).
- Use retention policies for evidence logs.
- Keep Prometheus labels low-cardinality.
- Track canonicalization_version changes explicitly.

---

# 9. Summary

Evidra’s security is based on:
- tamper-evident evidence (hash-linked + signed)
- deterministic, versioned canonicalization
- signal-based detection of behavioral risk
- optional corroboration pointers to reduce self-report gaming

Evidra does not prevent incidents; it makes automation behavior observable,
auditable, and comparable so ops teams can detect dangerous patterns early.
