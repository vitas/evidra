# Quickstart

## Demo modes

- `make evidra-demo`: start interactive sandbox (kind + Gitea + Argo CD + Evidra).
- `make evidra-demo-test`: run deterministic case suite against running sandbox.
- `make evidra-demo-all`: run sandbox setup and case suite in one command.
- `make evidra-demo-clean`: stop and remove demo resources.

## Prerequisites

| Tool | Required for | Mandatory |
| ---- | ------------ | --------- |
| make | demo commands | yes |
| Docker | local Evidra runtime | yes |
| kind | in-cluster sandbox | yes |
| kubectl | cluster operations | yes |
| git | case suite commit flow | yes |
| jq | token/bootstrap helpers | yes |

## Interactive sandbox

```bash
cp .env.example .env
make evidra-demo
```

After startup:
- Argo CD UI: `https://localhost:8081`
- Evidra UI: `http://localhost:8080/ui/`
- Port-forward helper: `bash scripts/argocd-port-forward.sh start`

The sandbox stays running for manual exploration:
- commit directly to in-cluster Gitea repo
- sync from Argo CD
- inspect resulting Changes in Evidra

## Structured case suite

Run against a running sandbox:

```bash
make evidra-demo-test
```

The suite validates:
- Case01: successful deploy
- Case02: correlated deploy (`CHG777000`, `OPS-900`)
- Case03: controlled failed/degraded deploy
- Final: Evidra reports 3+ Changes with failed/degraded and correlated entries

## One-command run

```bash
make evidra-demo-all
```

## Cleanup

```bash
make evidra-demo-clean
```

## Optional: UI E2E

```bash
make evidra-ui-e2e-up
npm --prefix ui run e2e
make evidra-ui-e2e-down
```

## Operations (DevOps minimum)

For real Argo CD environment evaluation guidance (permissions, token rotation, polling load, observability, troubleshooting), see:

- `docs/setup/ops-minimum.md`
