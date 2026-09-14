# Public Documentation and Landing Cleanup Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Replace the contradictory pre-vNext documentation and hosted-product landing page with one accurate OSS Core story, five canonical public documents, and automated guards that prevent product-language drift.

**Architecture:** Treat public documentation as a tested product surface. README owns orientation and first run; five non-overlapping documents own setup, CLI, architecture, evidence/trust, and validation; the React/Vite application becomes a single static marketing/trust page with no hosted API state. Git history, not a tracked archive directory, preserves superseded material.

**Tech Stack:** Markdown, Bash repository guards, Go command-line binaries and fixture, React 19, TypeScript 5, Vite 7, Tailwind CSS 4, Vitest, Testing Library, ESLint 9.

---

## Execution Rules

- Work in a dedicated branch/worktree such as cleanup/public-docs-landing. Do not continue implementation directly on main.
- Use @superpowers:test-driven-development for every guard or UI behavior change.
- Use @ai-ui-design-skills:saas-product-ui-system for the landing implementation.
- Use @browser:control-in-app-browser for final responsive and accessibility inspection.
- Use @superpowers:verification-before-completion before declaring the cleanup complete.
- Preserve unrelated user changes.
- Make every tracked document English.
- Use apply_patch for file content changes and explicit deletions.
- Sign every commit with git commit -s.
- Do not push without explicit user approval.
- Do not bump a version, tag, publish, or remove the release refusal in this plan.

## Target Public Tree

After Task 4, the tracked Markdown files under docs/, excluding docs/plans/, must be exactly:

~~~text
docs/architecture.md
docs/cli-reference.md
docs/evidence-format.md
docs/getting-started.md
docs/validation.md
~~~

Root public documents remain:

~~~text
README.md
CONTRIBUTING.md
SECURITY.md
CHANGELOG.md
LICENSE
~~~

CHANGELOG.md remains historical. Its old relative document references are not current navigation and are excluded from the live-document link contract. Add one note near its top explaining that removed historical paths remain available through the tagged revision or Git history.

### Task 1: Lock the OSS Core Positioning and Rewrite README

**Files:**
- Modify: tests/test_supported_core_positioning.sh
- Modify: README.md

**Step 1: Replace the stale positioning assertions with the new public contract**

Keep the guard executable. Replace its content with:

~~~bash
#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT_DIR"

fail() {
  echo "FAIL: $*" >&2
  exit 1
}

require() {
  grep -Fq "$1" "$2" || fail "$2 is missing required text: $1"
}

forbid() {
  if grep -Eiq "$1" "$2"; then
    fail "$2 contains obsolete product language matching: $1"
  fi
}

require "MCP execution evidence" README.md
require "Declared" README.md
require "Observed" README.md
require "Reported" README.md
require "evidra-mcp --proxy" README.md
require "evidra summarize --dir" README.md
require "evidra verify --dir" README.md
require "docs/getting-started.md" README.md
require "docs/architecture.md" README.md
require "docs/evidence-format.md" README.md
require "docs/validation.md" README.md

forbid "DevOps MCP Server|reliability scoring|risk assessment|hosted API|scorecard|webhooks|built-in kubectl|built-in helm|built-in terraform|built-in aws" README.md
forbid "pre-vNext product|waits on Gate C|vnext/mcp-recorder" README.md

echo "PASS: test_supported_core_positioning"
~~~

**Step 2: Run the guard to prove the existing README fails**

Run:

~~~bash
bash tests/test_supported_core_positioning.sh
~~~

Expected: FAIL on obsolete positioning or missing canonical links.

**Step 3: Rewrite README as the two-minute orientation page**

Use this exact section order and product language:

~~~markdown
# Evidra

Evidra is an open-source MCP execution-evidence recorder. It wraps one upstream
MCP server and records what an agent declared, what the proxy observed, and what
the agent reported.

Agent → Evidra MCP endpoint → upstream MCP server
                ↓
        signed local evidence

## Why Evidra

- Declared — the objective and expected outcome supplied by the agent.
- Observed — tool calls and responses seen at the proxy boundary.
- Reported — the agent's final status and outcome claim.

Evidra reconciles these sources without treating an agent claim or a successful
tool response as proof that the real-world objective was achieved.

## Quick start

## Read the evidence

## What Evidra verifies

## Current scope

## Documentation

## Contributing and security
~~~

Under Quick start:

1. Require Go 1.26+.
2. Build with make build and build the fixture explicitly.
3. Show the current MCP client command:

~~~bash
make build
go build -o bin/evidra-fixture ./cmd/evidra-fixture

./bin/evidra-mcp \
  --proxy \
  --evidence-dir ./evidence \
  --server-name fixture \
  -- \
  ./bin/evidra-fixture
~~~

Explain that an MCP client normally owns this process and that docs/getting-started.md contains a complete client configuration. Do not imply that running the command alone is an interactive demo.

Under Read the evidence, use only real current commands:

~~~bash
./bin/evidra summarize --dir ./evidence
./bin/evidra verify --dir ./evidence
~~~

Under What Evidra verifies, distinguish:

- chain integrity;
- signature validity;
- evidence coverage;
- proxy observations;
- agent declarations and reports.

State explicitly:

> Evidra does not independently prove that the external system reached the agent's intended state. It records and reconciles claims and observations available at the MCP boundary.

Under Current scope, state:

- one stdio upstream per endpoint process;
- enforcement all or observe-only off;
- local evidence directories;
- supported protocol versions sourced from pkg/proxy/endpoint_compose.go;
- no hosted service required;
- no built-in operational tool catalogue.

Bench may appear once, after the Core documentation links:

> Looking for repeatable agent and gateway evaluation? Evidra Bench is a separate project.

The primary README CTA must remain Core getting started, not Bench.

**Step 4: Run the README guard**

Run:

~~~bash
bash tests/test_supported_core_positioning.sh
~~~

Expected: PASS.

**Step 5: Check current commands referenced by the README**

Run:

~~~bash
make build
go build -o bin/evidra-fixture ./cmd/evidra-fixture
./bin/evidra --help
./bin/evidra-mcp --help
~~~

Expected: both help commands exit 0 and show only the documented command surface.

**Step 6: Commit**

~~~bash
git add README.md tests/test_supported_core_positioning.sh
git commit -s -m "docs: position Evidra as the OSS evidence core"
~~~

### Task 2: Add the Canonical Documentation Inventory Guard

**Files:**
- Create: tests/test_public_docs_structure.sh
- Modify: tests/run_guards.sh only if discovery behavior needs no code change; normally no edit is required

**Step 1: Write the failing inventory and link-source guard**

Create an executable tests/test_public_docs_structure.sh:

~~~bash
#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT_DIR"

fail() {
  echo "FAIL: $*" >&2
  exit 1
}

expected="$(
  printf '%s\n' \
    docs/architecture.md \
    docs/cli-reference.md \
    docs/evidence-format.md \
    docs/getting-started.md \
    docs/validation.md
)"

actual="$(
  git ls-files docs |
    grep -E '\.md$' |
    grep -v '^docs/plans/' |
    sort
)"

if [[ "$actual" != "$expected" ]]; then
  echo "Expected live public docs:" >&2
  printf '%s\n' "$expected" >&2
  echo "Actual live public docs:" >&2
  printf '%s\n' "$actual" >&2
  fail "public documentation tree is not canonical"
fi

for path in $expected; do
  [[ -s "$path" ]] || fail "$path is missing or empty"
done

stale_refs="$(
  git grep -nE 'docs/(ARCHITECTURE|guides|integrations|proposals|system-design)/|docs/external-evidence-bundle-v1\.md|docs/supported-tools\.md' \
    -- . \
    ':!CHANGELOG.md' \
    ':!docs/plans/**' || true
)"
if [[ -n "$stale_refs" ]]; then
  printf '%s\n' "$stale_refs" >&2
  fail "tracked current-state files still reference removed documentation"
fi

echo "PASS: test_public_docs_structure"
~~~

**Step 2: Mark it executable**

Run:

~~~bash
chmod +x tests/test_public_docs_structure.sh
~~~

**Step 3: Run it to verify it fails**

Run:

~~~bash
bash tests/test_public_docs_structure.sh
~~~

Expected: FAIL and print the existing scattered documentation tree.

Do not commit the failing guard alone. Tasks 3 and 4 bring it to green.

### Task 3: Create the Five Canonical Documents

**Files:**
- Create: docs/getting-started.md
- Create: docs/cli-reference.md
- Create: docs/architecture.md
- Create: docs/evidence-format.md
- Create: docs/validation.md
- Read and extract from: all existing tracked docs/

**Step 1: Create docs/getting-started.md**

Required outline:

~~~markdown
# Getting Started

## Prerequisites
## Build from source
## Wrap the included fixture
## Configure an MCP client
## Complete one operation
## Verify and summarize
## Wrap a real upstream
## Configuration
## Troubleshooting
## Next steps
~~~

Requirements:

- Use Go 1.26+ and current make targets only.
- Explain absolute paths in MCP client JSON.
- Provide one complete configuration with arguments in this order:
  --proxy, --evidence-dir, --server-name, --, upstream.
- Explain evidra_prescribe before operational tools and evidra_report at completion.
- Explain all versus off without calling off a no-recording mode.
- State that write-side recording requires --evidence-dir; EVIDRA_EVIDENCE_DIR belongs to the read side.
- Document EVIDRA_ACTOR_ID and --actor-id.
- Explain recorder-root versus individual recorder directories.
- Use the included fixture for the reproducible first run and generic placeholders for real upstreams.
- Do not document editor-specific screenshots, package-manager releases, Docker, hosted endpoints, or removed skills.

**Step 2: Create docs/cli-reference.md**

Required outline:

~~~markdown
# CLI Reference

## evidra-mcp
### Usage
### Flags
### Exit behavior

## evidra
### summarize
### verify
### version

## Environment variables
## Trust boundary
~~~

Source every command and flag from:

- cmd/evidra-mcp/main.go
- cmd/evidra/command_registry.go
- cmd/evidra/vnext.go

Document exit codes only where current code defines them. State that the wrapped upstream command has the same execution trust as launching that command directly; Evidra is not a sandbox.

**Step 3: Create docs/architecture.md**

Keep it under roughly 250 lines. Required sections:

~~~markdown
# Architecture

## System boundary
## Components
## MCP composition
## Operation state machine
## Evidence write ordering
## Storage-failure behavior
## Concurrency boundary
## Supported and unsupported scope
## Package map
~~~

Include the runtime flow:

~~~text
client stdin/stdout
       ↕
evidra-mcp endpoint
  ├─ local: evidra_prescribe, evidra_report
  ├─ evidence: signed JSONL recorder directory
  └─ child stdio: one upstream MCP server
~~~

Do not copy roadmap sections, experiment diary, commit history, or speculative transports from the long vNext design.

**Step 4: Create docs/evidence-format.md**

Required sections:

~~~markdown
# Evidence Format and Trust Model

## Recorder directory
## Event envelope
## Event types
## Provenance
## Hash chain and canonicalization
## Signatures
## Argument and result fingerprints
## Coverage and degraded recording
## Verification
## Threat model and limitations
~~~

Derive field names from pkg/evidence/event_v2.go and store filenames from pkg/evidence/store_v2.go. Include:

- the three provenance values;
- the lifecycle, operation, execution, blocked, and degraded event families;
- JCS before SHA-256;
- Ed25519 signing;
- HMAC argument digest behavior;
- result-size bound;
- chain, signature, and coverage as separate conclusions;
- local ephemeral key limitations;
- no raw observed payload guarantee beyond what the actual schema enforces;
- no external-world outcome proof.

Do not promise crash durability beyond the implementation. Until fsync and torn-tail defects are fixed, say that each event is flushed to the process-visible file and that abrupt host failure remains a limitation.

**Step 5: Create docs/validation.md**

Required outline:

~~~markdown
# Validation Status

## What has been measured
## Gate A: protocol adoption
## Gate B: supported-profile fidelity
## Gate C: reconciliation value
## Reproduce the local checks
## What remains unproven
~~~

Carry forward only these conclusions:

- Gate A was measured, but thresholds were revised after measurement; enforcement did not fire in 80 runs, so recovery was not measurable.
- Gate B passed against the project fixture, not a real third-party operational MCP server.
- Gate C did not pass because there was no real operational upstream and no independent reader.
- The reconciliation-value hypothesis remains neither proven nor disproven.

Link conclusions to named current tests and commands, not to deleted report files. Preserve enough reproduction detail for a contributor to rerun free/local checks. Do not include a chronological experiment diary.

**Step 6: Check language, size, and overlap**

Run:

~~~bash
wc -l docs/*.md
grep -RInE 'reliability scoring|hosted API|DevOps MCP Server|pre-vNext product' docs/*.md
~~~

Expected:

- no canonical page is a multi-thousand-line design document;
- the grep returns no unsupported product claim;
- repeated paragraphs are removed rather than copied between pages.

Do not commit yet; Task 4 removes the superseded sources and repairs references.

### Task 4: Delete Superseded Docs and Repair Current References

**Files:**
- Delete: docs/ARCHITECTURE.md
- Delete: docs/external-evidence-bundle-v1.md
- Delete: docs/guides/acceptance-fixture-status.md
- Delete: docs/guides/argocd-gitops-integration.md
- Delete: docs/guides/evidence-export.md
- Delete: docs/guides/mcp-registry-publication.md
- Delete: docs/guides/mcp-setup.md
- Delete: docs/guides/observability-quickstart.md
- Delete: docs/guides/self-hosted-setup.md
- Delete: docs/guides/setup-evidra-action.md
- Delete: docs/guides/signal-validation.md
- Delete: docs/guides/skill-setup.md
- Delete: docs/guides/terraform-ci-quickstart.md
- Delete: docs/integrations/cli-reference.md
- Delete: docs/integrations/scanner-sarif-quickstart.md
- Delete: docs/proposals/0000-ext-audit-mcp-extension.md
- Delete: docs/proposals/ext-audit-system-design.md
- Delete: docs/supported-tools.md
- Delete: docs/system-design/gate-a-results.md
- Delete: docs/system-design/gate-a-tasks.md
- Delete: docs/system-design/vnext-experiment-harness.md
- Delete: docs/system-design/vnext-gate-b-results.md
- Delete: docs/system-design/vnext-gate-c-reconciliation-probe.md
- Delete: docs/system-design/vnext-history-scrub.md
- Delete: docs/system-design/vnext-implementation-report.md
- Delete: docs/system-design/vnext-mcp-recorder.md
- Delete: docs/system-design/vnext-prune-record.md
- Delete: tests/test_mcp_registry_publication_guide.sh
- Modify: scripts/check-doc-commands.sh
- Modify: tests/test_doc_trust_alignment.sh
- Modify: tests/test_module_path_refs.sh
- Modify: tests/test_module_graph_tidy.sh
- Modify: tests/test_governance_baseline.sh
- Modify: cmd/evidra/command_registry.go
- Modify: cmd/evidra-fixture/main.go
- Modify: cmd/evidra-gatea/main.go
- Modify: .github/workflows/ci-vnext.yml
- Modify: .github/workflows/release.yml
- Modify: tests/vnext-workflows.txt
- Modify: .gitignore
- Modify: CHANGELOG.md

**Step 1: Delete the superseded documentation with apply_patch**

Delete exactly the files listed above. Do not create docs/archive/ and do not delete either current file under docs/plans/.

**Step 2: Move command-document checks to the canonical path**

In scripts/check-doc-commands.sh and tests/test_doc_trust_alignment.sh, replace docs/integrations/cli-reference.md with docs/cli-reference.md.

Update trust assertions to require current statements:

- Evidra does not sandbox the wrapped command.
- Chain validity, signature validity, and coverage are separate conclusions.
- A successful upstream response is not proof of the external outcome.

Remove assertions about evidra run because that command does not exist.

**Step 3: Remove tests that preserve deleted product claims**

- Delete tests/test_mcp_registry_publication_guide.sh.
- Rewrite tests/test_public_claims.sh to scan README.md, SECURITY.md, docs/*.md, ui/src/, and ui/index.html for forbidden current-state claims.
- Keep historical CHANGELOG content out of the forbidden-language scan.
- Update tests/test_module_path_refs.sh so it checks the module path in current source/configuration rather than the deleted MCP setup guide.

The new tests/test_public_claims.sh must forbid:

~~~text
DevOps MCP Server
reliability scoring
scorecard
hosted API
X-Evidra-API-Key
EVIDRA_SIGNING_MODE
EVIDRA_SIGNING_KEY
EVIDRA_ENVIRONMENT
~~~

Allow a phrase only if a canonical document uses it in an explicit negative statement. Prefer phrasing the docs without obsolete terms so the guard stays simple.

**Step 4: Redirect source and workflow references**

- cmd/evidra/command_registry.go: replace the prune-record reference with docs/cli-reference.md.
- cmd/evidra-fixture/main.go: reference docs/validation.md.
- cmd/evidra-gatea/main.go: change persisted current plan/task-set labels to stable validation anchors or source test names. Update affected unit tests/snapshots in the same step.
- .github/workflows/release.yml: point the release refusal message at docs/validation.md. Keep the refusal.
- .github/workflows/ci-vnext.yml and tests/vnext-workflows.txt: either remove the obsolete branch-only workflow together or retitle/update it so it contains no deleted document path. Prefer removing it if main CI fully supersedes it; prove tests/vnext-workflows.txt remains complete.
- .gitignore: remove comments that require the deleted history-scrub document while preserving the ignore rules.
- CHANGELOG.md: add a short note near the top that historical paths may have been removed from the working tree and remain available in the revision that introduced them. Do not rewrite old release history.

**Step 5: Run the structure and command checks**

Run:

~~~bash
bash tests/test_public_docs_structure.sh
bash scripts/check-doc-commands.sh
bash tests/test_doc_trust_alignment.sh
bash tests/test_module_path_refs.sh
bash tests/test_public_claims.sh
~~~

Expected: PASS for all commands.

**Step 6: Run the complete guard suite**

Run:

~~~bash
bash tests/run_guards.sh
~~~

Expected: every discovered guard passes. The total may decrease by one because the obsolete registry guide guard was deleted and increase by one because the canonical-doc guard was added.

**Step 7: Commit**

Stage exact paths; do not use git add -A.

~~~bash
git add README.md CHANGELOG.md .gitignore .github scripts tests cmd docs
git commit -s -m "docs: consolidate the public documentation set"
~~~

### Task 5: Rewrite Contributor, Security, and Agent Guidance

**Files:**
- Modify: CONTRIBUTING.md
- Modify: SECURITY.md
- Modify: CLAUDE.md
- Modify: tests/test_doc_trust_alignment.sh
- Modify: tests/test_ci_workflows_resolve.sh if workflow names changed in Task 4

**Step 1: Extend the failing trust-alignment test**

Add assertions that:

- CONTRIBUTING.md lists make build, make test, make lint, go vet ./..., the race command, UI commands, and bash tests/run_guards.sh.
- CONTRIBUTING.md does not contain make e2e or make test-signals.
- SECURITY.md identifies main as the active unreleased Core line and published pre-vNext releases as legacy.
- CLAUDE.md links only to the five canonical documents and the current plan.
- CLAUDE.md says Core is an MCP execution-evidence recorder, not a scoring platform.

Run:

~~~bash
bash tests/test_doc_trust_alignment.sh
~~~

Expected: FAIL on the stale contributor/security/agent guidance.

**Step 2: Rewrite CONTRIBUTING.md**

Required sections:

~~~markdown
# Contributing to Evidra

## Project scope
## Development setup
## Repository map
## Build and test
## Documentation rules
## Pull requests
## DCO
## Reporting issues
~~~

Use only commands that exist:

~~~bash
make build
make test
make lint
go vet ./...
go test -race ./pkg/evidence/... ./pkg/proxy/... ./pkg/report/... ./cmd/evidra-fixture/... ./cmd/evidra-gatea/...
bash tests/run_guards.sh
cd ui && npm ci && npm run lint && npm test && npm run build
~~~

Explain one-topic/one-page documentation ownership and require claim changes to update their guard/test in the same commit.

**Step 3: Rewrite SECURITY.md**

State:

- main is the actively developed unreleased OSS Core line;
- pre-vNext published releases describe a different legacy product and receive no implied feature support from main;
- vulnerability reports are still accepted privately;
- local recorder keys protect chain consistency within the stated directory threat model, not organizational identity against directory replacement;
- Evidra is not a sandbox or an external-state verifier;
- raw secrets should not be placed in agent declarations/reports;
- supported cryptographic and privacy behavior lives in docs/evidence-format.md.

Remove SARIF, file-locking, scoring, hosted tenant, and removed command language.

**Step 4: Rewrite CLAUDE.md navigation**

Keep implementation invariants that still map to code, but replace section-number references to the deleted vNext design with named invariants and canonical-document links.

Top reading order:

1. docs/architecture.md
2. docs/evidence-format.md
3. docs/validation.md
4. the approved design and implementation plan under docs/plans/

Remove:

- branch history and history-scrub instructions;
- claims that ui/ is retained for future relocation;
- deleted-document paths;
- obsolete section-number citations.

Keep:

- DCO and no-push rule;
- supported package map;
- write-ordering and failure-contract rules;
- privacy/provenance rules;
- measurement discipline.

**Step 5: Run focused checks**

Run:

~~~bash
bash tests/test_doc_trust_alignment.sh
bash tests/test_ci_workflows_resolve.sh
git grep -nE 'docs/(ARCHITECTURE|guides|integrations|proposals|system-design)/' -- ':!CHANGELOG.md' ':!docs/plans/**'
~~~

Expected: both guards pass and git grep prints nothing.

**Step 6: Commit**

~~~bash
git add CONTRIBUTING.md SECURITY.md CLAUDE.md tests/test_doc_trust_alignment.sh tests/test_ci_workflows_resolve.sh
git commit -s -m "docs: align contributor and security guidance"
~~~

### Task 6: Replace the Hosted Application Shell with a Single Core Landing Route

**Files:**
- Modify: ui/test/components/App.test.tsx
- Modify: ui/src/App.tsx
- Modify: ui/src/components/Layout.tsx
- Delete: ui/src/context/AuthContext.tsx
- Delete: ui/src/hooks/useApi.ts
- Delete: ui/src/hooks/useHealthCheck.ts
- Delete: ui/src/pages/Onboarding.tsx
- Delete: ui/src/pages/Dashboard.tsx
- Delete: ui/src/pages/Evidence.tsx
- Delete: ui/src/components/MermaidDiagram.tsx

**Step 1: Replace App tests with the Core-first contract**

The App test must cover:

~~~tsx
it("leads with the OSS Core outcome", () => {
  render(<App />);
  expect(
    screen.getByRole("heading", {
      name: /Evidence for what MCP agents actually did/i,
    }),
  ).toBeInTheDocument();
});

it("makes Core getting started the primary action", () => {
  render(<App />);
  expect(
    screen.getByRole("link", { name: /Get started/i }),
  ).toHaveAttribute(
    "href",
    "https://github.com/vitas/evidra/blob/main/docs/getting-started.md",
  );
});

it("shows the three source boundaries", () => {
  render(<App />);
  expect(screen.getByRole("heading", { name: "Declared" })).toBeInTheDocument();
  expect(screen.getByRole("heading", { name: "Observed" })).toBeInTheDocument();
  expect(screen.getByRole("heading", { name: "Reported" })).toBeInTheDocument();
});

it("does not expose removed hosted-product navigation", () => {
  render(<App />);
  expect(screen.queryByText(/Dashboard Access/i)).not.toBeInTheDocument();
  expect(screen.queryByRole("link", { name: /Get API Key/i })).not.toBeInTheDocument();
  expect(screen.queryByText(/Reliability Dashboard/i)).not.toBeInTheDocument();
});

it("keeps Bench secondary", () => {
  render(<App />);
  const bench = screen.getByRole("link", { name: /Evidra Bench/i });
  expect(bench).toHaveAttribute("href", "https://github.com/vitas/evidra-bench");
  expect(bench).not.toHaveAttribute("data-primary", "true");
});
~~~

Remove Mermaid mocks and Mermaid parser tests.

**Step 2: Run the UI test to verify failure**

Run:

~~~bash
cd ui && npm test -- App.test.tsx
~~~

Expected: FAIL because the old hero, routes, and primary Bench CTA remain.

**Step 3: Simplify App.tsx**

The app has one public route, so remove BrowserRouter and AuthProvider:

~~~tsx
import { Layout } from "./components/Layout";
import { Landing } from "./pages/Landing";

export function App() {
  return (
    <Layout>
      <Landing />
    </Layout>
  );
}
~~~

**Step 4: Simplify Layout.tsx**

Implement:

- semantic header/nav/main/footer;
- anchors Product, Docs, GitHub;
- ThemeToggle;
- no API key, health check, dashboard, evidence, or onboarding state;
- a compact mobile navigation that wraps without horizontal overflow;
- footer links to GitHub, documentation, security, and secondary Evidra Bench.

The Layout component must wrap children in main with id main-content and provide a skip link before the header.

**Step 5: Delete hosted-only files**

Use apply_patch to delete exactly the hosted/auth/API/Mermaid files listed for this task. Confirm no remaining import:

~~~bash
grep -RInE 'AuthProvider|useAuth|useApi|useHealthCheck|MermaidDiagram|/dashboard|/evidence|/onboarding' ui/src ui/test || true
~~~

Expected: no output.

**Step 6: Run tests**

Run:

~~~bash
cd ui && npm test
~~~

Expected: tests may still fail on old landing copy; App shell/import failures must be gone. Complete Task 7 before committing the UI rewrite.

### Task 7: Rewrite the Landing Content and Visual System

**Files:**
- Modify: ui/src/pages/Landing.tsx
- Modify: ui/src/components/CodeBlock.tsx
- Modify: ui/test/components/CodeBlock.test.tsx
- Modify: ui/src/styles/global.css
- Modify: ui/test/components/App.test.tsx

**Step 1: Add failing tests for trust boundaries and real commands**

Add assertions for:

- the runtime sequence Agent → Evidra → Upstream MCP server;
- exact labels Declared, Observed, Reported;
- the phrase successful tool response is not proof of the external outcome;
- current commands evidra-mcp --proxy, evidra summarize --dir, and evidra verify --dir;
- use cases Incident reconstruction, Agent evaluation, Operational review;
- no Hosted, Scorecard, Signals, API key, database status, or Start with Bench CTA.

Add a CodeBlock failure test:

~~~tsx
it("reports clipboard failure without an unhandled rejection", async () => {
  const user = userEvent.setup();
  vi.mocked(navigator.clipboard.writeText).mockRejectedValueOnce(
    new Error("denied"),
  );

  render(<CodeBlock code="echo hello" />);
  await user.click(screen.getByRole("button", { name: /copy/i }));

  expect(
    await screen.findByRole("status"),
  ).toHaveTextContent(/copy failed/i);
});
~~~

Run:

~~~bash
cd ui && npm test
~~~

Expected: FAIL on missing Core content and clipboard error handling.

**Step 2: Rewrite Landing.tsx using the approved page order**

Implement these semantic sections:

~~~text
hero
runtime-topology
source-boundaries
reconciliation-example
trust-boundaries
workflow
use-cases
open-source
faq
final-cta
~~~

Use this hero:

~~~text
Eyebrow: Open-source MCP execution evidence
Headline: Evidence for what MCP agents actually did.
Support: Evidra records what an agent declared, what the proxy observed, and
what the agent reported—without confusing any one of them with external truth.
Primary CTA: Get started
Secondary CTA: View on GitHub
Trust note: Local-first. Inspectable JSONL. No hosted service required.
~~~

Use an HTML/CSS topology, not Mermaid:

~~~text
Agent
  → Evidra MCP endpoint
      → Upstream MCP server
      ↓
    Signed evidence directory
~~~

Use this source-boundary copy:

- Declared: The objective and expected outcome supplied by the agent.
- Observed: Tool calls and responses that crossed the Evidra proxy.
- Reported: The final status and outcome claimed by the agent.

The reconciliation example must be clearly labelled Example and use realistic, non-verdict language:

~~~text
Declared   Restart the deployment and confirm recovery
Observed   restart_deployment → success; get_status → ready
Reported   completed / achieved
Finding    The proxy observed the requested calls and successful responses.
           External application health was not independently verified.
~~~

Do not include invented user counts, customer logos, reliability percentages, pricing, hosted signup, or comparison claims.

**Step 3: Implement calm responsive styling**

Refactor global.css around:

- neutral light/dark tokens;
- one green accent;
- 8 px spacing rhythm;
- bounded 72rem page container;
- hero copy bounded to 60ch;
- auto-fit cards with a minimum around 16–18rem;
- consistent 12/16/20 px radii;
- visible focus rings;
- reduced-motion media query;
- code overflow without page overflow;
- no glass blur on every surface;
- no React Flow styles;
- no perpetual animation.

Retain the existing theme hook and ThemeToggle. Do not introduce a second design system or arbitrary per-section color palette.

**Step 4: Add explicit copy success/failure state**

Update CodeBlock so handleCopy is async and catches errors. Render copied or copy failed in an aria-live status element. Keep the button label stable enough for assistive technology.

**Step 5: Run UI tests and build**

Run:

~~~bash
cd ui && npm test && npm run build
~~~

Expected: all UI tests pass and the production build succeeds without Mermaid chunks.

**Step 6: Commit Tasks 6 and 7**

~~~bash
git add ui/src ui/test
git commit -s -m "feat(ui): relaunch the site around Evidra Core"
~~~

### Task 8: Align SEO, Static Assets, Dependencies, and UI Lint

**Files:**
- Modify: ui/index.html
- Modify: ui/public/robots.txt
- Modify: ui/public/sitemap.xml
- Modify: ui/public/site.webmanifest
- Delete: ui/public/openapi.yaml
- Delete: ui/public/docs/api/index.html
- Modify: ui/test/seo/IndexHtml.test.ts
- Modify: ui/test/seo/PublicSeoFiles.test.ts
- Modify: ui/package.json
- Modify: ui/package-lock.json
- Modify: ui/vite.config.ts
- Create: ui/eslint.config.js
- Modify: Makefile
- Modify: .github/workflows/ci.yml

**Step 1: Rewrite SEO tests first**

Expected metadata:

~~~text
Title: Evidra — Verifiable Evidence for MCP Operations
Description: Open-source MCP execution evidence that reconciles what an agent declared, what the proxy observed, and what the agent reported.
Canonical URL: https://evidra.cc/
Open Graph type: website
Twitter card: summary
~~~

Tests must assert:

- Core, not Bench, owns title and description.
- No bench-hosted OG image is referenced.
- JSON-LD describes one SoftwareApplication, not a hosted evidence service.
- noscript text explains Core and links to getting started and GitHub.
- sitemap contains only https://evidra.cc/.
- no onboarding/dashboard/evidence URLs remain.
- public OpenAPI and Swagger files do not exist.

Run:

~~~bash
cd ui && npm test -- IndexHtml.test.ts PublicSeoFiles.test.ts
~~~

Expected: FAIL against the Bench-first metadata and stale sitemap/API files.

**Step 2: Update index.html and public SEO files**

- Apply the exact title and description above.
- Remove benchmark keywords, Service JSON-LD, benchmark image metadata, and API documentation link.
- Keep Organization and SoftwareApplication JSON-LD.
- Rewrite noscript using the same product boundary as README.
- Reduce sitemap.xml to root only and update lastmod to the implementation date.
- Keep robots.txt pointing to that sitemap.
- Update the web manifest description and colors to the current Core landing.
- Delete public OpenAPI and Swagger UI because no hosted API exists.

**Step 3: Remove unused runtime dependencies and dev mocks**

Remove from package.json:

~~~text
@xyflow/react
mermaid
react-router
~~~

Rewrite vite.config.ts to remove mockApiPlugin and all fake /v1, /healthz, and /readyz endpoints. Keep only React, Tailwind, build, and test configuration.

Run:

~~~bash
cd ui && npm install
~~~

Expected: package-lock.json updates and removed packages disappear from npm ls.

Verify:

~~~bash
cd ui && npm ls @xyflow/react mermaid react-router
~~~

Expected: none are installed as direct dependencies.

**Step 4: Add a real UI linter**

Add current compatible versions of:

- eslint;
- @eslint/js;
- typescript-eslint;
- eslint-plugin-react-hooks;
- eslint-plugin-react-refresh;
- globals.

Create eslint.config.js using the flat config and recommended TypeScript/React Hooks rules. Ignore dist/.

Add scripts:

~~~json
"lint": "eslint . --max-warnings 0",
"typecheck": "tsc --noEmit"
~~~

Update Makefile:

- add ui-lint to .PHONY;
- make ui-build use npm ci rather than npm install;
- add ui-lint that runs npm run lint and npm run typecheck.

Update CI after npm ci:

~~~yaml
- name: Lint and type-check UI
  run: cd ui && npm run lint && npm run typecheck
~~~

**Step 5: Run static, lint, test, build, and audit checks**

Run:

~~~bash
cd ui
npm run lint
npm run typecheck
npm test
npm run build
npm audit --omit=dev
~~~

Expected:

- lint and typecheck pass;
- all tests pass;
- build succeeds;
- no runtime critical/high vulnerability remains;
- the main bundle is materially smaller than the previous approximately 800 KB build.

Run full npm audit as information. If dev-only critical/high findings remain, update within compatible ranges or record a specific follow-up with package, advisory, exposure, and reason it cannot be fixed in this cleanup. Do not silently accept it.

**Step 6: Commit**

~~~bash
git add ui Makefile .github/workflows/ci.yml
git commit -s -m "chore(ui): remove the retired hosted surface"
~~~

### Task 9: Quarantine Stale Distribution Claims

**Files:**
- Delete: server.json
- Modify: .goreleaser.yaml
- Modify: tests/test_public_claims.sh
- Create: docs/getting-started.md already created; modify only if it accidentally advertises unsupported packaging

**Step 1: Add failing assertions**

Extend tests/test_public_claims.sh:

~~~bash
[[ ! -e server.json ]] || fail "stale MCP Registry manifest must not advertise the pre-vNext image"

if grep -Fq "cd ui && npm install && npm run build" .goreleaser.yaml; then
  fail "binary release must not build an unembedded marketing site"
fi
~~~

Run:

~~~bash
bash tests/test_public_claims.sh
~~~

Expected: FAIL on server.json and the GoReleaser UI hook.

**Step 2: Remove unsupported registry metadata**

Delete server.json. The current OCI image metadata describes the removed DevOps/scoring product and cannot express a runnable upstream-wrapping configuration. Reintroducing registry metadata requires a separate packaging design and a working release path.

**Step 3: Remove the unrelated UI build hook from GoReleaser**

In .goreleaser.yaml, keep go mod tidy only if the repository release policy intentionally allows it; remove the UI build hook because released binaries neither embed nor serve ui/dist.

Do not change Dockerfiles, version numbers, tags, or the release-refusal job in this task. Record the broken Docker builder/runtime path as a follow-up issue rather than widening a documentation cleanup into packaging implementation.

**Step 4: Run the claim guard**

Run:

~~~bash
bash tests/test_public_claims.sh
~~~

Expected: PASS.

**Step 5: Commit**

~~~bash
git add server.json .goreleaser.yaml tests/test_public_claims.sh
git commit -s -m "chore: remove stale registry and release claims"
~~~

### Task 10: Review the Ignored Local Planning Corpus

**Files:**
- Keep tracked: docs/plans/2026-09-14-public-docs-landing-cleanup-design.md
- Keep tracked: docs/plans/2026-09-14-public-docs-landing-cleanup.md
- Review ignored: docs/plans/*
- Review ignored: docs/product/*
- Review ignored: docs/superpowers/*

This is a local housekeeping checkpoint, not a Git commit.

**Step 1: Produce a complete inventory without deleting anything**

Run:

~~~bash
find docs/plans docs/product docs/superpowers -type f -print | sort
~~~

Expected: the two current tracked cleanup documents plus the ignored historical corpus.

**Step 2: Identify active material**

Search ignored notes for an item that is both:

- applicable to the current OSS Core architecture; and
- not represented in README, the five canonical docs, or an issue/backlog outside this repository.

Run:

~~~bash
grep -RInE 'TODO|blocked|next step|decision|required' docs/plans docs/product docs/superpowers
~~~

Review matches manually. Do not promote old plans into public docs merely because they contain an unfinished checkbox.

**Step 3: Present the move set for confirmation**

Prepare the exact list of ignored files proposed for removal from the working directory. Pause and obtain confirmation before moving it.

Recommended recoverable destination:

~~~text
/Users/vitas/git/evidra-local-plan-archive-2026-09-14/
~~~

Do not use rm. After confirmation, move only the listed ignored files to that explicit directory and leave the two tracked current documents in docs/plans/.

**Step 4: Verify local search noise is gone**

Run:

~~~bash
find docs/plans docs/product docs/superpowers -type f -print | sort
git status --short
~~~

Expected: repository paths contain only the two current tracked plan documents; Git status is unaffected by the local moves.

### Task 11: Full Verification and Visual QA

**Files:**
- Modify only files required by a discovered verification defect

**Step 1: Run documentation and repository guards**

Run:

~~~bash
bash tests/test_public_docs_structure.sh
bash scripts/check-doc-commands.sh
bash tests/run_guards.sh
~~~

Expected: all checks pass.

**Step 2: Run the repository linter before any completion claim**

Run:

~~~bash
make lint
~~~

Expected: golangci-lint reports 0 issues.

This step is mandatory even though most changes are documentation and UI.

**Step 3: Run Go formatting, vet, tests, race, and build**

Run:

~~~bash
test -z "$(gofmt -l .)"
go vet ./...
make test
go test -race ./pkg/evidence/... ./pkg/proxy/... ./pkg/report/... ./cmd/evidra-fixture/... ./cmd/evidra-gatea/...
make build
~~~

Expected: every command exits 0.

**Step 4: Run complete UI verification from a clean install**

Run:

~~~bash
cd ui
npm ci
npm run lint
npm run typecheck
npm test
npm run build
npm audit --omit=dev
~~~

Expected: every command exits 0 and no runtime critical/high advisory is reported.

**Step 5: Inspect the production landing page in the in-app browser**

Serve ui/dist locally and inspect:

- 320 px mobile;
- 768 px tablet;
- 1280 px laptop;
- wide desktop;
- light and dark themes;
- 200% zoom;
- keyboard-only navigation;
- reduced motion;
- long command overflow;
- no JavaScript/noscript source.

Verify:

- hero meaning is clear within three seconds;
- Get started is the single primary CTA;
- Bench is visibly secondary;
- all copy matches README terminology;
- no obsolete route or API request occurs;
- focus order and focus rings are visible;
- no horizontal page scroll appears;
- code samples scroll internally when required.

Capture screenshots only in an ignored temporary directory. Do not commit generated screenshots unless the user explicitly expands scope.

**Step 6: Run final repository hygiene checks**

Run:

~~~bash
git diff --check
git status --short
git grep -nE 'docs/(ARCHITECTURE|guides|integrations|proposals|system-design)/' -- ':!CHANGELOG.md' ':!docs/plans/**'
git grep -nEi 'DevOps MCP Server|reliability scoring|hosted API|X-Evidra-API-Key' -- README.md CONTRIBUTING.md SECURITY.md docs ui
~~~

Expected:

- no whitespace errors;
- only intended files are changed;
- no current-state stale path remains;
- no obsolete product claim remains.

**Step 7: Review the complete diff**

Run:

~~~bash
git diff --stat origin/main...HEAD
git diff origin/main...HEAD -- README.md CONTRIBUTING.md SECURITY.md docs ui tests scripts .github Makefile .goreleaser.yaml server.json CLAUDE.md
~~~

Confirm:

- documentation describes only current behavior;
- removed material remains retrievable from Git history;
- no evidence-core behavior changed;
- no release/version/tag operation occurred.

**Step 8: Commit any verification-only corrections**

If verification required changes:

~~~bash
git add <exact-corrected-paths>
git commit -s -m "fix: close public relaunch verification gaps"
~~~

If no changes were required, do not create an empty commit.

## Completion Report

The final handoff must include:

- links to README, the five canonical documents, and the landing source;
- the count of deleted and retained public documents;
- the exact verification commands and outcomes;
- before/after UI dependency and production bundle sizes;
- npm audit outcome and any explicitly accepted dev-only advisory;
- whether local ignored plans were moved, and the recoverable destination;
- the remaining known packaging follow-up;
- confirmation that no push, release, tag, or version bump was performed.
