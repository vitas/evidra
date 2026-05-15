# Evidra Core Simplification Design

**Date:** 2026-05-15
**Status:** Approved

## Goal

Make Evidra focused and easy to adopt by reducing the core product to an
intent/outcome evidence ledger.

The required product loop is:

```text
prescribe -> execute or decline -> report -> signals/scorecard
```

Evidra should know what the agent said it intended to do and what happened
afterward. Canonicalization and risk assessment should enrich that record when
available, but they must not be required to use Evidra.

## Problem

The current implementation makes canonicalization and risk assessment feel like
the center of the system:

- raw artifacts enter the core write path
- core packages know about Kubernetes, Terraform, Docker, SARIF, detector tags,
  and a static risk matrix
- `CanonicalAction` is treated as the main intent model in several lifecycle,
  ingest, MCP, and analytics paths

This creates too much adoption friction. External assessment is easy for users
to understand: run a scanner or policy check and attach the result. Mandatory
canonicalization is harder because there is no broad standard for every tool to
express intent in Evidra's canonical dialect.

The simpler product boundary is:

- Evidra records declared intent and outcome.
- External tools may attach assessment results.
- External tools may attach normalized/canonical identity.
- Evidra analytics use enrichments when present and degrade gracefully when they
  are absent.

## Design Direction

Evidra core must not import or depend on a separate assessment repository.
Assessment and canonicalization move behind a runtime JSON contract. They can be
implemented by a companion CLI/service, scanners, policy systems, gateway
integrations, or custom adapters.

### Required Core Fields

Every prescribe record should carry a declared intent that humans and analytics
can use without canonicalization:

```json
{
  "tool": "kubectl",
  "operation": "apply",
  "target": "deployment/web",
  "command": "kubectl apply -f deploy.yaml",
  "artifact_digest": "sha256:..."
}
```

The exact field names can be refined during implementation, but the model should
stay intentionally small:

- `tool`: caller-declared tool or integration name
- `operation`: caller-declared action
- `target`: optional human-readable target or scope
- `command`: optional command or tool request summary
- `artifact_digest`: optional digest of the material the agent intended to use

### Optional Enrichments

Canonical identity becomes optional:

```json
{
  "canonical_action": {
    "tool": "kubectl",
    "operation": "apply",
    "operation_class": "mutate",
    "resource_identity": [
      { "kind": "Deployment", "namespace": "prod", "name": "web" }
    ],
    "scope_class": "production",
    "resource_count": 1,
    "resource_shape_hash": "sha256:..."
  }
}
```

Assessment becomes optional:

```json
{
  "assessment": {
    "status": "provided",
    "provider": "trivy",
    "risk_inputs": [
      {
        "source": "trivy",
        "risk_level": "high",
        "risk_tags": ["trivy.AVD-KSV-0123"]
      }
    ],
    "effective_risk": "high"
  }
}
```

When assessment is absent, the prescribe entry should say that explicitly:

```json
{
  "assessment": {
    "status": "not_provided"
  }
}
```

Core must not silently derive matrix risk or detector tags. If assessment is not
provided, risk is unknown or absent.

## Architecture

```text
                         EVIDRA CORE
                         -----------

  agent/tooling
       |
       v
  prescribe declared intent
       |
       +-- optional canonical_action
       +-- optional assessment
       |
       v
  signed evidence chain
       |
       v
  report outcome or refusal
       |
       v
  signals, scorecard, explain, export


                  OPTIONAL EXTERNAL ENRICHMENT
                  ----------------------------

  scanner/policy/canonicalizer/gateway
       |
       +-- produces assessment JSON
       +-- may produce canonical_action JSON
       |
       v
  passed into Evidra prescribe or ingest
```

The external repository can still exist, but it is not required by Evidra core.
It can ship:

- canonicalizers for Kubernetes, Terraform, Docker, Helm, and future tools
- risk matrix and detector registry
- SARIF/scanner mapping
- a CLI-first interface such as `evidra-assessment assess ...`
- optional HTTP mode for hosted or gateway integrations

## Breaking Changes

Backward compatibility is not a goal for this refactor.

Remove from the core write path:

- built-in raw artifact canonicalization
- static risk matrix execution
- native detector execution
- SARIF-to-risk mapping
- generic canonicalization fallback

Core prescribe paths should reject raw artifact assessment requests that expect
Evidra to canonicalize or assess internally. Callers may still provide an
artifact digest and declared intent. Callers that want enrichment must provide
the enrichment explicitly or run an external companion first.

## Analytics Behavior

Signals should be layered by available evidence:

- Always available: prescribe/report lifecycle, missing reports, verdicts,
  declines, actor/tool/operation patterns, command or target repetition.
- Available with artifact digest: artifact reuse and drift-style checks.
- Available with canonical identity: more precise retry loops, blast radius,
  new scope, and resource-level comparisons.
- Available with assessment: risk escalation and risk-informed explanations.

This keeps Evidra useful on day one and more precise as teams attach richer
context.

## Validation

Core should validate protocol-level shape only:

- prescribe includes declared intent
- report links to a prescription when required
- optional digests use `sha256:<hex>` format
- optional `canonical_action` is well-formed enough to store and query
- optional assessment status is one of `provided`, `not_provided`, or `failed`
- optional risk levels are one of `low`, `medium`, `high`, `critical`, `unknown`

Core should not validate scanner-specific fields, policy-specific fields, or
tool-specific canonicalization rules.

## Test Strategy

Add focused regression coverage for:

- prescribe succeeds with declared intent and no canonical action
- prescribe succeeds with declared intent and no assessment
- prescribe stores optional canonical action when provided
- prescribe stores optional assessment when provided
- raw artifact paths no longer trigger built-in canonicalization or assessment
- core has no write-path imports from removed assessment packages
- signals degrade gracefully when canonical and assessment enrichment are absent
- docs and API examples show the simpler intent/outcome-first model

Before declaring the refactor complete, run the repository linter in addition to
tests.

## Non-Goals

- Do not keep an internal generic canonicalization fallback.
- Do not make the companion assessment repo a Go module dependency of Evidra
  core.
- Do not require every integration to implement `CanonicalAction`.
- Do not turn Evidra into a general-purpose scanner platform or policy engine.
