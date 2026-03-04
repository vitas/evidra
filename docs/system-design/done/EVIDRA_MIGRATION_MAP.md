# Evidra Benchmark — Migration Map from v0.2.0

## Purpose
This document is the source of truth for creating the new
`evidra-benchmark` project from the existing `evidra` v0.2.0 codebase.
It specifies exactly what to COPY, what to DROP, and what to CREATE NEW.

Claude Code reads this document and executes the migration.

---

## 1. Source and Target

```
SOURCE: evidra/                    (v0.2.0-rc5, OPA-based policy engine)
TARGET: {NEW_PROJECT_DIR}/         (benchmark product, no OPA)
```

The source is NOT modified. The target is a new Go module.

---

## 2. New Module Identity

```
module: samebits.com/evidra-benchmark
go version: 1.24.6
```

---

## 3. Dependencies

### KEEP (copy from source go.mod)
```
github.com/modelcontextprotocol/go-sdk v1.3.1
github.com/oklog/ulid/v2 v2.1.1
go.yaml.in/yaml/v3 v3.0.4
```

### ADD NEW
```
k8s.io/apimachinery   (latest stable — for unstructured.Unstructured)
github.com/hashicorp/terraform-json  (latest stable)
```

### DROP (do NOT include)
```
github.com/open-policy-agent/opa   ← entire OPA ecosystem gone
```

All transitive deps of OPA will disappear automatically.

---

## 4. Directory Structure

```
{NEW_PROJECT_DIR}/
├── go.mod
├── go.sum
├── cmd/
│   ├── evidra/                    # CLI: scorecard, compare, fleet, prescribe, report
│   │   └── main.go               # CREATE NEW (minimal, wire commands)
│   ├── evidra-mcp/               # MCP server for AI agents
│   │   └── main.go               # COPY + SIMPLIFY from source
│   └── evidra-api/               # Backend server (v0.5.0, stub for now)
│       └── main.go               # STUB (placeholder, not implemented in v0.3.0)
├── internal/
│   ├── canon/                     # NEW: domain adapters + canonicalization
│   │   ├── types.go              # CREATE NEW: CanonicalAction, CanonResult
│   │   ├── k8s.go                # CREATE NEW: K8s adapter (from intent.go extraction logic)
│   │   ├── terraform.go          # CREATE NEW: Terraform adapter
│   │   ├── generic.go            # CREATE NEW: generic fallback adapter
│   │   ├── noise.go              # CREATE NEW: frozen noise lists
│   │   └── canon_test.go         # CREATE NEW: golden corpus tests
│   ├── evidence/                  # COPY from source internal/evidence/
│   │   ├── signer.go             # COPY as-is
│   │   ├── signer_test.go        # COPY as-is
│   │   ├── payload.go            # COPY + EXTEND (add canon fields)
│   │   ├── payload_test.go       # COPY + EXTEND
│   │   ├── builder.go            # COPY as-is
│   │   ├── builder_test.go       # COPY as-is
│   │   └── types.go              # COPY + EXTEND (add prescription/report/protocol types)
│   ├── risk/                      # CREATE NEW: risk matrix + catastrophic detectors
│   │   ├── matrix.go             # CREATE NEW (~30 lines)
│   │   ├── detectors.go          # CREATE NEW (~200 lines, ported from Rego)
│   │   └── risk_test.go          # CREATE NEW
│   ├── signal/                    # CREATE NEW: 5 behavioral signals
│   │   ├── protocol_violation.go # CREATE NEW
│   │   ├── artifact_drift.go     # CREATE NEW
│   │   ├── retry_loop.go         # CREATE NEW
│   │   ├── blast_radius.go       # CREATE NEW
│   │   ├── new_scope.go          # CREATE NEW
│   │   └── signal_test.go        # CREATE NEW
│   └── score/                     # CREATE NEW: scorecard computation
│       ├── score.go              # CREATE NEW
│       ├── compare.go            # CREATE NEW
│       └── score_test.go         # CREATE NEW
├── pkg/
│   ├── mcpserver/                 # COPY + REFACTOR from source pkg/mcpserver/
│   │   ├── server.go             # COPY + REFACTOR (prescribe/report only, no validate/deny)
│   │   ├── intent.go             # COPY + SPLIT (identity → canon, security → risk)
│   │   └── server_test.go        # COPY + UPDATE
│   ├── evidence/                  # COPY from source pkg/evidence/
│   │   ├── io.go                 # COPY as-is
│   │   ├── segment.go            # COPY as-is
│   │   ├── resource_links.go     # COPY as-is
│   │   └── forwarder.go          # COPY as-is
│   ├── invocation/                # COPY + SIMPLIFY from source pkg/invocation/
│   │   └── invocation.go         # COPY + SIMPLIFY (remove OPA-specific validation)
│   └── version/                   # COPY from source pkg/version/
│       └── version.go            # COPY as-is
├── tests/
│   └── golden/                    # CREATE NEW: golden corpus for canonicalization
│       ├── k8s_deployment.yaml
│       ├── k8s_deployment_digest.txt
│       ├── k8s_multidoc.yaml
│       ├── k8s_multidoc_digest.txt
│       ├── k8s_multidoc_action.json     # action snapshot
│       ├── k8s_privileged.yaml
│       ├── k8s_privileged_digest.txt
│       ├── k8s_privileged_action.json   # action snapshot
│       ├── k8s_rbac.yaml
│       ├── k8s_rbac_digest.txt
│       ├── k8s_crd.yaml
│       ├── k8s_crd_digest.txt
│       ├── tf_create.json
│       ├── tf_create_digest.txt
│       ├── tf_destroy.json
│       ├── tf_destroy_digest.txt
│       ├── tf_mixed.json
│       ├── tf_mixed_digest.txt
│       ├── tf_mixed_action.json         # action snapshot
│       ├── tf_module.json
│       ├── tf_module_digest.txt
│       ├── tf_module_action.json        # action snapshot
│       ├── helm_output.yaml
│       └── helm_output_digest.txt
└── docs/
    ├── ARCHITECTURE_OVERVIEW.md          # from design docs
    ├── CANONICALIZATION_CONTRACT_V1.md   # from design docs
    └── README.md                         # CREATE NEW
```

---

## 5. File-by-File Instructions

### 5.1 COPY AS-IS (no changes needed)

These files are copied verbatim. Only change the module import path
from `samebits.com/evidra` to `samebits.com/evidra-benchmark`.

```
SOURCE                                    → TARGET
internal/evidence/signer.go               → internal/evidence/signer.go
internal/evidence/signer_test.go          → internal/evidence/signer_test.go
internal/evidence/builder.go              → internal/evidence/builder.go
internal/evidence/builder_test.go         → internal/evidence/builder_test.go
pkg/evidence/io.go                        → pkg/evidence/io.go
pkg/evidence/segment.go                   → pkg/evidence/segment.go
pkg/evidence/resource_links.go            → pkg/evidence/resource_links.go
pkg/evidence/forwarder.go                 → pkg/evidence/forwarder.go
pkg/version/version.go                    → pkg/version/version.go
```

For each file:
1. Copy the file
2. Replace `samebits.com/evidra` with `samebits.com/evidra-benchmark` in imports
3. Do NOT change any logic

### 5.2 COPY + EXTEND

These files are copied and then extended with new fields/types.

**internal/evidence/payload.go**
- Copy as-is
- Add fields to evidence entry types:
  - `canonicalization_version string`
  - `intent_digest string`
  - `resource_shape_hash string`
  - `canonical_action json.RawMessage`
  - `risk_level string`
  - `risk_tags []string`

**internal/evidence/types.go**
- Copy as-is
- Add new entry types: `prescription`, `report`, `protocol_entry`
- Existing types may need renaming to avoid collision

### 5.3 COPY + REFACTOR

These files need significant changes beyond import paths.

**pkg/mcpserver/server.go**
- Copy the file
- REMOVE: `validate` tool handler (this was the OPA evaluation path)
- REMOVE: all references to `runtime.Evaluator`, `policy.Engine`, `policy.Decision`
- REMOVE: deny/allow logic
- KEEP: MCP server setup, tool registration, JSON schema handling
- ADD: `prescribe` tool handler (replaces validate):
  - Accept raw artifact
  - Call canonicalization adapter
  - Compute risk matrix + detectors
  - Write prescription to evidence chain
  - Return prescription (risk_level, risk_details, digests)
- ADD: `report` tool handler:
  - Accept prescription_id, exit_code, artifact_digest
  - Match to open prescription
  - Evaluate signals
  - Write report + protocol entry to evidence chain

**pkg/mcpserver/intent.go**
- Copy the file
- SPLIT into two parts:
  - Identity extraction (namespace, kind, name, apiVersion) → moves to internal/canon/k8s.go
  - Security extraction (Images, SecurityPosture, CIDRs, IAMActions) → moves to internal/risk/detectors.go
- The SemanticIntent struct is REPLACED by CanonicalAction in internal/canon/types.go
- IntentKey function is REPLACED by intent_digest computation in internal/canon/

**pkg/mcpserver/deny_cache.go**
- Copy the file
- RENAME to retry_tracker.go
- CHANGE: instead of tracking "denied intents", track
  (intent_digest, resource_shape_hash, timestamp) tuples
- CHANGE: instead of "block if denied N times", fire retry_loop signal

**pkg/invocation/invocation.go**
- Copy the file
- REMOVE: OPA-specific key validation (allowedParamKeys, rejectUnknownKeys)
- SIMPLIFY: ToolInvocation becomes the prescribe request struct
- KEEP: Actor struct, basic field validation

### 5.4 CREATE NEW

These files don't exist in the source. Create from scratch following
the design documents.

**internal/canon/types.go**
```go
// Adapter transforms raw artifacts into canonical actions.
// Any tool can integrate by implementing this interface.
type Adapter interface {
    Name() string
    CanHandle(tool string) bool
    Canonicalize(tool, operation string, rawArtifact []byte) (CanonResult, error)
}

// CanonicalAction is the normalized representation of an infrastructure operation.
// See CANONICALIZATION_CONTRACT_V1.md §2.
type CanonicalAction struct {
    Tool              string            `json:"tool"`
    Operation         string            `json:"operation"`
    OperationClass    string            `json:"operation_class"`
    ResourceIdentity  []ResourceID      `json:"resource_identity"`
    ScopeClass        string            `json:"scope_class"`
    ResourceCount     int               `json:"resource_count"`
    ResourceShapeHash string            `json:"resource_shape_hash"`
    RiskTags          []string          `json:"risk_tags"`
}

type CanonResult struct {
    ArtifactDigest  string          // SHA256 of raw bytes
    IntentDigest    string          // SHA256 of canonical JSON
    CanonicalAction CanonicalAction
    CanonVersion    string          // e.g. "k8s/v1"
    ParseError      error           // non-nil if adapter couldn't parse
}
```

Pre-canonicalized prescribe: when the caller provides canonical_action
directly, the adapter is bypassed but CanonResult is still produced
(artifact_digest from raw bytes, intent_digest from provided action).
This lets any tool integrate without an Evidra adapter.

**internal/canon/k8s.go**
- Use k8s.io/apimachinery/pkg/apis/meta/v1/unstructured
- Parse YAML (multi-doc support)
- Remove noise fields (frozen list from contract)
- Extract identity: apiVersion, kind, namespace, name
- Sort objects by identity
- Compute resource_shape_hash from normalized spec
- Reference: existing intent.go extractK8sIntent() for field paths

**internal/canon/terraform.go**
- Use github.com/hashicorp/terraform-json
- Parse plan JSON
- Extract identity: type + name (NOT address) + actions
- Sort resource changes by identity
- Compute resource_shape_hash from sorted addresses+actions
- Reference: existing intent.go extractTerraformIntent() for field paths

**internal/canon/generic.go**
- No external library
- resource_identity = [SHA256(raw bytes)]
- resource_count = 1
- resource_shape_hash = SHA256(raw bytes)

**internal/risk/matrix.go**
- Fixed 2D table: operation_class × scope_class → risk_level
- ~30 lines of Go

**internal/risk/detectors.go**
- Port these Rego rules to pure Go:
  - From deny_host_namespace_test.rego → hostPID/hostIPC/hostNetwork detector
  - From deny_dangerous_capabilities_test.rego → privileged container detector
  - From deny_hostpath_mount_test.rego → hostPath mount detector
  - From deny_mass_delete_test.rego → mass destroy detector
  - From deny_aws_iam_wildcard_test.rego → wildcard IAM detector
  - From deny_terraform_iam_wildcard_test.rego → terraform IAM detector
  - From deny_aws_s3_no_versioning_test.rego → public S3 detector
  - From deny_argocd_dangerous_sync_test.rego → (defer to v0.5.0)
- Each detector: func(rawArtifact []byte) []string (returns risk_tags)
- Total: ~200 lines Go

**internal/signal/*.go**
- Each signal: func(chain EvidenceChain, entry Entry) SignalResult
- See EVIDRA_AGENT_RELIABILITY_BENCHMARK.md §2

**internal/score/score.go**
- Weighted penalty formula
- See EVIDRA_AGENT_RELIABILITY_BENCHMARK.md §3

---

## 6. What is DROPPED Entirely

These source directories are NOT copied. They have no equivalent
in the target.

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
pkg/config/                     Complex config → simplify for v0.3.0
policy/                         All .rego files (67 files, 4153 lines)
bundleembed.go                  OPA bundle embedding
promptsembed.go                 Prompt embedding (for old model)
prompts/                        MCP prompts (rewrite for benchmark)
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
docs/ENGINE_V3_DOMAIN_ADAPTERS.md  Partially relevant → cherry-pick ideas
Dockerfile*                     Rebuild for new binary
docker-compose.yml              Rebuild
server.json                     MCP server config → update
POLICY_CATALOG.md               Policy catalog → replaced by detectors list
```

---

## 7. Migration Verification

After migration, these checks must pass:

```bash
# 1. Compiles without OPA
cd {NEW_PROJECT_DIR}
go build ./...
# Must NOT see "open-policy-agent/opa" in go.sum

# 2. No Rego files
find . -name "*.rego" | wc -l
# Must be 0

# 3. No OPA imports
grep -r "open-policy-agent" . --include="*.go" | wc -l
# Must be 0

# 4. No policy/runtime/mode packages
ls internal/policy pkg/policy pkg/runtime pkg/mode 2>/dev/null
# Must all fail (not exist)

# 5. Evidence chain works
go test ./internal/evidence/... -v
# Must pass

# 6. Import paths updated
grep -r "samebits.com/evidra\"" . --include="*.go" | wc -l
# Must be 0 (all should be samebits.com/evidra-benchmark)

# 7. Golden corpus exists
ls tests/golden/*.yaml tests/golden/*.json | wc -l
# Must be >= 10
```

---

## 8. Priority Order

If doing this incrementally:

```
Phase 1 (compiles, evidence works):
  - go.mod
  - internal/evidence/ (COPY)
  - pkg/evidence/ (COPY)
  - pkg/invocation/ (COPY + SIMPLIFY)
  - pkg/version/ (COPY)
  → go build ./... passes
  → go test ./internal/evidence/... passes

Phase 2 (canonicalization works):
  - internal/canon/ (CREATE NEW)
  - tests/golden/ (CREATE NEW, 5 k8s cases)
  → golden corpus tests pass

Phase 3 (risk analysis works):
  - internal/risk/ (CREATE NEW)
  → detectors test pass

Phase 4 (MCP server works):
  - pkg/mcpserver/ (COPY + REFACTOR)
  - cmd/evidra-mcp/ (COPY + SIMPLIFY)
  → prescribe/report via MCP works

Phase 5 (signals + scorecard):
  - internal/signal/ (CREATE NEW)
  - internal/score/ (CREATE NEW)
  - cmd/evidra/ (CREATE NEW)
  → evidra scorecard CLI works
```
