#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT_DIR"

fail() {
  echo "FAIL: $*" >&2
  exit 1
}

# Scan one whitespace-normalized line so Markdown wrapping cannot hide or break a
# positioning statement.
readme_text="$(tr '\n\r\t' '   ' <README.md | tr -s ' ')"
getting_started_text="$(tr '\n\r\t' '   ' <docs/getting-started.md | tr -s ' ')"

preview_status="The relaunched Core line is currently an unreleased preview built from main."
grep -Fq -- "$preview_status" <<<"$readme_text" \
  || fail "README should contain the current Core release status"
grep -Fq -- "$preview_status" <<<"$getting_started_text" \
  || fail "getting-started should contain the current Core release status"

required_text=(
  "MCP execution evidence"
  "Declared"
  "Observed"
  "Reported"
  "Evidra does not sandbox the wrapped command"
  "evidra-mcp --proxy"
  "evidra summarize --dir"
  "evidra verify --dir"
  "docs/getting-started.md"
  "docs/architecture.md"
  "docs/evidence-format.md"
  "docs/validation.md"
)

for text in "${required_text[@]}"; do
  grep -Fq -- "$text" <<<"$readme_text" \
    || fail "README should contain: $text"
done

forbidden_patterns=(
  "DevOps MCP Server"
  "reliability scoring"
  "risk assessment"
  "hosted API"
  "scorecard"
  "webhooks"
  "built-in[^.]*[^[:alnum:]_](kubectl|helm|terraform|aws)([^[:alnum:]_]|$)"
  "waits on Gate C"
  # The redesign's research codename is retired language: the reader-facing
  # story is the Core recorder, not its development history.
  'v'"next"
)

for pattern in "${forbidden_patterns[@]}"; do
  if grep -Eiq -- "$pattern" <<<"$readme_text"; then
    fail "README should not contain obsolete product language matching: $pattern"
  fi
done

echo "PASS: test_supported_core_positioning"
