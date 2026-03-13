#!/usr/bin/env bash

export EVIDRA_API_KEY="${EVIDRA_API_KEY:-stand-static-api-key}"
export EVIDRA_INVITE_SECRET="${EVIDRA_INVITE_SECRET:-stand-invite-secret}"
export EVIDRA_WEBHOOK_SECRET_ARGOCD="${EVIDRA_WEBHOOK_SECRET_ARGOCD:-stand-argocd-webhook-secret}"
export DATABASE_URL="${DATABASE_URL:-postgres://evidra:evidra@localhost:5432/evidra?sslmode=disable}"
export LISTEN_ADDR="${LISTEN_ADDR:-:8090}"
