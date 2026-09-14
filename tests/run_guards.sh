#!/usr/bin/env bash
set -uo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT_DIR"

# Run every shell guard in tests/ and fail if any of them fails.
#
# There is deliberately no exclusion list. A guard that is expected to fail is a bug to fix
# in the same commit, not a line to comment out: that is the exact shape of the failure this
# repository keeps hitting - a checklist row (a CI step, a required file, a name every guard still checks) that
# points at something which no longer exists and therefore proves nothing.

self="$(basename "$0")"
total=0
failed=0
failed_names=()

for guard in tests/test_*.sh; do
  [[ -f "$guard" ]] || continue
  [[ "$(basename "$guard")" == "$self" ]] && continue
  if [[ ! -x "$guard" ]]; then
    echo "FAIL: $guard is not executable (CI runs it with bash, so humans run it with bash too)"
    failed=$((failed + 1))
    failed_names+=("$guard (not executable)")
    total=$((total + 1))
    continue
  fi
  total=$((total + 1))
  if out="$(bash "$guard" 2>&1)"; then
    printf 'PASS  %s\n' "$(basename "$guard")"
  else
    printf 'FAIL  %s\n' "$(basename "$guard")"
    printf '%s\n' "$out" | sed 's/^/        /'
    failed=$((failed + 1))
    failed_names+=("$(basename "$guard")")
  fi
done

echo
if ((failed > 0)); then
  echo "guard suite FAILED: $failed of $total guards did not pass:"
  printf '  - %s\n' "${failed_names[@]}"
  exit 1
fi

echo "guard suite passed: $total/$total"
