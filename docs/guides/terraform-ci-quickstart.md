# Terraform CI Quickstart

- Status: Guide
- Version: current
- Canonical for: Terraform CI integration quickstart
- Audience: public

Get Evidra measuring your Terraform operations in 10 minutes.

## Prerequisites

- `evidra` binary installed (`brew install samebits/tap/evidra` or `make build`)
- Terraform 1.x with JSON plan output support

## 1. Local Quickstart (5 minutes)

### Generate a signing key

```bash
evidra keygen
export EVIDRA_SIGNING_KEY=<base64 private key from output>
```

For local testing you can skip signing:

```bash
export EVIDRA_SIGNING_MODE=optional
```

### Wrap a terraform apply

Evidra needs the JSON plan as the artifact. Generate it first, then wrap the apply:

```bash
# Create the JSON plan
terraform plan -out=tfplan
terraform show -json tfplan > plan.json
```

Use the compact form for the fast path:

```bash
evidra record -f plan.json -- terraform apply -auto-approve tfplan
```

Use the expanded form when you want extra metadata in the evidence:

```bash
evidra record \
  -f plan.json \
  --environment staging \
  -- terraform apply -auto-approve tfplan
```

Output includes:

```json
{
  "ok": true,
  "effective_risk": "medium",
  "risk_inputs": [
    {
      "source": "evidra/native",
      "risk_level": "medium"
    }
  ],
  "score": 95,
  "score_band": "good",
  "basis": "preview",
  "confidence": 70,
  "verdict": "success"
}
```

### Understand first-run signals

The first prescription establishes the baseline scope and is never penalized.
`new_scope` only fires on a subsequent operation that introduces a previously
unseen `(actor, tool, operation_class, scope_class)` combination. See the
[Signal Specification](../system-design/EVIDRA_SIGNAL_SPEC_V1.md) for the exact
signal rules.

The score starts in `preview` mode until you reach 100 operations (configurable with `--min-operations`). To see meaningful scores earlier during evaluation:

```bash
evidra scorecard --min-operations 5
```

### View the scorecard

```bash
evidra scorecard
evidra explain
```

## 2. GitHub Actions CI

### Store the signing key

Add `EVIDRA_SIGNING_KEY` to your repository secrets (Settings > Secrets > Actions).

Generate it locally with `evidra keygen` and copy the base64 private key.

### Workflow using `evidra record`

This is the recommended path. Evidra wraps your terraform command and records the full lifecycle automatically.

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

      - name: Setup Terraform
        uses: hashicorp/setup-terraform@v3

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

      - name: Evidra Scorecard
        if: always()
        run: evidra scorecard --min-operations 5
```

### Workflow using `evidra import` (post-hoc)

Use `import` when you want to keep your existing pipeline unchanged and only add Evidra as an observer after the fact.

```yaml
      - name: Terraform Apply
        id: apply
        run: |
          START=$(date +%s%3N)
          terraform apply -auto-approve tfplan
          echo "exit_code=$?" >> "$GITHUB_OUTPUT"
          END=$(date +%s%3N)
          echo "duration_ms=$((END - START))" >> "$GITHUB_OUTPUT"
        continue-on-error: true

      - name: Record in Evidra
        run: |
          PLAN=$(cat plan.json | jq -c .)
          cat > record.json <<EOF
          {
            "contract_version": "v1",
            "session_id": "${{ github.run_id }}",
            "operation_id": "terraform-apply",
            "tool": "terraform",
            "operation": "apply",
            "environment": "${{ github.ref == 'refs/heads/main' && 'production' || 'staging' }}",
            "actor": {"type": "ci", "id": "gha-${{ github.repository }}", "provenance": "cli"},
            "exit_code": ${{ steps.apply.outputs.exit_code || 1 }},
            "duration_ms": ${{ steps.apply.outputs.duration_ms || 0 }},
            "raw_artifact": $PLAN
          }
          EOF
          evidra import --input record.json
```

Note: `raw_artifact` must be the JSON plan output from `terraform show -json`, not a text string like `"terraform apply"`. Without the plan JSON, Evidra falls back to the generic adapter and cannot extract resource-level risk information.

## 3. Terraform Destroy

Minimal form:

```bash
terraform plan -destroy -out=tfplan
terraform show -json tfplan > plan.json

evidra record -f plan.json --tool terraform --operation destroy -- terraform apply -auto-approve tfplan
```

Expanded form with explicit environment metadata:

```bash
terraform plan -destroy -out=tfplan
terraform show -json tfplan > plan.json

evidra record \
  -f plan.json \
  --tool terraform \
  --operation destroy \
  --environment staging \
  -- terraform apply -auto-approve tfplan
```

Destroy operations are classified as `risk_level: critical` automatically.

## 4. What Evidra Measures

For Terraform operations, Evidra extracts:

| Field | Source |
|---|---|
| Resources and types | `resource_changes` from plan JSON |
| Operation class | `mutate` (apply), `destroy`, or `plan` |
| Risk level | operation class x environment (production destroy = critical) |
| Artifact digest | SHA256 of the plan JSON |
| Resource shape hash | Deterministic hash of resource structure |

Behavioral signals tracked over time:

| Signal | What it detects |
|---|---|
| `protocol_violation` | Operations without prescribe/report pairs |
| `artifact_drift` | Plan changed between prescribe and apply |
| `retry_loop` | Same operation retried multiple times |
| `blast_radius` | Unusually large resource counts |
| `new_scope` | First operation in a new environment |
| `repair_loop` | Repeated fix attempts after failures |
| `thrashing` | Rapid create/delete cycles |

## Next Steps

- [Scanner SARIF Quickstart](../integrations/scanner-sarif-quickstart.md) — add Trivy/Checkov findings to evidence
- [Signal Spec](../system-design/EVIDRA_SIGNAL_SPEC_V1.md) — detailed signal definitions
- [CLI Reference](../integrations/cli-reference.md) — all flags and commands
