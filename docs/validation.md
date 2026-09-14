# Validation Status

Validation is kept separate from architecture: implemented mechanisms can pass
tests while the product-value hypothesis remains unresolved. The current evidence
supports three different gate conclusions.

## What has been measured

The repository contains:

- deterministic MCP fixture tests in `cmd/evidra-fixture`;
- endpoint composition, enforcement, failure, and concurrency tests in
  `pkg/proxy`;
- JCS, fingerprint, chain, signature, coverage, and tamper tests in
  `pkg/evidence`;
- reconciliation tests in `pkg/report`;
- model-runner provenance, invariant, baseline, and regrade tests in
  `cmd/evidra-gatea`.

These tests establish behavior against source-controlled fixtures. They do not
by themselves establish compatibility with every MCP server or usefulness to an
independent operator.

## Gate A: protocol adoption

Gate A measured whether model arms could use the prescribe/report protocol while
completing eight fixture tasks in `cmd/evidra-gatea/tasks.json`.

The official set planned 80 runs. Its original verdict was not passed. The
acceptance thresholds were subsequently revised after inspecting those same
measurements. Although the historical rows mechanically meet the revised bars,
data used to choose a threshold cannot also certify that threshold. A fresh set
would be required for a passing adoption claim.

Enforcement fired zero times across the 80 runs: the measured agents prescribed
before acting. Recovery after a protocol refusal was therefore not measurable,
not successful or unsuccessful. Scripted and endpoint tests exercise the refusal
path, but they do not replace an observed model-recovery denominator.

The current harness keeps live and regraded protocol-compliance derivations
aligned with `TestRegradeReproducesLiveCompliance`, guards result invariants in
`cmd/evidra-gatea/provenance_test.go`, and treats the direct-fixture `none` mode as
a descriptive harness baseline rather than a product enforcement mode.

## Gate B: supported-profile fidelity

Gate B passed against the project's own `cmd/evidra-fixture`. The suite runs the
real merged endpoint and covers initialization, paginated tool composition,
upstream tool calls, notifications, cancellation, direction-safe IDs, large
frames, parallel calls, reserved-name collisions, evidence ordering, privacy
canaries, and store-failure behavior.

Representative cases include:

- `TestInitializeProfileAndInstructions`;
- `TestWrappedToolsListEqualsDirectPlusLocal`;
- `TestExecutionsPairByIdUnderParallelCalls`;
- `TestLargeResultPassesThroughAndIsFingerprintedWithinBounds`;
- `TestEvidenceStoreReproducesEnforcementDecisions`;
- `TestStoreChainVerifiesAndDetectsTampering`.

This is a supported-profile fixture result, not a general gateway certification.
No real third-party operational MCP server was used, so server-specific metadata,
behavior, and side effects remain unmeasured.

## Gate C: reconciliation value

Gate C did not pass. The probes produced summaries that separated declared,
observed, and reported records and exposed per-operation facts, but both required
inputs for a value verdict were absent:

1. no real operational MCP upstream was recorded;
2. no independent reader documented that the reconciliation changed their
   understanding or decision.

The sample used the project fixture, whose expected behavior is known to its own
harness. The reconciliation-value hypothesis is therefore neither proven nor
disproven.

Tests such as `TestReconciliationViewPutsTheThreeLayersInOrder`,
`TestReconcileReportsClaimedAchievedWithoutExecutions`, and
`TestReportAckCarriesWhatTheProxyObserved` establish the mechanism and source
separation. They cannot establish human decision value.

## Reproduce the local checks

The implementation and fixture checks are local and require no hosted service or
paid model call:

```bash
make build
go build -o bin/evidra-fixture ./cmd/evidra-fixture
go build -o bin/evidra-gatea ./cmd/evidra-gatea

make lint
go test ./... -count=1
go test -race -count=1 \
  ./pkg/evidence/... \
  ./pkg/proxy/... \
  ./pkg/report/... \
  ./cmd/evidra-fixture/... \
  ./cmd/evidra-gatea/...
bash tests/run_guards.sh
```

Run a named Gate B slice with:

```bash
go test -count=1 -run \
  'TestInitializeProfileAndInstructions|TestWrappedToolsListEqualsDirectPlusLocal|TestExecutionsPairByIdUnderParallelCalls' \
  ./pkg/proxy
```

If a local Gate A artifact set is available, recompute its verdicts without model
calls or token spend:

```bash
./bin/evidra-gatea --regrade /absolute/path/to/gatea-artifacts
```

Recorder outputs and model-run artifacts are intentionally not committed because
they contain local key material and can be large. Reproduction from new model
calls additionally requires provider configuration and is not part of the free
local verification contract.

## What remains unproven

- protocol adoption against predeclared thresholds on a fresh model-run set;
- model recovery after a real enforcement refusal;
- supported-profile fidelity against a third-party operational MCP server;
- behavior outside the documented stdio/tool profile;
- whether an independent reader makes a better operational decision from the
  reconciliation;
- external-world outcome correctness, identity attestation, and complete
  observation of work outside the proxy boundary.

Public claims should remain within the boundaries above until new, reproducible
evidence changes a gate conclusion.
