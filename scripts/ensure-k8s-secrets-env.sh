#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
OVERLAY="${EVIDRA_OVERLAY:-trial}"
MODE="strict"
NAMESPACE="${EVIDRA_NAMESPACE:-evidra}"
SECRET_NAME="${EVIDRA_SECRET_NAME:-evidra-secrets}"

while [[ $# -gt 0 ]]; do
  case "$1" in
    --overlay)
      OVERLAY="${2:-}"
      shift 2
      ;;
    --demo|strict|--validate-secret)
      MODE="$1"
      shift
      ;;
    *)
      MODE="$1"
      shift
      ;;
  esac
done

SECRETS_DIR="${ROOT_DIR}/deploy/k8s/overlays/${OVERLAY}"
SECRETS_ENV="${SECRETS_DIR}/secrets.env"
SECRETS_EXAMPLE="${SECRETS_DIR}/secrets.env.example"

required_keys=(
  EVIDRA_DB_HOST
  EVIDRA_DB_PORT
  EVIDRA_DB_NAME
  EVIDRA_DB_USER
  EVIDRA_DB_PASSWORD
  EVIDRA_READ_TOKEN
)

if [[ "${OVERLAY}" == "prod" || "${OVERLAY}" == "openshift" ]]; then
  required_keys+=(EVIDRA_ARGO_API_TOKEN)
fi

fail() {
  echo "FAIL: $1" >&2
  exit 1
}

pass() {
  echo "PASS: $1"
}

random_token() {
  openssl rand -hex 16
}

replace_placeholders_for_demo() {
  local db_password read_token ingest_token argo_token
  db_password="$(random_token)"
  read_token="$(random_token)"
  ingest_token="$(random_token)"
  argo_token="$(random_token)"
  [[ -f "${SECRETS_EXAMPLE}" ]] || fail "missing ${SECRETS_EXAMPLE}"
  sed -i.bak \
    -e "s/CHANGE_ME_DB_PASSWORD/${db_password}/g" \
    -e "s/CHANGE_ME_READ_TOKEN/${read_token}/g" \
    -e "s/CHANGE_ME_INGEST_TOKEN/${ingest_token}/g" \
    -e "s/CHANGE_ME_ARGO_API_TOKEN/${argo_token}/g" \
    "${SECRETS_ENV}"
  rm -f "${SECRETS_ENV}.bak"
}

ensure_secrets_env() {
  [[ -f "${SECRETS_EXAMPLE}" ]] || fail "missing ${SECRETS_EXAMPLE}; expected overlay example file"
  if [[ ! -f "${SECRETS_ENV}" ]]; then
    if [[ "${MODE}" == "--demo" ]]; then
      cp "${SECRETS_EXAMPLE}" "${SECRETS_ENV}"
      replace_placeholders_for_demo
      pass "generated ${SECRETS_ENV} for demo runtime"
    else
      fail "missing ${SECRETS_ENV}; copy ${SECRETS_EXAMPLE} and set values"
    fi
  elif [[ "${MODE}" == "--demo" ]] && rg -q "CHANGE_ME_|REQUIRED_SET_ME" "${SECRETS_ENV}"; then
    replace_placeholders_for_demo
    pass "replaced placeholder values in ${SECRETS_ENV}"
  fi
}

validate_secrets_env() {
  [[ -f "${SECRETS_ENV}" ]] || fail "missing ${SECRETS_ENV}"
  for key in "${required_keys[@]}"; do
    if ! rg -q "^${key}=.+" "${SECRETS_ENV}"; then
      fail "missing required key in ${SECRETS_ENV}: ${key}"
    fi
  done
  if [[ "${MODE}" != "--demo" ]] && rg -q "CHANGE_ME_|REQUIRED_SET_ME" "${SECRETS_ENV}"; then
    fail "${SECRETS_ENV} contains placeholder values; set real secrets before apply"
  fi
  pass "local secrets env contains required keys"
}

validate_cluster_secret() {
  kubectl -n "${NAMESPACE}" get secret "${SECRET_NAME}" >/dev/null 2>&1 || fail "missing secret/${SECRET_NAME} in namespace ${NAMESPACE}"
  for key in "${required_keys[@]}"; do
    local value
    value="$(kubectl -n "${NAMESPACE}" get secret "${SECRET_NAME}" -o "jsonpath={.data.${key}}" 2>/dev/null || true)"
    [[ -n "${value}" ]] || fail "secret/${SECRET_NAME} missing key: ${key}"
  done
  pass "secret/${SECRET_NAME} contains required keys"
}

case "${MODE}" in
  --demo|strict)
    ensure_secrets_env
    validate_secrets_env
    ;;
  --validate-secret)
    validate_cluster_secret
    ;;
  *)
    fail "unsupported mode: ${MODE} (use --demo, strict, or --validate-secret)"
    ;;
esac
