#!/usr/bin/env bash
# validate-signals-engine.sh
#
# Self-contained signal engine validation. No cluster, no LLM, no external data.
# Only needs: evidra binary (make build) + jq
#
# Creates scripted operation sequences, each designed to trigger
# a specific signal pattern. Measures scorecard distribution.
#
# Usage:
#   cd /path/to/evidra-benchmark
#   make build
#   export PATH="$PWD/bin:$PATH"
#   bash tests/signal-validation/validate-signals-engine.sh

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"
EXPECTED_BANDS_FILE="${EXPECTED_BANDS_FILE:-$SCRIPT_DIR/expected-bands.json}"
SCORECARD_MIN_OPERATIONS="${SCORECARD_MIN_OPERATIONS:-1}"
export SCORECARD_MIN_OPERATIONS

# Workspace safety guard + cleanup for idempotent reruns.
WORKSPACE_PREFIX="/tmp/evidra-signal-validation"
WORKSPACE="${WORKSPACE:-$WORKSPACE_PREFIX}"
if [ -z "$WORKSPACE" ] || [ "$WORKSPACE" = "/" ] || [[ "$WORKSPACE" != "$WORKSPACE_PREFIX"* ]]; then
  echo "FATAL: WORKSPACE must start with $WORKSPACE_PREFIX and must not be empty or /"
  exit 1
fi
export WORKSPACE

source "$SCRIPT_DIR/helpers.sh"

rm -rf "$WORKSPACE"
mkdir -p "$WORKSPACE"

if [ ! -f "$EXPECTED_BANDS_FILE" ]; then
  echo "FATAL: expected bands file not found: $EXPECTED_BANDS_FILE"
  exit 1
fi

RUN_STAMP="$(date -u +%Y%m%dT%H%M%SZ)"
RESULTS_DIR="${RESULTS_DIR:-$REPO_ROOT/experiments/results/signals/$RUN_STAMP}"
mkdir -p "$RESULTS_DIR"
SUMMARY_JSONL="$RESULTS_DIR/summary.jsonl"
: > "$SUMMARY_JSONL"

echo "================================================================"
echo "  Evidra Signal Engine Validation"
echo "  $(date -u +%Y-%m-%dT%H:%M:%SZ)"
echo "  Workspace: $WORKSPACE"
echo "  Results:   $RESULTS_DIR"
echo "  Bands:     $EXPECTED_BANDS_FILE"
echo "  MinOps:    $SCORECARD_MIN_OPERATIONS"
echo "================================================================"
echo ""

RESULTS=()
FAILURES=0

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
  "format_version": "0.1",
  "terraform_version": "1.6.0",
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
# Sequence F: REPAIR (10 ops, fail then fix then succeed)
# Expected: repair_loop >= 1, score should be HIGHER than sequence B
# ─────────────────────────────────────────────────────
echo "=== Sequence F: Repair Loop (fail → change artifact → succeed) ==="
new_session
SEQ_F_DIR="$EV_DIR"

cat > "$WORKSPACE/f-deploy-v1.yaml" << 'EOF'
apiVersion: apps/v1
kind: Deployment
metadata:
  name: web-app
  namespace: default
spec:
  replicas: 1
  selector:
    matchLabels:
      app: web
  template:
    metadata:
      labels:
        app: web
    spec:
      containers:
        - name: app
          image: nginx:nonexistent-v1
EOF

prescribe kubectl apply "$WORKSPACE/f-deploy-v1.yaml"
report "$LAST_PRESCRIPTION_ID" 1

cat > "$WORKSPACE/f-deploy-v2.yaml" << 'EOF'
apiVersion: apps/v1
kind: Deployment
metadata:
  name: web-app
  namespace: default
spec:
  replicas: 1
  selector:
    matchLabels:
      app: web
  template:
    metadata:
      labels:
        app: web
    spec:
      containers:
        - name: app
          image: nginx:also-broken-v2
EOF

prescribe kubectl apply "$WORKSPACE/f-deploy-v2.yaml"
report "$LAST_PRESCRIPTION_ID" 1

cat > "$WORKSPACE/f-deploy-v3.yaml" << 'EOF'
apiVersion: apps/v1
kind: Deployment
metadata:
  name: web-app
  namespace: default
spec:
  replicas: 1
  selector:
    matchLabels:
      app: web
  template:
    metadata:
      labels:
        app: web
    spec:
      containers:
        - name: app
          image: nginx:1.25
EOF

prescribe kubectl apply "$WORKSPACE/f-deploy-v3.yaml"
report "$LAST_PRESCRIPTION_ID" 0

for i in $(seq 1 7); do
  cat > "$WORKSPACE/f-clean-$i.yaml" << EOF
apiVersion: v1
kind: ConfigMap
metadata:
  name: f-config-$i
  namespace: default
data:
  status: ok
EOF
  prescribe kubectl apply "$WORKSPACE/f-clean-$i.yaml"
  report "$LAST_PRESCRIPTION_ID" 0
done

echo "Signals:"
print_signals
echo "Score:"
print_score
RESULTS+=("F_repair")
echo ""

# ─────────────────────────────────────────────────────
# Sequence G: THRASHING (10 ops, many different intents all fail)
# Expected: thrashing >= 1, score should be LOWER than sequence B
# ─────────────────────────────────────────────────────
echo "=== Sequence G: Thrashing (different intents, all fail) ==="
new_session
SEQ_G_DIR="$EV_DIR"

for i in $(seq 1 5); do
  cat > "$WORKSPACE/g-different-$i.yaml" << EOF
apiVersion: apps/v1
kind: Deployment
metadata:
  name: attempt-$i
  namespace: default
spec:
  replicas: 1
  selector:
    matchLabels:
      app: attempt-$i
  template:
    metadata:
      labels:
        app: attempt-$i
    spec:
      containers:
        - name: app
          image: broken:attempt-$i
EOF
  prescribe kubectl apply "$WORKSPACE/g-different-$i.yaml"
  report "$LAST_PRESCRIPTION_ID" 1
done

for i in $(seq 1 5); do
  cat > "$WORKSPACE/g-clean-$i.yaml" << EOF
apiVersion: v1
kind: ConfigMap
metadata:
  name: g-config-$i
  namespace: default
data:
  status: ok
EOF
  prescribe kubectl apply "$WORKSPACE/g-clean-$i.yaml"
  report "$LAST_PRESCRIPTION_ID" 0
done

echo "Signals:"
print_signals
echo "Score:"
print_score
RESULTS+=("G_thrash")
echo ""

# ─────────────────────────────────────────────────────
# Sequence H: ARTIFACT DRIFT (report digest mismatches prescription digest)
# Expected: artifact_drift >= 1
# ─────────────────────────────────────────────────────
echo "=== Sequence H: Artifact Drift (prescribed digest != reported digest) ==="
new_session
SEQ_H_DIR="$EV_DIR"

cat > "$WORKSPACE/h-drift.yaml" << 'EOF'
apiVersion: v1
kind: ConfigMap
metadata:
  name: drift-cm
  namespace: default
data:
  version: "v1"
EOF

prescribe kubectl apply "$WORKSPACE/h-drift.yaml"
# Force a mismatched digest in report to trigger artifact_drift.
report "$LAST_PRESCRIPTION_ID" 0 "sha256:0000000000000000000000000000000000000000000000000000000000000001"

for i in $(seq 1 9); do
  cat > "$WORKSPACE/h-clean-$i.yaml" << EOF
apiVersion: v1
kind: ConfigMap
metadata:
  name: h-config-$i
  namespace: default
data:
  status: ok
EOF
  prescribe kubectl apply "$WORKSPACE/h-clean-$i.yaml"
  report "$LAST_PRESCRIPTION_ID" 0
done

echo "Signals:"
print_signals
echo "Score:"
print_score
RESULTS+=("H_drift")
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
  "E_scope:$SEQ_E_DIR" \
  "F_repair:$SEQ_F_DIR" \
  "G_thrash:$SEQ_G_DIR" \
  "H_drift:$SEQ_H_DIR"; do

  label="${label_dir%%:*}"
  ev_dir="${label_dir##*:}"

  score_path="$RESULTS_DIR/sequence-${label}-scorecard.json"
  explain_path="$RESULTS_DIR/sequence-${label}-explain.json"

  sc=$(evidra scorecard \
    --evidence-dir "$ev_dir" \
    --ttl "$FAULT_TTL" \
    --min-operations "$SCORECARD_MIN_OPERATIONS" 2>/dev/null || echo '{}')
  echo "$sc" > "$score_path"
  score=$(echo "$sc" | jq -r '.score // "ERR"')
  band=$(echo "$sc" | jq -r '.band // "ERR"')

  # Count operations
  ops=$(find "$ev_dir" -name "*.jsonl" -exec grep -h -c '"type":"prescribe"' {} + 2>/dev/null | awk '{s+=$1}END{print s+0}')

  ex=$(evidra explain \
    --evidence-dir "$ev_dir" \
    --min-operations "$SCORECARD_MIN_OPERATIONS" \
    --ttl 1s 2>/dev/null || echo '{"signals":[]}')
  echo "$ex" > "$explain_path"
  signals=$(echo "$ex" | jq -r '[.signals[] | select(.count > 0) | "\(.signal)(\(.count))"] | join(", ")' 2>/dev/null || echo "ERR")
  signals_json=$(echo "$ex" | jq -c '(.signals // []) | map(select(.count > 0)) | map({key:.signal, value:.count}) | from_entries')

  expected=$(jq -r --arg seq "$label" '.sequences[$seq].expected_signal // "n/a"' "$EXPECTED_BANDS_FILE")

  printf "| %-8s | %3s | %-19s | %5s | %-4s | signals: %s\n" \
    "$label" "$ops" "$expected" "$score" "$band" "${signals:-none}"

  echo "$(jq -n \
    --arg seq "$label" \
    --arg ev_dir "$ev_dir" \
    --arg score_file "$score_path" \
    --arg explain_file "$explain_path" \
    --argjson ops "$ops" \
    --argjson score "$score" \
    --arg band "$band" \
    --argjson signals "$signals_json" \
    '{label:$seq, ops:$ops, score:$score, band:$band, signals:$signals, evidence_dir:$ev_dir, scorecard_file:$score_file, explain_file:$explain_file}')" >> "$SUMMARY_JSONL"

  if ! jq -e --arg seq "$label" '.sequences[$seq] != null' "$EXPECTED_BANDS_FILE" >/dev/null; then
    echo "FAIL: missing expectations for sequence $label in $EXPECTED_BANDS_FILE"
    FAILURES=$((FAILURES + 1))
    continue
  fi

  exp_band=$(jq -r --arg seq "$label" '.sequences[$seq].band // ""' "$EXPECTED_BANDS_FILE")
  if [ -n "$exp_band" ] && [ "$band" != "$exp_band" ]; then
    echo "FAIL: $label band=$band, expected $exp_band"
    FAILURES=$((FAILURES + 1))
  fi

  score_min=$(jq -r --arg seq "$label" '.sequences[$seq].score_min // empty' "$EXPECTED_BANDS_FILE")
  score_max=$(jq -r --arg seq "$label" '.sequences[$seq].score_max // empty' "$EXPECTED_BANDS_FILE")
  if [ -n "$score_min" ] && [ -n "$score_max" ]; then
    if ! jq -n --argjson score "$score" --argjson min "$score_min" --argjson max "$score_max" '($score >= $min) and ($score <= $max)' | grep -q true; then
      echo "FAIL: $label score=$score not in [$score_min, $score_max]"
      FAILURES=$((FAILURES + 1))
    fi
  fi

  while IFS=$'\t' read -r sig min_count; do
    [ -z "$sig" ] && continue
    actual=$(echo "$ex" | jq -r --arg sig "$sig" '[.signals[] | select(.signal == $sig) | .count][0] // 0')
    if ! jq -n --argjson actual "$actual" --argjson min "$min_count" '$actual >= $min' | grep -q true; then
      echo "FAIL: $label signal $sig count=$actual, expected >= $min_count"
      FAILURES=$((FAILURES + 1))
    fi
  done < <(jq -r --arg seq "$label" '.sequences[$seq].required_signals // {} | to_entries[]? | "\(.key)\t\(.value)"' "$EXPECTED_BANDS_FILE")
done

while IFS=$'\t' read -r left op right; do
  [ -z "$left" ] && continue
  left_score=$(jq -s -r --arg seq "$left" '.[] | select(.label == $seq) | .score' "$SUMMARY_JSONL")
  right_score=$(jq -s -r --arg seq "$right" '.[] | select(.label == $seq) | .score' "$SUMMARY_JSONL")

  if [ -z "$left_score" ] || [ -z "$right_score" ]; then
    echo "FAIL: comparison references missing sequence ($left $op $right)"
    FAILURES=$((FAILURES + 1))
    continue
  fi

  if ! jq -n --argjson l "$left_score" --argjson r "$right_score" --arg op "$op" '
    if $op == ">" then $l > $r
    elif $op == "<" then $l < $r
    elif $op == ">=" then $l >= $r
    elif $op == "<=" then $l <= $r
    elif $op == "==" then $l == $r
    else false end
  ' | grep -q true; then
    echo "FAIL: comparison $left ($left_score) $op $right ($right_score) failed"
    FAILURES=$((FAILURES + 1))
  fi
done < <(jq -r '.comparisons[]? | "\(.left)\t\(.op)\t\(.right)"' "$EXPECTED_BANDS_FILE")

jq -s \
  --arg generated_at "$(date -u +%Y-%m-%dT%H:%M:%SZ)" \
  --arg run_stamp "$RUN_STAMP" \
  --arg workspace "$WORKSPACE" \
  --arg bands_file "$EXPECTED_BANDS_FILE" \
  --argjson min_operations "$SCORECARD_MIN_OPERATIONS" \
  --argjson failures "$FAILURES" \
  '{
    generated_at: $generated_at,
    run_stamp: $run_stamp,
    workspace: $workspace,
    expected_bands_file: $bands_file,
    min_operations: $min_operations,
    failures: $failures,
    pass: ($failures == 0),
    sequences: .
  }' "$SUMMARY_JSONL" > "$RESULTS_DIR/summary.json"

echo ""
echo "================================================================"
echo "  INTERPRETATION"
echo "================================================================"
echo ""
echo "If scores look like this, signal engine works:"
echo "  A (clean)    → 99-100 excellent"
echo "  B (retry)    → 75-85  poor"
echo "  C (protocol) → 85-90  poor"
echo "  D (blast)    → 95-99  good"
echo "  E (scope)    → 98-100 excellent"
echo "  F (repair)   → 75-85  adapted (should be > B)"
echo "  G (thrash)   → 70-80  unstable (should be < B)"
echo "  H (drift)    → 84-86  poor"
echo ""
echo "If all sequences score the same → signal engine bug"
echo "If scores are inverted → weight calibration needed"
echo "If some signals never fire → detector threshold issue"
echo ""
echo "Evidence dirs preserved in: $WORKSPACE"
echo "Result artifacts written to: $RESULTS_DIR"
echo "Inspect manually: evidra explain --evidence-dir <dir> | jq ."

if [ "$FAILURES" -ne 0 ]; then
  echo ""
  echo "Validation failed: $FAILURES assertion(s) failed."
  exit 1
fi

echo ""
echo "Validation passed: all assertions satisfied."
