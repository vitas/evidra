# Setup Evidra GitHub Action

`setup-evidra` is a standalone install action.
It only installs the `evidra` binary and exposes its path; it does not run scoring, validation, or benchmark logic.

Action path in this repository:

```yaml
uses: samebits/evidra/.github/actions/setup-evidra@main
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
        uses: samebits/evidra/.github/actions/setup-evidra@main
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
          evidra record \
            -f plan.json \
            --environment staging \
            --actor ci-gha \
            -- terraform apply -auto-approve tfplan
```

Once `plan.json` exists, the minimal wrapped command is:

```bash
evidra record -f plan.json -- terraform apply -auto-approve tfplan
```

Use the expanded workflow form above when you want environment and actor labels recorded in CI.

## Generic CI (GitLab, Jenkins, CircleCI, etc.)

Evidra is a single static binary. Install it in any CI with curl:

```bash
VERSION="latest"
REPO="samebits/evidra"

# Resolve latest tag
if [ "$VERSION" = "latest" ]; then
  VERSION=$(curl -fsSL "https://api.github.com/repos/${REPO}/releases/latest" \
    | grep -o '"tag_name": *"[^"]*"' | head -1 | cut -d'"' -f4)
fi

# Detect platform
OS=$(uname -s | tr '[:upper:]' '[:lower:]')
ARCH=$(uname -m | sed 's/x86_64/amd64/;s/aarch64/arm64/')

# Download and extract
curl -fsSL "https://github.com/${REPO}/releases/download/${VERSION}/evidra_${VERSION#v}_${OS}_${ARCH}.tar.gz" \
  | tar -xz -C /usr/local/bin evidra
```

### GitLab CI example

```yaml
evidra-terraform:
  image: ubuntu:latest
  script:
    - apt-get update && apt-get install -y curl
    - |
      VERSION=$(curl -fsSL "https://api.github.com/repos/samebits/evidra/releases/latest" \
        | grep -o '"tag_name": *"[^"]*"' | head -1 | cut -d'"' -f4)
      OS=$(uname -s | tr '[:upper:]' '[:lower:]')
      ARCH=$(uname -m | sed 's/x86_64/amd64/;s/aarch64/arm64/')
      curl -fsSL "https://github.com/samebits/evidra/releases/download/${VERSION}/evidra_${VERSION#v}_${OS}_${ARCH}.tar.gz" \
        | tar -xz -C /usr/local/bin evidra
    - terraform plan -out=tfplan && terraform show -json tfplan > plan.json
    - evidra record -f plan.json --environment staging -- terraform apply -auto-approve tfplan
  variables:
    EVIDRA_SIGNING_MODE: optional
```
