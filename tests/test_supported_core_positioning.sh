#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT_DIR"

fail() {
  echo "FAIL: $*" >&2
  exit 1
}

grep -Fq "CLI and MCP are the authoritative analytics surfaces today." README.md \
  || fail "README should make the supported analytics path explicit"

grep -Fq "append-only evidence chain" README.md \
  || fail "README should describe the evidence-chain core"

grep -Fq "flight recorder for AI agents that touch infrastructure" docs/guides/mcp-setup.md \
  || fail "MCP guide should lead with the flight-recorder positioning"

grep -Fq "The agent reports voluntarily; Evidra observes, scores, and explains." docs/guides/mcp-setup.md \
  || fail "MCP guide should explain the non-intercepting model"

grep -Fq "Self-hosted remains supported for centralized evidence collection" docs/guides/self-hosted-setup.md \
  || fail "self-hosted status should keep the centralized evidence boundary explicit"

echo "PASS: test_supported_core_positioning"
