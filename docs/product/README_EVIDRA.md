# Evidra

**Reliability inspector for infrastructure automation and AI agents**

Evidra records infrastructure automation activity and produces reliability signals and evidence.

Works with:

- AI DevOps agents
- CI pipelines
- Terraform workflows
- Kubernetes manifests

---

## Evidence

Every operation generates an evidence record.

Evidence can be:

- local JSONL chain
- hosted Postgres store
- Ed25519 signed

---

## Signals

Evidra computes automation behavior signals:

- Protocol Violations
- Artifact Drift
- Retry Loops
- Blast Radius
- New Scope

Signals are aggregated into reliability scorecards.

---

## Example

terraform plan
terraform show -json > plan.json

evidra prescribe plan.json
terraform apply
evidra report

Generate scorecard:

evidra scorecard
