#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
UI_DIR="${ROOT_DIR}/ui"
OUT_DIR="${ROOT_DIR}/tmp/argocd/extensions/evidra"

command -v npm >/dev/null 2>&1 || {
  echo "npm is required to build the Argo CD extension bundle." >&2
  exit 1
}

hash_file() {
  local f="$1"
  if command -v sha256sum >/dev/null 2>&1; then
    sha256sum "$f" | awk '{print $1}'
    return
  fi
  if command -v shasum >/dev/null 2>&1; then
    shasum -a 256 "$f" | awk '{print $1}'
    return
  fi
  echo "missing sha256 tool (sha256sum or shasum)" >&2
  exit 1
}

mkdir -p "${OUT_DIR}"

(
  cd "${UI_DIR}"
  npm run build:argocd-extension >/dev/null
)

cp "${UI_DIR}/dist-argocd-extension/evidra-argocd-extension.js" "${OUT_DIR}/extension-evidra.js"
hash_file "${OUT_DIR}/extension-evidra.js" > "${OUT_DIR}/extension-evidra.sha256"

cat > "${OUT_DIR}/extension-evidra.js.map.note.txt" <<'EOF'
This extension bundle is generated from ui/src/argocd-extension.js.
Rebuild with: npm --prefix ui run build:argocd-extension
EOF

echo "Prepared Argo CD extension bundle at:"
echo "  ${OUT_DIR}/extension-evidra.js"
echo "Bundle checksum (sha256):"
echo "  $(cat "${OUT_DIR}/extension-evidra.sha256")"
