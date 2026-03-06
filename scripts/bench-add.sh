#!/usr/bin/env bash
set -euo pipefail

usage() {
  cat <<'EOF'
Usage:
  scripts/bench-add.sh <case-id> [--artifact <path>] [--source <source-id>] [options]

Options:
  --tool <name>          Tool name for optional artifact processing (default: kubectl)
  --operation <name>     Operation name for optional artifact processing (default: apply)
  --evidra-bin <path>    Explicit evidra binary for process-artifact helper
  --no-process           Do not run process-artifact autofill even when --artifact is provided
  -h, --help             Show this help

Examples:
  scripts/bench-add.sh k8s-hostpath-mount-fail --artifact /tmp/hostpath.yaml --source kubescape-regolibrary
  scripts/bench-add.sh tf-s3-public-access-fail --source checkov-terraform --tool terraform
EOF
}

if [[ $# -lt 1 ]]; then
  usage >&2
  exit 1
fi

if [[ "$1" == "-h" || "$1" == "--help" ]]; then
  usage
  exit 0
fi

CASE_ID="$1"
shift

ARTIFACT=""
SOURCE=""
TOOL="kubectl"
OPERATION="apply"
EVIDRA_BIN=""
NO_PROCESS="false"

while [[ $# -gt 0 ]]; do
  case "$1" in
    --artifact)
      [[ $# -ge 2 ]] || { echo "bench-add: --artifact requires a value" >&2; exit 1; }
      ARTIFACT="$2"
      shift 2
      ;;
    --source)
      [[ $# -ge 2 ]] || { echo "bench-add: --source requires a value" >&2; exit 1; }
      SOURCE="$2"
      shift 2
      ;;
    --tool)
      [[ $# -ge 2 ]] || { echo "bench-add: --tool requires a value" >&2; exit 1; }
      TOOL="$2"
      shift 2
      ;;
    --operation)
      [[ $# -ge 2 ]] || { echo "bench-add: --operation requires a value" >&2; exit 1; }
      OPERATION="$2"
      shift 2
      ;;
    --evidra-bin)
      [[ $# -ge 2 ]] || { echo "bench-add: --evidra-bin requires a value" >&2; exit 1; }
      EVIDRA_BIN="$2"
      shift 2
      ;;
    --no-process)
      NO_PROCESS="true"
      shift 1
      ;;
    -h|--help)
      usage
      exit 0
      ;;
    *)
      echo "bench-add: unknown arg: $1" >&2
      usage >&2
      exit 1
      ;;
  esac
done

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT_DIR"

CASE_DIR="tests/benchmark/cases/$CASE_ID"
SOURCE_FILE="tests/benchmark/sources/${SOURCE:-TODO}.md"

if [[ -d "$CASE_DIR" ]]; then
  echo "bench-add: case already exists at $CASE_DIR" >&2
  exit 1
fi

mkdir -p "$CASE_DIR/artifacts" "$CASE_DIR/golden"

ARTIFACT_REF="TODO"
ARTIFACT_DIGEST="TODO"
CATEGORY="TODO"
DIFFICULTY="TODO"
GROUND_TRUTH_PATTERN="TODO"
RISK_LEVEL="TODO"
RISK_DETAILS_JSON="[]"
TAGS_JSON="[]"
PROCESSING_JSON="{}"

case "$TOOL" in
  kubectl) CATEGORY="kubernetes" ;;
  terraform) CATEGORY="terraform" ;;
  helm) CATEGORY="helm" ;;
  argocd) CATEGORY="argocd" ;;
esac

if [[ -n "$ARTIFACT" ]]; then
  if [[ ! -f "$ARTIFACT" ]]; then
    echo "bench-add: artifact not found: $ARTIFACT" >&2
    exit 1
  fi
  ARTIFACT_BASENAME="$(basename "$ARTIFACT")"
  cp "$ARTIFACT" "$CASE_DIR/artifacts/$ARTIFACT_BASENAME"
  ARTIFACT_REF="artifacts/$ARTIFACT_BASENAME"
  if command -v shasum >/dev/null 2>&1; then
    ARTIFACT_DIGEST="sha256:$(shasum -a 256 "$CASE_DIR/artifacts/$ARTIFACT_BASENAME" | awk '{print $1}')"
  elif command -v sha256sum >/dev/null 2>&1; then
    ARTIFACT_DIGEST="sha256:$(sha256sum "$CASE_DIR/artifacts/$ARTIFACT_BASENAME" | awk '{print $1}')"
  fi

  if [[ "$NO_PROCESS" == "false" && -x "tests/benchmark/scripts/process-artifact.sh" ]]; then
    process_tmp="$(mktemp)"
    process_cmd=(bash tests/benchmark/scripts/process-artifact.sh --artifact "$CASE_DIR/artifacts/$ARTIFACT_BASENAME" --tool "$TOOL" --operation "$OPERATION" --out "$process_tmp")
    if [[ -n "$EVIDRA_BIN" ]]; then
      process_cmd+=(--evidra-bin "$EVIDRA_BIN")
    fi

    process_ok="false"
    if "${process_cmd[@]}" >/tmp/bench-add-process.log 2>&1; then
      process_ok="true"
    elif [[ "$TOOL" != "generic" ]]; then
      process_cmd=(bash tests/benchmark/scripts/process-artifact.sh --artifact "$CASE_DIR/artifacts/$ARTIFACT_BASENAME" --tool generic --operation "$OPERATION" --out "$process_tmp")
      if [[ -n "$EVIDRA_BIN" ]]; then
        process_cmd+=(--evidra-bin "$EVIDRA_BIN")
      fi
      if "${process_cmd[@]}" >/tmp/bench-add-process.log 2>&1; then
        process_ok="true"
        echo "bench-add: autofill fallback used tool=generic"
      fi
    fi

    if [[ "$process_ok" == "true" ]]; then
      processed_digest="$(jq -r '.artifact_digest // empty' "$process_tmp")"
      [[ -n "$processed_digest" ]] && ARTIFACT_DIGEST="$processed_digest"

      processed_level="$(jq -r '.risk_level // empty' "$process_tmp")"
      case "$processed_level" in
        low|medium|high|critical) RISK_LEVEL="$processed_level" ;;
      esac

      RISK_DETAILS_JSON="$(jq -c '.risk_details // []' "$process_tmp")"
      TAGS_JSON="$RISK_DETAILS_JSON"

      processed_pattern="$(jq -r '.ground_truth_pattern // empty' "$process_tmp")"
      if [[ -n "$processed_pattern" ]]; then
        GROUND_TRUTH_PATTERN="$processed_pattern"
      else
        single_risk_detail="$(jq -r 'if ((.risk_details // []) | length) == 1 then .risk_details[0] else empty end' "$process_tmp")"
        [[ -n "$single_risk_detail" ]] && GROUND_TRUTH_PATTERN="$single_risk_detail"
      fi

      case "$RISK_LEVEL" in
        low) DIFFICULTY="easy" ;;
        medium) DIFFICULTY="medium" ;;
        high) DIFFICULTY="hard" ;;
        critical) DIFFICULTY="catastrophic" ;;
      esac

      PROCESSING_JSON="$(jq -c '{
        dataset_evidra_version: (.processing.dataset_evidra_version // ""),
        processed_at: (.processing.processed_at // ""),
        tool: (.processing.tool // ""),
        operation: (.processing.operation // ""),
        evidra_version: (.evidra_version // "")
      }' "$process_tmp")"

      echo "bench-add: autofill from process-artifact applied"
    else
      echo "bench-add: WARN process-artifact failed, keeping TODO defaults (see /tmp/bench-add-process.log)" >&2
    fi
    rm -f "$process_tmp"
  fi
fi

cat > "$CASE_DIR/README.md" <<EOF
# $CASE_ID

## Scenario: TODO title

**Category:** TODO  
**Difficulty:** TODO  
**Dataset label:** limited-contract-baseline

**Story:** TODO describe what automation does.

**Impact:** TODO describe concrete operational impact.

**Risk:** TODO describe why this is risky.

**Real-world parallel:** TODO cite CVE/incident/pattern.
EOF

cat > "$CASE_DIR/expected.json" <<EOF
{
  "case_id": "$CASE_ID",
  "dataset_label": "limited-contract-baseline",
  "case_kind": "artifact",
  "category": "$CATEGORY",
  "difficulty": "$DIFFICULTY",
  "ground_truth_pattern": "$GROUND_TRUTH_PATTERN",
  "artifact_ref": "$ARTIFACT_REF",
  "artifact_digest": "$ARTIFACT_DIGEST",
  "risk_details_expected": $RISK_DETAILS_JSON,
  "risk_level": "$RISK_LEVEL",
  "signals_expected": {},
  "tags": $TAGS_JSON,
  "processing": $PROCESSING_JSON,
  "source_refs": [
    {
      "source_id": "${SOURCE:-TODO}",
      "composition": "real-derived"
    }
  ]
}
EOF

if [[ -n "$SOURCE" ]] && [[ ! -f "$SOURCE_FILE" ]]; then
  cat > "$SOURCE_FILE" <<EOF
# Benchmark Source Manifest

\`\`\`yaml
source_id: $SOURCE
source_type: oss
source_composition: real-derived
source_url: TODO
source_path: TODO
source_commit_or_tag: TODO
source_license: TODO
retrieved_at: $(date -u +%Y-%m-%d)
retrieved_by: TODO
transformation_notes: |
  TODO
reviewer: TODO
linked_cases:
  - $CASE_ID
\`\`\`
EOF
  echo "bench-add: created source manifest template: $SOURCE_FILE"
fi

echo "bench-add: created case scaffold: $CASE_DIR"
echo "bench-add: next steps:"
echo "  1) Fill TODO fields in $CASE_DIR/README.md and $CASE_DIR/expected.json"
echo "  2) Ensure source manifest is complete (if created): $SOURCE_FILE"
echo "  3) Run: bash tests/benchmark/scripts/validate-dataset.sh"
