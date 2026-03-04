# Evidra Benchmark — Migration Map from v0.2.0

## Status: COMPLETE

Migration executed across 5 phases. All verification checks pass.
Post-migration architecture update applied (Adapter interface,
pre-canonicalized prescribe path, operational files).

---

## 1. Source and Target

```
SOURCE: evidra/                    (v0.2.0-rc5, OPA-based policy engine)
TARGET: evidra-benchmark/          (benchmark product, no OPA)
```

The source was NOT modified. The target is a new Go module.

---

## 2. Module Identity

```
module: samebits.com/evidra-benchmark
go version: 1.24.6
```

---

## 3. Dependencies

### Kept from source
```
github.com/modelcontextprotocol/go-sdk/mcp v1.3.1
github.com/oklog/ulid/v2 v2.1.1
go.yaml.in/yaml/v3 v3.0.4
```

### Added new
```
github.com/hashicorp/terraform-json  (terraform plan parsing)
```

### Dropped
```
github.com/open-policy-agent/opa   ← entire OPA ecosystem removed
k8s.io/apimachinery                ← not needed (plain YAML parsing used instead)
```

---

## 4. Directory Structure (implemented)

```
evidra-benchmark/
├── go.mod
├── go.sum
├── Dockerfile                     # MCP server (distroless, no OPA)
├── Dockerfile.cli                 # CLI binary (distroless)
├── Makefile                       # build, test, docker, fmt, lint, tidy, clean
├── docker-compose.yml             # local dev (MCP + evidence volume)
├── server.json                    # MCP registry manifest (v0.3.0)
├── cmd/
│   ├── evidra/                    # CLI: scorecard, compare, prescribe, report
│   │   └── main.go               # ✅ Created
│   └── evidra-mcp/               # MCP server for AI agents
│       └── main.go               # ✅ Created (--evidence-dir, --environment, --forward-url)
├── internal/
│   ├── canon/                     # Domain adapters + canonicalization
│   │   ├── types.go              # ✅ Adapter interface, CanonicalAction, CanonResult,
│   │   │                         #    SelectAdapter(), DefaultAdapters(), Canonicalize()
│   │   ├── k8s.go                # ✅ K8sAdapter (kubectl, oc), YAML multi-doc, noise removal
│   │   ├── terraform.go          # ✅ TerraformAdapter (terraform), plan JSON parsing
│   │   ├── generic.go            # ✅ GenericAdapter (fallback), SHA256Hex()
│   │   ├── noise.go              # ✅ Frozen noise field lists (removeK8sNoiseFields)
│   │   └── canon_test.go         # ✅ Golden corpus tests (k8s, terraform, generic, multi-doc)
│   ├── evidence/                  # Copied from source + extended
│   │   ├── signer.go             # ✅ Copied
│   │   ├── signer_test.go        # ✅ Copied
│   │   ├── payload.go            # ✅ Copied + extended (canon fields)
│   │   ├── payload_test.go       # ✅ Copied + extended
│   │   ├── builder.go            # ✅ Copied
│   │   ├── builder_test.go       # ✅ Copied
│   │   └── types.go              # ✅ Copied + extended (prescription/report types)
│   ├── risk/                      # Risk matrix + catastrophic detectors
│   │   ├── matrix.go             # ✅ operation_class × scope_class → risk_level
│   │   ├── detectors.go          # ✅ 7 detectors ported from Rego (~250 lines)
│   │   └── risk_test.go          # ✅ 24 tests
│   ├── signal/                    # 5 behavioral signals
│   │   ├── types.go              # ✅ Entry, SignalResult, AllSignals()
│   │   ├── protocol_violation.go # ✅ Matches prescriptions to reports
│   │   ├── artifact_drift.go     # ✅ Compares artifact digests
│   │   ├── retry_loop.go         # ✅ Same intent+shape repeated N times in window
│   │   ├── blast_radius.go       # ✅ Resource count above threshold
│   │   ├── new_scope.go          # ✅ First-time (tool, operation_class) tuples
│   │   └── signal_test.go        # ✅ 14 tests
│   └── score/                     # Scorecard computation
│       ├── score.go              # ✅ Weighted penalty: score = 100 × (1 - penalty)
│       ├── compare.go            # ✅ WorkloadOverlap via Jaccard similarity
│       └── score_test.go         # ✅ 8 tests
├── pkg/
│   ├── mcpserver/                 # MCP server (prescribe/report/get_event)
│   │   ├── server.go             # ✅ BenchmarkService with prescribe, report, get_event
│   │   │                         #    PrescribeInput.CanonicalAction for pre-canonicalized path
│   │   ├── retry_tracker.go      # ✅ Adapted from deny_cache.go
│   │   ├── schema_embed.go       # ✅ Embeds prescribe, report, get_event schemas
│   │   ├── server_test.go        # ✅ 5 tests
│   │   └── schemas/
│   │       ├── prescribe.schema.json  # ✅ actor, tool, operation, raw_artifact,
│   │       │                          #    canonical_action (optional), actor_meta (optional)
│   │       ├── report.schema.json     # ✅ prescription_id, exit_code, artifact_digest,
│   │       │                          #    external_refs (optional)
│   │       └── get_event.schema.json  # ✅ event_id
│   ├── evidence/                  # Copied from source
│   │   ├── io.go                 # ✅ Copied
│   │   ├── segment.go            # ✅ Copied
│   │   ├── resource_links.go     # ✅ Copied
│   │   └── forwarder.go          # ✅ Copied
│   ├── evlock/                    # Copied from source
│   │   └── evlock.go             # ✅ Copied
│   ├── invocation/                # Copied + simplified
│   │   └── invocation.go         # ✅ Actor struct, no OPA validation
│   └── version/                   # Copied from source
│       └── version.go            # ✅ Copied
├── prompts/
│   └── mcpserver/
│       ├── initialize/
│       │   └── instructions.txt       # ✅ Inspector model (no deny/block language)
│       ├── tools/
│       │   ├── prescribe_description.txt  # ✅ Record intent before operations
│       │   └── report_description.txt     # ✅ Report result after operations
│       └── resources/
│           └── content/
│               └── agent_contract_v1.md   # ✅ Inspector contract (no deny/stop/block)
├── tests/
│   └── golden/                    # Golden corpus for canonicalization
│       ├── k8s_*.yaml             # ✅ K8s test fixtures
│       ├── k8s_*.json             # ✅ K8s action snapshots
│       ├── tf_*.json              # ✅ Terraform test fixtures + action snapshots
│       └── *.txt                  # ✅ Expected digests
└── docs/
    └── system-design/             # Architecture and design documents
```

---

## 5. Post-Migration Architecture Changes

### 5.1 Adapter Interface (internal/canon/types.go)

Added `Adapter` interface to formalize adapter dispatch:

```go
type Adapter interface {
    Name() string                    // "k8s/v1", "tf/v1", "generic/v1"
    CanHandle(tool string) bool
    Canonicalize(tool, operation string, rawArtifact []byte) (CanonResult, error)
}
```

- `K8sAdapter` — handles "kubectl", "oc"
- `TerraformAdapter` — handles "terraform"
- `GenericAdapter` — fallback, always matches
- `SelectAdapter(tool, adapters)` — returns first matching adapter
- `DefaultAdapters()` — returns built-in chain [k8s, terraform, generic]
- `Canonicalize()` convenience function uses `SelectAdapter` internally

### 5.2 Pre-Canonicalized Prescribe Path

`PrescribeInput` has optional `CanonicalAction *canon.CanonicalAction`.
When provided, the adapter step is skipped:

- `ArtifactDigest` = SHA256 of raw artifact bytes
- `IntentDigest` = SHA256 of canonical action JSON
- `CanonVersion` = `"external/v1"`
- Risk detectors still run on raw artifact
- Evidence chain and signals work identically

This lets any tool integrate without writing an Evidra adapter.

### 5.3 Exported SHA256Hex

`canon.SHA256Hex(data []byte) string` exported from generic.go
for use by the pre-canonicalized path and external consumers.

---

## 6. What Was Dropped

```
SOURCE                          WHY DROPPED
pkg/policy/                     OPA engine wrapper → replaced by risk/matrix.go
pkg/policysource/               OPA bundle loading → no bundles
pkg/bundlesource/               OPA bundle source → no bundles
pkg/runtime/                    OPA evaluator → no OPA
pkg/mode/                       Enforce/observe modes → always inspector
pkg/scenario/                   Policy simulation → no policies
pkg/outputlimit/                Policy output limiting → no policies
pkg/validate/                   OPA input validation → simplified
pkg/tokens/                     Token management → defer
pkg/config/                     Complex config → simplified for v0.3.0
policy/                         All .rego files (67 files, 4153 lines)
bundleembed.go                  OPA bundle embedding
promptsembed.go                 Prompt embedding (for old model)
skills/                         MCP skills (rewrite for benchmark)
ui/                             React dashboard → replaced by CLI scorecard
internal/api/                   HTTP API → defer, CLI first
internal/db/                    Database layer → defer, JSONL first
internal/store/                 KV store → defer
internal/auth/                  Auth middleware → defer
cmd/evidra-api/                 API server → defer to v0.5.0
cmd/evidra/policy_sim_cmd.go    Policy simulation CLI → dropped
tests/corpus/                   OPA corpus tests → replaced by golden/
tests/e2e/                      OPA e2e tests → new e2e for benchmark
tests/golden_real/              OPA golden tests → replaced
tests/inspector/                OPA inspector tests → replaced
scripts/                        OPA-related scripts → new scripts
examples/                       OPA examples → new examples
docs/ENGINE_LOGIC_V2.md         OPA engine docs → superseded
docs/ENGINE_V3_DOMAIN_ADAPTERS.md  Partially relevant → cherry-picked ideas
POLICY_CATALOG.md               Policy catalog → replaced by detectors list
```

---

## 7. Operational Files (added post-migration)

| File | Purpose |
|---|---|
| `Dockerfile` | MCP server binary (distroless, no OPA bundle) |
| `Dockerfile.cli` | CLI binary for CI runners |
| `server.json` | MCP registry manifest (v0.3.0, `io.github.vitas/evidra-benchmark`) |
| `Makefile` | build, test, golden-update, docker-mcp, docker-cli, fmt, lint, tidy, clean |
| `docker-compose.yml` | Local dev: MCP + evidence volume, no Postgres |
| `prompts/mcpserver/` | Agent prompts rewritten for inspector model |

---

## 8. Migration Verification (all passing)

```bash
# 1. Compiles without OPA ✅
go build ./...

# 2. No Rego files ✅
find . -name "*.rego" | wc -l    # 0

# 3. No OPA imports ✅
grep -r "open-policy-agent" . --include="*.go" | wc -l    # 0

# 4. No policy/runtime/mode packages ✅
ls internal/policy pkg/policy pkg/runtime pkg/mode 2>/dev/null    # all fail

# 5. Evidence chain works ✅
go test ./internal/evidence/... -v

# 6. Import paths updated ✅
grep -r "samebits.com/evidra\"" . --include="*.go" | wc -l    # 0

# 7. Golden corpus exists ✅
ls tests/golden/*.yaml tests/golden/*.json | wc -l    # >= 10

# 8. All tests pass with race detector ✅
go test -race ./... -count=1

# 9. Adapter interface exists ✅
grep "type Adapter interface" internal/canon/types.go

# 10. Pre-canonicalized path exists ✅
grep "CanonicalAction" pkg/mcpserver/server.go

# 11. No validate tool (old OPA tool) ✅
grep -r '"validate"' pkg/mcpserver/ --include="*.go"    # nothing

# 12. No deny/stop/block in agent contract ✅
grep -L "deny\|STOP\|block" prompts/mcpserver/resources/content/agent_contract_v1.md

# 13. server.json updated ✅
grep "reliability benchmark" server.json

# 14. Dockerfiles exist ✅
ls Dockerfile Dockerfile.cli

# 15. MCP schemas exist ✅
ls pkg/mcpserver/schemas/{prescribe,report,get_event}.schema.json
```

---

## 9. Phase Execution Summary

```
Phase 1 (compiles, evidence works):              ✅ COMPLETE
  - go.mod, internal/evidence/, pkg/evidence/,
    pkg/invocation/, pkg/version/, pkg/evlock/

Phase 2 (canonicalization works):                 ✅ COMPLETE
  - internal/canon/ (types, k8s, terraform, generic, noise)
  - tests/golden/ (k8s + terraform fixtures)

Phase 3 (risk analysis works):                    ✅ COMPLETE
  - internal/risk/ (matrix, 7 detectors, 24 tests)

Phase 4 (MCP server works):                       ✅ COMPLETE
  - pkg/mcpserver/ (server, retry_tracker, schema_embed, schemas)
  - cmd/evidra-mcp/ (CLI wiring)

Phase 5 (signals + scorecard):                    ✅ COMPLETE
  - internal/signal/ (5 signals, 14 tests)
  - internal/score/ (scoring, compare, 8 tests)
  - cmd/evidra/ (scorecard, compare, prescribe, report commands)

Post-migration (architecture update):             ✅ COMPLETE
  - Adapter interface + SelectAdapter()
  - Pre-canonicalized prescribe path (CanonicalAction)
  - Operational files (Dockerfile, Makefile, server.json,
    docker-compose.yml, prompts/, get_event schema)
```

---

## 10. Test Coverage

| Package | Tests | Notes |
|---|---|---|
| `internal/canon` | golden corpus | K8s, Terraform, generic, multi-doc, noise |
| `internal/evidence` | signer, payload, builder | Copied from source |
| `internal/risk` | 24 tests | All 7 detectors + matrix |
| `internal/signal` | 14 tests | All 5 signals |
| `internal/score` | 8 tests | Scoring bands, workload overlap |
| `pkg/mcpserver` | 5 tests | Prescribe, report, retry tracker |

Total: 109 tests, all passing with `-race`.
