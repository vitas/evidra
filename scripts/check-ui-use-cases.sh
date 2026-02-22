#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
DOC_FILE="${ROOT_DIR}/docs/testing/ui-use-cases.md"
TEST_FILE="${ROOT_DIR}/ui/e2e/evidra.spec.ts"

if [[ ! -f "${DOC_FILE}" ]]; then
  echo "FAIL: missing ${DOC_FILE}" >&2
  exit 1
fi

if [[ ! -f "${TEST_FILE}" ]]; then
  echo "FAIL: missing ${TEST_FILE}" >&2
  exit 1
fi

doc_ids="$(rg -o 'UC-[0-9]{3}' "${DOC_FILE}" | sort -u)"
test_ids="$(rg -o 'UC-[0-9]{3}' "${TEST_FILE}" | sort -u)"

if [[ -z "${doc_ids}" ]]; then
  echo "FAIL: no use case IDs found in ${DOC_FILE}" >&2
  exit 1
fi

if [[ -z "${test_ids}" ]]; then
  echo "FAIL: no use case IDs found in ${TEST_FILE}" >&2
  exit 1
fi

missing_in_tests="$(comm -23 <(printf "%s\n" "${doc_ids}") <(printf "%s\n" "${test_ids}") || true)"
missing_in_docs="$(comm -13 <(printf "%s\n" "${doc_ids}") <(printf "%s\n" "${test_ids}") || true)"

if [[ -n "${missing_in_tests}" ]]; then
  echo "FAIL: use case IDs documented but not covered in tests:" >&2
  printf "  %s\n" ${missing_in_tests} >&2
  exit 1
fi

if [[ -n "${missing_in_docs}" ]]; then
  echo "FAIL: use case IDs found in tests but not documented:" >&2
  printf "  %s\n" ${missing_in_docs} >&2
  exit 1
fi

echo "PASS: UI use case mapping is consistent (${DOC_FILE} ↔ ${TEST_FILE})"
