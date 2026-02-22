# UI E2E Testing (Playwright)

This suite validates the investigation flow in the Argo CD extension:

- search changes
- select change
- inspect timeline and payload
- export evidence
- verify responsive containment

## Prerequisites

- Docker
- kind
- kubectl
- Node.js 20+
- npm

## Local Run

1. Install UI dependencies and Playwright browser:

```bash
npm --prefix ui ci
npm --prefix ui run e2e:install
```

2. Start sandbox services:

```bash
make evidra-ui-e2e-up
```

3. Get Argo CD password:

```bash
ARGO_PASSWORD="$(kubectl -n argocd get secret argocd-initial-admin-secret -o jsonpath='{.data.password}' | base64 -d)"
```

4. Run tests:

```bash
ARGO_BASE_URL=https://localhost:8081 \
ARGO_USERNAME=admin \
ARGO_PASSWORD="$ARGO_PASSWORD" \
npm --prefix ui run e2e
```

Optional headed mode:

```bash
ARGO_BASE_URL=https://localhost:8081 \
ARGO_USERNAME=admin \
ARGO_PASSWORD="$ARGO_PASSWORD" \
npm --prefix ui run e2e:headed
```

5. Tear down:

```bash
make evidra-ui-e2e-down
```

## Environment Variables

- `ARGO_BASE_URL` (default: `https://localhost:8081`) — Argo CD base URL for login
- `ARGO_USERNAME` (default: `admin`)
- `ARGO_PASSWORD` (required for Argo CD login)
- `EVIDRA_TAB_NAME` (default: `Evidra`) — name of the Evidra tab in the Argo CD extension
- `EVIDRA_EXTENSION_PATH` (default: `/evidra-evidence`) — URL path for the Argo CD extension
- `EVIDRA_STANDALONE_URL` (default: `http://localhost:8080/ui/`) — Evidra standalone UI URL; used automatically when Argo CD login fails
- `E2E_TIMEOUT_SECONDS` (default: `120`) — per-test timeout

> **Standalone mode**: if Argo CD login fails, the test suite falls back to hitting `EVIDRA_STANDALONE_URL` directly. This allows running e2e tests against a plain `docker compose` stack without a Kind cluster.

## Scenarios

- `S1` happy path list/select/timeline/payload
- `S2` search by external correlation key + permalink open
- `S3` empty-state clarity
- `S4` export completion
- `S5` responsive containment (no horizontal scroll, no panel overlap)
- `S6` root-cause banner summary and actions
- `S7` selected change header primary/secondary layout
- `S8` subject selector structured display (app + namespace/cluster subtitle, in-cluster mapping)

Additional trust-anchor assertions:

- Root Cause Banner renders immediate what/why summary.
- Banner correlators and permalink/export actions are usable.
- Selected Change header keeps primary data visible and secondary data expandable.
- Raw evidence stays collapsed by default and opens on explicit action.

Use case mapping:

- `docs/testing/ui-use-cases.md` maps `UC-*` definitions to Playwright tests.
- `bash scripts/check-ui-use-cases.sh` validates mapping consistency.

## Failure Diagnostics

When a test fails:

- screenshots and videos are captured in `ui/test-results/`
- traces are available on retry
- HTML report is generated in `ui/playwright-report/`

Open report:

```bash
npx playwright show-report ui/playwright-report
```

## Troubleshooting

- Extension tab not visible: rerun `make evidra-ui-refresh`.
- Login fails: verify `ARGO_PASSWORD` from `argocd-initial-admin-secret`.
- Argo not reachable: check port-forward with `bash scripts/argocd-port-forward.sh status`.
- Empty UI data: run `make evidra-demo-test` to generate deterministic demo Changes.
- Playwright Chromium fails on macOS with `MachPortRendezvous ... Permission denied (1100)`:
  - Run E2E from a normal local terminal session, not from a restricted/sandboxed runner.
  - Reinstall Playwright browsers:
    ```bash
    rm -rf ~/Library/Caches/ms-playwright
    npm --prefix ui run e2e:install
    ```
  - Retry E2E:
    ```bash
    make evidra-ui-e2e-up
    ARGO_PASSWORD="$(kubectl -n argocd get secret argocd-initial-admin-secret -o jsonpath='{.data.password}' | base64 -d)" npm --prefix ui run e2e
    ```
  - If the error persists, switch Playwright to system Chrome (`channel: "chrome"` in `ui/playwright.config.ts`) for local runs.
