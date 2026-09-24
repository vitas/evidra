# Operation Review Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Render an actionable Markdown review for one recorded Evidra operation.

**Architecture:** Add a renderer to `pkg/report` that selects exactly one operation from a reconciled summary and keeps claims, observations, and trust status separate. Extend the existing `summarize` command with `--operation` so normal summary behavior remains intact.

**Tech Stack:** Go, existing Evidence v2 and reconciliation packages, Go tests, golangci-lint.

---

### Task 1: Define the review contract in tests

**Files:** `pkg/report/operation_review_test.go`, `cmd/evidra/vnext_test.go`

1. Write failing tests for one-operation selection, rendered sections, safe handling of agent text, missing and duplicate IDs, and command output.
2. Run `go test ./pkg/report ./cmd/evidra -count=1` and confirm failures are caused by the missing review feature.

### Task 2: Render a source-aware review

**Files:** `pkg/report/operation_review.go`

1. Implement exact operation selection and Markdown output from existing summary fields.
2. Render observed calls as MCP boundary events; preserve the agent's claims as claims.
3. Include coverage and validity status plus a concrete next check.
4. Run the focused tests until green.

### Task 3: Add CLI access and docs

**Files:** `cmd/evidra/read_side.go`, `docs/cli-reference.md`, `README.md`

1. Parse `--operation` only for `summarize`; reject `--since` with it.
2. Print the review to stdout and avoid writing `summary.json` in operation mode.
3. Document the command and source boundaries, then run CLI tests.

### Task 4: Verify

1. Run `gofmt` on changed Go files.
2. Run `go test ./... -count=1` and `bash tests/run_guards.sh`.
3. Run `make lint` as the final release-hygiene gate.
