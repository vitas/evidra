#!/usr/bin/env bash
set -euo pipefail

BASE_URL="${EVIDRA_BASE_URL:-http://localhost:8080}"
EVENT_DIR="testdata/events"

curl -fsS "${BASE_URL}/healthz" >/tmp/evidra_replay_health.json

for file in "${EVENT_DIR}"/*_valid.json; do
  echo "replay: ${file}"
  curl -fsS -X POST "${BASE_URL}/v1/events" \
    -H 'Content-Type: application/json' \
    --data-binary "@${file}" >/tmp/evidra_replay_ingest.json

done

curl -fsS "${BASE_URL}/v1/timeline?subject=payments-api:prod-eu:eu-1&from=2026-02-16T00:00:00Z&to=2026-02-16T23:59:59Z&limit=10" >/tmp/evidra_replay_timeline.json

echo "fixture replay complete"
