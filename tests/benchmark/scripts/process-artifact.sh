#!/usr/bin/env bash
set -euo pipefail

usage() {
  cat <<'EOF'
Usage:
  tests/benchmark/scripts/process-artifact.sh --artifact <path> [options]

Options:
  --artifact <path>      Artifact file to process (required)
  --tool <name>          Tool name for prescribe (default: kubectl)
  --operation <name>     Operation for prescribe (default: apply)
  --out <path>           Write contract JSON to file (default: stdout)
  --evidence-dir <path>  Evidence dir (default: temp dir)
  --evidra-bin <path>    Explicit evidra binary path (skip resolver)
  --cache-dir <path>     Cache directory for pinned binaries (default: ./bin)
  --repo <owner/name>    GitHub repo for release downloads (default: vitas/evidra)
  --force-download       Re-download pinned binary even if cached
  -h, --help             Show this help
EOF
}

fail() {
  echo "process-artifact: $*" >&2
  exit 1
}

ARTIFACT=""
TOOL="kubectl"
OPERATION="apply"
OUT_PATH=""
EVIDENCE_DIR=""
EVIDRA_BIN=""
CACHE_DIR="bin"
REPO="vitas/evidra"
FORCE_DOWNLOAD="false"

while [[ $# -gt 0 ]]; do
  case "$1" in
    --artifact)
      [[ $# -ge 2 ]] || fail "--artifact requires a value"
      ARTIFACT="$2"
      shift 2
      ;;
    --tool)
      [[ $# -ge 2 ]] || fail "--tool requires a value"
      TOOL="$2"
      shift 2
      ;;
    --operation)
      [[ $# -ge 2 ]] || fail "--operation requires a value"
      OPERATION="$2"
      shift 2
      ;;
    --out)
      [[ $# -ge 2 ]] || fail "--out requires a value"
      OUT_PATH="$2"
      shift 2
      ;;
    --evidence-dir)
      [[ $# -ge 2 ]] || fail "--evidence-dir requires a value"
      EVIDENCE_DIR="$2"
      shift 2
      ;;
    --evidra-bin)
      [[ $# -ge 2 ]] || fail "--evidra-bin requires a value"
      EVIDRA_BIN="$2"
      shift 2
      ;;
    --cache-dir)
      [[ $# -ge 2 ]] || fail "--cache-dir requires a value"
      CACHE_DIR="$2"
      shift 2
      ;;
    --repo)
      [[ $# -ge 2 ]] || fail "--repo requires a value"
      REPO="$2"
      shift 2
      ;;
    --force-download)
      FORCE_DOWNLOAD="true"
      shift 1
      ;;
    -h|--help)
      usage
      exit 0
      ;;
    *)
      fail "unknown argument: $1"
      ;;
  esac
done

[[ -n "$ARTIFACT" ]] || { usage >&2; fail "--artifact is required"; }
[[ -f "$ARTIFACT" ]] || fail "artifact not found: $ARTIFACT"
command -v jq >/dev/null 2>&1 || fail "jq is required"
command -v curl >/dev/null 2>&1 || fail "curl is required"
command -v tar >/dev/null 2>&1 || fail "tar is required"

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../../.." && pwd)"
cd "$ROOT_DIR"

DATASET_JSON="tests/benchmark/dataset.json"
[[ -f "$DATASET_JSON" ]] || fail "missing $DATASET_JSON"

DATASET_EVIDRA_VERSION="$(jq -r '.evidra_version_processed // empty' "$DATASET_JSON")"
[[ -n "$DATASET_EVIDRA_VERSION" ]] || fail "dataset.json missing evidra_version_processed"

normalize_arch() {
  case "$1" in
    x86_64|amd64) echo "amd64" ;;
    arm64|aarch64) echo "arm64" ;;
    *) echo "$1" ;;
  esac
}

is_concrete_version() {
  local v="$1"
  [[ "$v" =~ ^v?[0-9]+\.[0-9]+\.[0-9]+([.-][A-Za-z0-9]+)*$ ]]
}

download_pinned_binary() {
  local requested_version="$1"
  local out_bin="$2"
  local os arch normalized_version release_tag archive filename url

  os="$(uname -s | tr '[:upper:]' '[:lower:]')"
  arch="$(normalize_arch "$(uname -m)")"
  normalized_version="${requested_version#v}"
  release_tag="v${normalized_version}"

  mkdir -p "$(dirname "$out_bin")"
  tmpdir="$(mktemp -d)"
  trap 'rm -rf "$tmpdir"' RETURN

  filename="evidra_${normalized_version}_${os}_${arch}.tar.gz"
  url="https://github.com/${REPO}/releases/download/${release_tag}/${filename}"

  if ! curl -fsSL "$url" -o "$tmpdir/evidra.tgz"; then
    fail "failed to download pinned evidra binary from $url"
  fi

  tar -xzf "$tmpdir/evidra.tgz" -C "$tmpdir"
  if [[ ! -f "$tmpdir/evidra" ]]; then
    fail "downloaded archive does not contain evidra binary"
  fi

  mv "$tmpdir/evidra" "$out_bin"
  chmod +x "$out_bin"
}

resolve_evidra_bin() {
  local cache_bin
  if [[ -n "$EVIDRA_BIN" ]]; then
    [[ -x "$EVIDRA_BIN" ]] || fail "--evidra-bin is not executable: $EVIDRA_BIN"
    echo "$EVIDRA_BIN"
    return
  fi

  cache_bin="${CACHE_DIR%/}/evidra-${DATASET_EVIDRA_VERSION}"
  if [[ "$FORCE_DOWNLOAD" == "false" && -x "$cache_bin" ]]; then
    echo "$cache_bin"
    return
  fi

  if is_concrete_version "$DATASET_EVIDRA_VERSION"; then
    download_pinned_binary "$DATASET_EVIDRA_VERSION" "$cache_bin"
    echo "$cache_bin"
    return
  fi

  # Non-concrete dataset version (e.g. v0.3.x): fallback to local binary.
  if [[ -x "${CACHE_DIR%/}/evidra" ]]; then
    echo "${CACHE_DIR%/}/evidra"
    return
  fi
  if command -v evidra >/dev/null 2>&1; then
    command -v evidra
    return
  fi

  fail "cannot resolve evidra binary for non-concrete version '$DATASET_EVIDRA_VERSION'; provide --evidra-bin"
}

RESOLVED_BIN="$(resolve_evidra_bin)"

if [[ -z "$EVIDENCE_DIR" ]]; then
  EVIDENCE_DIR="$(mktemp -d)"
  CLEAN_EVIDENCE_DIR="true"
else
  mkdir -p "$EVIDENCE_DIR"
  CLEAN_EVIDENCE_DIR="false"
fi

cleanup() {
  if [[ "${CLEAN_EVIDENCE_DIR:-false}" == "true" ]]; then
    rm -rf "$EVIDENCE_DIR"
  fi
}
trap cleanup EXIT

prescribe_out="$("$RESOLVED_BIN" prescribe \
  --tool "$TOOL" \
  --operation "$OPERATION" \
  --artifact "$ARTIFACT" \
  --signing-mode optional \
  --evidence-dir "$EVIDENCE_DIR")"

echo "$prescribe_out" | jq -e . >/dev/null 2>&1 || fail "prescribe output is not valid JSON"

if echo "$prescribe_out" | jq -e '.ok == false' >/dev/null; then
  err_code="$(echo "$prescribe_out" | jq -r '.error.code // "unknown_error"')"
  err_msg="$(echo "$prescribe_out" | jq -r '.error.message // "unknown error"')"
  fail "prescribe failed: ${err_code}: ${err_msg}"
fi

runtime_version="$("$RESOLVED_BIN" version 2>/dev/null | awk '{print $2}')"
runtime_version="${runtime_version:-unknown}"
processed_at="$(date -u +%Y-%m-%dT%H:%M:%SZ)"

contract_json="$(echo "$prescribe_out" | jq -c \
  --arg dataset_version "$DATASET_EVIDRA_VERSION" \
  --arg runtime_version "$runtime_version" \
  --arg processed_at "$processed_at" \
  --arg tool "$TOOL" \
  --arg operation "$OPERATION" '
{
  ground_truth_pattern: (.ground_truth_pattern // ""),
  risk_level: (.risk_level // "unknown"),
  risk_details: (.risk_details // .risk_tags // []),
  operation_class: (.operation_class // ""),
  scope_class: (.scope_class // ""),
  resource_count: (.resource_count // 0),
  canon_version: (.canon_version // ""),
  artifact_digest: (.artifact_digest // ""),
  intent_digest: (.intent_digest // ""),
  prescription_id: (.prescription_id // ""),
  evidra_version: $runtime_version,
  processing: {
    dataset_evidra_version: $dataset_version,
    processed_at: $processed_at,
    tool: $tool,
    operation: $operation
  }
}')"

if [[ -n "$OUT_PATH" ]]; then
  mkdir -p "$(dirname "$OUT_PATH")"
  echo "$contract_json" | jq '.' > "$OUT_PATH"
  echo "process-artifact: wrote contract to $OUT_PATH" >&2
else
  echo "$contract_json" | jq '.'
fi

echo "process-artifact: used binary $RESOLVED_BIN (dataset evidra_version_processed=$DATASET_EVIDRA_VERSION)" >&2
