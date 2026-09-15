# Post-Relaunch Hardening Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Align the Evidra relaunch, documentation, release posture, and AgentGateway integration around one truthful, tested, and reproducible product path.

**Architecture:** Keep Core and the AgentGateway adapter in separate repositories. Core owns source-neutral evidence, trust, and reconciliation; the adapter owns AgentGateway-specific ExtMCP, OTLP, configuration, and compatibility code. Execute public-surface cleanup first, harden the adapter second, and expand Core's observer contract only if a measured capture-path spike passes explicit correlation and completeness gates.

**Tech Stack:** Go 1.26, MCP JSON-RPC, Evidence v2 JSONL, React 19, TypeScript 5, Vite 7, Tailwind CSS 4, Vitest, Testing Library, Bash guards, GitHub Actions, Docker, AgentGateway 1.5, OpenTelemetry OTLP, ExtMCP gRPC.

---

## Execution Rules

- Use dedicated worktrees for both repositories. Suggested paths:
  - `/Users/vitas/git/evidra/.worktrees/post-relaunch-hardening`
  - `/Users/vitas/git/evidra-agentgateway-bridge/.worktrees/integration-hardening`
- Before editing, read both repositories' `AGENTS.md`/`CLAUDE.md` files if present.
- Use @superpowers:test-driven-development for every behavior change.
- Use @ai-ui-design-skills:saas-product-ui-system for Tasks 1-3.
- Use @ai-ui-design-skills:image-logo-icons and @imagegen for the social card in Task 3.
- Use @browser:control-in-app-browser for the final landing inspection when an in-app browser is available.
- Use @superpowers:systematic-debugging for any failing baseline or unexpected integration behavior.
- Use @superpowers:verification-before-completion before each repository handoff.
- Sign every commit with `git commit -s`.
- Do not push, tag, publish, change GitHub repository settings, or remove the release refusal without a separate explicit human approval.
- Before any release action, use the repository helper `scripts/bump-version.sh`; do not perform a manual version edit.
- Preserve the source boundaries: an AgentGateway observation must never be written as an agent declaration or as `proxy_observed`.
- Tasks 11-13 are gated by Task 10. Stop if the spike cannot establish a non-heuristic operation context.

## Phase 0: Baselines and Worktrees

### Task 0: Create isolated worktrees and capture baselines

**Files:**
- No tracked files.

**Step 1: Confirm both worktrees are clean**

Run:

```bash
git -C /Users/vitas/git/evidra status --short --branch
git -C /Users/vitas/git/evidra-agentgateway-bridge status --short --branch
```

Expected: both report `main...origin/main` with no changed paths.

**Step 2: Create the Core worktree**

Run:

```bash
git -C /Users/vitas/git/evidra worktree add \
  /Users/vitas/git/evidra/.worktrees/post-relaunch-hardening \
  -b hardening/post-relaunch main
```

Expected: a new worktree on `hardening/post-relaunch`.

**Step 3: Create the bridge worktree**

Run:

```bash
git -C /Users/vitas/git/evidra-agentgateway-bridge worktree add \
  /Users/vitas/git/evidra-agentgateway-bridge/.worktrees/integration-hardening \
  -b hardening/agentgateway-integration main
```

Expected: a new worktree on `hardening/agentgateway-integration`.

**Step 4: Run the Core baseline**

Run from the Core worktree:

```bash
make lint
go test ./... -count=1
bash tests/run_guards.sh
cd ui
npm ci
npm run lint
npm run typecheck
npm test
npm run build
npm audit --audit-level=high
```

Expected: lint passes, all Go tests pass, 13 guards pass, 25 or more UI tests pass, production build succeeds, and npm reports no high/critical vulnerabilities.

**Step 5: Run the bridge baseline**

Run from the bridge worktree:

```bash
test -z "$(gofmt -l .)"
go vet ./...
go test ./... -count=1
go test -race ./... -count=1
```

Expected: all commands pass. Record the absence of repository CI as a known gap, not as a baseline failure.

## Phase 1: Truthful Public Surface

### Task 1: Lock truthful landing claims

**Files:**
- Modify: `ui/test/components/App.test.tsx`
- Modify: `ui/src/pages/Landing.tsx:32-247`
- Modify: `tests/test_doc_trust_alignment.sh`

**Step 1: Write failing UI assertions**

Add tests that require boundary-accurate language and reject the current overclaims:

```tsx
it("describes the MCP boundary rather than external action truth", () => {
  render(<App />);
  expect(
    screen.getByRole("heading", {
      name: /Verifiable evidence from the MCP execution boundary/i,
    }),
  ).toBeInTheDocument();
  expect(screen.queryByText(/agents actually did/i)).not.toBeInTheDocument();
});

it("does not guarantee complete recording", () => {
  render(<App />);
  expect(screen.queryByText(/guarantees is that the execution is recorded/i))
    .not.toBeInTheDocument();
  expect(screen.getByText(/unknown loss can still exist/i)).toBeInTheDocument();
});

it("attributes generated reconciliation and human judgment separately", () => {
  render(<App />);
  expect(
    screen.getByText(/Evidra generates the reconciliation summary/i),
  ).toBeInTheDocument();
  expect(screen.getByText(/a human judges the external outcome/i))
    .toBeInTheDocument();
});
```

Extend `tests/test_doc_trust_alignment.sh` to reject these exact stale phrases on public surfaces:

```bash
reject 'agents actually did'
reject 'guarantees is that the execution is recorded'
reject 'results are fingerprinted, never stored raw'
```

Use the guard's existing helper names and path scope; do not create a second scanner.

**Step 2: Run tests to verify failure**

Run:

```bash
cd ui && npm test -- App.test.tsx
bash tests/test_doc_trust_alignment.sh
```

Expected: failures identify the current hero, trust-boundary, and reconciliation copy.

**Step 3: Replace the inaccurate copy**

In `Landing.tsx`, make these semantic changes:

```tsx
<h1 className="text-[2.4rem] sm:text-[3rem] leading-tight font-bold text-fg tracking-tight">
  Verifiable evidence from the MCP execution boundary.
</h1>
```

Use this Observed description:

```tsx
desc: "Execution boundaries, status, timing, metadata, and bounded keyed fingerprints recorded by the proxy.",
```

Replace the example note with:

```tsx
Evidra generates the reconciliation summary from the recorded chain; a human
judges what it means for the external outcome.
```

Replace the trust statements with wording that preserves the evidence-format limits:

```tsx
const TRUST_BOUNDARIES = [
  "Evidra is not a sandbox: the upstream executes with its existing authority. When recording is healthy, Evidra records the MCP execution boundary; known degraded windows are reported, and unknown loss can still exist.",
  "A successful tool response is not proof of the external outcome. The chain records what crossed the proxy; external-state judgment remains a human step.",
  "Declared and reported entries remain agent claims. Their origin and position are preserved within the recorder directory's local trust boundary; they do not establish organizational identity.",
  "Raw observed arguments and response bodies are not stored. Arguments use keyed HMAC digests; results carry a bounded fingerprint or an explicit omitted or unavailable status.",
  "Chain validity, signature validity, and evidence coverage are separate conclusions. A valid chain can still be incomplete.",
];
```

Rename the workflow heading to:

```tsx
<h2 className="section-title">One operation, then verify and summarize</h2>
```

**Step 4: Run focused tests**

Run:

```bash
cd ui && npm test -- App.test.tsx
bash tests/test_doc_trust_alignment.sh
```

Expected: both pass.

**Step 5: Commit**

```bash
git add ui/src/pages/Landing.tsx ui/test/components/App.test.tsx tests/test_doc_trust_alignment.sh
git commit -s -m "fix(landing): align claims with the evidence boundary"
```

### Task 2: Repair landing accessibility and interaction semantics

**Files:**
- Modify: `ui/src/pages/Landing.tsx:73-104`
- Modify: `ui/src/components/ThemeToggle.tsx`
- Modify: `ui/src/components/Layout.tsx`
- Modify: `ui/test/components/App.test.tsx`
- Modify: `ui/test/components/ThemeToggle.test.tsx`

**Step 1: Write failing accessibility tests**

Add:

```tsx
it("exposes the complete topology to assistive technology", () => {
  render(<App />);
  const topology = screen.getByRole("img", { name: /Agent.*Evidra MCP endpoint.*Upstream MCP server.*Signed evidence directory/i });
  expect(topology).toBeInTheDocument();
});
```

Change the theme-toggle test to require an action-specific label:

```tsx
expect(screen.getByRole("button", { name: /Switch to dark theme/i }))
  .toBeInTheDocument();
```

After clicking, require `Switch to light theme`.

Add a navigation test requiring Docs to remain in the current browsing context:

```tsx
expect(screen.getByRole("link", { name: "Docs" }))
  .not.toHaveAttribute("target");
```

**Step 2: Verify the tests fail**

Run:

```bash
cd ui && npm test -- App.test.tsx ThemeToggle.test.tsx
```

Expected: topology role/name, toggle label, and Docs target assertions fail.

**Step 3: Implement semantic topology and controls**

Give the topology container one complete accessible representation:

```tsx
<div
  className="topology mt-8"
  role="img"
  aria-label="Agent connects to the Evidra MCP endpoint, which forwards calls to one upstream MCP server and writes a signed evidence directory."
>
```

Keep arrows `aria-hidden`, but remove `aria-hidden="true"` from
`.topology-branches` so meaningful text is not hidden. Because the container has
an image label, add `aria-hidden="true"` to the visual children as one group if
the accessibility tree otherwise announces both representations; assert only
one named topology in the test.

Change the toggle to:

```tsx
aria-label={theme === "dark" ? "Switch to light theme" : "Switch to dark theme"}
title={theme === "dark" ? "Switch to light theme" : "Switch to dark theme"}
```

Remove `external: true` and the `target` attributes from the Docs navigation
link. GitHub itself may remain external; documentation is the primary product
journey and should not force a new tab.

**Step 4: Run focused tests and lint**

Run:

```bash
cd ui
npm test -- App.test.tsx ThemeToggle.test.tsx
npm run lint
npm run typecheck
```

Expected: all pass.

**Step 5: Commit**

```bash
git add ui/src/pages/Landing.tsx ui/src/components/ThemeToggle.tsx ui/src/components/Layout.tsx ui/test/components/App.test.tsx ui/test/components/ThemeToggle.test.tsx
git commit -s -m "fix(landing): expose complete accessible navigation and topology"
```

### Task 3: Add a Core social card and remove false PWA surface

**Files:**
- Create: `ui/public/og/evidra-core.png`
- Modify: `ui/index.html`
- Modify: `ui/test/seo/IndexHtml.test.ts`
- Modify: `ui/test/seo/PublicSeoFiles.test.ts`
- Modify: `ui/public/sitemap.xml`
- Delete: `ui/public/site.webmanifest`

**Step 1: Write failing SEO tests**

Replace the assertion that forbids any `og:image` with:

```ts
expect(metaContent(doc, 'meta[property="og:image"]')).toBe(
  "https://evidra.cc/og/evidra-core.png",
);
expect(metaContent(doc, 'meta[name="twitter:card"]')).toBe(
  "summary_large_image",
);
expect(metaContent(doc, 'meta[name="twitter:image"]')).toBe(
  "https://evidra.cc/og/evidra-core.png",
);
```

In `PublicSeoFiles.test.ts`, assert that the PNG exists, the manifest does not,
and `index.html` has no manifest link:

```ts
expect(existsSync(resolve(process.cwd(), "public", "og", "evidra-core.png"))).toBe(true);
expect(existsSync(resolve(process.cwd(), "public", "site.webmanifest"))).toBe(false);
expect(index).not.toContain('rel="manifest"');
```

Also parse the sitemap and assert it has no hard-coded `<lastmod>`.

**Step 2: Verify tests fail**

Run:

```bash
cd ui && npm test -- IndexHtml.test.ts PublicSeoFiles.test.ts
```

Expected: missing social metadata/image and retained manifest/lastmod cause failures.

**Step 3: Generate the social card**

Use @ai-ui-design-skills:image-logo-icons to define the asset and @imagegen to
generate a 1200x630 PNG. Keep it neutral and text-light:

- off-white background matching `--color-bg`;
- `evidra.` wordmark with one green accent;
- short line: `Verifiable MCP execution evidence`;
- no gradients, screenshots, badges, fake UI, or unsupported claims;
- safe text margins of at least 80 px.

Save the final file at `ui/public/og/evidra-core.png`. Inspect it at original
resolution before accepting it.

**Step 4: Update metadata and static files**

Add to `ui/index.html`:

```html
<meta property="og:image" content="https://evidra.cc/og/evidra-core.png" />
<meta property="og:image:width" content="1200" />
<meta property="og:image:height" content="630" />
<meta property="og:image:alt" content="Evidra — verifiable MCP execution evidence" />
<meta name="twitter:card" content="summary_large_image" />
<meta name="twitter:image" content="https://evidra.cc/og/evidra-core.png" />
```

Remove the existing `summary` card declaration and the manifest link. Delete
`site.webmanifest`. Remove `<lastmod>` from the one sitemap entry; a false date
is less useful than no date.

**Step 5: Run tests and build**

Run:

```bash
cd ui
npm test -- IndexHtml.test.ts PublicSeoFiles.test.ts
npm run lint
npm run typecheck
npm run build
test -f dist/og/evidra-core.png
test ! -f dist/site.webmanifest
```

Expected: all pass.

**Step 6: Commit**

```bash
git add ui/index.html ui/public/sitemap.xml ui/test/seo/IndexHtml.test.ts ui/test/seo/PublicSeoFiles.test.ts
git add -f ui/public/og/evidra-core.png
git add -u ui/public/site.webmanifest
git commit -s -m "feat(landing): add Core social preview metadata"
```

### Task 4: Make preview release status explicit

**Files:**
- Modify: `README.md`
- Modify: `docs/getting-started.md`
- Modify: `ui/src/pages/Landing.tsx`
- Modify: `ui/test/components/App.test.tsx`
- Modify: `tests/test_supported_core_positioning.sh`

**Step 1: Write failing status guards**

Require all three entry surfaces to contain the same release-status sentence:

```text
The relaunched Core line is currently an unreleased preview built from main.
```

In the shell guard, check README, getting-started, and Landing source. In the UI
test, render the status with `screen.getByText`.

**Step 2: Verify failure**

Run:

```bash
bash tests/test_supported_core_positioning.sh
cd ui && npm test -- App.test.tsx
```

Expected: both fail because the status is absent.

**Step 3: Add the status without weakening the CTA**

- Put one short blockquote after the README introduction.
- Put one note before prerequisites in `docs/getting-started.md`.
- Replace the hero microcopy with the status followed by `Build from source; no hosted service required.`
- Do not mention an unapproved future version number.

**Step 4: Run focused and full documentation checks**

Run:

```bash
bash tests/test_supported_core_positioning.sh
bash tests/test_doc_trust_alignment.sh
cd ui && npm test -- App.test.tsx
```

Expected: all pass.

**Step 5: Commit**

```bash
git add README.md docs/getting-started.md ui/src/pages/Landing.tsx ui/test/components/App.test.tsx tests/test_supported_core_positioning.sh
git commit -s -m "docs: label the relaunched Core line as preview"
```

## Phase 2: Core Repository Hygiene

### Task 5: Keep Go tests out of UI dependencies

**Files:**
- Modify: `Makefile`
- Modify: `.github/workflows/ci.yml`
- Modify: `CONTRIBUTING.md`
- Modify: `tests/test_supported_package_graph.sh`

**Step 1: Add a failing package-scope guard**

In `tests/test_supported_package_graph.sh`, require a shared Make variable:

```bash
grep -Fq 'GO_PACKAGES := ./cmd/... ./pkg/...' Makefile \
  || fail "Makefile must scope Go checks away from ui/node_modules"
grep -Fq 'go test $(GO_PACKAGES)' Makefile \
  || fail "make test must use GO_PACKAGES"
```

Also reject bare `go test ./...` in the current CI workflow and contributor
commands after stripping comments.

**Step 2: Verify the guard fails**

Run:

```bash
bash tests/test_supported_package_graph.sh
```

Expected: failure identifies bare `./...` commands.

**Step 3: Scope commands to product packages**

Add:

```make
GO_PACKAGES := ./cmd/... ./pkg/...
```

Use it in `test`. In GitHub Actions and CONTRIBUTING, use the literal
`./cmd/... ./pkg/...` because Make variables are unavailable there. Keep the
existing narrower race list if it is intentionally smaller, or expand it only
after measuring runtime.

**Step 4: Prove UI dependencies are excluded**

Run after `ui/node_modules` exists:

```bash
make test 2>&1 | tee /tmp/evidra-make-test.out
! grep -Fq 'ui/node_modules' /tmp/evidra-make-test.out
bash tests/test_supported_package_graph.sh
```

Expected: tests pass and no third-party Go package under `node_modules` is listed.

**Step 5: Commit**

```bash
git add Makefile .github/workflows/ci.yml CONTRIBUTING.md tests/test_supported_package_graph.sh
git commit -s -m "build: scope Go checks to Core packages"
```

### Task 6: Make the Core module importable by integrations

**Files:**
- Modify: `go.mod`
- Modify: all tracked Go imports under `cmd/` and `pkg/`
- Modify: `tests/test_module_path_refs.sh`
- Modify: `README.md`
- Modify: `docs/architecture.md`

**Step 1: Write the failing module-path guard**

Change `tests/test_module_path_refs.sh` so the canonical module is:

```bash
canonical_module='github.com/vitas/evidra'
legacy_module='samebits.com/evidra'
```

Require the first `go.mod` line to equal `module $canonical_module` and reject
the legacy path outside `CHANGELOG.md` and the soon-to-be-deleted plan corpus.

**Step 2: Verify failure**

Run:

```bash
bash tests/test_module_path_refs.sh
```

Expected: failure reports `samebits.com/evidra`.

**Step 3: Migrate the module path mechanically**

Use a repository-wide mechanical replacement over tracked `.go` files and
`go.mod`; do not touch Git history or CHANGELOG entries. Set:

```go
module github.com/vitas/evidra
```

Run `gofmt` and `go mod tidy` afterward.

**Step 4: Document the integration boundary**

Add a short Architecture paragraph saying public `pkg/evidence` and
`pkg/report` packages are importable from `github.com/vitas/evidra`, while no
stability promise extends to `pkg/proxy` internals beyond documented releases.
Do not advertise the AgentGateway integration as complete.

**Step 5: Verify the migration**

Run:

```bash
bash tests/test_module_path_refs.sh
go mod tidy
git diff --exit-code go.mod go.sum
go test ./cmd/... ./pkg/... -count=1
make lint
bash tests/run_guards.sh
GOWORK=off go list ./pkg/evidence ./pkg/report
```

Expected: all pass; `go list` prints GitHub module paths.

**Step 6: Commit**

```bash
git add go.mod go.sum cmd pkg README.md docs/architecture.md tests/test_module_path_refs.sh
git commit -s -m "refactor: publish Core packages from the GitHub module path"
```

## Phase 3: Harden the Separate AgentGateway Adapter

All tasks in this phase run in the bridge worktree.

### Task 7: Add bridge build, lint, CI, and container hygiene

**Files:**
- Create: `Makefile`
- Create: `.golangci.yml`
- Create: `.github/workflows/ci.yml`
- Create: `Dockerfile`
- Create: `.dockerignore`
- Create: `tests/test_repo_contract.sh`
- Modify: `.gitignore`
- Modify: `cmd/bridge/main.go`
- Create: `cmd/bridge/main_test.go`
- Modify: `README.md`

**Step 1: Write the failing local CI contract**

Create `tests/test_repo_contract.sh`:

```bash
#!/usr/bin/env bash
set -euo pipefail
for required in Makefile .golangci.yml .github/workflows/ci.yml Dockerfile; do
  test -s "$required" || { echo "missing $required" >&2; exit 1; }
done
grep -Fq 'Status: experimental' README.md
grep -Fq 'make lint' .github/workflows/ci.yml
grep -Fq 'go test -race ./...' .github/workflows/ci.yml
```

**Step 2: Verify failure**

Run:

```bash
bash tests/test_repo_contract.sh
```

Expected: missing files are reported.

**Step 3: Add the Makefile and lint config**

Create targets:

```make
.PHONY: build test race lint vet verify docker

build:
	go build -o bin/evidra-agentgateway ./cmd/bridge

test:
	go test ./... -count=1

race:
	go test -race ./... -count=1

lint:
	golangci-lint run

vet:
	go vet ./...

verify: lint vet test race

docker:
	docker build -t evidra-agentgateway:dev .
```

Use the same golangci-lint major and baseline settings as Core unless bridge code
requires an explicitly justified stricter rule.

Add a standard `--version` branch to `cmd/bridge` and test that it exits without
opening listeners or output files. Use the existing `internal/version` package;
do not create a second version source.

**Step 4: Add CI**

The workflow must check formatting, `go vet`, unit tests, race, golangci-lint,
build both commands, build the container, run `--version`, and execute
`tests/test_repo_contract.sh`. Pin actions to the same major versions used by
Core.

**Step 5: Add a minimal multi-stage container**

Build `./cmd/bridge` in a Go image matching `go.mod`; copy it into a non-root
runtime image; expose 4317 and 4318; persist `/data`; default
`EVIDRA_BRIDGE_OBSERVATIONS=/data/observations.jsonl`; and add a container
healthcheck against `/healthz`.

**Step 6: Label the README honestly**

Add immediately after the title:

```text
Status: experimental AgentGateway observation adapter. Its JSONL output is not yet an Evidra Evidence v2 store and is not consumed by `evidra summarize`.
```

Do not call it an official integration yet.

**Step 7: Run verification**

Run:

```bash
bash tests/test_repo_contract.sh
make verify
make build
./bin/evidra-agentgateway --version
docker build -t evidra-agentgateway:dev .
docker run --rm evidra-agentgateway:dev --version
```

Expected: all pass.

**Step 8: Commit**

```bash
git add Makefile .golangci.yml .github/workflows/ci.yml Dockerfile .dockerignore .gitignore cmd/bridge README.md tests/test_repo_contract.sh
git commit -s -m "build: add bridge verification and container pipeline"
```

### Task 8: Make sink failures retryable and observable

**Files:**
- Modify: `internal/bridge/processor.go:315-349`
- Modify: `internal/bridge/processor_test.go`
- Modify: `internal/sink/jsonl.go`
- Create: `internal/sink/jsonl_test.go`

**Step 1: Write a failing processor test**

Create a sink that fails once and succeeds on retry:

```go
type failOnceSink struct {
	writes int
	got    []observation.Execution
}

func (s *failOnceSink) Write(_ context.Context, ex observation.Execution) error {
	s.writes++
	if s.writes == 1 {
		return errors.New("disk unavailable")
	}
	s.got = append(s.got, ex)
	return nil
}
```

Test that the first `Flush` returns an error, `Stats().Emitted == 0`, the record
remains buffered, a second `Flush` succeeds, and then `Emitted == 1` with exactly
one stored record.

**Step 2: Write failing JSONL sink tests**

Cover:

- one write survives close/readback;
- a close or write failure is returned;
- a JSONL record larger than the read buffer produces a clear scanner error;
- file mode is `0600` on a new file.

**Step 3: Verify failure**

Run:

```bash
go test ./internal/bridge ./internal/sink -count=1
```

Expected: processor retry semantics and missing sink coverage fail.

**Step 4: Acknowledge only successful writes**

Refactor assembly so records are not removed from `spans`, `logs`, and
`firstSeen`, and keys are not added to `emitted`, until `sink.Write` succeeds.
Update success counters only after that write. Add `SinkWriteFailures` to
`Stats`; increment it on each failed attempt. Preserve lock safety without
holding the processor mutex during filesystem I/O.

The minimal invariant is:

```go
if err := sink.Write(ctx, ex); err != nil {
	p.count(func(s *Stats) { s.SinkWriteFailures++ })
	return fmt.Errorf("sink: %w", err)
}
p.markEmitted(key, ex, merged, logOnly)
```

The exact helper shape may differ, but the tests must prove retryability and
counter truth.

**Step 5: Run focused and race tests**

Run:

```bash
go test ./internal/bridge ./internal/sink -count=1
go test -race ./internal/bridge ./internal/sink -count=1
make lint
```

Expected: all pass and sink coverage is non-zero.

**Step 6: Commit**

```bash
git add internal/bridge/processor.go internal/bridge/processor_test.go internal/sink/jsonl.go internal/sink/jsonl_test.go
git commit -s -m "fix(bridge): retain observations until sink persistence succeeds"
```

### Task 9: Make the AgentGateway OTLP path reproducible

**Files:**
- Modify: `examples/agentgateway-config.yaml`
- Modify: `examples/otel-collector-config.yaml`
- Modify: `docs/agentgateway-telemetry-matrix.md`
- Modify: `README.md`
- Create: `examples/compose.yaml`
- Create: `examples/Dockerfile.agentgateway-test`
- Create: `tests/e2e_otlp.sh`
- Modify: `tests/test_repo_contract.sh`
- Create: `testdata/expected/otlp-stats.json`

**Step 1: Write a failing configuration contract**

Extend `tests/test_repo_contract.sh` to require:

```bash
grep -Fq 'evidra_op:' examples/agentgateway-config.yaml
grep -Fq '/v1/logs' examples/otel-collector-config.yaml
grep -Fq '/v1/traces' examples/otel-collector-config.yaml
grep -Fq 'agentgateway' examples/compose.yaml
grep -Fq 'evidra-agentgateway' examples/compose.yaml
```

Require README commands to reference the committed compose file rather than an
unpublished `CORR-0` scaffold.

**Step 2: Verify failure**

Run:

```bash
bash tests/test_repo_contract.sh
```

Expected: current example lacks the documented correlation/log path.

**Step 3: Build a complete pinned environment**

Create a compose environment with:

- AgentGateway pinned to `v1.5.0`, never `latest`;
- the local bridge image;
- an OpenTelemetry Collector pinned to a version verified during execution;
- one deterministic third-party MCP server;
- mounted configs and output directories;
- healthchecks and no production credentials.

Build the test-only AgentGateway image from the pinned AgentGateway 1.5.0 image
and add only Node 22 plus `@modelcontextprotocol/server-everything` at a pinned
package version. Verify the source image's binary path and package manager once,
then lock both facts in `examples/Dockerfile.agentgateway-test`; never install
the package dynamically during the E2E run.

Configure both logs and traces, including the documented `evidra_op` projection.
Do not enable raw arguments or results.

**Step 4: Add the end-to-end assertion**

`tests/e2e_otlp.sh` must:

1. start the compose project with a unique project name;
2. wait on health endpoints with a bounded retry loop;
3. initialize one MCP session;
4. execute successful and failing tool calls;
5. stop traffic and flush the bridge;
6. assert exact emitted, merged, correlated/unattributed, and ignored counts;
7. scan output for privacy canaries and fail if raw values appear;
8. tear down in a trap while retaining logs on failure.

Do not claim correlated output unless the operation ID is actually delivered by
the test client. If the client cannot deliver it without unsupported behavior,
assert `unattributed` and record that result for Task 10.

**Step 5: Run the E2E test**

Run:

```bash
bash tests/e2e_otlp.sh
```

Expected: deterministic pass on AgentGateway 1.5.0 with no raw-payload canary in
the JSONL output.

**Step 6: Update documentation from measured output**

Replace “verified in CORR-0” with the committed command and expected counters.
Make `docs/agentgateway-telemetry-matrix.md` agree with the actual direct/fanout
log transport. Correct the topology so it does not describe Evidra as a
claims-only MCP target.

**Step 7: Commit**

```bash
git add README.md docs/agentgateway-telemetry-matrix.md examples tests testdata/expected/otlp-stats.json
git commit -s -m "test(bridge): add reproducible AgentGateway OTLP parity run"
```

## Phase 4: Capture-Path Decision

### Task 10: Spike AgentGateway ExtMCP against the same matrix

**Files:**
- Create: `cmd/extmcp-spike/main.go`
- Create: `internal/extmcp/server.go`
- Create: `internal/extmcp/server_test.go`
- Create: `examples/agentgateway-extmcp-config.yaml`
- Create: `tests/e2e_extmcp.sh`
- Create: `docs/agentgateway-capture-decision.md`
- Modify: `go.mod`
- Modify: `go.sum`

**Step 1: Define table-driven failing contract tests**

Build one capture matrix covering:

```go
var captureCases = []struct {
	name string
	call string
twantStart bool
	wantFinish bool
	wantOperationContext bool
}{
	{name: "success", call: "echo", wantStart: true, wantFinish: true, wantOperationContext: true},
	{name: "tool_error", call: "fail", wantStart: true, wantFinish: true, wantOperationContext: true},
	{name: "cancelled", call: "slow_then_cancel", wantStart: true, wantFinish: true, wantOperationContext: true},
	{name: "missing_response", call: "disconnect", wantStart: true, wantFinish: false, wantOperationContext: true},
}
```

The server tests must also prove that operational requests and successful
responses are passed unchanged and raw payloads are not written to disk.

**Step 2: Verify tests fail**

Run:

```bash
go test ./internal/extmcp -count=1
```

Expected: package does not exist yet.

**Step 3: Implement the minimal ExtMCP server**

Generate bindings from AgentGateway's pinned `ext_mcp.proto` or import its
published Go API if versioned and stable. Implement `CheckRequest` and
`CheckResponse` as pass-through hooks that capture only bounded metadata needed
by the matrix. Do not add policy enforcement or production persistence in the
spike.

Expose the following measured fields to an in-memory test sink:

```go
type Capture struct {
	ServiceNames []string
	Method string
	RequestSeen bool
	ResponseSeen bool
	OperationContext string
	RawPersisted bool
}
```

Any operation context must come from a documented ExtMCP metadata/header field
or an exactly observed prescribe/report protocol state on the same authoritative
connection. Time, tool, target, and trace-only guesses are forbidden.

**Step 4: Run the ExtMCP E2E matrix**

Configure AgentGateway `mcpGuardrails` for request and response phases and run:

```bash
bash tests/e2e_extmcp.sh
```

Expected: the script writes a machine-readable result containing every matrix
row, including explicit gaps for response errors that do not invoke the hook.

**Step 5: Compare OTLP and ExtMCP**

Write `docs/agentgateway-capture-decision.md` with a table for:

- successful finish coverage;
- error/cancel/missing-response coverage;
- authoritative operation context;
- raw payload exposure;
- ordering and retry behavior;
- configuration count;
- AgentGateway version coupling.

Decision rule:

- choose ExtMCP primary only if operation context is authoritative and terminal
  gaps have a documented fallback;
- choose OTLP primary only if it carries explicit operation identity and exact
  pairing without client-specific heuristics;
- choose hybrid only if each path owns a distinct stated fact and deduplication
  is exact;
- if none qualifies, stop at unattributed experimental observations.

**Step 6: Run bridge verification**

Run:

```bash
make verify
bash tests/e2e_otlp.sh
bash tests/e2e_extmcp.sh
```

Expected: all mechanics pass; the decision document records the measured
limitations even if the product gate fails.

**Step 7: Commit the spike**

```bash
git add cmd/extmcp-spike internal/extmcp examples/agentgateway-extmcp-config.yaml tests/e2e_extmcp.sh docs/agentgateway-capture-decision.md go.mod go.sum
git commit -s -m "spike(agentgateway): compare ExtMCP and OTLP capture paths"
```

**Step 8: Human checkpoint**

Stop and present the decision table. Do not start Tasks 11-13 unless a human
approves a capture path that passes the non-heuristic operation-context rule.

## Phase 5: Source-Neutral Observer Evidence

Tasks 11-13 are conditional on approval at Task 10.

### Task 11: Add an independently signed observer store to Core

**Files:**
- Modify: `pkg/evidence/event_v2.go`
- Modify: `pkg/evidence/store_v2.go`
- Modify: `pkg/evidence/verify_v2.go`
- Create: `pkg/evidence/observer_v2.go`
- Create: `pkg/evidence/observer_v2_test.go`
- Modify: `docs/evidence-format.md`
- Modify: `docs/architecture.md`
- Modify: `tests/test_doc_trust_alignment.sh`

**Step 1: Write failing evidence tests**

Define tests requiring:

- `observer_observed` provenance;
- `execution_observed` as a source-neutral event;
- a distinct observer writer identity in metadata;
- separate Ed25519 keys per observer store;
- explicit `complete`, `degraded`, and `unknown` coverage states;
- rejection of `agent_declared` payloads in observer stores;
- no raw argument/result fields in the observer payload;
- verification that never presents two stores as one signature boundary.

Use this minimal public payload shape:

```go
type ObserverExecutionPayload struct {
	ExecutionID             string `json:"execution_id"`
	ObserverType            string `json:"observer_type"`
	ObserverID              string `json:"observer_id"`
	Tool                    string `json:"tool,omitempty"`
	Target                  string `json:"target,omitempty"`
	Status                  ExecutionStatus `json:"status"`
	StartedAt               time.Time `json:"started_at"`
	FinishedAt              time.Time `json:"finished_at,omitempty"`
	ArgumentsFingerprintStatus string `json:"arguments_fingerprint_status"`
	ResultFingerprintStatus string `json:"result_fingerprint_status"`
	Correlation             string `json:"correlation"`
	CorrelationDetail       string `json:"correlation_detail,omitempty"`
}
```

The event envelope carries `OperationID`; do not duplicate it in the payload.

**Step 2: Verify failure**

Run:

```bash
go test ./pkg/evidence -run Observer -count=1
```

Expected: missing provenance, event, metadata, and store APIs fail.

**Step 3: Implement the minimal observer store**

Add:

```go
const ProvenanceObserverObserved Provenance = "observer_observed"
const EventExecutionObserved EventType = "execution_observed"
```

Extend store metadata with a writer kind and observer identity. Reuse hashing,
signing, sequence, append, flush, and degraded-window machinery; do not create a
second cryptographic implementation. Provide a constructor that cannot emit
agent-declared events.

**Step 4: Document trust boundaries**

State explicitly:

- observer and proxy stores are separate trust roots;
- matching operation IDs connect records semantically, not cryptographically;
- observer coverage applies only to that observer;
- raw payloads remain forbidden;
- an observer signature does not prove external-world outcomes or organization
  identity.

**Step 5: Run evidence verification**

Run:

```bash
go test ./pkg/evidence -count=1
go test -race ./pkg/evidence -count=1
make lint
bash tests/test_doc_trust_alignment.sh
```

Expected: all pass.

**Step 6: Commit**

```bash
git add pkg/evidence docs/evidence-format.md docs/architecture.md tests/test_doc_trust_alignment.sh
git commit -s -m "feat(evidence): add independent observer stores"
```

### Task 12: Reconcile observer records without collapsing trust sources

**Files:**
- Modify: `pkg/report/reconcile.go`
- Modify: `pkg/report/reconcile_test.go`
- Modify: `docs/cli-reference.md`
- Modify: `docs/evidence-format.md`

**Step 1: Write failing reconciliation tests**

Cover four cases:

1. exactly one declared operation plus one correlated observer execution;
2. observer execution with no matching operation;
3. duplicate operation IDs in two recorder stores;
4. observer store with degraded or unknown coverage.

Require a separate field:

```go
type ObserverExecution struct {
	ObserverType string `json:"observer_type"`
	ObserverID string `json:"observer_id"`
	Tool string `json:"tool,omitempty"`
	Status string `json:"status"`
	Correlation string `json:"correlation"`
}

type Operation struct {
	// existing fields...
	ObserverExecutions []ObserverExecution `json:"observer_executions,omitempty"`
}
```

Require unattached and ambiguous records to remain visible at summary level;
never guess a target operation.

**Step 2: Verify failure**

Run:

```bash
go test ./pkg/report -run Observer -count=1
```

Expected: report has no observer model or cross-store attachment.

**Step 3: Implement a two-pass reconciliation**

First ingest claim/proxy stores and index operation IDs. Then ingest observer
stores:

- one exact operation match: attach under `observer_executions`;
- zero matches: append to `unattributed_observer_executions`;
- multiple matches: append to `ambiguous_observer_executions` with an explicit
  reason.

Do not append observer records to the existing proxy `Executions` slice. Do not
combine signature or coverage conclusions across writers.

**Step 4: Run report and CLI tests**

Run:

```bash
go test ./pkg/report ./cmd/evidra -count=1
go test -race ./pkg/report -count=1
make lint
```

Expected: all pass.

**Step 5: Commit**

```bash
git add pkg/report docs/cli-reference.md docs/evidence-format.md
git commit -s -m "feat(report): reconcile independent observer records"
```

### Task 13: Replace bridge JSONL output with the Core observer writer

Run this task in the bridge worktree after Core Tasks 11-12 are available on a
reviewable branch or temporary local tag.

**Files:**
- Modify: `go.mod`
- Modify: `go.sum`
- Create: `internal/sink/evidence.go`
- Create: `internal/sink/evidence_test.go`
- Modify: `internal/config/config.go`
- Modify: `internal/config/config_test.go`
- Modify: `cmd/bridge/main.go`
- Modify: `README.md`
- Modify: `examples/bridge.env.example`
- Modify: `tests/e2e_otlp.sh` or `tests/e2e_extmcp.sh` according to Task 10

**Step 1: Write failing adapter tests**

Require the sink to convert a normalized `observation.Execution` into one
`evidence.EventExecutionObserved` with:

- `ProvenanceObserverObserved`;
- exact operation ID only when correlation is `correlated`;
- observer type/id/version;
- explicit fingerprint availability;
- no raw arguments/results;
- source coverage and sink errors preserved.

Use `evidence.VerifyRoot` and `evidence.ReadStore` in the test; do not assert only
against in-memory structs.

**Step 2: Verify failure**

Run:

```bash
go test ./internal/sink -run Evidence -count=1
```

Expected: the Evidence sink is absent.

**Step 3: Import Core without a permanent replace**

Require a released pseudo-version or review tag of
`github.com/vitas/evidra`. A local `go.work` may be used during coordinated
development, but it must remain ignored and must not be required by CI.

**Step 4: Implement and select the Evidence sink**

Make the signed observer directory the default output. Retain raw normalized
JSONL only behind an explicit diagnostics flag such as
`EVIDRA_BRIDGE_DEBUG_JSONL`; label it non-Evidra evidence in help and docs.

**Step 5: Extend E2E verification**

After the AgentGateway calls, run the Core CLI against the combined recorder
root and assert:

- observer signature is valid under its own key;
- observer coverage is displayed separately;
- correlated records attach to exactly one operation;
- unattributed/ambiguous records remain visible;
- privacy canaries do not appear in either store or summary.

**Step 6: Run both repository suites**

Run:

```bash
make verify
bash tests/e2e_otlp.sh   # or the selected Task 10 path
```

Then in Core:

```bash
make lint
go test ./cmd/... ./pkg/... -count=1
bash tests/run_guards.sh
```

Expected: all pass.

**Step 7: Commit**

```bash
git add go.mod go.sum internal/sink internal/config cmd/bridge README.md examples tests
git commit -s -m "feat(bridge): write signed Evidra observer evidence"
```

## Phase 6: Product Validation and Release Decision

### Task 14: Produce the real Gate C artifact

**Files:**
- Create locally, do not track: `local-notes/agentgateway-gate-c.md`
- Modify after review: `docs/validation.md`
- Modify: `README.md`
- Modify: `ui/src/pages/Landing.tsx`
- Modify: related claim guards and UI tests.

**Step 1: Pre-register the Gate C questions**

Before running, record:

```text
1. What did the agent declare?
2. What did the Evidra proxy observe?
3. What did AgentGateway independently observe?
4. What did the agent report?
5. Did reconciliation change, clarify, or fail to change the reader's operational decision?
6. Which evidence gap mattered most?
```

Name the real third-party operational MCP server and the exact AgentGateway and
Evidra commits. Do not change acceptance language after reading the result.
Keep this pre-registration in the ignored `local-notes/` directory until the
independent reader approves the publishable conclusion.

**Step 2: Run one real operation**

Use the pinned integration environment and a non-destructive but operationally
meaningful task. Save recorder output outside Git because it contains keys.
Record commands, timestamps, versions, and checksums in the review template.

**Step 3: Obtain an independent read**

Stop and give only the generated summary plus the pre-registered questions to a
reader who did not implement the run. Record their answer verbatim or as an
approved paraphrase. This step cannot be self-certified by the implementer.

**Step 4: Update validation honestly**

- If both Gate C inputs exist and the reader reports a concrete changed or
  clarified decision, document Gate C as passed with reproduction metadata.
- If not, keep Gate C unpassed and document the exact missing condition.

Do not promote the bridge or remove preview wording merely because integration
mechanics passed.

**Step 5: Update tests first, then public wording**

Change the claim guards to require the new measured status. Run them to see the
old docs fail. Then update README, landing, and validation with no claim broader
than the artifact.

**Step 6: Run full Core verification**

Run:

```bash
make lint
go test ./cmd/... ./pkg/... -count=1
go test -race ./pkg/evidence/... ./pkg/proxy/... ./pkg/report/... ./cmd/evidra-fixture/... ./cmd/evidra-gatea/...
bash tests/run_guards.sh
cd ui
npm ci
npm run lint
npm run typecheck
npm test
npm run build
npm audit --audit-level=high
```

Expected: all pass.

**Step 7: Commit the reviewed conclusion**

```bash
git add docs/validation.md README.md ui/src/pages/Landing.tsx tests ui/test
git commit -s -m "docs: record the AgentGateway Gate C result"
```

Do not commit private keys, digest keys, raw outputs, or unapproved reader notes.

### Task 15: Make the release decision explicit

**Files:**
- Conditional modify: `.github/workflows/release.yml`
- Conditional modify: `tests/core-workflows.txt`
- Conditional modify: release-related shell guards.

**Step 1: Human checkpoint**

Present:

- Gate A, B, and C status;
- full Core verification output;
- bridge verification and compatibility matrix;
- proposed release version;
- exact files that would remove the release refusal.

Stop until the user explicitly authorizes a release-path change. Approval of
this implementation plan is not approval to release.

**Step 2A: If Gate C did not pass**

Keep `core-release-refusal` and preview wording unchanged. Open or record a
specific follow-up from the missing Gate C condition. Commit nothing merely to
restage the same refusal.

**Step 2B: If Gate C passed and release is authorized**

Write a failing workflow guard that expects the refusal marker to be absent and
expects the snapshot/test/docker/goreleaser graph to remain intact. Then remove
only the refusal job/step and update its guard references.

Run:

```bash
bash tests/run_guards.sh
make lint
go test ./cmd/... ./pkg/... -count=1
```

Expected: all pass.

**Step 3: Commit the release-path decision**

```bash
git add .github/workflows/release.yml tests
git commit -s -m "ci: enable Core releases after Gate C approval"
```

Do not bump, tag, or push in this step.

**Step 4: Prepare a release only under separate shipping authorization**

First confirm the helper still exists and passes its test:

```bash
test -x scripts/bump-version.sh
bash tests/test_bump_version_script.sh
```

Then use it, for example:

```bash
scripts/bump-version.sh 0.6.0
```

Review the changelog and version diff, run the full linter and test suite, and
commit with sign-off. Stop again before tag or push unless those external
actions were explicitly authorized.

## Phase 7: Remove Planning Debris and Verify Deployment

### Task 16: Make Git history the only plan archive

**Files:**
- Modify: `.gitignore`
- Modify: `CONTRIBUTING.md`
- Modify: `tests/test_doc_trust_alignment.sh`
- Modify: `tests/test_module_path_refs.sh`
- Modify: `tests/test_public_claims.sh`
- Modify: `tests/test_public_docs_structure.sh`
- Modify: `tests/test_repo_cleanup_hygiene.sh`
- Delete: `docs/plans/2026-09-14-public-docs-landing-cleanup-design.md`
- Delete: `docs/plans/2026-09-14-public-docs-landing-cleanup.md`
- Delete: `docs/plans/2026-09-15-post-relaunch-hardening-design.md`
- Delete: `docs/plans/2026-09-15-post-relaunch-hardening.md`

**Step 1: Write the failing no-plan-corpus guard**

Add to `tests/test_repo_cleanup_hygiene.sh`:

```bash
if git ls-files docs/plans | grep -q .; then
  git ls-files docs/plans >&2
  fail "completed plans belong in Git history, not the current tree"
fi
```

Remove `docs/plans` exclusions from `test_public_docs_structure.sh`; the complete
tracked Markdown inventory under `docs/` should now equal the five canonical
documents.
Remove the now-dead `docs/plans/**` exclusions from
`test_doc_trust_alignment.sh`, `test_module_path_refs.sh`, and
`test_public_claims.sh` as well.

**Step 2: Verify failure**

Run:

```bash
bash tests/test_repo_cleanup_hygiene.sh
```

Expected: it lists the tracked plan files.

**Step 3: Delete the plan corpus and exceptions**

Delete every tracked file under `docs/plans/`. In `.gitignore`, retain one simple
`docs/plans/` ignore rule with no allow-list exceptions. Change the repository
map in CONTRIBUTING from “five canonical documents plus the docs/plans corpus”
to “five canonical documents”. Remove all live references to the plan corpus.
Do not delete ignored, untracked personal plans; they are outside repository
history and belong to the workspace owner.

**Step 4: Run documentation and repository guards**

Run:

```bash
bash tests/test_public_docs_structure.sh
bash tests/test_repo_cleanup_hygiene.sh
bash tests/test_doc_trust_alignment.sh
bash tests/run_guards.sh
git grep -n 'docs/plans' -- . ':!CHANGELOG.md' ':!.gitignore'
```

Expected: guards pass and the final grep has no output.

**Step 5: Commit**

```bash
git add .gitignore CONTRIBUTING.md tests
git add -u docs/plans
git commit -s -m "docs: remove completed planning corpus"
```

### Task 17: Perform final local and deployed verification

**Files:**
- No source changes unless verification finds a defect; any fix requires a new failing test and its own commit.

**Step 1: Run final Core verification**

Run:

```bash
make lint
go vet ./cmd/... ./pkg/...
go test ./cmd/... ./pkg/... -count=1
go test -race ./pkg/evidence/... ./pkg/proxy/... ./pkg/report/... ./cmd/evidra-fixture/... ./cmd/evidra-gatea/...
bash tests/run_guards.sh
cd ui
npm ci
npm run lint
npm run typecheck
npm test
npm run build
npm audit --audit-level=high
```

Expected: every command passes with zero high/critical npm vulnerabilities.

**Step 2: Run final bridge verification**

Run:

```bash
make verify
bash tests/e2e_otlp.sh
bash tests/e2e_extmcp.sh
docker build -t evidra-agentgateway:verify .
docker run --rm evidra-agentgateway:verify --version
```

Expected: all selected/supported paths pass; an intentionally rejected capture
path may remain as a documented non-gating comparison test.

**Step 3: Inspect the landing in the in-app browser**

Use @browser:control-in-app-browser against the local production preview and,
after an authorized deploy, `https://evidra.cc/`.

Check at 320, 768, 1280, and 1600 px:

- no horizontal page overflow;
- topology readable and keyboard/screen-reader coherent;
- focus states visible;
- theme toggle labels describe the resulting action;
- code blocks scroll internally;
- headings form one logical outline;
- social card URL returns a 1200x630 PNG;
- primary CTA reaches current getting-started docs.

**Step 4: Verify live HTTP assets after deployment**

Run:

```bash
curl -fsSIL https://evidra.cc/
curl -fsSIL https://evidra.cc/og/evidra-core.png
curl -fsSL https://evidra.cc/ | grep -F 'Verifiable Evidence for MCP Operations'
curl -fsSL https://evidra.cc/ | grep -F 'https://evidra.cc/og/evidra-core.png'
```

Expected: HTTP 200, correct image content type, current title, and social image
metadata.

**Step 5: Confirm both worktrees are clean**

Run:

```bash
git status --short --branch
git -C /Users/vitas/git/evidra-agentgateway-bridge/.worktrees/integration-hardening status --short --branch
```

Expected: no uncommitted changes.

**Step 6: Request code review**

Use @superpowers:requesting-code-review for both branches. Review source-boundary
claims, failure semantics, release gating, and cross-repository version coupling
before offering merge or PR options.

## Recommended Execution Order

1. Tasks 0-6: public/core hardening.
2. Tasks 7-9: bridge reliability and reproducibility.
3. Task 10: capture-path spike and mandatory human decision.
4. Tasks 11-13 only after the spike passes.
5. Task 14: real Gate C run and independent read.
6. Task 15: separate release decision.
7. Tasks 16-17: remove planning debris and final verification.

The first reviewable PR should contain Tasks 1-6 only. The bridge hardening is a
separate PR in its own repository. Observer evidence and reconciliation should
be a third Core PR so that the trust-model change is not hidden inside landing
or repository cleanup.
