# Scanner SARIF Quickstart

- Status: Guide
- Version: current
- Canonical for: SARIF ingestion workflow
- Audience: public

Evidra ingests SARIF scanner reports as evidence entries alongside your infrastructure operations.
This lets you correlate security findings with deployment reliability signals.

Supported scanners: any tool that produces [SARIF v2.1.0](https://docs.oasis-open.org/sarif/sarif/v2.1.0/sarif-v2.1.0.html) output.
Recommended defaults:

- **Trivy** for Terraform / general IaC
- **Kubescape** for Kubernetes manifests

## Two ingestion patterns

| Pattern | When to use |
|---|---|
| `evidra import-findings` (standalone) | Scanner runs as a separate CI step, independent of apply |
| `evidra prescribe --findings` | Scanner findings bundled with prescribe in advanced flows |

Both write SARIF findings as evidence entries linked to the same session. The bundled prescribe path also folds them into the prescribe-time `risk_inputs` panel and `effective_risk`.

## Pattern 1: Standalone ingestion (recommended)

Use `import-findings` alongside `evidra record`. This is the simplest path — scan and apply are independent steps that share a session ID.

### Trivy + Terraform

```bash
# 1) Scan IaC
trivy config . --format sarif --output scanner_report.sarif

# 2) Plan
terraform plan -out=tfplan
terraform show -json tfplan > plan.json
```

Apply with the compact form for the fast path:

```bash
evidra record -f plan.json -- terraform apply -auto-approve tfplan
```

Use the expanded form when you want extra metadata:

```bash
evidra record \
  -f plan.json \
  --environment staging \
  -- terraform apply -auto-approve tfplan
```

Then ingest scanner findings into the same evidence store:

```bash
evidra import-findings \
  --sarif scanner_report.sarif \
  --artifact plan.json
```

### Kubescape + Kubernetes

```bash
# 1) Scan manifests
kubescape scan . --format sarif --output scanner_report_k8s.sarif

# 2) Apply with Evidra observing (compact)
evidra record -f manifest.yaml -- kubectl apply -f manifest.yaml

# 3) Expanded form when you want metadata
evidra record \
  -f manifest.yaml \
  --environment staging \
  -- kubectl apply -f manifest.yaml

# 4) Ingest scanner findings
evidra import-findings \
  --sarif scanner_report_k8s.sarif \
  --artifact manifest.yaml
```

## Pattern 2: Bundled with prescribe

Use `--findings` on `evidra prescribe` when you want findings linked directly to the prescription entry. This requires the manual prescribe/report flow instead of `evidra record`.

```bash
# 1) Scan
trivy config . --format sarif --output scanner_report.sarif

# 2) Plan
terraform plan -out=tfplan
terraform show -json tfplan > plan.json

# 3) Prescribe with scanner report attached — capture prescription_id
PRESCRIPTION_ID=$(evidra prescribe \
  --tool terraform \
  --operation apply \
  --artifact plan.json \
  --environment staging \
  --findings scanner_report.sarif \
  | jq -r .prescription_id)

# 4) Apply
terraform apply -auto-approve tfplan
EXIT_CODE=$?

# 5) Report outcome
evidra report \
  --prescription "$PRESCRIPTION_ID" \
  --verdict failure \
  --exit-code "$EXIT_CODE"
```

## GitHub Actions CI

```yaml
jobs:
  deploy:
    runs-on: ubuntu-latest
    env:
      EVIDRA_SIGNING_KEY: ${{ secrets.EVIDRA_SIGNING_KEY }}
      EVIDRA_EVIDENCE_DIR: ${{ runner.temp }}/evidra-evidence
    steps:
      - uses: actions/checkout@v4

      - name: Setup Evidra
        uses: samebits/evidra/.github/actions/setup-evidra@main

      - name: Trivy scan
        run: |
          trivy config . --format sarif --output scanner_report.sarif

      - name: Terraform Plan
        run: |
          terraform init
          terraform plan -out=tfplan
          terraform show -json tfplan > plan.json

      - name: Terraform Apply (observed by Evidra)
        run: |
          evidra record \
            -f plan.json \
            --environment ${{ github.ref == 'refs/heads/main' && 'production' || 'staging' }} \
            --actor ci-${{ github.repository }} \
            -- terraform apply -auto-approve tfplan

      - name: Ingest Trivy findings
        if: always()
        run: |
          evidra import-findings \
            --sarif scanner_report.sarif \
            --artifact plan.json

      - name: Scorecard
        if: always()
        run: evidra scorecard --min-operations 5
```

## GitLab CI

```yaml
deploy:
  variables:
    EVIDRA_SIGNING_MODE: optional
  script:
    - trivy config . --format sarif --output scanner_report.sarif
    - terraform plan -out=tfplan && terraform show -json tfplan > plan.json
    - evidra record -f plan.json --environment staging -- terraform apply -auto-approve tfplan
    - evidra import-findings --sarif scanner_report.sarif --artifact plan.json
    - evidra scorecard --min-operations 5
```

## Signing

- Default (`strict`): configure `EVIDRA_SIGNING_KEY` or `EVIDRA_SIGNING_KEY_PATH`
- Local testing: `export EVIDRA_SIGNING_MODE=optional` or pass `--signing-mode optional` per command

## What Evidra records from SARIF

Each SARIF result becomes a `finding` evidence entry containing:

| Field | Source |
|---|---|
| `rule_id` | SARIF `result.ruleId` |
| `severity` | SARIF `result.level` mapped to Evidra severity |
| `message` | SARIF `result.message.text` |
| `location` | SARIF `result.locations[0]` (file + line) |
| `tool_name` | SARIF `run.tool.driver.name` |
| `tool_version` | SARIF `run.tool.driver.version` (or `--tool-version` override) |
| `artifact_digest` | SHA256 of the artifact file (links findings to the operation) |

Findings are correlated with operations through `session_id` and `artifact_digest`, making them visible in `evidra scorecard` and `evidra explain` output.
