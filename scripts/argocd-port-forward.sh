#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
STATE_DIR="${ROOT_DIR}/tmp/argocd"
PID_FILE="${STATE_DIR}/port-forward.pid"
LOG_FILE="${STATE_DIR}/port-forward.log"
NAMESPACE="${ARGOCD_NAMESPACE:-argocd}"
LOCAL_PORT="${ARGOCD_LOCAL_PORT:-8081}"

mkdir -p "${STATE_DIR}"

is_running() {
  [[ -f "${PID_FILE}" ]] || return 1
  kill -0 "$(cat "${PID_FILE}")" >/dev/null 2>&1
}

has_listener() {
  lsof -iTCP:"${LOCAL_PORT}" -sTCP:LISTEN -n -P >/dev/null 2>&1
}

cleanup_stale_forwarders() {
  # Remove stale kubectl forwarders that keep the port busy.
  pkill -f "kubectl -n ${NAMESPACE} port-forward svc/argocd-server ${LOCAL_PORT}:443" >/dev/null 2>&1 || true
}

wait_until_ready() {
  local i=0
  while [[ "${i}" -lt 20 ]]; do
    if curl -k -fsS "https://localhost:${LOCAL_PORT}" >/dev/null 2>&1; then
      return 0
    fi
    i=$((i + 1))
    sleep 1
  done
  return 1
}

start() {
  if is_running && has_listener; then
    echo "Argo CD port-forward already running (pid $(cat "${PID_FILE}"))."
    return 0
  fi

  cleanup_stale_forwarders

  nohup kubectl -n "${NAMESPACE}" port-forward svc/argocd-server "${LOCAL_PORT}:443" >"${LOG_FILE}" 2>&1 < /dev/null &
  echo $! > "${PID_FILE}"
  sleep 1

  if is_running && has_listener && wait_until_ready; then
    echo "Started Argo CD port-forward on https://localhost:${LOCAL_PORT} (pid $(cat "${PID_FILE}"))."
    echo "Logs: ${LOG_FILE}"
    return 0
  fi

  echo "Failed to start Argo CD port-forward."
  [[ -f "${LOG_FILE}" ]] && tail -n 40 "${LOG_FILE}" || true
  rm -f "${PID_FILE}"
  return 1
}

stop() {
  if ! [[ -f "${PID_FILE}" ]]; then
    echo "No Argo CD port-forward pid file."
    return 0
  fi

  pid="$(cat "${PID_FILE}")"
  if kill -0 "${pid}" >/dev/null 2>&1; then
    kill "${pid}" >/dev/null 2>&1 || true
    sleep 1
    if kill -0 "${pid}" >/dev/null 2>&1; then
      kill -9 "${pid}" >/dev/null 2>&1 || true
    fi
    echo "Stopped Argo CD port-forward (pid ${pid})."
  else
    echo "Argo CD port-forward process not running."
  fi
  rm -f "${PID_FILE}"
}

status() {
  if is_running && has_listener; then
    echo "Argo CD port-forward running (pid $(cat "${PID_FILE}")) on https://localhost:${LOCAL_PORT}"
  else
    echo "Argo CD port-forward not running."
  fi
}

cmd="${1:-start}"
case "${cmd}" in
  start) start ;;
  stop) stop ;;
  status) status ;;
  *)
    echo "Usage: $0 {start|stop|status}"
    exit 1
    ;;
esac
