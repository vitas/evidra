#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
CONTRACT_VERSION="${EVIDRA_PROMPT_CONTRACT_VERSION:-v1.4.0}"

cd "${ROOT_DIR}"
go run ./cmd/evidra prompts verify --contract "${CONTRACT_VERSION}" --root "${ROOT_DIR}"
