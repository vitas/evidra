# Post-Relaunch Hardening Design

## Status

Approved on 2026-09-15 through the request to turn the post-launch review into
an implementation plan.

## Objective

Bring the public relaunch, repository documentation, release posture, and the
AgentGateway integration to one honest product state. A new visitor must be able
to tell what is shipped, what is preview-only, what Evidra observes, and how the
AgentGateway integration relates to Core without reading historical plans.

## Chosen Approach

Use a staged, two-repository hardening effort.

1. Correct public claims and small accessibility/SEO defects in `evidra`.
2. Make the unreleased state explicit until Gate C passes; do not bypass the
   existing release refusal.
3. Remove completed planning documents from the final public tree after their
   durable decisions have been absorbed into canonical docs and tests.
4. Keep `evidra-agentgateway-bridge` separate and label it experimental.
5. Harden the bridge as a reproducible integration laboratory, then compare its
   OTLP path with AgentGateway's official ExtMCP request/response extension.
6. Define the source-neutral observer contract in Core only after the spike
   proves the required data flow. AgentGateway-specific aliases, protobufs,
   deployment manifests, and compatibility tests remain outside Core.

This avoids importing AgentGateway's rapidly changing OTLP and gRPC dependency
surface into the small Core module. It also avoids presenting the current JSONL
normalizer as an Evidra evidence integration before `evidra summarize` can read
its output.

## Alternatives Considered

### Merge the current bridge into Core now

Rejected. The bridge does not write Evidence v2, Core cannot reconcile its
records, the documented operation-ID transport is incomplete, and the bridge
has no CI or release artifact. Moving it would create code proximity without a
working product path and would pull gateway-specific dependencies into Core.

### Leave both repositories unchanged and document the bridge as experimental

Rejected as the final state. This is honest but does not produce the real
third-party run and independent reconciliation review required by Gate C. It
also leaves unresolved data-loss behavior and an unreproducible AgentGateway
configuration.

### Staged dual-repository integration

Selected. Core owns neutral evidence semantics and reconciliation. The adapter
owns AgentGateway integration mechanics. A measured spike chooses the capture
path before the shared contract is expanded.

## Public Surface Design

The landing remains a comfortable-density public trust page with the current
neutral palette and one green accent. Structure and visual direction remain
unchanged; this effort corrects semantics and accessibility instead of adding
decorative sections.

The hero must describe evidence at the MCP execution boundary, not external
truth. Trust copy must distinguish persisted observations, known degraded
windows, and unknown loss. The reconciliation example must say that Evidra
generates the summary while a human judges external outcome. Workflow copy must
count CLI commands and protocol calls accurately.

The topology must expose its meaningful nodes to assistive technology. Social
sharing receives a Core-specific Open Graph card. The unused PWA manifest is
removed rather than expanded into an installability claim. Static deployment
tests own metadata, MIME-independent assets, canonical URLs, and the sitemap.

## Release Posture

Gate C remains the release boundary. Until it passes, README and landing label
the current line as an unreleased preview built from `main`; no version bump,
tag, release push, or removal of `core-release-refusal` is part of the cleanup
stage.

The Gate C run must use a real third-party operational MCP upstream and an
independent reader. The run records whether the reconciliation changed or
clarified an operational decision. Only after that artifact is reviewed does a
human authorize re-enabling the repository's existing release helper and
shipping a new minor release.

## Documentation Lifecycle

The five canonical documents remain the only current-state pages. Decisions
needed by contributors move into those pages or executable guards. Completed
design and implementation plans are deleted from the final working tree; Git
history is the archive.

Section-number comments such as `§18` are converted incrementally to named
invariants or test references when touched. They must not depend on an absent or
superseded planning document.

## AgentGateway Boundary

Core will eventually own:

- a source-neutral observer identity and provenance vocabulary;
- an independently signed observer store or equivalent import boundary;
- explicit coverage/degraded semantics for observer failures;
- read-side reconciliation of agent claims and external observations by an
  explicit operation identifier;
- tests proving that separate writers are never represented as one trust root.

The AgentGateway repository owns:

- OTLP logs/traces and AgentGateway attribute aliases;
- ExtMCP protobuf integration and version compatibility;
- AgentGateway configuration, containers, and end-to-end fixtures;
- source-specific correlation and privacy canaries.

The current `samebits.com/evidra` module path must resolve through Go vanity
metadata or be deliberately migrated before an external adapter imports Core.
The plan must not introduce a consumer-side `replace` directive as a permanent
solution.

## Capture-Path Spike

The spike compares two paths against AgentGateway 1.5:

1. OTLP logs plus traces, joined by trace and span IDs with explicit operation
   correlation.
2. ExtMCP request/response hooks, using only supported metadata and without
   mutating operational requests.

The comparison measures completeness for successful, failed, cancelled,
parallel, and missing-response tool calls; access to operation identity; raw
payload exposure; ordering; failure visibility; and configuration complexity.

ExtMCP is preferred if it supplies exact request/response boundaries and an
authoritative operation context. OTLP remains valuable as an independent
corroborating observer. If neither path can carry operation identity without a
client-specific heuristic, the integration stops at unattributed observations
and Core's schema is not expanded.

## Failure Handling

An observation is counted as emitted only after its sink write succeeds. A sink
failure retains or requeues the record, exposes unhealthy/degraded state, and
must never increment successful-emission counters. Shutdown flush failures are
fatal and visible.

Partial log-only or span-only observations remain explicit. Late counterpart
signals must be counted and diagnosed rather than silently discarded. Raw tool
arguments and results must never be persisted by the bridge; any capture path
that receives them computes bounded keyed fingerprints in-process or records an
explicit unavailable/refused status.

## Validation

Core retains its existing lint, test, race, guard, UI lint/typecheck/test/build,
and audit checks. New claim tests assert exact landing wording and accessibility
semantics. Deployment smoke checks verify the live HTML, hashed assets, social
metadata, and static-file content types.

The bridge gains CI with formatting, `go vet`, tests, race detection, lint, and
container smoke tests. Its end-to-end fixture must start AgentGateway, a real
third-party MCP server, the selected capture component, and the Evidra read
side. A checked-in expected artifact proves correlation, source attribution,
privacy, coverage, and reconciliation behavior.

## Completion Criteria

- Public copy makes no stronger claim than the evidence format and validation
  documents.
- The landing is keyboard- and screen-reader coherent and shares with a
  Core-specific social card.
- Users can see immediately that Core is preview-only or install a newly
  authorized release.
- The final current-state documentation tree contains no completed plan corpus.
- The bridge has CI, reproducible configuration, honest failure behavior, and a
  clear experimental status.
- One capture path passes the AgentGateway compatibility matrix, writes data the
  Evidra read side can reconcile, and produces the real Gate C review artifact.
- The integration code remains in the adapter repository unless later evidence
  shows that both projects must release atomically and users require one binary.
