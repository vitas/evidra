# Behavioral Bugfixes Log

Tracks fixes that changed scoring, signal detection, or output semantics.
Each entry documents the broken behavior, the fix, and affected versions.

---

## BUG-001: Migration files not found in Docker image

**Fixed in:** v0.4.1
**Package:** `internal/db`
**Symptom:** `evidra-api` crash loop: `db.Connect: migrate: run migrations: first migrations: file does not exist`
**Root cause:** Migration files named `001_tenants_and_keys.sql` instead of `001_tenants_and_keys.up.sql`. The `golang-migrate` `iofs` driver requires the `.up.sql` suffix to identify migration direction.
**Fix:** Renamed all migration files from `*.sql` to `*.up.sql`.
**Impact:** API server could not start with `DATABASE_URL` configured.

---

## BUG-002: `new_scope` signal penalizes cold start

**Fixed in:** v0.4.1
**Package:** `internal/signal`
**Symptom:** First-ever operation in a fresh evidence chain triggers `new_scope` signal with a 0.05 penalty, producing score 95 instead of 100.
**Root cause:** `DetectNewScope` flagged every unique `(actor, tool, operation_class, scope_class)` combination, including the very first prescription. There is no "prior scope" to compare against on cold start.
**Fix:** The first prescription in the evidence chain establishes the baseline scope and is never flagged. Only subsequent prescriptions introducing unseen combinations fire the signal.
**Impact:** All new deployments started with a penalty. Score 95 on first run instead of 100.

---

## BUG-003: `evidra run` reports `"ok": true` on failure

**Fixed in:** v0.4.1
**Package:** `cmd/evidra` (`run.go`, `record.go`)
**Symptom:** When the wrapped command exits non-zero (e.g., `terraform apply` fails), the JSON output still contains `"ok": true`. The `exit_code` and `verdict` fields were correct, but `ok` was misleading.
**Root cause:** `"ok": true` was hardcoded in both `run.go` and `record.go`.
**Fix:** Changed to `"ok": exitCode == 0`.
**Impact:** Consumers checking the `ok` field could not distinguish success from failure.
