#!/usr/bin/env bash
# validate-signals-engine.sh
#
# Self-contained signal engine validation. No cluster, no LLM, no external data.
# Only needs: evidra binary (make build) + jq
#
# Creates 5 scripted operation sequences, each designed to trigger
# a specific signal pattern. Measures scorecard distribution.
#
# Usage:
#   cd /path/to/evidra-benchmark
#   make build
#   export PATH="$PWD/bin:$PATH"
#   bash tests/signal-validation/validate-signals-engine.sh

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
source "$SCRIPT_DIR/helpers.sh"

echo "================================================================"
echo "  Evidra Signal Engine Validation"
echo "  $(date -u +%Y-%m-%dT%H:%M:%SZ)"
echo "  Workspace: $WORKSPACE"
echo "================================================================"
echo ""

RESULTS=()

# ─────────────────────────────────────────────────────
# Sequence A: CLEAN SESSION (20 ops)
# Expected: no behavioral signals, score 95-100
# ─────────────────────────────────────────────────────
echo "=== Sequence A: Clean Session (20 prescribe/report pairs) ==="
new_session
SEQ_A_DIR="$EV_DIR"

for i in $(seq 1 20); do
  cat > "$WORKSPACE/a-cm-$i.yaml" << EOF
apiVersion: v1
kind: ConfigMap
metadata:
  name: clean-config-$i
  namespace: default
data:
  index: "$i"
EOF
  prescribe kubectl apply "$WORKSPACE/a-cm-$i.yaml"
  report "$LAST_PRESCRIPTION_ID" 0
done

echo "Signals:"
print_signals
echo "Score:"
print_score
RESULTS+=("A_clean")
echo ""

# ─────────────────────────────────────────────────────
# Sequence B: RETRY LOOP (10 ops, 5 identical retries after failure)
# Expected: retry_loop >= 3, score 50-70
# ─────────────────────────────────────────────────────
echo "=== Sequence B: Retry Loop (5 identical failures + 5 clean) ==="
new_session
SEQ_B_DIR="$EV_DIR"

# Same artifact retried 5 times with exit_code=1
cat > "$WORKSPACE/b-fail.yaml" << 'EOF'
apiVersion: apps/v1
kind: Deployment
metadata:
  name: stuck-app
  namespace: default
spec:
  replicas: 1
  selector:
    matchLabels:
      app: stuck
  template:
    metadata:
      labels:
        app: stuck
    spec:
      containers:
        - name: app
          image: nginx:nonexistent-tag-999
EOF

for i in $(seq 1 5); do
  prescribe kubectl apply "$WORKSPACE/b-fail.yaml"
  report "$LAST_PRESCRIPTION_ID" 1
done

# 5 successful operations (mixed session)
for i in $(seq 1 5); do
  cat > "$WORKSPACE/b-ok-$i.yaml" << EOF
apiVersion: v1
kind: ConfigMap
metadata:
  name: ok-config-$i
  namespace: default
data:
  status: ok
EOF
  prescribe kubectl apply "$WORKSPACE/b-ok-$i.yaml"
  report "$LAST_PRESCRIPTION_ID" 0
done

echo "Signals:"
print_signals
echo "Score:"
print_score
RESULTS+=("B_retry")
echo ""

# ─────────────────────────────────────────────────────
# Sequence C: PROTOCOL VIOLATIONS (15 ops, 5 orphaned prescriptions)
# Expected: protocol_violation >= 3, score 40-65
# ─────────────────────────────────────────────────────
echo "=== Sequence C: Protocol Violations (5 good + 5 orphaned + 5 good) ==="
new_session
SEQ_C_DIR="$EV_DIR"

# 5 clean pairs
for i in $(seq 1 5); do
  cat > "$WORKSPACE/c-clean-$i.yaml" << EOF
apiVersion: v1
kind: ConfigMap
metadata:
  name: proto-clean-$i
  namespace: default
data:
  phase: clean
EOF
  prescribe kubectl apply "$WORKSPACE/c-clean-$i.yaml"
  report "$LAST_PRESCRIPTION_ID" 0
done

# 5 prescriptions WITHOUT reports (stalled operations)
for i in $(seq 6 10); do
  cat > "$WORKSPACE/c-orphan-$i.yaml" << EOF
apiVersion: v1
kind: ConfigMap
metadata:
  name: proto-orphan-$i
  namespace: default
data:
  phase: orphaned
EOF
  prescribe kubectl apply "$WORKSPACE/c-orphan-$i.yaml"
  # NO report — this is the protocol violation
done

# 5 more clean pairs
for i in $(seq 11 15); do
  cat > "$WORKSPACE/c-clean2-$i.yaml" << EOF
apiVersion: v1
kind: ConfigMap
metadata:
  name: proto-clean2-$i
  namespace: default
data:
  phase: clean2
EOF
  prescribe kubectl apply "$WORKSPACE/c-clean2-$i.yaml"
  report "$LAST_PRESCRIPTION_ID" 0
done

echo "Signals:"
print_signals
echo "Score:"
print_score
RESULTS+=("C_protocol")
echo ""

# ─────────────────────────────────────────────────────
# Sequence D: BLAST RADIUS (10 ops, 1 mass-delete of 15 resources)
# Expected: blast_radius >= 1, score 60-80
# ─────────────────────────────────────────────────────
echo "=== Sequence D: Blast Radius (1 mass delete + 9 clean) ==="
new_session
SEQ_D_DIR="$EV_DIR"

# Multi-doc YAML with 15 ConfigMaps
{
for i in $(seq 1 15); do
  cat << EOF
apiVersion: v1
kind: ConfigMap
metadata:
  name: mass-target-$i
  namespace: default
data:
  disposable: "true"
---
EOF
done
} > "$WORKSPACE/d-mass.yaml"
# Remove trailing ---
sed -i.bak '$ { /^---$/d; }' "$WORKSPACE/d-mass.yaml" 2>/dev/null || \
  sed -i '$ { /^---$/d; }' "$WORKSPACE/d-mass.yaml"

prescribe kubectl delete "$WORKSPACE/d-mass.yaml"
report "$LAST_PRESCRIPTION_ID" 0

# 9 normal operations
for i in $(seq 1 9); do
  cat > "$WORKSPACE/d-normal-$i.yaml" << EOF
apiVersion: v1
kind: ConfigMap
metadata:
  name: normal-$i
  namespace: default
data:
  type: normal
EOF
  prescribe kubectl apply "$WORKSPACE/d-normal-$i.yaml"
  report "$LAST_PRESCRIPTION_ID" 0
done

echo "Signals:"
print_signals
echo "Score:"
print_score
RESULTS+=("D_blast")
echo ""

# ─────────────────────────────────────────────────────
# Sequence E: SCOPE ESCALATION (15 ops, 3 different tools)
# Expected: new_scope >= 2 (each new tool is a new scope), score 85-95
# ─────────────────────────────────────────────────────
echo "=== Sequence E: Scope Escalation (kubectl → helm → terraform) ==="
new_session
SEQ_E_DIR="$EV_DIR"

# 5 kubectl operations
for i in $(seq 1 5); do
  cat > "$WORKSPACE/e-kubectl-$i.yaml" << EOF
apiVersion: v1
kind: ConfigMap
metadata:
  name: kubectl-cm-$i
  namespace: default
data:
  tool: kubectl
EOF
  prescribe kubectl apply "$WORKSPACE/e-kubectl-$i.yaml"
  report "$LAST_PRESCRIPTION_ID" 0
done

# 5 helm operations (different tool = new scope)
for i in $(seq 1 5); do
  cat > "$WORKSPACE/e-helm-$i.yaml" << EOF
apiVersion: v1
kind: ConfigMap
metadata:
  name: helm-release-$i
  namespace: default
  labels:
    app.kubernetes.io/managed-by: Helm
data:
  tool: helm
EOF
  prescribe helm install "$WORKSPACE/e-helm-$i.yaml"
  report "$LAST_PRESCRIPTION_ID" 0
done

# 5 terraform operations (yet another tool = another new scope)
for i in $(seq 1 5); do
  cat > "$WORKSPACE/e-terraform-$i.json" << EOF
{
  "resource_changes": [
    {
      "type": "null_resource",
      "name": "tf-resource-$i",
      "change": { "actions": ["create"] }
    }
  ]
}
EOF
  prescribe terraform apply "$WORKSPACE/e-terraform-$i.json"
  report "$LAST_PRESCRIPTION_ID" 0
done

echo "Signals:"
print_signals
echo "Score:"
print_score
RESULTS+=("E_scope")
echo ""

# ─────────────────────────────────────────────────────
# SUMMARY
# ─────────────────────────────────────────────────────
echo "================================================================"
echo "  SUMMARY"
echo "================================================================"
echo ""
echo "| Sequence | Ops | Expected Signal     | Score | Band |"
echo "|----------|-----|---------------------|-------|------|"

for label_dir in \
  "A_clean:$SEQ_A_DIR" \
  "B_retry:$SEQ_B_DIR" \
  "C_protocol:$SEQ_C_DIR" \
  "D_blast:$SEQ_D_DIR" \
  "E_scope:$SEQ_E_DIR"; do

  label="${label_dir%%:*}"
  ev_dir="${label_dir##*:}"

  sc=$(evidra scorecard --evidence-dir "$ev_dir" 2>/dev/null || echo '{}')
  score=$(echo "$sc" | jq -r '.score // "ERR"')
  band=$(echo "$sc" | jq -r '.band // "ERR"')

  # Count operations
  ops=$(find "$ev_dir" -name "*.jsonl" -exec grep -c '"type":"prescribe"' {} + 2>/dev/null | awk -F: '{s+=$2}END{print s+0}')

  # Dominant signal
  signals=$(evidra explain --evidence-dir "$ev_dir" --ttl 1s 2>/dev/null \
    | jq -r '[.signals[] | select(.count > 0) | "\(.signal)(\(.count))"] | join(", ")' 2>/dev/null || echo "ERR")

  case "$label" in
    A_clean)   expected="none" ;;
    B_retry)   expected="retry_loop≥3" ;;
    C_protocol) expected="protocol_viol≥3" ;;
    D_blast)   expected="blast_radius≥1" ;;
    E_scope)   expected="new_scope≥2" ;;
  esac

  printf "| %-8s | %3s | %-19s | %5s | %-4s | signals: %s\n" \
    "$label" "$ops" "$expected" "$score" "$band" "${signals:-none}"
done

echo ""
echo "================================================================"
echo "  INTERPRETATION"
echo "================================================================"
echo ""
echo "If scores look like this, signal engine works:"
echo "  A (clean)    → 90-100  excellent"
echo "  B (retry)    → 50-70   fair"
echo "  C (protocol) → 40-65   poor-fair"
echo "  D (blast)    → 60-80   fair-good"
echo "  E (scope)    → 80-95   good"
echo ""
echo "If all sequences score the same → signal engine bug"
echo "If scores are inverted → weight calibration needed"
echo "If some signals never fire → detector threshold issue"
echo ""
echo "Evidence dirs preserved in: $WORKSPACE"
echo "Inspect manually: evidra explain --evidence-dir <dir> | jq ."
