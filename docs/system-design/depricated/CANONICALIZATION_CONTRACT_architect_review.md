# Evidra Canonicalization Contract — Architect Review

## Status
Review of CANONICALIZATION_CONTRACT.md v0.1.

---

## Review Summary

This is the most important engineering document in the project.
Signals are easy. Scorecard is easy. Canonicalization is where
Evidra either earns trust or loses it. The draft is solid — it
covers the right concerns. This review tightens five areas.

---

## 1. Artifact Digest vs Intent Digest — Clarify the Boundary

The draft defines both but doesn't make the boundary crisp enough.
These serve completely different purposes:

**artifact_digest** = SHA256 of raw bytes as received from the agent.
Purpose: detect if the agent modified the artifact between prescribe
and report (Signal 2: Artifact Drift). Computed BEFORE any parsing.
Raw bytes in, hash out. No canonicalization. If the agent sends the
same YAML with a trailing newline added, the digest changes. That's
correct — the agent changed what it sent.

**intent_digest** = SHA256 of canonical JSON of canonical_actions.
Purpose: detect retry loops (Signal 3) and track behavioral patterns.
Computed AFTER full canonicalization. Two semantically identical
manifests with different formatting produce the same intent_digest.
That's correct — the intent is the same.

These must never be confused. artifact_digest is for protocol
integrity (did the agent send the same bytes?). intent_digest is
for behavioral analysis (is the agent doing the same thing?).

The draft should add this as a principle in Section 2:

```
artifact_digest: integrity of the protocol (raw bytes, no parsing)
intent_digest:   identity of the intent (canonical form, post-parsing)

These are independent. Same artifact_digest implies same intent_digest.
Same intent_digest does NOT imply same artifact_digest.
```

---

## 2. Kubernetes Noise List — Be Explicit and Frozen

Section 6.3 says "remove metadata.managedFields, etc." and mentions
"implementation-defined list" for annotation noise. This is where
drift will creep in. Every new noise field added changes the
canonical output and breaks intent_digest stability.

Proposal: freeze the noise list per canonicalization_version. The
list is part of the contract, not an implementation detail.

```yaml
# k8s/v1 noise fields (frozen, changes require version bump)
remove_fields:
  - metadata.creationTimestamp
  - metadata.managedFields
  - metadata.resourceVersion
  - metadata.uid
  - metadata.generation
  - metadata.selfLink
  - status

remove_annotations:
  - kubectl.kubernetes.io/last-applied-configuration
  - deployment.kubernetes.io/revision
  - meta.helm.sh/release-name
  - meta.helm.sh/release-namespace
```

This list is part of k8s/v1 spec. Adding a new annotation to the
noise list requires bumping to k8s/v2 and updating golden corpus.

---

## 3. Terraform Unknown Values — Simplify

The draft proposes `__UNKNOWN__` and `__SENSITIVE__` tokens. This
is clever but creates a problem: the same terraform plan run twice
may have different unknown fields depending on provider state. So
intent_digest becomes unstable across runs for the same logical
change.

Simpler approach: intent_digest for Terraform should not include
field values at all. Only structure matters:

```yaml
terraform canonical_action:
  address: "aws_security_group.web"
  type: "aws_security_group"
  actions: ["create"]
  # NO field values in intent. Only resource identity + action.
```

Rationale: Terraform plan JSON is a diff, not a desired state.
Two plans for the "same change" can have different field values
because of computed fields, provider versions, state timing.
Trying to canonicalize field values is a losing battle.

What matters for the benchmark: which resources are being changed,
what type of change (create/update/delete), and how many. Not what
the specific field values are.

For catastrophic risk detectors that need field values (open SG,
wildcard IAM), those run on the raw plan JSON before
canonicalization. Detectors read raw artifacts. Canonicalization
produces intent identity. Separate concerns.

```
Raw artifact → Catastrophic risk detectors (read field values)
Raw artifact → Canonical actions (resource identity + action type)
                   → intent_digest (stable across runs)
```

---

## 4. Golden Corpus — Structure and Maintenance

The draft says "golden corpus tests" but doesn't specify structure.
Here's a concrete proposal:

```
tests/corpus/
  k8s/
    v1/
      deployment_simple/
        input.yaml              # raw artifact
        canonical_actions.json  # expected canonical output
        intent_digest.txt       # expected SHA256
        metadata.json           # { description, added_version, rationale }
      deployment_multicontainer/
        ...
      helm_output_nginx/
        ...
      cronjob_with_security/
        ...
  terraform/
    v1/
      sg_create/
        input.json              # terraform show -json output
        canonical_actions.json
        intent_digest.txt
        metadata.json
      iam_wildcard/
        ...
```

Rules:

- Each case is a directory with fixed file names.
- `canonical_actions.json` is the golden output. Test asserts exact
  byte-for-byte match after canonical JSON serialization.
- `intent_digest.txt` is the expected SHA256. Test asserts match.
- `metadata.json` records why this case exists and when it was added.
- Cases are append-only. Removing or modifying a case requires
  canonicalization_version bump.

Minimum corpus size for v1 launch:

| Domain | Cases | Must cover |
|--------|-------|------------|
| k8s | 15-20 | Deployment, Pod, CronJob, StatefulSet, Service, multi-doc, helm output, kustomize output, CRD, RBAC |
| terraform | 10-15 | Create, update, destroy, mixed, SG, IAM, S3, multi-resource plan |

These are the adapter's ABI. If a code change breaks a golden
test, either the change is wrong or the corpus needs a versioned
update with rationale.

---

## 5. Canonicalization Version in Practice

The draft says "bump canonicalization_version when output changes."
In practice, how does this work in the scorecard?

Scenario: Evidra 0.4.0 ships with k8s/v1. Six months later,
Evidra 0.5.0 ships with k8s/v2 (added a new noise field). The
evidence chain has 6 months of k8s/v1 entries and new k8s/v2
entries.

**Scorecard rule: never mix versions in a comparison.**

```
evidra scorecard --agent claude-code --period 30d

# If the 30d window spans a version change:
WARNING: Canonicalization version changed during scoring period.
  k8s/v1: days 1-15 (2,100 operations)
  k8s/v2: days 16-30 (2,300 operations)
  
  Scoring with k8s/v2 data only (2,300 operations).
  For full period, use: evidra scorecard --canon-version k8s/v1
```

The scorecard defaults to the latest version and warns if data is
mixed. User can force a specific version. This is simple, honest,
and doesn't require re-canonicalization of historical data.

**Compare command with version awareness:**

```
evidra compare --agent claude-code --versions v1.2,v1.3

# If the two versions used different canonicalization:
WARNING: Agent versions used different canonicalization.
  v1.2: k8s/v1 (1,800 operations)
  v1.3: k8s/v2 (600 operations)
  
  Comparison may not be perfectly comparable.
  Intent digests computed with different rules.
```

Honest warnings beat silent inconsistency.

---

## 6. Parse Failures as First-Class Events

Section 9 says "failures should be treated as protocol entries
with parse_error fields." This is correct but needs to be stronger.

A parse failure means Evidra cannot inspect the artifact. This is
the worst state — blind operation. The prescription should reflect
this:

```yaml
prescription:
  prescription_id: "prs-..."
  risk_level: "high"              # always high on parse failure
  risk_details:
    - "artifact parse failure: invalid YAML at line 42"
    - "canonicalization incomplete — operation cannot be inspected"
  parse_error:
    adapter: "k8s"
    error: "yaml: line 42: mapping values are not allowed here"
  artifact_digest: "sha256:..."   # raw digest still computed
  intent_digest: null             # cannot compute — artifact unparseable
  canonicalization_version: null
```

Parse failures are also a signal source. Not one of the five core
signals — but useful metadata in the scorecard:

```
SCORECARD NOTES
  Parse failures: 3 (0.07%)
  → 2x invalid YAML from helm template
  → 1x unsupported terraform plan schema version
```

Parse failure rate should not be in the reliability score (it's
not the agent's behavioral fault if the artifact is malformed —
it might be an infrastructure issue). But it's worth tracking
for operational health.

---

## 7. What This Means for Implementation Priority

The canonicalization contract changes the implementation order.
Domain adapters are not a nice-to-have for v0.3.0 — they are the
foundation. Without stable canonicalization:

- Retry loop detection doesn't work (intent_digest unstable)
- Blast radius signal doesn't work (resource_count unavailable)
- Benchmark comparisons are meaningless (scores based on noise)

Revised priority:

```
v0.3.0:
  1. Canonicalization contract (this document, frozen)
  2. K8s adapter with golden corpus (15-20 cases)
  3. Terraform adapter with golden corpus (10-15 cases)
  4. prescribe/report MCP tools
  5. Evidence chain with canonicalization_version

v0.4.0:
  6. Five signal detectors
  7. Reliability score
  8. Scorecard CLI
  9. CI integration
```

Adapters first. Signals second. Without stable adapters, signals
are noise.

---

## Summary

The canonicalization contract is right. This review tightens it:

1. **artifact_digest vs intent_digest** — different purposes,
   never confuse them. Artifact = protocol integrity (raw bytes).
   Intent = behavioral identity (canonical form).

2. **Frozen noise list** — per canonicalization_version. Adding a
   noise field requires version bump and golden corpus update.

3. **Terraform intent without field values** — only resource
   identity + action type. Field values are for risk detectors,
   not for intent identity. Avoids computed-field instability.

4. **Golden corpus structure** — directories with fixed file names,
   append-only, minimum 25-35 cases across k8s and terraform.

5. **Version-aware scorecard** — never silently mix versions. Warn
   and default to latest. User can force specific version.

6. **Parse failures as first-class** — always high risk, intent_digest
   null, tracked in scorecard notes.

7. **Implementation priority** — adapters + corpus first, signals
   second. Canonicalization is the foundation, not a feature.
