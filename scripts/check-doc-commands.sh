#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT_DIR"

fail() {
  echo "doc check failed: $*" >&2
  exit 1
}

has_pattern() {
  local pattern="$1"
  local file="$2"
  if command -v rg >/dev/null 2>&1; then
    rg -q --fixed-strings -- "$pattern" "$file"
    return
  fi
  grep -Fq -- "$pattern" "$file"
}

require_pattern() {
  local file="$1"
  local pattern="$2"
  if ! has_pattern "$pattern" "$file"; then
    fail "missing pattern '$pattern' in $file"
  fi
}

require_file() {
  [[ -e "$1" ]] || fail "documented file does not exist: $1"
}

# --------------------------------------------------------------------------------------------
# The commands named in docs/integrations/cli-reference.md must exist, and the binaries must
# still behave the way that file says. A previous version of this script drove
# `evidra prescribe` and `evidra report` and required README to mention a signing-mode
# variable; the §43 prune deleted both commands and that variable never existed in the Go
# sources. The check stayed green for exactly as long as nobody ran it, which is the failure
# mode every other guard in tests/ was built to prevent.
# --------------------------------------------------------------------------------------------

require_file "docs/integrations/cli-reference.md"

# Every flag documented in the CLI reference has to be one the binary advertises.
mcp_help="$(go run ./cmd/evidra-mcp --help 2>&1 || true)"
for flag in --proxy --enforce --evidence-dir --server-name --advertise-passthrough --max-message --actor-id; do
  # grep without -q: -q closes the pipe early, and under `set -o pipefail` the writer's
  # SIGPIPE becomes the pipeline's status, so the check fails on success.
  printf '%s\n' "$mcp_help" | grep -- "$flag" >/dev/null || fail "docs advertise $flag but --help does not list it"
  require_pattern "docs/integrations/cli-reference.md" "$flag"
done

evidra_help="$(go run ./cmd/evidra --help 2>&1 || true)"
summarize_help="$(go run ./cmd/evidra summarize --help 2>&1 || true)"
verify_help="$(go run ./cmd/evidra verify --help 2>&1 || true)"
for cmd in summarize verify version; do
  printf '%s\n' "$evidra_help" | grep -- "$cmd" >/dev/null || fail "evidra $cmd missing from the command list"
done
for flag in -dir -since; do
  printf '%s\n' "$summarize_help" | grep -- "$flag" >/dev/null || fail "summarize $flag missing"
  printf '%s\n' "$verify_help" | grep -- "$flag" >/dev/null || fail "verify $flag missing"
done

# A documented environment variable must be one the code actually reads. The table rows are
# the documented set; the prose around them may name removed variables to explain them.
# Only the table rows count as documented; the prose under them names removed variables in
# order to explain why they are gone.
documented="$(awk -F'|' '/^\| `EVIDRA_[A-Z_]+`/ { gsub(/[ `]/, "", $2); print $2 }' README.md | sort -u)"
in_code="$(grep -rhoE 'EVIDRA_[A-Z_]+' --include=*.go cmd pkg | sort -u || true)"
for var in $documented; do
  printf '%s\n' "$in_code" | grep -x -- "$var" >/dev/null || fail "README documents $var, which no Go source reads"
done
for var in $in_code; do
  printf '%s\n' "$documented" | grep -x -- "$var" >/dev/null || fail "$var is read by the code but missing from the README table"
done
[[ -n "$documented" ]] || fail "README documents no environment variables at all"

# The endpoint records, so it must refuse to start with nothing to wrap.
if go run ./cmd/evidra-mcp --proxy >/dev/null 2>&1; then
  fail "evidra-mcp --proxy accepted an empty upstream command"
fi

go run ./cmd/evidra version >/dev/null

echo "doc checks passed"
