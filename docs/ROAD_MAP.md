# Evidra Roadmap

**Behavioral reliability for infrastructure automation.**

Evidra is a new observability layer for CI/CD, IaC, and AI agents. This
roadmap is the public development path for the project: it shows what is
working today, what the next milestones are, and how the project is moving
toward broader CNCF ecosystem adoption.

## Current State

- CLI and MCP are the authoritative analytics surfaces today.
- `evidra record`, `import`, `report`, `scorecard`, and `explain` already produce
  evidence-backed behavioral assessments.
- Self-hosted supports centralized evidence ingestion, key issuance, and entry
  browsing today.
- Self-hosted `scorecard` and `explain` now reuse the same signal/scoring path
  over centralized stored evidence.
- OTLP/HTTP metrics export is already available for `record` and `import`,
  including Prometheus and OpenTelemetry-oriented guidance.
- The core contracts are defined around append-only evidence, canonicalized
  actions, behavioral signals, and scorecards rather than policy enforcement.

## Roadmap Principles

- Keep the supported path explicit: CLI and MCP first, self-hosted analytics
  only when parity is real.
- Prefer ecosystem compatibility over proprietary formats when mature
  standards already exist.
- Improve adoption with practical assets such as CI examples, dashboards,
  alerts, and clear first-run workflows.
- Grow governance, security, and contributor maturity alongside product scope.
- Use time horizons instead of speculative release promises.

## Now (0-6 Months)

### Product Maturity

- Harden the supported CLI and MCP analytics path and reduce contract drift.
- Improve first-run scorecard and `report` usability for humans and agents.
- Keep self-hosted boundaries explicit, with DB-backed `scorecard` and
  `explain` supported today and hosted `compare` still future work.
- Tighten the evidence-to-score path so versioning, docs, and outputs remain
  consistent.
- Add decision tracking so agents can record when they intentionally decline
  to act and why, making refusal evidence a first-class part of the chain.
- Build a reviewed OSS artifact corpus with exact provenance so benchmark and
  future acceptance coverage rely less on local synthetic fixtures.

### Ecosystem Integrations

- Expand and harden the existing OTLP/HTTP metrics path for observability
  backends.
- Publish concrete Prometheus and Grafana examples, alerts, and dashboard
  assets.
- Improve CI/CD onboarding with stronger GitHub Actions and pipeline
  documentation.
- Keep CloudEvents, OpenTelemetry, SARIF, and in-toto alignment visible in the
  public contract and docs.
- Vendor practical upstream examples from Kubescape, Checkov, and Kubernetes
  docs as the first shared dataset wave.

### Community, Governance, and Trust

- Improve contributor and maintainer onboarding for a first external
  contributor path.
- Tighten release discipline, compatibility notes, and changelog clarity.
- Strengthen security documentation and project maturity signals needed for
  public adoption.
- Keep supported versus experimental surfaces obvious in all public docs.

## Next (6-12 Months)

### Product Maturity

- Deliver a credible path to server-side analytics parity for self-hosted
  deployments.
- Improve centralized evidence workflows for teams operating multiple runners,
  pipelines, or agents.
- Expand supported workload examples without turning Evidra into a general
  scanner platform.
- Improve comparison, explanation, and score interpretation for team-level use.
- Add analytics for declined decisions, trigger breakdowns, and judgment
  patterns once real decision evidence exists in the field.

### Ecosystem Integrations

- Add practical compatibility adapters for CloudEvents envelopes and SARIF
  ingestion.
- Add an in-toto-compatible export path for evidence-backed reliability
  artifacts.
- Improve integration with Prometheus, Grafana, and OpenTelemetry collector
  workflows.
- Broaden onboarding assets for CI/CD systems and common platform stacks.

### Community, Governance, and Trust

- Build a clearer maintainer model and contribution workflow as usage grows.
- Publish more public examples, reference environments, and adopter stories.
- Improve project metadata expected by CNCF and enterprise adopters.
- Make it easier for external contributors to validate changes locally and in
  CI.

## Later (12+ Months)

### Product Maturity

- Mature centralized and fleet-level analytics for organizations running many
  automation actors.
- Extend reliability analysis where real user demand exists, without losing the
  product boundary around behavioral reliability.
- Improve long-term evidence packaging, portability, and compliance-oriented
  reporting.

### Ecosystem Integrations

- Deepen standards-aligned adapters where adoption proves they are useful.
- Support richer observability packaging for dashboards, alerting, and
  downstream analysis.
- Integrate more naturally with CNCF-native delivery and policy ecosystems
  while remaining an inspector rather than an enforcer.

### Community, Governance, and Trust

- Grow maintainer sustainability and project governance beyond a single-company
  footprint.
- Expand security and supply-chain maturity as the deployment surface grows.
- Build the contributor and adopter signals expected of a long-lived CNCF
  project.

## Why This Roadmap Fits CNCF Adoption

- It keeps project scope clear: Evidra is an observability layer for
  behavioral reliability, not a policy engine or a full scanner platform.
- It shows concrete ecosystem fit through Prometheus, OpenTelemetry,
  CloudEvents, SARIF, and in-toto compatibility work.
- It is honest about what is authoritative today and which hosted surfaces are
  already supported versus still future work.
- It balances product hardening with governance, security, and contributor
  maturity instead of treating those as afterthoughts.
- It gives adopters a practical path from single-team CLI usage to broader
  platform and eventually centralized analytics workflows.
