# Setup Evidra GitHub Action

`setup-evidra` is a standalone install action.
It only installs the `evidra` binary and exposes its path; it does not run scoring, validation, or benchmark logic.

Action path in this repository:

```yaml
uses: samebits/evidra-benchmark/.github/actions/setup-evidra@main
```

## Inputs

| Input | Default | Description |
|---|---|---|
| `evidra-version` | `latest` | Release tag to install (for example `v0.5.0`) |
| `install-dir` | `${{ runner.temp }}/evidra-bin` | Target install directory |
| `add-to-path` | `true` | Add install directory to `PATH` |

## Outputs

| Output | Description |
|---|---|
| `evidra-path` | Absolute path to installed binary |
| `evidra-version` | Resolved release version |

## Usage

```yaml
jobs:
  reliability:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4

      - name: Setup Evidra
        id: setup-evidra
        uses: samebits/evidra-benchmark/.github/actions/setup-evidra@main
        with:
          evidra-version: latest

      - name: Terraform Plan
        run: |
          terraform init
          terraform plan -out=tfplan
          terraform show -json tfplan > plan.json

      - name: Terraform Apply (observed by Evidra)
        env:
          EVIDRA_SIGNING_MODE: optional
        run: |
          evidra run \
            --tool terraform \
            --operation apply \
            --artifact plan.json \
            --environment staging \
            --actor ci-gha \
            -- terraform apply -auto-approve tfplan
```

## Migration from benchmark composite action

If your workflow currently uses:

```yaml
uses: samebits/evidra-benchmark/.github/actions/evidra@main
```

move to:

1. `setup-evidra` for installation
2. explicit `evidra run` / `evidra record` / `evidra scorecard` steps in your workflow

This keeps install concerns separate from product workflow logic.
