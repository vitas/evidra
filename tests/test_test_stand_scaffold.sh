#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
STAND_DIR="$ROOT_DIR/tests/stand"

fail() {
  printf 'FAIL: %s\n' "$1" >&2
  exit 1
}

assert_file() {
  local path="$1"
  [[ -f "$path" ]] || fail "missing file: ${path#$ROOT_DIR/}"
}

assert_executable() {
  local path="$1"
  assert_file "$path"
  [[ -x "$path" ]] || fail "not executable: ${path#$ROOT_DIR/}"
}

assert_grep() {
  local pattern="$1"
  local path="$2"
  grep -Fq "$pattern" "$path" || fail "missing pattern '$pattern' in ${path#$ROOT_DIR/}"
}

assert_help() {
  local path="$1"
  "$path" --help >/dev/null || fail "help failed: ${path#$ROOT_DIR/}"
}

assert_dir() {
  local path="$1"
  [[ -d "$path" ]] || fail "missing directory: ${path#$ROOT_DIR/}"
}

assert_file "$STAND_DIR/stand.yaml"
assert_file "$STAND_DIR/infra.yaml"
assert_file "$STAND_DIR/.gitignore"
assert_file "$STAND_DIR/lib.sh"

assert_grep "01-happy-path" "$STAND_DIR/stand.yaml"
assert_grep "02-retry-loop" "$STAND_DIR/stand.yaml"
assert_grep "03-declined-decision" "$STAND_DIR/stand.yaml"
assert_grep "04-argocd-sync" "$STAND_DIR/stand.yaml"
assert_grep "05-scorecard-parity" "$STAND_DIR/stand.yaml"

assert_grep "k3d_cluster" "$STAND_DIR/infra.yaml"
assert_grep "argocd" "$STAND_DIR/infra.yaml"
assert_grep "evidra_api" "$STAND_DIR/infra.yaml"
assert_grep "postgres" "$STAND_DIR/infra.yaml"
assert_grep "evidra_cli" "$STAND_DIR/infra.yaml"

assert_file "$STAND_DIR/cluster/k3d-config.yaml"
assert_file "$STAND_DIR/evidra/docker-compose.yaml"
assert_file "$STAND_DIR/evidra/env.sh"
assert_file "$STAND_DIR/argocd/install.sh"
assert_file "$STAND_DIR/argocd/values.yaml"
assert_file "$STAND_DIR/argocd/notifications-cm.yaml"
assert_file "$STAND_DIR/workloads/nginx-dev.yaml"
assert_file "$STAND_DIR/workloads/nginx-prod.yaml"
assert_file "$STAND_DIR/workloads/risky-deploy.yaml"
assert_file "$STAND_DIR/workloads/broken-deploy.yaml"
assert_file "$STAND_DIR/workloads/nginx-dev-application.yaml"
assert_file "$STAND_DIR/results/.gitkeep"
assert_file "$STAND_DIR/logs/.gitkeep"

assert_grep "8090:8080" "$STAND_DIR/evidra/docker-compose.yaml"
assert_grep "http://host.k3d.internal:8090/v1/hooks/argocd" "$STAND_DIR/argocd/notifications-cm.yaml"
assert_grep "X-Evidra-API-Key" "$STAND_DIR/argocd/notifications-cm.yaml"
assert_grep "name: Authorization" "$STAND_DIR/argocd/notifications-cm.yaml"
assert_grep "value: Bearer __EVIDRA_WEBHOOK_SECRET_ARGOCD__" "$STAND_DIR/argocd/notifications-cm.yaml"

assert_executable "$STAND_DIR/setup.sh"
assert_executable "$STAND_DIR/reset.sh"
assert_executable "$STAND_DIR/teardown.sh"
assert_executable "$STAND_DIR/run-all.sh"
assert_help "$STAND_DIR/setup.sh"
assert_help "$STAND_DIR/reset.sh"
assert_help "$STAND_DIR/teardown.sh"
assert_help "$STAND_DIR/run-all.sh"

for script in "$STAND_DIR"/setup.sh "$STAND_DIR"/reset.sh "$STAND_DIR"/teardown.sh "$STAND_DIR"/run-all.sh "$STAND_DIR"/argocd/install.sh "$STAND_DIR"/scenarios/*/run.sh; do
  bash -n "$script" || fail "bash -n failed: ${script#$ROOT_DIR/}"
done

for scenario in \
  01-happy-path \
  02-retry-loop \
  03-declined-decision \
  04-argocd-sync \
  05-scorecard-parity
do
  scenario_dir="$STAND_DIR/scenarios/$scenario"
  assert_dir "$scenario_dir"
  assert_file "$scenario_dir/scenario.yaml"
  assert_executable "$scenario_dir/run.sh"
  assert_help "$scenario_dir/run.sh"
  assert_grep "scenario_id: \"$scenario\"" "$scenario_dir/scenario.yaml"
done

printf 'PASS: tests/stand scaffold verified\n'
