#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
CLUSTER_NAME="${KIND_CLUSTER_NAME:-evidra}"
ARGOCD_NS="${ARGOCD_NAMESPACE:-argocd}"
ARGOCD_VERSION="${ARGOCD_VERSION:-v3.3.0}"
ARGOCD_INSTALL_URL="${ARGOCD_INSTALL_URL:-https://raw.githubusercontent.com/argoproj/argo-cd/${ARGOCD_VERSION}/manifests/install.yaml}"

pass() { echo "PASS: $1"; }
fail() { echo "FAIL: $1" >&2; exit 1; }

debug_argocd() {
  echo "---- argocd pods ----"
  kubectl -n "${ARGOCD_NS}" get pods -o wide || true
  echo "---- argocd events (latest 40) ----"
  kubectl -n "${ARGOCD_NS}" get events --sort-by=.lastTimestamp | tail -n 40 || true
}

debug_workload_pods() {
  local workload="$1"
  local selector="app.kubernetes.io/name=${workload}"
  local pods
  pods="$(kubectl -n "${ARGOCD_NS}" get pods -l "${selector}" -o jsonpath='{range .items[*]}{.metadata.name}{"\n"}{end}' 2>/dev/null || true)"
  if [[ -z "${pods}" ]]; then
    return
  fi
  while IFS= read -r pod; do
    [[ -n "${pod}" ]] || continue
    echo "---- describe pod/${pod} ----"
    kubectl -n "${ARGOCD_NS}" describe pod "${pod}" || true
    echo "---- logs pod/${pod} (all containers) ----"
    kubectl -n "${ARGOCD_NS}" logs "${pod}" --all-containers=true --tail=200 || true
    echo "---- logs pod/${pod} (init containers) ----"
    init_names="$(kubectl -n "${ARGOCD_NS}" get pod "${pod}" -o jsonpath='{range .spec.initContainers[*]}{.name}{"\n"}{end}' 2>/dev/null || true)"
    while IFS= read -r initc; do
      [[ -n "${initc}" ]] || continue
      kubectl -n "${ARGOCD_NS}" logs "${pod}" -c "${initc}" --tail=200 || true
    done <<< "${init_names}"
  done <<< "${pods}"
}

rollout_or_debug() {
  local kind_name="$1"
  local resource="$2"
  local timeout="$3"
  if ! kubectl -n "${ARGOCD_NS}" rollout status "${kind_name}/${resource}" --timeout="${timeout}"; then
    echo "Rollout failed for ${kind_name}/${resource}"
    debug_argocd
    echo "---- describe ${kind_name}/${resource} ----"
    kubectl -n "${ARGOCD_NS}" describe "${kind_name}" "${resource}" || true
    echo "---- logs ${kind_name}/${resource} (all containers) ----"
    kubectl -n "${ARGOCD_NS}" logs "${kind_name}/${resource}" --all-containers=true --tail=200 || true
    debug_workload_pods "${resource}"
    fail "rollout failed for ${kind_name}/${resource}"
  fi
}

check_evidra_api() {
  local base_url="${EVIDRA_EXTENSION_API_BASE_URL:-${EVIDRA_EXTENSION_BASE_URL:-http://localhost:8080}}"
  local code=""

  code="$(curl -sS -o /tmp/evidra_kind_healthz.json -w "%{http_code}" --max-time 3 "${base_url}/healthz" 2>/dev/null || true)"
  if [[ "${code}" != "200" ]]; then
    echo "WARN: Evidra API is not reachable at ${base_url} (healthz status=${code:-n/a})."
    echo "      Start the sandbox first:"
    echo "      make evidra-demo"
    return 0
  fi

  code="$(curl -sS -o /tmp/evidra_kind_subjects.json -w "%{http_code}" --max-time 3 "${base_url}/v1/subjects" 2>/dev/null || true)"
  case "${code}" in
    200)
      pass "Evidra API preflight passed at ${base_url}"
      ;;
    401|403)
      echo "WARN: Evidra API at ${base_url} requires auth (status=${code})."
      echo "      Configure extension auth if needed:"
      echo "      EVIDRA_EXTENSION_AUTH_MODE=bearer EVIDRA_EXTENSION_AUTH_TOKEN=<token> make evidra-ui-refresh"
      ;;
    *)
      echo "WARN: Evidra API preflight returned unexpected status at ${base_url}/v1/subjects (status=${code:-n/a})."
      ;;
  esac
}

require_cmd() {
  command -v "$1" >/dev/null 2>&1 || fail "missing required tool: $1"
}

require_cmd kind
require_cmd kubectl
require_cmd bash

if bash "${ROOT_DIR}/scripts/ensure-k8s-secrets-env.sh" --overlay trial --demo >/dev/null 2>&1; then
  pass "trial overlay secrets prepared for optional in-cluster deploy"
fi

if ! kind get clusters | grep -qx "${CLUSTER_NAME}"; then
  echo "Creating kind cluster: ${CLUSTER_NAME}"
  kind create cluster --name "${CLUSTER_NAME}"
else
  echo "Using existing kind cluster: ${CLUSTER_NAME}"
fi

kubectl cluster-info >/dev/null 2>&1 || fail "kubectl cannot reach the active cluster"
pass "cluster is reachable"

kubectl create namespace "${ARGOCD_NS}" --dry-run=client -o yaml | kubectl apply -f - >/dev/null
pass "argocd namespace ready"

echo "Installing Argo CD manifests from: ${ARGOCD_INSTALL_URL}"
kubectl apply --server-side=true -n "${ARGOCD_NS}" -f "${ARGOCD_INSTALL_URL}" >/dev/null

# Avoid bootstrap false negatives on slow image pulls/network.
kubectl -n "${ARGOCD_NS}" patch deploy argocd-server --type merge -p '{"spec":{"progressDeadlineSeconds":1800}}' >/dev/null || true
kubectl -n "${ARGOCD_NS}" patch deploy argocd-repo-server --type merge -p '{"spec":{"progressDeadlineSeconds":1800}}' >/dev/null || true

rollout_or_debug deployment argocd-server 420s
rollout_or_debug deployment argocd-repo-server 420s
rollout_or_debug statefulset argocd-application-controller 420s
pass "argocd core workloads ready"

ARGOCD_NAMESPACE="${ARGOCD_NS}" bash "${ROOT_DIR}/scripts/install-argocd-extension.sh" >/dev/null
pass "argocd-server patched with evidra extension"
check_evidra_api

echo "Run this command to open Argo CD UI locally:"
echo "  bash scripts/argocd-port-forward.sh start"
echo "Then open:"
echo "  http://localhost:8081"
echo "Initial admin password:"
if admin_pw="$(kubectl -n "${ARGOCD_NS}" get secret argocd-initial-admin-secret -o jsonpath='{.data.password}' 2>/dev/null | base64 -d 2>/dev/null)"; then
  if [[ -n "${admin_pw}" ]]; then
    echo "  ${admin_pw}"
  else
    echo "  (empty password output)"
  fi
else
  echo "  (not available yet, retry: kubectl -n ${ARGOCD_NS} get secret argocd-initial-admin-secret -o jsonpath='{.data.password}' | base64 -d)"
fi
