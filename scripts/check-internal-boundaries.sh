#!/usr/bin/env bash
set -euo pipefail

internal_dir="__internal"
internal_prefix="${internal_dir}/"

echo "[boundaries] checking references to internal path outside internal directory"
if rg -n --hidden --glob '!.git/**' --glob "!${internal_dir}/**" --glob '!scripts/check-internal-boundaries.sh' --glob '!.github/workflows/boundaries.yml' "${internal_prefix}" . >/tmp/evidra_boundaries_refs.out 2>/dev/null; then
  echo "[boundaries] found forbidden references outside ${internal_dir}:"
  cat /tmp/evidra_boundaries_refs.out
  exit 1
fi

echo "[boundaries] checking README links to internal path"
if [ -f README.md ] && rg -n "\]\(${internal_prefix}|\]\(\.?/${internal_prefix}|${internal_prefix}" README.md >/tmp/evidra_boundaries_readme.out 2>/dev/null; then
  echo "[boundaries] README.md links or references internal path:"
  cat /tmp/evidra_boundaries_readme.out
  exit 1
fi

echo "[boundaries] checking docs links to internal path"
if [ -d docs ] && rg -n "\]\(${internal_prefix}|\]\(\.?/${internal_prefix}|${internal_prefix}" docs >/tmp/evidra_boundaries_docs.out 2>/dev/null; then
  echo "[boundaries] docs/ links or references internal path:"
  cat /tmp/evidra_boundaries_docs.out
  exit 1
fi

echo "[boundaries] checking internal file copies outside internal directory"
while IFS= read -r internal_file; do
  name="$(basename "$internal_file")"

  if [ -z "$name" ]; then
    continue
  fi

  while IFS= read -r external_file; do
    if cmp -s "$internal_file" "$external_file"; then
      echo "[boundaries] internal file copied outside ${internal_dir}: ${internal_file}"
      echo "[boundaries] duplicate path: ${external_file}"
      exit 1
    fi
  done < <(find . -type f -name "$name" ! -path "./${internal_dir}/*" ! -path './.git/*')
done < <(find "${internal_dir}" -type f)

echo "[boundaries] ok"
