# Integration Tests

Integration tests are isolated behind a build tag.

## Scope

- Postgres container via `testcontainers-go`
- Evidra API in-process
- Legacy provider payload replay (compatibility tests retained for future extension)
- Timeline/correlation/export assertions

Test file:

- `integration/e2e_test.go`

## Run

Install dependencies (once):

```bash
go get github.com/testcontainers/testcontainers-go
go get github.com/testcontainers/testcontainers-go/modules/postgres
```

Run:

```bash
go test -tags=integration ./integration/...
```

## Notes

- Docker must be available on the test runner.
- Standard `go test ./...` does not execute integration-tagged tests.
- Auth integration coverage is executed in CI via `.github/workflows/auth-integration.yml`.
- These tests validate compatibility paths; Argo CD read-only ingestion remains the v1 primary path.
