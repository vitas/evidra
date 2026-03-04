# Evidra Integration Roadmap

## Status
Active. These integrations build adoption surface before the
first paying customer. Each integration is a published package
that lets users discover Evidra through their existing tools.

## Principle
Integrations are the moat. Code that sits in a repo without
distribution is invisible. Every integration is a published,
discoverable package in the ecosystem where users already live.

---

## Security Scanner Integration Strategy

### Principle

Evidra does NOT validate infrastructure. Checkov, Trivy, tfsec,
Kics already do this better than we ever will. Instead of building
detectors that duplicate their work, **Evidra consumes their output
as risk context**.

This gives us:
- Hundreds of security rules without writing any
- Instant credibility ("works with Checkov/Trivy")
- Faster time to market (no validation code to maintain)
- Stronger signals (their context + our behavioral telemetry)

### How it works

```
terraform plan → tfplan.json
                     │
         ┌───────────┼───────────┐
         ▼           ▼           ▼
     checkov      trivy       tfsec
         │           │           │
         └─────┬─────┘───────────┘
               ▼
         scanner_report.json
               │
               ▼
    evidra prescribe \
      --tool terraform \
      --artifact tfplan.json \
      --scanner-report scanner_report.json
```

Evidra receives the scanner report alongside the raw artifact.
Scanner findings become risk_tags on the prescription — not
Evidra's own detectors, but imported context.

### Prescription with scanner enrichment

```json
{
  "prescription_id": "prs-01HX...",
  "risk_level": "high",
  "risk_tags": [
    "checkov:CKV_AWS_18:S3 bucket logging disabled",
    "checkov:CKV_AWS_19:S3 bucket encryption disabled",
    "trivy:AVD-AWS-0086:S3 bucket public access"
  ],
  "risk_details": [
    "3 security findings from external scanners",
    "2 from Checkov, 1 from Trivy"
  ],
  "canonical_action": { ... },
  "artifact_digest": "sha256:..."
}
```

Risk tags are prefixed with scanner name. Evidra doesn't interpret
them — it records and includes them in evidence.

### What changes in risk_level

Scanner findings elevate risk_level:

```
risk_level = max(
    matrix_risk(operation_class, scope_class),
    scanner_risk(findings_count, severity)
)
```

Simple rule:
- Any critical/high finding → risk_level = high
- Any medium finding → risk_level = max(current, medium)
- Low/info findings → informational, don't change risk_level

### Scanner report format

Evidra accepts scanner output in SARIF (Static Analysis Results
Interchange Format). Checkov, Trivy, tfsec all export SARIF.

```bash
# Checkov
checkov -d . --output sarif > scanner_report.sarif

# Trivy
trivy config . --format sarif > scanner_report.sarif

# tfsec
tfsec . --format sarif > scanner_report.sarif

# Then prescribe with scanner context
evidra prescribe \
  --tool terraform \
  --artifact tfplan.json \
  --scanner-report scanner_report.sarif
```

One flag. Any SARIF-compatible scanner. No custom integration per
scanner.

### What Evidra adds that scanners don't

Scanners tell you: "this resource has a misconfiguration."
Evidra tells you: "this agent keeps deploying misconfigured
resources, ignoring scanner warnings, and retrying after failures."

| Scanner (static) | Evidra (behavioral) |
|------------------|---------------------|
| S3 bucket is public | Agent deployed public S3 3 times this week |
| IAM policy too broad | Agent ignored high-risk prescription and applied anyway |
| Missing encryption | Same misconfiguration drifted between prescribe and apply |
| Pod is privileged | Agent is in retry loop deploying privileged pods |

Scanners are point-in-time. Evidra is longitudinal. The combination
is more powerful than either alone.

### New signal potential: risk_ignorance

When an agent receives a prescription with scanner findings
(risk_level: high) and proceeds without modification → this is
**risk_ignorance**. The agent saw the risk and did it anyway.

This is not a v0.3.0 signal (only 5 signals). But scanner
integration makes it possible in v0.4.0+:

```
risk_ignorance_rate = high_risk_prescriptions_executed_unchanged / high_risk_prescriptions_total
```

This is stronger than any scanner alone: it measures the agent's
**decision quality**, not just the infrastructure's state.

### Delivery

| Integration | Effort | Version |
|------------|--------|---------|
| `--scanner-report` flag accepting SARIF | 1-2 days | v0.3.0 |
| SARIF parser (extract findings, severity) | 1 day | v0.3.0 |
| Scanner findings → risk_tags mapping | half day | v0.3.0 |
| Scanner findings → risk_level elevation | half day | v0.3.0 |
| GitHub Action example with Checkov + Evidra | 1 day | v0.3.0 |
| GitHub Action example with Trivy + Evidra | 1 day | v0.3.0 |
| risk_ignorance signal | 2 days | v0.4.0 |

Total for v0.3.0: ~5 days. Zero custom scanner code — just a
SARIF parser.

---


---

## Tool & Scanner Priority Matrix

Priorities based on real market adoption, not wishful thinking.

### Infrastructure Tools (by adoption)

| Rank | Tool | Market position | Evidra integration | Version |
|------|------|----------------|-------------------|---------|
| 1 | **Terraform / OpenTofu** | Dominant. Every IaC team uses it. ~80% market. | Built-in adapter (plan JSON) | v0.3.0 |
| 2 | **Kubernetes (kubectl)** | Dominant for container orchestration. | Built-in adapter (YAML manifests) | v0.3.0 |
| 3 | **Helm** | Standard K8s package manager. Most K8s teams. | Via K8s adapter (helm template output) | v0.3.0 |
| 4 | **Ansible** | Huge for config management. Red Hat backed. | Pre-canonicalized prescribe | v0.3.0 (ready) |
| 5 | **Pulumi** | Fastest growing alternative to TF. | Pre-canonicalized prescribe | v0.3.0 (ready) |
| 6 | **AWS CloudFormation** | AWS-only but massive AWS user base. | Pre-canonicalized prescribe | v0.3.0 (ready) |
| 7 | **ArgoCD** | GitOps standard for K8s. Growing fast. | Built-in adapter | v0.5.0 |
| 8 | **Crossplane** | K8s-native IaC. Niche but growing. | Pre-canonicalized prescribe | v0.4.0 |
| 9 | **AWS CDK** | Dev-friendly CloudFormation. | Pre-canonicalized prescribe | v0.4.0 |
| 10 | **Flux** | GitOps alternative to ArgoCD. | Pre-canonicalized prescribe | v0.5.0 |

**Key insight:** top 3 (Terraform, kubectl, Helm) ship with built-in
adapters in v0.3.0. Items 4-6 work DAY ONE via pre-canonicalized
prescribe — no adapter code needed. We cover 80%+ of the market
at launch.

### Security Scanners (by adoption)

| Rank | Scanner | Market position | SARIF support | Evidra priority |
|------|---------|----------------|---------------|-----------------|
| 1 | **Checkov** | Most popular IaC scanner. 1000+ policies. Palo Alto backed. | Yes | v0.3.0 (day one) |
| 2 | **Trivy** | All-in-one scanner. Absorbed tfsec. Aqua backed. K8s + TF + containers. | Yes | v0.3.0 (day one) |
| 3 | **tfsec** | Terraform-focused. Now part of Trivy. Still widely used standalone. | Yes | v0.3.0 (via Trivy) |
| 4 | **KICS** | Checkmarx. Broad IaC support. 1900+ queries. | Yes | v0.3.0 (SARIF) |
| 5 | **Terrascan** | Tenable. Rego-based custom policies. | Yes | v0.3.0 (SARIF) |
| 6 | **Snyk IaC** | Commercial but popular. Developer-friendly. | Yes | v0.3.0 (SARIF) |
| 7 | **Semgrep** | Generic code analysis with IaC rules. | Yes | v0.4.0 |
| 8 | **Kubescape** | K8s-specific security. NSA/CISA hardening. | Partial | v0.4.0 |
| 9 | **Prowler** | AWS-specific. CIS benchmarks. | JSON | v0.5.0 |

**Key insight:** ALL top scanners support SARIF. One SARIF parser
covers 90%+ of the scanner market. We don't build per-scanner
integrations — we build one `--scanner-report` flag.

### CI/CD Platforms (by adoption)

| Rank | Platform | Market position | Integration type | Version |
|------|----------|----------------|-----------------|---------|
| 1 | **GitHub Actions** | Dominant CI/CD. >80M developers. | GitHub Action on Marketplace | v0.3.0 |
| 2 | **GitLab CI** | Second largest. Strong enterprise. | CI template in repo | v0.3.0 |
| 3 | **Jenkins** | Legacy but massive installed base. | Shell steps (just CLI) | v0.3.0 (CLI works) |
| 4 | **Azure DevOps** | Microsoft enterprise shops. | Shell steps (CLI) | v0.3.0 (CLI works) |
| 5 | **CircleCI** | Popular with startups. | Orb (wrapper) | v0.4.0 |
| 6 | **Spacelift / env0** | IaC-specific platforms. | Custom integration | v0.5.0 |

**Key insight:** GitHub Action + GitLab template covers top 2.
Jenkins and Azure DevOps work with bare CLI — no wrapper needed.

### Agent Frameworks (by adoption)

| Rank | Framework | Language | Integration type | Version |
|------|-----------|---------|-----------------|---------|
| 1 | **Claude Code** | TS | MCP native (evidra-mcp) | v0.3.0 |
| 2 | **Cursor / Windsurf** | TS | MCP native (evidra-mcp) | v0.3.0 |
| 3 | **LangChain** | Python | Python SDK | v0.4.0 |
| 4 | **CrewAI** | Python | Python SDK | v0.4.0 |
| 5 | **Vercel AI SDK** | TS | TypeScript SDK | v0.4.0 |
| 6 | **AutoGen** | Python | Python SDK | v0.4.0 |

**Key insight:** MCP-native agents (Claude Code, Cursor, Windsurf)
get evidra-mcp for free at v0.3.0. Python/TS SDKs extend reach
to all other frameworks at v0.4.0.

---

## Delivery Plan (by version)

### v0.3.0 — Launch (cover 80% of market)

**Built-in adapters** (Evidra parses artifacts):
1. Terraform / OpenTofu — plan JSON adapter
2. Kubernetes (kubectl) — YAML manifest adapter
3. Helm — via K8s adapter (helm template output)

**Pre-canonicalized** (tools send own identity, works day one):
4. Ansible, Pulumi, CloudFormation, custom tools — no code needed

**Scanner integration** (one feature, all scanners):
5. `--scanner-report` flag accepting SARIF
6. SARIF parser (extract findings + severity)
7. Scanner findings → risk_tags + risk_level elevation
8. GH Action example: Checkov + Evidra pipeline
9. GH Action example: Trivy + Evidra pipeline

**CI/CD distribution:**
10. GitHub Action on Marketplace (`evidra-io/setup-evidra`)
11. GitLab CI template in repo
12. Docker images on GHCR (CLI + MCP, multi-arch)
13. MCP registry entry (for Claude Code / Cursor / Windsurf)

**Agent support:**
14. evidra-mcp server (MCP-native agents get it for free)

**v0.3.0 total: 3 adapters + SARIF parser + 4 distributions + MCP server**
Days of work: ~30 days for one developer.

### v0.3.x — Distribution (no features, only reach)

15. Homebrew tap
16. curl | sh install script
17. Terraform External Data Source example
18. Blog post: "Measure your Terraform pipeline reliability in 5 minutes"
19. Blog post: "Checkov + Evidra: security findings meet behavioral telemetry"

### v0.4.0 — SDKs + More Scanners

20. Python SDK on PyPI (LangChain, CrewAI, AutoGen)
21. TypeScript SDK on npm (Vercel AI SDK, LangChain.js)
22. Grafana dashboard JSON
23. ArgoCD notification integration
24. risk_ignorance signal (agent saw scanner findings, applied anyway)
25. CircleCI Orb

### v0.5.0 — Platform

26. evidra-api backend
27. OpenTelemetry export
28. Spacelift / env0 integration
29. Slack / PagerDuty alerts
30. Hosted scorecard dashboard

---

## Scanner Integration Checklist

For every scanner we claim to support:

| Requirement | Status |
|------------|--------|
| Scanner outputs SARIF | Verified (Checkov, Trivy, tfsec, KICS, Terrascan, Snyk) |
| Evidra parses their SARIF correctly | Tested with sample output |
| Example CI pipeline with scanner + Evidra | In repo under integrations/ |
| README shows scanner + Evidra combo | Published |

We do NOT write per-scanner code. If it outputs SARIF, it works.

---

## Integration Checklist (every integration MUST have)

| Requirement | Why |
|------------|-----|
| README with copy-paste example | 30 seconds to first use |
| Published to native registry | Discoverability in user's ecosystem |
| Works without evidra-api | No server dependency for v0.3.0 |
| Works without account | Zero friction for trial |
| Version pinned | Reproducible builds |
| < 100 lines of integration code | Maintainable |

## What NOT to build

- Full Terraform provider (External Data Source is enough)
- Kubernetes operator (Evidra is CLI/sidecar, not a controller)
- VS Code extension (no value add over CLI)
- Web dashboard before v0.5.0 (scorecard CLI is sufficient)
- Custom agent protocol (MCP is the standard)
- pre-commit hook (Evidra works with operations, not files — linting is Checkov/Trivy territory)
- Per-scanner integration code (SARIF is the standard, one parser covers all)
- Chef / Puppet / SaltStack adapters (declining market, pre-canonicalized path covers them)

## Publication Surface (v0.3.0)

| Registry | Package | Priority |
|----------|---------|----------|
| GitHub Marketplace | evidra-io/setup-evidra Action | P0 |
| GHCR | evidra, evidra-mcp images | P0 |
| MCP Registry | evidra server entry | P0 |
| Homebrew | evidra-io/tap/evidra | P1 |
| GitLab CI catalog | evidra template | P1 |
| PyPI | evidra | P2 (v0.4.0) |
| npm | evidra | P2 (v0.4.0) |
| Grafana | evidra dashboard | P2 (v0.4.0) |
