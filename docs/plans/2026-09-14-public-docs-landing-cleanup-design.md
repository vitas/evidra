# Public Documentation and Landing Cleanup Design

## Status

Approved on 2026-09-14.

## Objective

Make every public Evidra surface describe the shipped OSS Core accurately and make the repository easy to understand for a first-time user or contributor. Evidra Core is positioned as an MCP execution-evidence recorder and reconciliation layer. Evidra Bench remains a separate product and may appear only as a secondary ecosystem link.

This cleanup adds no product functionality. It removes obsolete product claims, consolidates public documentation, and replaces the retained pre-vNext web application with a focused public landing page.

## Chosen Approach

Use hard consolidation rather than staged deprecation or an in-repository archive.

- Rebuild the public information architecture around the current binary and data model.
- Extract the still-correct information from historical documents.
- Delete superseded documents from the working tree after extraction; Git history remains the archive.
- Keep exactly one canonical public document for each topic.
- Do not move obsolete material to `docs/archive/`, because users, search engines, and coding agents would continue to treat it as current context.

## Canonical Product Statement

Evidra records what an MCP agent declared, what the proxy observed, and what the agent reported, then stores those claims and observations in a verifiable evidence chain.

Public surfaces must not claim that Evidra Core currently provides:

- a general-purpose DevOps MCP tool server;
- reliability scoring or automatic risk assessment;
- a hosted API, hosted dashboard, or managed storage;
- built-in kubectl, Helm, Terraform, AWS, Argo CD, webhook, or SARIF workflows;
- proof that an agent achieved its real-world objective;
- independent organizational identity when a recorder uses a local ephemeral signing key.

## Public Information Architecture

The public documentation set becomes:

```text
README.md
CONTRIBUTING.md
SECURITY.md

docs/
├── getting-started.md
├── cli-reference.md
├── architecture.md
├── evidence-format.md
└── validation.md
```

Responsibilities are deliberately non-overlapping:

- `README.md`: explain the product in two minutes and reach a working first run in five to ten minutes.
- `docs/getting-started.md`: the single supported installation and upstream-wrapping path.
- `docs/cli-reference.md`: the current command and flag surface generated or checked against the binaries.
- `docs/architecture.md`: runtime topology, component boundaries, operation lifecycle, and failure contracts.
- `docs/evidence-format.md`: event schema, provenance, hash chain, signatures, digest behavior, storage recovery, and threat boundaries.
- `docs/validation.md`: Gate A/B/C status, measurement limits, reproducibility, and the distinction between implemented mechanisms and proven user value.
- `CONTRIBUTING.md`: development prerequisites, package map, tests, lint, documentation rules, DCO, and pull-request workflow.
- `SECURITY.md`: only the current supported threat model, local key limitations, dependency reporting, and vulnerability disclosure process.

## Source Disposition

### Rewrite in place

- `README.md`
- `CONTRIBUTING.md`
- `SECURITY.md`
- `docs/ARCHITECTURE.md`, moved to `docs/architecture.md`
- `docs/integrations/cli-reference.md`, moved to `docs/cli-reference.md`

### Merge into the canonical set

- Current setup instructions from `docs/guides/mcp-setup.md` and `docs/guides/self-hosted-setup.md` go to `docs/getting-started.md`.
- Current event, storage, canonicalization, signing, and verification details go to `docs/evidence-format.md`.
- Gate A, Gate B, Gate C, experiment-harness, and implementation-status material is reduced to the facts a user or contributor needs in `docs/validation.md`.
- Relevant security limitations from historical evidence-bundle documentation go to `docs/evidence-format.md` and `SECURITY.md`.

### Delete after extraction

- Pre-vNext setup, hosted, scoring, SARIF, Terraform, Argo CD, observability, action, and skill guides.
- `docs/supported-tools.md` because Core has no fixed operational tool catalogue.
- `docs/external-evidence-bundle-v1.md` because it describes a removed contract.
- Completed or superseded proposals.
- Individual Gate task/result documents, history-scrub notes, prune bookkeeping, the long vNext design, and the vNext implementation report.
- Registry-publication guidance until the published package describes and starts the current product correctly.

The deletion pass must be evidence-driven: every file receives a `keep`, `merge`, or `delete` disposition before removal, and every retained fact receives a canonical destination.

## README Design

The README uses this order:

1. One-sentence product definition.
2. Small `agent -> Evidra -> upstream MCP server` topology.
3. Declared, observed, and reported source boundaries.
4. A tested quick start using the real current command.
5. A compact `evidra summarize` example.
6. A sober statement of what Evidra proves and does not prove.
7. Supported MCP versions and known operating limits.
8. Links to the five canonical documents.
9. Contributing, security, and license links.

The README contains no product history, stale migration narrative, hosted CTA, scorecard language, unsupported installation path, or fixed upstream-tool list.

## Landing Page Design

### User and surface

The primary visitor is an agent-infrastructure engineer, platform engineer, security engineer, or OSS evaluator. The page is a public trust surface with comfortable density. Its job is to explain the category, establish precise trust boundaries, and send the visitor to the tested quick start or source repository.

### Visual direction

- Calm, technical, neutral-first presentation.
- Typography and whitespace carry hierarchy.
- One restrained accent is used for actions and selected states.
- No decorative dashboards, unsupported metrics, noisy gradients, or fictitious customer proof.
- Dark-first is acceptable only with complete light-mode parity and readable code samples.

### Page structure

1. Header: Product, Docs, GitHub; Evidra Bench is a quiet external ecosystem link.
2. Hero: one outcome-led headline, short supporting copy, `Get started` primary CTA, and `View on GitHub` secondary CTA.
3. Runtime topology: agent, Evidra wrapper, upstream MCP server, and local evidence directory.
4. Declared / Observed / Reported: source ownership and why reconciliation matters.
5. Real reconciliation example generated from a stable fixture.
6. Trust boundaries: signatures, chain verification, coverage, local-key limitations, and no claim of external outcome truth.
7. Three-step workflow: wrap, run, verify/summarize.
8. Use cases: incident reconstruction, agent evaluation, and operational review.
9. OSS properties: local files, inspectable formats, no required hosted service.
10. FAQ and final quick-start CTA.

### UI scope

- Keep the existing React/Vite application only as the delivery shell for the public page.
- Remove `/onboarding`, `/dashboard`, and `/evidence`; they represent the deleted hosted product.
- Replace the Mermaid-heavy presentation with semantic HTML/CSS or a small static component.
- Remove hosted API-key state and unused API hooks with the deleted routes.
- Use route-level or component-level loading only if a remaining dependency justifies it; prefer deleting dependencies over optimizing unused features.
- Provide loading, copy-success, copy-failure, and no-JavaScript-readable content where relevant.

### Responsive and accessibility behavior

- Mobile-first single-column layout; split sections only when both columns retain readable width.
- Bound prose to approximately 60–72 characters and hero support text to 50–65 characters.
- CTAs wrap or stack before labels truncate.
- Preserve semantic heading order, keyboard focus, reduced-motion behavior, contrast, and readable code overflow.
- Verify at 320 px, 768 px, 1280 px, wide desktop, and 200% zoom.

## Historical and Local Planning Material

`docs/plans/` and `docs/product/` are currently ignored and contain personal planning history. They do not belong to the public documentation tree, but they still create local search and agent-context noise.

Handle them separately from tracked documentation:

- identify the current relaunch design/plan and any still-active decision record;
- move personal historical material outside the repository or remove it only after an explicit inventory review;
- keep public architectural decisions in the canonical tracked documents, not in ignored plan files;
- do not create a tracked archive of the old plans.

## Documentation Governance

- One topic, one canonical page.
- Public repository content is English.
- Every command example is executable and checked against the current binaries.
- Current-state documentation never mixes history into setup instructions.
- Historical context is one short note with a Git tag or commit link when it materially helps a contributor.
- Public capability claims must map to a test, source path, or reproducible command.
- New public documents require a clear owner and must be linked from either the README or another canonical page.

## Verification Strategy

The cleanup is complete only when:

- a documentation inventory test allows only the canonical public set;
- an internal-link checker reports no missing local targets or obsolete document links;
- a stale-language guard rejects removed commands, hosted/scoring claims, and pre-vNext banners on public surfaces;
- the README quick start succeeds against the fixture from a clean temporary directory;
- CLI reference assertions match current `--help` output;
- landing tests prove obsolete routes and copy are absent and current source-boundary copy is present;
- the landing page passes keyboard, responsive, light/dark, and 200% zoom review;
- `npm audit` has no unreviewed critical or high finding in shipped runtime dependencies;
- `make lint`, formatting checks, Go tests, race tests, repository guards, UI lint, UI tests, and UI production build pass;
- the final diff contains no unrelated product or architecture changes.

## Non-Goals

- Fixing evidence-ledger correctness defects found in the architecture review.
- Adding agentgateway or ExtMCP integration.
- Building or merging Evidra Bench into this repository.
- Restoring hosted APIs, authentication, dashboards, scoring, or risk analysis.
- Publishing a release or changing a version.
- Preserving every historical document in the working tree.
