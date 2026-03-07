# Evidra — Parallel Execution Plan

**Two tracks, one release.** Deterministic detectors (baseline) and LLM prediction (breadth) ship together.

---

## Current State

### What Works (7 Go detectors)

| # | Tag | Detector | Lines of Code |
|---|-----|----------|--------------|
| 1 | `k8s.privileged_container` | securityContext.privileged=true | ~15 |
| 2 | `k8s.hostpath_mount` | hostPath volume | ~15 |
| 3 | `k8s.host_namespace_escape` | hostPID/hostIPC/hostNetwork | ~10 |
| 4 | `ops.mass_delete` | >10 resources in destroy op | ~20 |
| 5 | `aws_iam.wildcard_policy` | Action:* AND Resource:* | ~10 |
| 6 | `terraform.iam_wildcard_policy` | Action:* OR Resource:* | ~10 |
| 7 | `terraform.s3_public_access` | S3 without public access block | ~25 |

### What Also Works (not detectors, but infrastructure)

- 3 adapters (K8s, Terraform, Generic)
- Risk matrix + scope elevation
- 5 signal detectors (retry_loop, protocol_violation, artifact_drift, blast_radius, new_scope)
- Hash-chained evidence with optional Ed25519 signing
- MCP server (prescribe/report/get_event)
- Scorecard + explain CLI
- 31 test files

### What's Missing for Release

- More detectors (7 is too few for meaningful scoring)
- Docker/Compose support (adapter + detectors)
- LLM prediction layer for patterns that are hard to detect deterministically
- REST API (in progress separately)

---

## Track 1: Deterministic Detectors (baseline signal)

### Priority Order

New detectors ordered by: implementation simplicity × frequency in real configs × value for scoring.

**Week 1 — K8s security (6 new detectors, ~1 day)**

| # | Tag | Detection Logic | Effort |
|---|-----|----------------|--------|
| 8 | `k8s.docker_socket` | hostPath.path contains "docker.sock" | 30 min |
| 9 | `k8s.run_as_root` | runAsUser=0 OR runAsNonRoot absent/false | 30 min |
| 10 | `k8s.dangerous_capabilities` | capabilities.add contains SYS_ADMIN, NET_ADMIN, NET_RAW, or ALL | 30 min |
| 11 | `k8s.cluster_admin_binding` | ClusterRoleBinding referencing cluster-admin | 45 min |
| 12 | `k8s.writable_rootfs` | readOnlyRootFilesystem absent or false | 20 min |
| 13 | `ops.kube_system` | namespace = kube-system on mutate/destroy | 20 min |

Total: ~3 hours of Go code. Each detector: one function, one test, one golden fixture.

**Implementation pattern (same for all):**

```go
// k8s.docker_socket — detect Docker socket mount
type DockerSocketDetector struct{}

func (d *DockerSocketDetector) Name() string { return "docker_socket" }
func (d *DockerSocketDetector) Detect(_ canon.CanonicalAction, raw []byte) []string {
    for _, obj := range parseK8sYAML(raw) {
        spec := getPodSpec(obj)
        if spec == nil {
            continue
        }
        volumes, ok := spec["volumes"].([]interface{})
        if !ok {
            continue
        }
        for _, v := range volumes {
            vol, ok := v.(map[string]interface{})
            if !ok {
                continue
            }
            hp, ok := vol["hostPath"].(map[string]interface{})
            if !ok {
                continue
            }
            path, _ := hp["path"].(string)
            if strings.Contains(path, "docker.sock") {
                return []string{"k8s.docker_socket"}
            }
        }
    }
    return nil
}
```

Register in `DefaultDetectors()`. Add test. Done.

**Week 1 — Terraform/AWS (3 new detectors, ~half day)**

| # | Tag | Detection Logic | Effort |
|---|-----|----------------|--------|
| 14 | `tf.security_group_open` | ingress with cidr 0.0.0.0/0 on ports 22, 3389, 3306, 5432 | 45 min |
| 15 | `tf.rds_public` | aws_rds_instance with publicly_accessible=true | 30 min |
| 16 | `tf.unencrypted_volume` | aws_ebs_volume without encryption=true | 30 min |

**Week 2 — Docker/Compose (3 new detectors + adapter)**

| # | Tag | Detection Logic | Effort |
|---|-----|----------------|--------|
| 17 | `docker.privileged` | docker run --privileged or privileged: true in compose | 45 min |
| 18 | `docker.host_network` | network_mode: host in compose | 20 min |
| 19 | `docker.socket_mount` | /var/run/docker.sock in volumes | 30 min |

Docker adapter needed: parse docker-compose.yaml, extract services, map to CanonicalAction.

```go
type DockerComposeAdapter struct{}

func (a *DockerComposeAdapter) Name() string          { return "docker" }
func (a *DockerComposeAdapter) CanHandle(tool string) bool {
    return tool == "docker" || tool == "docker-compose" || tool == "compose"
}
```

**Week 2 — Operational (1 new detector)**

| # | Tag | Detection Logic | Effort |
|---|-----|----------------|--------|
| 20 | `ops.namespace_delete` | kubectl delete namespace on non-dev namespace | 30 min |

### After 2 Weeks: 20 Deterministic Detectors

```
K8s:        8 detectors (privileged, hostpath, hostns, docker_socket, run_as_root,
                         dangerous_capabilities, cluster_admin, writable_rootfs)
Terraform:  6 detectors (iam_wildcard_both, iam_wildcard_any, s3_public,
                         security_group_open, rds_public, unencrypted_volume)
Docker:     3 detectors (privileged, host_network, socket_mount)
Ops:        3 detectors (mass_delete, kube_system, namespace_delete)
Total:      20
```

### Validation: Run on Real Repos

```bash
# Collect 20 public repos with known misconfigs
repos=(
  "kubescape/regolibrary"
  "bridgecrewio/checkov"
  "kyverno/policies"
  # ... 17 more
)

for repo in "${repos[@]}"; do
  for fixture in $(find "$repo" -name "*.yaml" -o -name "*.tf" | head -20); do
    evidra prescribe --tool kubectl --operation apply --artifact "$fixture" --signing-mode optional \
      | jq '{risk_level, risk_tags: (.risk_tags // [])}'
  done
done | sort | uniq -c | sort -rn
```

**Success criterion:** meaningful distribution of scores across repos. Some repos = low risk, some = critical. If everything is "low" or everything is "critical", detectors need tuning.

---

## Track 2: LLM Prediction (breadth for release)

LLM runs in parallel with detectors. It handles patterns that are hard to detect deterministically — contextual risks, unusual configurations, complex Terraform plans.

### Role: Hypothesis Generator, Not Sensor

| LLM does | LLM does NOT |
|----------|-------------|
| Propose candidate tags for new patterns | Determine final risk_level |
| Augment registered tags from Go detectors | Override Go detector results |
| Explain risks in human-readable form | Serve as the only detector for any pattern |
| Discover patterns for future Go detectors | Block on prediction failure |

### Week 1: Baseline LLM Experiment

**Goal:** Measure agreement rate between Go detectors and LLM on the same 100 artifacts.

```python
# experiment: compare L1 vs L2 on same artifacts
results = []
for artifact in test_artifacts:
    # Layer 1
    go_output = run_evidra_prescribe(artifact)
    go_tags = go_output["risk_tags"]
    
    # Layer 2
    llm_output = run_llm_prediction(artifact, model="claude-haiku-4-5")
    llm_tags = validate_prediction(llm_output)
    
    results.append({
        "artifact": artifact.name,
        "go_tags": go_tags,
        "llm_registered": llm_tags["registered_tags"],
        "llm_candidates": llm_tags["candidate_tags"],
        "agreement": set(go_tags) == set(llm_tags["registered_tags"]),
    })

# Analysis
agreement_rate = sum(r["agreement"] for r in results) / len(results)
llm_only_tags = [r for r in results if r["llm_registered"] - set(r["go_tags"])]
go_only_tags = [r for r in results if set(r["go_tags"]) - set(r["llm_registered"])]

print(f"Agreement: {agreement_rate:.0%}")
print(f"LLM found additional registered tags: {len(llm_only_tags)} artifacts")
print(f"Go detected but LLM missed: {len(go_only_tags)} artifacts")
```

**Expected results:**
- Agreement > 80% on registered tags → LLM is consistent with Go detectors
- LLM candidates per artifact: 0-2 → reasonable discovery rate
- Rejected (prose) rate: < 5% → prompt contract working

### Week 1: Multi-Model Comparison

Run the same 100 artifacts through 4 models:

| Model | Cost/artifact | Purpose |
|-------|-------------|---------|
| Claude Haiku 4.5 | $0.0005 | Primary — cheapest Anthropic |
| GPT-4o-mini | $0.0003 | Cross-vendor comparison |
| Gemini 2.0 Flash | $0.0002 | Cheapest option |
| DeepSeek V3 | $0.0002 | Open-weight comparison |

Total: 400 predictions × ~$0.0003 = **$0.12**

Measure per model:
- Format compliance (% valid tags)
- Registered tag agreement with Go
- Candidate tag fragmentation
- Prose rejection rate

### Week 2: LLM for Explanation (separate prompt)

The release needs human-readable risk explanations. This is a **separate call**, not mixed with tag prediction:

```python
EXPLAIN_PROMPT = """
You are a security advisor. Given infrastructure artifact and its risk tags,
explain each risk in 1-2 sentences. Be specific to the artifact, not generic.

Risk tags: {tags}
Risk level: {level}

Explain each tag's specific risk for this artifact.
Keep explanations actionable — what should the team fix?
"""
```

This is the safe use of LLM prose — it's never parsed, only displayed. Format doesn't matter. Model can be creative. Costs nothing because output isn't validated.

### Week 2: Integration into REST API

```python
# REST API endpoint
@app.post("/api/v1/assess")
async def assess_artifact(artifact: str, tool: str, operation: str):
    # Step 1: Go detectors (always, fast, deterministic)
    go_result = evidra_prescribe(tool, operation, artifact)
    
    # Step 2: LLM prediction (async, can fail gracefully)
    try:
        llm_result = await llm_predict_tags(artifact, model="haiku")
        validated = validate_prediction(llm_result)
    except Exception:
        validated = {"registered_tags": [], "candidate_tags": [], "rejected_tags": []}
    
    # Step 3: Server-side merge + scoring
    final = compute_risk_assessment(
        go_tags=go_result["risk_tags"],
        llm_registered=validated["registered_tags"],
        llm_candidates=validated["candidate_tags"],
        operation_class=go_result["operation_class"],
        scope_class=go_result["scope_class"],
    )
    
    # Step 4: LLM explanation (async, fire-and-forget for UI)
    explanation = await llm_explain_risks(artifact, final["risk_details"], final["risk_level"])
    
    return {
        "risk_level": final["risk_level"],           # from Layer 3 (server)
        "risk_details": final["risk_details"],        # L1 ∪ L2 registered
        "candidate_tags": final["candidate_tags"],    # L2 unregistered
        "explanation": explanation,                    # L2 prose (display only)
        "provenance": final["provenance"],
    }
```

**Graceful degradation:** If LLM is down, slow, or broken — the response still works. Go detectors provide baseline. LLM adds breadth when available. This is the architecture the reviewer asked for.

---

## Track 3: Docker Support

### DockerAdapter — COMPLETE

> Docker adapter (`internal/canon/docker.go`) is implemented and tested (`docker_test.go`).
> Handles: docker, nerdctl, podman, lima (command strings) and docker-compose, compose (YAML).
> Registered in `DefaultAdapters()`. See `internal/canon/docker.go` for implementation.

### Docker Detectors — TODO

```go
// internal/risk/docker_detectors.go

type DockerPrivilegedDetector struct{}

func (d *DockerPrivilegedDetector) Name() string { return "docker_privileged" }
func (d *DockerPrivilegedDetector) Detect(action canon.CanonicalAction, raw []byte) []string {
    if action.Tool != "docker" && action.Tool != "docker-compose" && action.Tool != "compose" {
        return nil
    }
    var compose struct {
        Services map[string]struct {
            Privileged bool `yaml:"privileged"`
        } `yaml:"services"`
    }
    if err := yaml.Unmarshal(raw, &compose); err != nil {
        return nil
    }
    for _, svc := range compose.Services {
        if svc.Privileged {
            return []string{"docker.privileged"}
        }
    }
    return nil
}

// DockerSocketMountDetector — /var/run/docker.sock in volumes
// DockerHostNetworkDetector — network_mode: host
// (same pattern, different field checks)
```

---

## Track 4: Signals Engine Validation (the actual product)

Detectors find misconfigurations. Every scanner does that. **Signals measure operational behavior** — how an agent operates infrastructure over time. This is what no other tool does. This is the product.

### What Signals Measure (not misconfigs)

| Signal | Measures | Example |
|--------|----------|---------|
| `retry_loop` | Agent stuck, not making progress | terraform apply × 3, same error, no change |
| `protocol_violation` | Agent breaking execution contract | Prescribe without report, report without prescribe |
| `artifact_drift` | Intent ≠ execution | Prescribe replicas=3, apply replicas=10 |
| `blast_radius` | Disproportionate operational impact | Delete 15 configmaps in one command |
| `new_scope` | Agent crossing boundaries | Switch from kubectl to helm mid-session |

Detectors answer: "is this config dangerous?" Signals answer: **"is this agent behaving reliably?"** Different question. Different product. Different market.

### Validation: 100 Operations, 5 Signal Distribution

Script 10 sequences of operations (not artifacts, OPERATIONS) with deliberately injected behavioral patterns:

```bash
#!/usr/bin/env bash
# tests/signal-validation/validate-signals-engine.sh
source tests/signal-validation/helpers.sh
export EVIDRA_SIGNING_MODE=optional

echo "=== Signals Engine Validation: 100 operations ==="

# --- Sequence A: Clean session (20 ops, expect: no signals except new_scope) ---
new_session
for i in $(seq 1 20); do
  cat > "$WORKSPACE/clean-$i.yaml" << EOF
apiVersion: v1
kind: ConfigMap
metadata:
  name: config-$i
  namespace: bench-app
data:
  key: value-$i
EOF
  prescribe kubectl apply "$WORKSPACE/clean-$i.yaml"
  report "$LAST_PRESCRIPTION_ID" 0
done
echo "Sequence A (clean 20 ops):"
get_signals | jq '.signals[] | select(.count > 0) | "\(.signal): \(.count)"' -r

# --- Sequence B: Retry loop (10 ops, expect: retry_loop >= 3) ---
new_session
cat > "$WORKSPACE/fail-deploy.yaml" << 'EOF'
apiVersion: apps/v1
kind: Deployment
metadata:
  name: fail-app
  namespace: bench-app
spec:
  replicas: 1
  selector:
    matchLabels:
      app: fail
  template:
    metadata:
      labels:
        app: fail
    spec:
      containers:
        - name: app
          image: nginx:nonexistent-tag
EOF

# 5 retries of same failed operation
for i in $(seq 1 5); do
  prescribe kubectl apply "$WORKSPACE/fail-deploy.yaml"
  report "$LAST_PRESCRIPTION_ID" 1  # exit_code=1 (failure)
done

# 5 successful operations (to show mixed session)
for i in $(seq 1 5); do
  cat > "$WORKSPACE/ok-$i.yaml" << EOF
apiVersion: v1
kind: ConfigMap
metadata:
  name: ok-config-$i
  namespace: bench-app
data:
  key: ok
EOF
  prescribe kubectl apply "$WORKSPACE/ok-$i.yaml"
  report "$LAST_PRESCRIPTION_ID" 0
done
echo "Sequence B (retry + clean):"
get_signals | jq '.signals[] | select(.count > 0) | "\(.signal): \(.count)"' -r

# --- Sequence C: Protocol violations (15 ops, expect: protocol_violation >= 3) ---
new_session
for i in $(seq 1 5); do
  cat > "$WORKSPACE/proto-$i.yaml" << EOF
apiVersion: v1
kind: ConfigMap
metadata:
  name: proto-config-$i
  namespace: bench-app
data:
  key: proto
EOF
  prescribe kubectl apply "$WORKSPACE/proto-$i.yaml"
  report "$LAST_PRESCRIPTION_ID" 0
done
# 5 prescribes WITHOUT reports (stalled operations)
for i in $(seq 6 10); do
  cat > "$WORKSPACE/proto-$i.yaml" << EOF
apiVersion: v1
kind: ConfigMap
metadata:
  name: proto-config-$i
  namespace: bench-app
data:
  key: orphan
EOF
  prescribe kubectl apply "$WORKSPACE/proto-$i.yaml"
  # intentionally no report
done
# 5 more clean operations
for i in $(seq 11 15); do
  cat > "$WORKSPACE/proto-$i.yaml" << EOF
apiVersion: v1
kind: ConfigMap
metadata:
  name: proto-config-$i
  namespace: bench-app
data:
  key: clean
EOF
  prescribe kubectl apply "$WORKSPACE/proto-$i.yaml"
  report "$LAST_PRESCRIPTION_ID" 0
done
echo "Sequence C (protocol violations):"
get_signals | jq '.signals[] | select(.count > 0) | "\(.signal): \(.count)"' -r

# --- Sequence D: Blast radius (10 ops, expect: blast_radius >= 1) ---
new_session
# Create 15-doc multi-resource YAML for mass delete
{
for i in $(seq 1 15); do
  cat << EOF
apiVersion: v1
kind: ConfigMap
metadata:
  name: mass-$i
  namespace: bench-cleanup
data:
  key: delete-me
---
EOF
done
} > "$WORKSPACE/mass-delete.yaml"

prescribe kubectl delete "$WORKSPACE/mass-delete.yaml"
report "$LAST_PRESCRIPTION_ID" 0

# 9 more normal ops
for i in $(seq 1 9); do
  cat > "$WORKSPACE/blast-$i.yaml" << EOF
apiVersion: v1
kind: ConfigMap
metadata:
  name: blast-config-$i
  namespace: bench-app
data:
  key: normal
EOF
  prescribe kubectl apply "$WORKSPACE/blast-$i.yaml"
  report "$LAST_PRESCRIPTION_ID" 0
done
echo "Sequence D (blast radius):"
get_signals | jq '.signals[] | select(.count > 0) | "\(.signal): \(.count)"' -r

# --- Sequence E: Scope escalation (15 ops, expect: new_scope >= 3) ---
new_session
# kubectl operations
for i in $(seq 1 5); do
  cat > "$WORKSPACE/scope-k8s-$i.yaml" << EOF
apiVersion: v1
kind: ConfigMap
metadata:
  name: scope-$i
  namespace: bench-app
data:
  key: kubectl
EOF
  prescribe kubectl apply "$WORKSPACE/scope-k8s-$i.yaml"
  report "$LAST_PRESCRIPTION_ID" 0
done
# switch to helm
for i in $(seq 1 5); do
  cat > "$WORKSPACE/scope-helm-$i.yaml" << EOF
apiVersion: v1
kind: ConfigMap
metadata:
  name: helm-scope-$i
  namespace: bench-app
data:
  key: helm
EOF
  prescribe helm install "$WORKSPACE/scope-helm-$i.yaml"
  report "$LAST_PRESCRIPTION_ID" 0
done
# switch to terraform
for i in $(seq 1 5); do
  cat > "$WORKSPACE/scope-tf-$i.json" << EOF
{"resource_changes": [{"type": "null_resource", "change": {"actions": ["create"]}}]}
EOF
  prescribe terraform apply "$WORKSPACE/scope-tf-$i.json"
  report "$LAST_PRESCRIPTION_ID" 0
done
echo "Sequence E (scope escalation):"
get_signals | jq '.signals[] | select(.count > 0) | "\(.signal): \(.count)"' -r

echo ""
echo "=== Summary: Scorecard per sequence ==="
# Run scorecard on each session
for ev_dir in "$WORKSPACE"/evidence-*/; do
  session=$(basename "$ev_dir")
  score=$(evidra scorecard --evidence-dir "$ev_dir" 2>/dev/null | jq -r '.score // "N/A"')
  band=$(evidra scorecard --evidence-dir "$ev_dir" 2>/dev/null | jq -r '.band // "N/A"')
  echo "  $session: score=$score band=$band"
done
```

### Expected Distribution

| Sequence | Operations | Dominant Signal | Expected Score | Expected Band |
|----------|-----------|----------------|---------------|---------------|
| A (clean) | 20 | none (new_scope only) | 95-100 | excellent |
| B (retry) | 10 | retry_loop ≥ 3 | 50-70 | fair |
| C (protocol) | 15 | protocol_violation ≥ 3 | 40-65 | poor-fair |
| D (blast) | 10 | blast_radius ≥ 1 | 60-80 | fair-good |
| E (scope) | 15 | new_scope ≥ 3 | 85-95 | good |

**The key test:** If sequence A scores 95+ and sequence C scores 40-65, the signals engine produces meaningful differentiation. The scorecard distribution IS the product validation. Not the detectors — the signals.

### What This Proves

If the 5 sequences produce 5 different scores in the expected ranges:

1. **Signal engine works** — behavioral patterns are detected from operational telemetry
2. **Scoring is meaningful** — scores differentiate reliable from unreliable behavior
3. **This is not a scanner** — no other tool produces this output from operation sequences
4. **The product is real** — artifact→detectors→signals→score pipeline produces actionable results

If all 5 sequences score the same → signal engine has a bug.
If scores are inverted (clean=low, violations=high) → weight calibration needed.
If some signals never fire → detector or threshold issue.

---

## Timeline

```
Week 1 (days 1-5)
├── Track 1: Add 9 Go detectors (#8-#16)
│   ├── Day 1: k8s.docker_socket, k8s.run_as_root, k8s.dangerous_capabilities
│   ├── Day 2: k8s.cluster_admin_binding, k8s.writable_rootfs, ops.kube_system
│   └── Day 3: tf.security_group_open, tf.rds_public, tf.unencrypted_volume
│
├── Track 2: LLM baseline experiment
│   ├── Day 1: Run 100 artifacts through Go + LLM, measure agreement
│   ├── Day 2: Run same 100 through 4 models, compare fragmentation
│   └── Day 3: Write explanation prompt, test on 20 artifacts
│
├── Track 3: Docker adapter scaffold
│   └── Day 4-5: DockerComposeAdapter + 3 detectors
│
└── Track 4: SIGNALS ENGINE VALIDATION ← most important deliverable of week 1
    └── Day 5: Run validate-signals-engine.sh — 5 sequences, 100 operations
              Does the scorecard produce meaningful distribution?

Week 2 (days 6-10)
├── Track 1: Add detector #20 + validate on real repos
│   ├── Day 6: ops.namespace_delete + final integration tests
│   └── Day 7: Run 20 detectors on 100 OSS repo fixtures
│
├── Track 2: LLM integration into REST API
│   ├── Day 8: Wire LLM prediction into /api/v1/assess
│   ├── Day 9: Wire LLM explanation (separate call)
│   └── Day 10: End-to-end test: artifact → detectors → LLM → merged response
│
├── Track 4: Signal tuning based on Week 1 results
│   ├── Day 8: Adjust weights if scores don't differentiate
│   └── Day 9: Run with real agent (EXPERIMENT_DESIGN scenario 01) — does the agent
│              produce different signal profiles than scripted sequences?
│
└── Milestone: 20 detectors + LLM augmentation + Docker + validated signal engine
```

**Week 1 Day 5 is the gate.** If Track 4 validation produces meaningful score distribution, the product is real. Everything else (more detectors, LLM breadth, Docker) is incremental value. If Track 4 fails, fix signals engine before adding anything else.

---

## Release Readiness Checklist

| Component | Priority | Status |
|-----------|----------|--------|
| **Signal engine produces meaningful score distribution** | **P0 — gate** | **Track 4 validation** |
| **5 signal detectors working** | **P0** | **Done** |
| ≥15 Go risk detectors | P1 | 7 done, 13 planned |
| K8s adapter | P1 | Done |
| Terraform adapter | P1 | Done |
| Docker adapter | P1 | Track 3 |
| Risk matrix + elevation | P1 | Done |
| Scorecard + explain | P1 | Done |
| MCP server | P1 | Done |
| REST API with LLM augmentation | P1 | Track 2 |
| LLM graceful degradation | P1 | Track 2 week 2 |
| Evidence chain + signing | P1 | Done |
| Fault injection passing | P2 | After new detectors |

Signal engine validation (Track 4) is the **release gate**. If score distribution is not meaningful, no amount of detectors or LLM integration matters.

---

## Key Principle

```
Detectors     = event source (finds misconfigs — commodity, Trivy/Checkov do this)
Signal engine = the product (measures operational behavior — nobody else does this)
LLM           = breadth layer (discovers new patterns, explains risks)
Scorecard     = the deliverable (score + band from behavioral signals)

Evidra is NOT another security scanner.
Evidra is an operational reliability measurement system for infrastructure automation.

The question is not "is this config dangerous?"
The question is "is this agent operating reliably?"

If signal engine is broken → no product (even with 100 detectors)
If detectors are missing → degraded product (signals still work on fewer events)
If LLM is down → product works (Go detectors + signals + scorecard)
```

Detectors feed the signal engine. LLM augments the detectors. But the signal engine — retry_loop, protocol_violation, artifact_drift, blast_radius, new_scope — is the unique value. This is what makes Evidra "OpenTelemetry for automation behavior" rather than "yet another Checkov."
