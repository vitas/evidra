#!/usr/bin/env bash
set -euo pipefail

bash scripts/argocd-port-forward.sh stop || true
bash scripts/demo-kind-argocd-down.sh || true
docker compose down || true

echo "Sandbox cleaned."
