# External Evidence Bundle — v1

Spec: `evidra-external-bundle/v1`
Status: stable contract. Additive changes only; breaking changes require a new spec id.

An **external evidence bundle** is a complete, self-describing Evidra evidence
store produced by a tool other than the Evidra CLI/MCP/API — for example
[Evidra Bench](https://github.com/vitas/evidra-bench), which records a sandbox
agent run locally and hands the bundle to a human to open with
`evidra validate` / `evidra scorecard`.

The bundle lets a producer write **exactly the same append-only chain** that
`evidra record`/MCP write in production, using only the public
`samebits.com/evidra/pkg/evidence` API. No HTTP server, no internal packages,
no key management.

## Layout

A bundle is a regular segmented evidence store plus one sidecar file:

```text
my-bundle/
├── manifest.json                  # store manifest (owned by pkg/evidence)
├── bundle.json                    # bundle spec (owned by pkg/evidence.BundleManifest)
└── segments/
    └── evidence-000001.jsonl      # one EvidenceEntry per line, chained
```

Store internals (`manifest.json`, segment naming, chaining, entry hashing) are
defined by `pkg/evidence` and must not be hand-written — producers build
entries with `evidence.BuildEntry` and append them with
`evidence.AppendEntryAtPath`, which keeps the chain and manifest consistent.

## Chain and hashing

- Entry hash: `sha256:<hex>` over the JSON projection of all entry fields
  except `hash` and `signature` (`hashableEntry` in `pkg/evidence`).
- The first entry of a store MUST have `previous_hash: ""` (genesis).
- Each subsequent entry MUST carry the previous entry's `hash` in
  `previous_hash`.
- Required entry fields: `session_id`, `trace_id`, `type`, `timestamp`,
  `payload`, `spec_version`.

Producers normally keep all entries of one logical run in a single session
(`session_id`), grouping operations with `operation_id`.

## Signing and trust levels

Every entry is signed with an Ed25519 key: the signature is
base64(`Sign(hash_string)`), which is what `ValidateChainWithSignatures`
verifies.

`pkg/evidence` exposes **`NewEphemeralSigner()`** for external producers: a
key generated once per run, held in memory, with the public half written into
`bundle.json`. The private half MUST be discarded when the bundle is complete.

| trust_level | key lifecycle | guarantees |
|---|---|---|
| `ephemeral` | generated per run, public key embedded in bundle, private key discarded | chain integrity and atomic authorship; **no identity** |
| `verified` | long-lived producer key | identity of the producer (future extension; not required for Bench v1) |

Consumers treat `ephemeral` bundles the way they treat any local file: proof
that nobody edited it after creation, not proof of who created it.

## `bundle.json`

```json
{
  "spec": "evidra-external-bundle/v1",
  "producer": {
    "name": "evidra-bench",
    "version": "v0.5.61",
    "url": "https://github.com/vitas/evidra-bench"
  },
  "trust_level": "ephemeral",
  "public_key": "<base64 std Ed25519 public key>",
  "created_at": "2026-09-08T22:00:00Z",
  "notes": "optional human-readable string"
}
```

Written with `evidence.SaveBundleManifest`, read with
`evidence.LoadBundleManifest`.

## Minimum entry set for a run

A benchmark run should emit, in order:

1. `session_start` — `SessionStartPayload{Labels: {...run metadata...}}`
2. per executed action: `prescribe` (`PrescriptionPayload`, intent filled,
   assessment optional) immediately followed by `report`
   (`ReportPayload` with `prescription_id`, `exit_code`, `verdict` —
   `success` iff exit code 0; `declined` reports set `decision_context` and no
   exit code)
3. `session_end` — `SessionEndPayload{Status: "completed" | "aborted" | "error"}`

`verdict` must agree with `exit_code` (`evidence.VerdictFromExitCode`).
Unknown JSON fields inside payloads are ignored by readers — producers may add
extension fields, consumers must not reject bundles because of them.

## Producer example (Go)

```go
signer, _ := evidence.NewEphemeralSigner()
var prev string
for _, step := range run.Steps {
    entry, _ := evidence.BuildEntry(evidence.EntryBuildParams{
        Type:         evidence.EntryTypeReport,
        SessionID:    run.SessionID,
        OperationID:  step.OperationID,
        TraceID:      run.TraceID,
        Actor:        evidence.Actor{Type: "agent", ID: step.AgentID, Provenance: "external-bundle"},
        Payload:      mustJSON(evidence.ReportPayload{ /* ... */ }),
        PreviousHash: prev,
        SpecVersion:  version.SpecVersion,
        Signer:       signer,
    })
    _ = evidence.AppendEntryAtPath(bundleDir, entry)
    prev = entry.Hash
}
_ = evidence.SaveBundleManifest(bundleDir, evidence.BundleManifest{
    Spec:       evidence.BundleSpecV1,
    Producer:   evidence.BundleProducer{Name: "my-tool", Version: version},
    TrustLevel: evidence.TrustEphemeral,
    PublicKey:  signer.PublicKeyBase64(),
})
```

## Usage / consumption

```bash
# verify chain + embedded signatures
evidra validate --evidence-dir ./my-bundle

# score the bundle (session grouping applies as usual)
evidra scorecard --evidence-dir ./my-bundle
```

## Conformance fixture

`tests/external_bundle_v1/` is the committed reference bundle (generated by
`scripts/gen_external_bundle`, which deliberately uses only the public API).
CI validates it in `pkg/evidence/external_bundle_test.go`, including
tamper-detection. Any producer implementation is conformant when its bundles
pass the same `ValidateBundle` checks.
