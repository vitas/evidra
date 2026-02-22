# Configuration Modes

This page lists required environment variables and configuration properties for supported setup modes.
With the introduction of Viper, all `EVIDRA_*` environment variables can now also be supplied via a `config.yaml` file placed in the working directory or `/etc/evidra/config.yaml`.

## Mode 1: Local Docker Compose (default)

Required:
- `EVIDRA_DB_DRIVER=pgx`
- `EVIDRA_DB_DIALECT=postgres`
- `EVIDRA_DB_DSN` (compose default is already set)

Optional:
- `EVIDRA_READ_TOKEN`

## Mode 2: Kubernetes Trial Overlay

Required:
- `deploy/k8s/overlays/trial/secrets.env` with:
  - `EVIDRA_DB_HOST`
  - `EVIDRA_DB_PORT`
  - `EVIDRA_DB_NAME`
  - `EVIDRA_DB_USER`
  - `EVIDRA_DB_PASSWORD`
  - `EVIDRA_READ_TOKEN`

Optional:
- `EVIDRA_ARGO_API_TOKEN` (when collector uses token auth)
- `EVIDRA_INGEST_TOKEN` (only when ingest is enabled)

Recommended:
- Enable `EVIDRA_ARGO_COLLECTOR_ENABLED=true` with Argo endpoint and read-only token

## Mode 3: Production Overlay

Required:
- `deploy/k8s/overlays/prod/secrets.env` with:
  - `EVIDRA_DB_HOST`
  - `EVIDRA_DB_PORT`
  - `EVIDRA_DB_NAME`
  - `EVIDRA_DB_USER`
  - `EVIDRA_DB_PASSWORD`
  - `EVIDRA_READ_TOKEN`
  - `EVIDRA_ARGO_API_TOKEN` (collector enabled by default)
- Production split DB settings via `Secret/evidra-secrets`

Required (one JWT verifier source if direct JWT mode is used):
- `EVIDRA_AUTH_JWT_HS256_SECRET`
- or `EVIDRA_AUTH_JWT_RS256_PUBLIC_KEY_PEM`
- or `EVIDRA_AUTH_JWT_JWKS_URL`

Recommended hardening:
- `EVIDRA_AUTH_RATE_LIMIT_ENABLED=true`
- `EVIDRA_AUTH_AUDIT_LOG_FILE=/var/log/evidra-auth.log`
- `EVIDRA_DB_SSLMODE=verify-full`

## Validation Checklist

- Start from `.env.example`.
- Run `go test ./...` before release images.
- Run `bash scripts/mvp-local-check.sh` for local smoke.
- Run `bash scripts/smoke-k8s-trial.sh` after trial apply.
