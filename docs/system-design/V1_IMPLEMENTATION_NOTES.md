# Evidra v1 Implementation Notes

**Status:** Active reference for delivered v1 architecture work.
**Supersedes:** `DETECTOR_ARCHITECTURE.md`, `IMPLEMENTATION_PROMPT.md`.

---

## 1. Why This File Exists

This file preserves the implementation-critical decisions that were previously
split across a design-heavy detector document and a step-by-step implementation
prompt.

Use this as the stable handoff reference for:

- detector architecture and contribution model
- signal and score integration rules
- release-gate validation expectations

---

## 2. Detector Architecture (Delivered)

### Package layout

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
  terraform/gcp/
  terraform/azure/
  docker/*.go
  ops/*.go
```

### Core model

- `Detector` is self-registering (`init()` + `Register`).
- One detector pattern lives in one file.
- `TagMetadata` is required for every detector and exported via registry calls.
- `RunAll` provides native deterministic tags.
- `TagProducer` is the extension boundary for non-native sources.
- `ProduceAll` merges producers with de-duplication.

### Vocabulary levels

Evidra keeps three distinct vocabularies:

- resource risk (detectors on artifact content)
- operation risk (detectors on canonical action context)
- behavior signals (signal engine on evidence sequences)

Detectors emit only resource/operation risks; signals are computed later from
prescribe/report behavior.

---

## 3. Delivered Detector Scope

Current deterministic detector set is 20 tags:

- K8s: privileged, host namespace escape, hostPath, docker socket, run as root,
  dangerous capabilities, cluster-admin binding, writable rootfs
- Ops: mass delete, kube-system mutation, namespace delete
- Terraform/AWS: wildcard IAM (strict + broad), S3 public access, security group
  open, RDS public, EBS unencrypted
- Docker/Compose: privileged, host network, socket mount

CLI verification command:

```bash
evidra detectors list
```

---

## 4. Signal + Scoring Rules (Delivered)

Signal pipeline currently includes 7 behavior signals:

- `protocol_violation`
- `artifact_drift`
- `retry_loop`
- `blast_radius`
- `new_scope`
- `repair_loop`
- `thrashing`

Score model adds:

- `repair_loop` bonus (negative weight, reduces penalty)
- `thrashing` penalty (positive weight, increases penalty)
- `signal_profiles` map (`none|low|medium|high`) for each signal

Scoring confidence/min-operations behavior remains unchanged (`MinOperations=100`).

---

## 5. Validation Gate

Signal validation scripts are the operational quality gate:

- `tests/signal-validation/helpers.sh`
- `tests/signal-validation/validate-signals-engine.sh`

The sequence harness now covers A-G scenarios, including explicit repair and
thrashing cases.

Important: score comparison between scenarios is only meaningful once operation
count reaches scorecard sufficiency (`MinOperations`).

---

## 6. Remaining Scope (Not Delivered Here)

- REST API + hosted LLM integration remains a separate dependency stream.
- External scanner mappings are scaffolded through `TagProducer` and SARIF
  producer but need production mapping/config lifecycle.
- Intent graph is not required for current delivered signal set.
