# Evidra Canonicalization Contract v1

## Status
Frozen contract. Changes to canonical output require version bump.

## Purpose
This document is the ABI of Evidra's domain adapters. It defines
exactly what is parsed, what is discarded, what produces the
intent_digest, and which libraries are used. If the output changes,
the version bumps. If the version bumps, golden corpus updates.
No exceptions.

---

## Adapter Interface (Tool-Agnostic)

Evidra's canonicalization is extensible. Any system that produces
infrastructure artifacts can integrate by implementing one interface:

```go
type Adapter interface {
    // Name returns the adapter identifier (e.g. "k8s/v1", "terraform/v1").
    Name() string

    // CanHandle returns true if this adapter can parse the given tool name.
    CanHandle(tool string) bool

    // Canonicalize transforms raw artifact bytes into a CanonResult.
    Canonicalize(tool, operation string, rawArtifact []byte) (CanonResult, error)
}
```

Adapter selection at prescribe time:

```
tool="kubectl" → K8s adapter (built-in)
tool="terraform" → Terraform adapter (built-in)
tool="pulumi" → no built-in adapter → generic fallback
tool="pulumi" + registered PulumiAdapter → Pulumi adapter
```

External integrations have two paths:

**Path A — implement Adapter.** For tools that produce raw artifacts
Evidra should parse. The adapter extracts resource identity and
computes shape hash from the artifact format.

**Path B — pre-canonicalized prescribe.** For tools that already
know their resource identity. They send canonical_action directly
in the prescribe call. Evidra computes artifact_digest, runs risk
detectors, writes evidence. The adapter is bypassed.

Both paths produce identical evidence entries, signals, and scores.

Built-in adapters shipped with Evidra:

| Adapter | Tools handled | Delivery |
|---------|--------------|----------|
| k8s/v1 | kubectl, oc | v0.3.0 |
| tf/v1 | terraform | v0.3.0 |
| helm/v1 | helm (via k8s) | v0.3.0 |
| generic/v1 | everything else | v0.3.0 |
| argocd/v1 | argocd | v0.5.0 (spec reserved) |

---

## 1. Two Digests, Two Purposes

```
artifact_digest = SHA256(raw bytes as received)
  → protocol integrity (did the agent modify what it sent?)
  → computed BEFORE any parsing
  → raw bytes in, hash out
  → trailing newline = different digest (correct)

intent_digest = SHA256(canonical JSON of canonical_action)
  → behavioral identity (is the agent doing the same thing?)
  → computed AFTER full canonicalization
  → YAML reordering = same digest (correct)
```

Same artifact_digest implies same intent_digest.
Same intent_digest does NOT imply same artifact_digest.

These are independent and must never be confused.

---

## 2. Canonical Action Schema

Every adapter produces the same output structure. This is the
contract between adapters and signals/scorecard.

```json
{
  "tool": "kubectl",
  "operation": "apply",
  "operation_class": "mutating",
  "resource_identity": [
    {
      "api_version": "apps/v1",
      "kind": "Deployment",
      "namespace": "staging",
      "name": "api-server"
    }
  ],
  "scope_class": "staging",
  "resource_count": 1,
  "risk_tags": []
}
```

Fields:

| Field | Type | Source | Purpose |
|-------|------|--------|---------|
| tool | string | from request | kubectl, terraform, helm, argocd |
| operation | string | from request | apply, delete, destroy, upgrade, sync |
| operation_class | string | derived | mutating, destructive, read-only |
| resource_identity | array | from adapter | sorted list of resource identifiers |
| scope_class | string | derived | production, staging, development, unknown |
| resource_count | int | from adapter | number of resources in artifact |
| resource_shape_hash | string | from adapter | SHA256 of normalized spec content |
| risk_tags | array | from detectors | catastrophic risk patterns found |

**resource_identity** is the stable identity. Same deployment with
different image tags → same resource_identity. This is intentional —
identity tracks "what resources" not "what configuration."

**resource_shape_hash** captures the configuration content. Same
deployment with different image tags → different shape_hash. This
field exists for one purpose: retry loop detection. Without it,
three sequential deploys of different versions would false-trigger
as a retry loop (same intent_digest, but different actual content).

**resource_shape_hash computation:**
- K8s: SHA256 of the canonical JSON of the full spec subtree
  (after noise removal, keys sorted).
- Terraform: SHA256 of sorted resource addresses + action types
  (no field values — already the intent for TF).
- Generic: SHA256 of the raw artifact bytes.

**risk_tags** are populated by catastrophic risk detectors AFTER
canonicalization. They are NOT part of intent_digest. They are
metadata attached to the prescription.

### intent_digest computation

```
intent_digest = SHA256(canonical_json({
  tool,
  operation,
  operation_class,
  resource_identity,  // sorted
  scope_class,
  resource_count
}))
```

resource_shape_hash excluded from intent_digest. Intent captures
"what resources are being touched." Shape captures "what's the
content." These are different questions.

risk_tags excluded. They are analysis output, not intent identity.

### Retry loop detection uses BOTH digests

```
retry_loop triggers when:
  same intent_digest (same resources targeted)
  AND same resource_shape_hash (same content, not just same target)
  AND N occurrences within T minutes
  AND all denied or all failed
```

This means:
- apply deployment A (image:v1) → apply deployment A (image:v2)
  → different shape_hash → NOT a retry loop (correct)
- apply deployment A (image:v1) → apply deployment A (image:v1)
  → same shape_hash → counted toward retry loop (correct)

---

## 3. Canonical JSON Serialization

All canonical JSON follows these rules:

- Object keys: sorted lexicographically (a-z)
- Arrays in resource_identity: sorted by (api_version, kind, namespace, name)
- Numbers: integers only (no floats in identity fields)
- Strings: UTF-8, no trailing whitespace
- Null fields: omitted entirely
- No pretty-printing: compact single-line JSON
- Encoding: Go's `encoding/json` with sorted map keys

```go
import "encoding/json"

func canonicalJSON(v interface{}) ([]byte, error) {
    return json.Marshal(v)  // Go sorts map keys by default
}
```

This is deterministic. Same input → same bytes → same SHA256.

---

## 4. Kubernetes Adapter (k8s/v1)

**Delivery: v0.3.0 (SUPPORTED)**

### 4.1 Library

```
k8s.io/apimachinery v0.31+
```

Specifically:
- `k8s.io/apimachinery/pkg/apis/meta/v1/unstructured` — parse YAML/JSON into unstructured objects
- `k8s.io/apimachinery/pkg/runtime/serializer/yaml` — YAML decoding with GVK detection
- `k8s.io/apimachinery/pkg/util/yaml` — multi-doc YAML stream splitting

Why this library:
- Official Kubernetes project. Used by kubectl, helm, kustomize, Gatekeeper, Kyverno.
- Handles every apiVersion, every Kind, including CRDs.
- Unstructured access — no typed structs needed. ~2MB binary cost.
- GVK detection built-in.
- Battle-tested against every Kubernetes version since 1.10.

Why NOT k8s.io/api:
- k8s.io/api pulls typed structs for every API resource. ~15MB binary cost.
- Not needed. We access fields by path, not by struct.

Why NOT manual YAML parsing (gopkg.in/yaml.v3):
- No GVK detection.
- No multi-doc YAML splitting.
- No Kubernetes-specific normalization.
- Must handle every encoding edge case manually.

### 4.2 Input Types

Accepted raw artifacts:

| Source | Format | Notes |
|--------|--------|-------|
| kubectl manifest | YAML/JSON, single or multi-doc | Most common |
| helm template output | Multi-doc YAML | `helm template` renders to YAML |
| kustomize build output | Multi-doc YAML | `kustomize build` renders to YAML |
| kubectl diff output | Multi-doc YAML | Pre-apply diff |

All sources produce Kubernetes YAML/JSON. The adapter treats them
identically. It does not care where the YAML came from.

### 4.3 Parsing Pipeline

```
Raw bytes
  → Split multi-doc YAML (k8s.io/apimachinery/pkg/util/yaml)
  → For each document:
      → Decode into unstructured.Unstructured
         (yaml.NewDecodingSerializer)
      → Extract GVK (apiVersion + kind)
      → Extract identity (namespace, name)
      → Discard noise fields
  → Sort objects by (apiVersion, kind, namespace, name)
  → Produce canonical_action
```

### 4.4 Identity Extraction

Per object:

```go
identity := ResourceIdentity{
    APIVersion: obj.GetAPIVersion(),   // "apps/v1"
    Kind:       obj.GetKind(),         // "Deployment"
    Namespace:  obj.GetNamespace(),    // "staging" (empty for cluster-scoped)
    Name:       obj.GetName(),         // "api-server"
}
```

These four fields are the ONLY fields that enter intent_digest
for Kubernetes. Not the spec. Not the labels. Not the annotations.
Not the containers. Not the images.

Why: intent identity is "what resources are being touched." The
specific configuration of those resources is captured separately
in resource_shape_hash (for retry loop detection) and read by
risk detectors (from the raw artifact).

### 4.5a Resource Shape Hash

```go
func computeShapeHash(obj *unstructured.Unstructured) string {
    // Extract spec subtree (after noise removal)
    spec, exists, _ := unstructured.NestedMap(obj.Object, "spec")
    if !exists {
        // For resources without spec (ConfigMap, Secret)
        data, exists, _ := unstructured.NestedMap(obj.Object, "data")
        if !exists {
            return ""
        }
        spec = data
    }
    // Canonical JSON of spec → SHA256
    canonical, _ := json.Marshal(spec)  // Go sorts map keys
    hash := sha256.Sum256(canonical)
    return fmt.Sprintf("sha256:%x", hash)
}
```

For multi-doc artifacts: resource_shape_hash is the SHA256 of
the concatenated per-object shape hashes (sorted by identity).
This gives a single hash for the entire artifact's content.

### 4.5 Noise Fields — Frozen List (k8s/v1)

These fields are discarded during canonicalization. The list is
frozen for k8s/v1. Adding or removing a field requires k8s/v2.

```yaml
# k8s/v1 noise fields — FROZEN
remove_metadata_fields:
  - creationTimestamp
  - managedFields
  - resourceVersion
  - uid
  - generation
  - selfLink
  - deletionTimestamp
  - deletionGracePeriodSeconds

remove_top_level:
  - status           # entire subtree

remove_annotations:
  - kubectl.kubernetes.io/last-applied-configuration
  - deployment.kubernetes.io/revision
  - meta.helm.sh/release-name
  - meta.helm.sh/release-namespace
  - app.kubernetes.io/managed-by
  - kubernetes.io/change-cause
  - control-plane.alpha.kubernetes.io/leader
```

Why these:
- metadata.managedFields: changes on every server-side apply, no user intent
- metadata.resourceVersion: changes on every API write
- metadata.uid: unique per object instance, not per intent
- status: runtime state, not desired state
- last-applied-configuration: copy of the manifest itself, recursive noise
- helm annotations: management metadata, not deployment intent
- change-cause: set by `kubectl apply --record`, drifts with every apply
- leader annotation: control plane election state, not user intent

What we KEEP:
- metadata.labels (all, sorted by key)
- metadata.annotations (all except noise list, sorted by key)
- spec (entire subtree — for risk detectors only, NOT for intent_digest)

### 4.6 Noise Removal Implementation

```go
func removeNoise(obj *unstructured.Unstructured) {
    // Remove metadata noise
    metadata := obj.Object["metadata"].(map[string]interface{})
    for _, field := range noiseMetadataFields {
        delete(metadata, field)
    }

    // Remove noisy annotations
    annotations := obj.GetAnnotations()
    for _, key := range noiseAnnotations {
        delete(annotations, key)
    }
    if len(annotations) > 0 {
        obj.SetAnnotations(annotations)
    } else {
        delete(metadata, "annotations")
    }

    // Remove status
    delete(obj.Object, "status")
}
```

### 4.7 Resource Count

```go
resource_count = len(documents)  // number of objects in multi-doc
```

For List kinds: resource_count = number of items in the list.

### 4.8 Risk Detector Field Access

Catastrophic risk detectors read the raw parsed object (post-noise-
removal, pre-canonicalization). They use unstructured nested access:

```go
// privileged container detector
containers, _, _ := unstructured.NestedSlice(
    obj.Object, "spec", "template", "spec", "containers")
for _, c := range containers {
    container := c.(map[string]interface{})
    privileged, _, _ := unstructured.NestedBool(
        container, "securityContext", "privileged")
    if privileged {
        riskTags = append(riskTags, "privileged security context")
    }
}

// hostPath detector
volumes, _, _ := unstructured.NestedSlice(
    obj.Object, "spec", "template", "spec", "volumes")
for _, v := range volumes {
    volume := v.(map[string]interface{})
    _, hasHostPath, _ := unstructured.NestedMap(volume, "hostPath")
    if hasHostPath {
        riskTags = append(riskTags, "host filesystem access")
    }
}
```

Detectors work on any workload shape (Deployment, DaemonSet,
StatefulSet, Job, CronJob) because they navigate the common
`spec.template.spec` path. For Pods, path is `spec` directly.

---

## 5. Terraform Adapter (tf/v1)

**Delivery: v0.3.0 (SUPPORTED)**

### 5.1 Library

```
github.com/hashicorp/terraform-json v0.24+
```

Specifically the `tfjson.Plan` type. This is HashiCorp's official
library for parsing `terraform show -json` output.

Why this library:
- Official HashiCorp project. Designed exactly for this use case.
- Typed Go structs for the plan JSON schema.
- Supports plan format versions >= 0.1, < 2.0.
- Built-in version validation.
- Used by Terraform Cloud, Spacelift, Atlantis, Terramate.

Why NOT manual JSON parsing:
- Plan JSON schema has nested structures (resource_changes, change, before/after).
- Unknown values, sensitive values need typed handling.
- Schema versioning built into the library.

### 5.2 Input Type

One input: `terraform show -json <planfile>` output.

This is the ONLY accepted Terraform input. Not HCL. Not state
files. Not terraform graph output. Only plan JSON.

### 5.3 Parsing Pipeline

```
Raw bytes (plan JSON)
  → json.Unmarshal into tfjson.Plan
  → Validate format version (plan.Validate())
  → Extract resource_changes[]
  → For each resource_change:
      → Extract address, type, actions
      → Skip data sources (mode == "data")
  → Sort by address
  → Produce canonical_action
```

### 5.4 Identity Extraction

Per resource change:

```go
type TerraformResourceIdentity struct {
    Type    string   `json:"type"`      // "aws_security_group"
    Name    string   `json:"name"`      // "web"
    Actions []string `json:"actions"`   // ["create"] or ["update"] or ["delete"]
}
```

Three fields per resource. address is NOT used as identity.

Why not address: Terraform addresses include module paths
(`module.vpc.aws_security_group.web`). When users refactor
modules (`module.vpc` → `module.network`), the address changes
but the resource is the same. This causes intent_digest churn
and false drift signals.

Instead, identity is `type + name + actions`. This is stable
across module refactors. Two resources with the same type and
name in different modules are considered the same identity —
this is a trade-off for stability. In practice, name collisions
across modules are rare, and when they occur, the resource_count
and action types still capture the intent correctly.

```go
func extractTerraformIdentity(rc *tfjson.ResourceChange) TerraformResourceIdentity {
    return TerraformResourceIdentity{
        Type:    rc.Type,             // "aws_security_group"
        Name:    rc.Name,             // "web" (not rc.Address)
        Actions: rc.Change.Actions,   // ["create"]
    }
}
```

Full address is preserved in the evidence chain for human
readability but is NOT part of the canonical_action or
intent_digest.

### 5.5 Resource Count

```go
resource_count = len(resource_changes)  // excluding data sources
```

For blast radius signal: count only destructive changes:

```go
destroy_count = count(rc where "delete" in rc.Actions)
```

### 5.6 Noise Elimination

Terraform plan JSON contains a lot of structure we don't need:

```yaml
# tf/v1 — what we READ
read_fields:
  - resource_changes[].address
  - resource_changes[].type
  - resource_changes[].change.actions
  - resource_changes[].mode         # to filter data sources
  - resource_changes[].change.after # for risk detectors ONLY

# tf/v1 — what we IGNORE for intent
ignore_for_intent:
  - format_version
  - terraform_version
  - planned_values         # redundant with resource_changes
  - prior_state            # previous state, not intent
  - configuration          # HCL config, not plan
  - resource_changes[].change.before
  - resource_changes[].change.after_unknown
  - resource_changes[].change.before_sensitive
  - resource_changes[].change.after_sensitive
  - resource_changes[].provider_name
  - resource_changes[].change.after  # for intent (used by detectors separately)
  - output_changes
  - variables
```

### 5.7 Risk Detector Field Access

Catastrophic risk detectors read `change.after` from the raw
parsed plan (the `tfjson.Plan` struct):

```go
// open security group detector
for _, rc := range plan.ResourceChanges {
    if rc.Type == "aws_security_group_rule" || rc.Type == "aws_security_group" {
        after := rc.Change.After.(map[string]interface{})
        ingress, _ := after["ingress"].([]interface{})
        for _, rule := range ingress {
            r := rule.(map[string]interface{})
            cidrs, _ := r["cidr_blocks"].([]interface{})
            for _, cidr := range cidrs {
                if cidr == "0.0.0.0/0" {
                    riskTags = append(riskTags, "world-open ingress")
                }
            }
        }
    }
}

// wildcard IAM detector
for _, rc := range plan.ResourceChanges {
    if strings.Contains(rc.Type, "iam") && strings.Contains(rc.Type, "policy") {
        after := rc.Change.After.(map[string]interface{})
        // parse policy document JSON, check for Action: "*"
    }
}
```

Detectors read the full `change.after` for specific resource types.
This data is NOT in the canonical_action. Separate concerns.

---

## 6. Helm Adapter (helm/v1)

**Delivery: v0.3.0 via K8s adapter (SUPPORTED — helm template output
parsed as Kubernetes YAML, no separate Helm library)**

### 6.1 Approach

Helm is NOT parsed as Helm. Helm template output is Kubernetes
YAML. The Helm adapter is a thin wrapper around the K8s adapter.

```
helm template <chart> → multi-doc YAML → K8s adapter (k8s/v1)
```

The agent must send rendered YAML (output of `helm template`),
not the chart archive. Evidra does not parse Helm charts, values
files, or Chart.yaml.

### 6.2 Library

Same as K8s adapter: `k8s.io/apimachinery`. No Helm-specific
library needed.

### 6.3 Additional Identity

Helm adds one field to canonical_action that kubectl doesn't:

```json
{
  "tool": "helm",
  "operation": "upgrade",
  "operation_class": "mutating",
  "resource_identity": [
    // same as k8s: apiVersion, kind, namespace, name per object
  ],
  "scope_class": "production",
  "resource_count": 12
}
```

The tool field is "helm" (not "kubectl") to distinguish in signals.
The resource_identity is identical to k8s — because the artifact
IS k8s YAML.

### 6.4 Helm-Specific Noise

In addition to k8s/v1 noise list, Helm output often contains:

```yaml
# helm/v1 additional noise annotations
remove_annotations:
  - helm.sh/hook
  - helm.sh/hook-weight
  - helm.sh/hook-delete-policy
  - helm.sh/resource-policy
```

These are Helm lifecycle annotations. They don't affect resource
identity.

---

## 7. ArgoCD Adapter (argocd/v1)

**Delivery: SPEC RESERVED (contract defined, implementation v0.5.0+)**

### 7.1 Approach

ArgoCD is the first "extension" adapter beyond the core two
(k8s, terraform). It demonstrates how new tools plug in.

ArgoCD operations target applications, not raw manifests. The
adapter handles two types of input:

**Sync operations:** ArgoCD syncs K8s manifests. The raw artifact
is the rendered manifest set (same as kubectl). Parsed by K8s
adapter.

**App management operations:** Create/delete/modify ArgoCD
Application resources. The raw artifact is the Application YAML.

### 7.2 Library

Same as K8s adapter: `k8s.io/apimachinery`. ArgoCD Application
is a Kubernetes CRD — parsed as unstructured.

### 7.3 Identity

For sync: same as K8s adapter (list of resource identities).
For app management:

```json
{
  "tool": "argocd",
  "operation": "sync",
  "operation_class": "mutating",
  "resource_identity": [
    {
      "api_version": "argoproj.io/v1alpha1",
      "kind": "Application",
      "namespace": "argocd",
      "name": "payments-service"
    }
  ],
  "scope_class": "production",
  "resource_count": 1
}
```

### 7.4 Adapter Extensibility Pattern

ArgoCD demonstrates the pattern for adding any new tool:

1. Define input format (what raw artifact does the agent send?)
2. Choose parsing library (prefer official, from tool creators)
3. Extract resource_identity (what's being touched?)
4. Map to operation_class (destructive, mutating, read-only)
5. Map to scope_class (production, staging, etc.)
6. Write golden corpus (minimum 5 cases)
7. Version as `<tool>/v1`

Future adapters follow the same pattern:
- Ansible: playbook output → task identities
- Pulumi: preview JSON → resource identities
- Crossplane: composite resource YAML → K8s adapter
- CloudFormation: changeset JSON → resource identities

Each new adapter adds one dependency (the official parsing library)
and one corpus directory. The canonical_action schema is identical.
Signals, scorecard, and comparison work without changes — they
consume canonical_actions, not tool-specific data.

---

## 8. OpenShift Adapter

**Delivery: v0.3.0 via K8s adapter (SUPPORTED — no separate adapter needed)**

OpenShift is Kubernetes. The K8s adapter handles OpenShift
manifests without modification. OpenShift-specific resources
(DeploymentConfig, Route, BuildConfig) are parsed as unstructured
CRDs — same path as any CRD.

No separate adapter needed. No additional library.

---

## 9. Universal Fallback Adapter (generic/v1)

**Delivery: v0.3.0 (SUPPORTED)**

For tools not covered by specific adapters (Ansible, Pulumi,
Crossplane, CloudFormation, custom scripts), a generic adapter
provides minimal canonicalization.

### 9.1 Input

Any JSON or YAML that the agent sends as raw_artifact.

### 9.2 Library

Standard library only:
- `encoding/json`
- `gopkg.in/yaml.v3` (for YAML-to-JSON conversion)

### 9.3 Identity

Generic adapter cannot extract structured resource identity (no
schema knowledge). Instead, it uses the artifact hash as a single
opaque identity:

```json
{
  "tool": "unknown",
  "operation": "apply",
  "operation_class": "mutating",
  "resource_identity": [
    {
      "kind": "opaque",
      "digest": "sha256:abc123..."
    }
  ],
  "scope_class": "unknown",
  "resource_count": 1,
  "resource_shape_hash": "sha256:abc123...",
  "risk_tags": []
}
```

resource_identity contains a single entry with the SHA256 of the
raw artifact. resource_count is always 1 (we can't know the real
count without schema knowledge).

This gives:
- **Retry loop detection:** works. Same artifact → same digest →
  retry detected. Different artifact → different digest → not a
  retry.
- **Artifact drift:** works. prescribe digest vs report digest.
- **Protocol violations:** works. prescribe/report protocol is
  tool-independent.
- **Blast radius:** always 1 (we can't count resources). Signal
  never fires but doesn't false-fire either.
- **New scope:** works at tool+scope_class level (coarser than
  specific adapters).

### 9.4 Limitations

- Blast radius signal effectively disabled (always 1)
- No catastrophic risk detectors (no schema to inspect)
- risk_level from matrix only (operation_class × scope_class)
- resource_identity is opaque — human-readable scorecard shows
  digest, not resource names
- Lower-quality scorecard (fewer signals, less detail)

Generic adapter is explicitly a fallback. It provides protocol +
basic signals, not full inspection. When a specific adapter is
needed, it's built.

---

## 10. Scope Class Resolution

scope_class is derived from the environment and namespace fields.

### 10.1 Resolution Order

```
1. If request.environment is set → map to scope_class
2. If namespace is set → map to scope_class
3. If neither → scope_class = "unknown"
```

### 10.2 Mapping Table (frozen for v1)

```yaml
scope_class_mappings:
  production:
    environments: [production, prod, prd, live]
    namespaces:   [production, prod, prd, live]
  staging:
    environments: [staging, stg, stage, preprod, pre-prod, uat]
    namespaces:   [staging, stg, stage, preprod, uat]
  development:
    environments: [dev, development, sandbox, test, local]
    namespaces:   [dev, development, sandbox, test]
  unknown: []   # default for anything not matched
```

Matching: case-insensitive, prefix match. "prod-team-a" matches
"prod" → production. "staging-b" matches "staging" → staging.

Unknown is not silent. Operations in unknown scope_class are
tracked. new_scope fires on first operation in unknown.

### 10.3 Custom Mappings

Users can extend mappings via config (not override frozen list):

```yaml
# ~/.evidra/config.yaml
scope_class_overrides:
  production:
    namespaces: [blue, green]  # canary environments are production
  staging:
    namespaces: [qa, perf]
```

Custom mappings don't change the canonical contract — they only
add patterns to the resolution logic. Same scope_class values.

---

## 11. Operation Class Resolution

```yaml
operation_class_mappings:
  destructive:
    - kubectl.delete
    - terraform.destroy
    - helm.uninstall
    - argocd.delete
  mutating:
    - kubectl.apply
    - kubectl.patch
    - terraform.apply
    - helm.upgrade
    - helm.install
    - argocd.sync
    - argocd.create
  read-only:
    - kubectl.get
    - kubectl.describe
    - terraform.plan
    - terraform.show
    - helm.list
    - helm.status
    - argocd.get
```

Unrecognized operations default to "mutating" (conservative).

---

## 12. Canonicalization Guarantees

These guarantees define the contract. If any guarantee is broken,
it's a bug.

### intent_digest stability

| Input change | intent_digest | Guarantee |
|-------------|---------------|-----------|
| YAML field reorder | unchanged | Same object, different formatting |
| Multi-doc YAML reorder | unchanged | Objects sorted by identity |
| Whitespace / indentation change | unchanged | Canonical JSON is compact |
| Annotation noise field changes | unchanged | Noise list filtered |
| metadata.resourceVersion changes | unchanged | Noise field |
| metadata.managedFields changes | unchanged | Noise field |
| status subtree changes | unchanged | Removed entirely |
| Comment added to YAML | unchanged | Comments stripped by parser |
| Image tag changed | unchanged | Spec not in intent |
| Replicas changed | unchanged | Spec not in intent |
| Resource name changed | **changed** | Identity field |
| Namespace changed | **changed** | Identity field |
| Kind changed | **changed** | Identity field |
| New resource added to multi-doc | **changed** | resource_count changed |
| Resource removed from multi-doc | **changed** | resource_count changed |

### resource_shape_hash stability

| Input change | resource_shape_hash | Guarantee |
|-------------|---------------------|-----------|
| YAML field reorder | unchanged | Canonical JSON sorts keys |
| Image tag changed | **changed** | Spec content changed |
| Replicas changed | **changed** | Spec content changed |
| Label added | **changed** | Spec content changed |
| Annotation noise changed | unchanged | Noise filtered before hashing |
| Comment added | unchanged | Comments stripped by parser |

### artifact_digest stability

| Input change | artifact_digest | Guarantee |
|-------------|-----------------|-----------|
| Any byte change | **changed** | Raw bytes, no parsing |
| Trailing newline added | **changed** | Raw bytes |
| Same content, different encoding | **changed** | Raw bytes |

---

## 13. Rename / Recreate Patterns (Metadata, Not Identity)

Kubernetes "rename" is delete + create. Deployment `api-v1`
renamed to `api-v2` produces two different resource_identities.
This is correct — they ARE different resources.

For teams that want to track rename patterns (e.g., "this agent
frequently renames deployments"), the evidence chain can store
optional metadata:

```json
{
  "rename_hints": {
    "previous_names_in_namespace": ["api-v1"],
    "owner_references_hash": "sha256:..."
  }
}
```

This is NOT part of canonical_action, intent_digest, or any signal.
It's optional metadata for human analysis. Implementation priority:
low. Backlog item for v0.5.0+.

---

## 14. Versioning

### 12.1 Version Format

```
<domain>/<version>
```

Examples: `k8s/v1`, `tf/v1`, `helm/v1`, `argocd/v1`, `generic/v1`

### 12.2 Version Bump Rules

| Change | Version bump? |
|--------|--------------|
| Add field to noise list | YES — k8s/v1 → k8s/v2 |
| Remove field from noise list | YES |
| Change sort order | YES |
| Change canonical JSON rules | YES |
| Add new risk detector | NO (risk_tags not in intent_digest) |
| Fix parser bug that changes output | YES |
| Fix parser bug that doesn't change output | NO |
| Update library minor version | NO (unless output changes) |
| Update library major version | YES (assume output may change) |

### 12.3 Protocol Entry Fields

Every prescription includes:

```json
{
  "canonicalization_version": "k8s/v1",
  "adapter_version": "0.3.0"
}
```

### 12.4 Scorecard Version Awareness

Scorecard never silently mixes versions. If the scoring window
spans a version change:

```
WARNING: Canonicalization version changed during scoring period.
  k8s/v1: 2,100 operations
  k8s/v2: 2,300 operations

  Scoring with k8s/v2 data only.
```

---

## 15. Golden Corpus

### 13.1 Structure

```
tests/corpus/
  k8s/v1/
    deployment_simple/
      input.yaml
      canonical_action.json
      intent_digest.txt
      metadata.json
    deployment_multicontainer/
    statefulset_with_volumes/
    cronjob_privileged/
    multidoc_helm_output/
    multidoc_kustomize/
    service_loadbalancer/
    ingress_tls/
    rbac_clusterrole/
    crd_unstructured/
    pod_hostpath/
    daemonset_hostipc/
    namespace_create/
    configmap_simple/
    list_kind/
  terraform/v1/
    sg_create/
      input.json
      canonical_action.json
      intent_digest.txt
      metadata.json
    sg_open_world/
    iam_wildcard/
    s3_public/
    multi_resource_create/
    destroy_single/
    destroy_mass/
    update_in_place/
    create_and_delete_mixed/
    module_resources/
```

### 13.2 Rules

- Cases are append-only.
- Modifying or removing a case requires canonicalization_version bump.
- Each case has metadata.json with: description, added_in_version,
  rationale.
- Tests assert byte-for-byte match of canonical_action.json.
- Tests assert exact match of intent_digest.txt.
- CI runs corpus tests on every commit. Failure = build failure.

### 13.3 Minimum Corpus for v1 Launch

| Domain | Cases | Required |
|--------|-------|----------|
| k8s/v1 | 15 | Deployment, Pod, CronJob, StatefulSet, DaemonSet, Service, Ingress, RBAC, ConfigMap, Namespace, List, multi-doc, CRD, hostPath, privileged |
| tf/v1 | 10 | Create, destroy, update, mixed, SG, IAM, S3, multi-resource, module, mass-destroy |
| helm/v1 | 3 | Simple chart output, chart with CRDs, chart with hooks |

28 cases minimum. Each one is an adapter ABI guarantee.

---

## 16. Library Summary

| Adapter | Library | Import | Binary cost | License |
|---------|---------|--------|-------------|---------|
| k8s/v1 | k8s.io/apimachinery | `k8s.io/apimachinery/pkg/apis/meta/v1/unstructured` | ~2MB | Apache 2.0 |
| tf/v1 | hashicorp/terraform-json | `github.com/hashicorp/terraform-json` | ~200KB | MPL 2.0 |
| helm/v1 | (reuses k8s) | — | 0 | — |
| argocd/v1 | (reuses k8s) | — | 0 | — |
| generic/v1 | stdlib + yaml.v3 | `encoding/json`, `gopkg.in/yaml.v3` | ~100KB | MIT |

Total binary cost for all adapters: ~2.3MB.
Compare: OPA runtime alone was ~15MB.

Two external dependencies: k8s.io/apimachinery and terraform-json.
Both are official, widely-used, actively-maintained libraries from
the creators of the tools they parse.

---

## 17. What Each Adapter Does NOT Parse

| Adapter | Does NOT parse |
|---------|---------------|
| k8s/v1 | Helm charts, Kustomize overlays, HCL, Jsonnet, CUE, cdk8s |
| tf/v1 | HCL files, .tf files, state files, graph output |
| helm/v1 | Chart.yaml, values.yaml, chart archives (.tgz) |
| argocd/v1 | ArgoCD server API responses, git repo contents |
| generic/v1 | Anything structured — just identity extraction |

Evidra accepts RENDERED OUTPUT only. The tool renders its own
output. Evidra parses the standard format. No source language
parsing.

---

## 18. Do NOT

- Do not parse HCL, Helm charts, Kustomize overlays, or any
  source format. Accept rendered output only.
- Do not add k8s.io/api (typed API structs). k8s.io/apimachinery
  (unstructured) is sufficient.
- Do not put spec fields into intent_digest. Intent = identity.
  Spec = for risk detectors.
- Do not put risk_tags into intent_digest. They are analysis
  output, not intent identity.
- Do not modify the noise list without a version bump.
- Do not modify golden corpus cases without a version bump.
- Do not add field values to Terraform intent. Only address +
  type + actions.
- Do not write custom YAML/JSON parsers. Use the official
  libraries from the tool creators.
