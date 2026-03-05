# Pre-Release Domain Model Alignment

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Make breaking domain model changes while there are zero customers and no public repo — close normative gaps, add missing fields from the session/operation model, and align with CNCF standards.

**Architecture:** Six targeted changes across the evidence entry model, scorecard, and signing. Each change updates struct definitions, all callers, all tests, and normative docs. No new packages or external dependencies.

**Tech Stack:** Go 1.24, existing packages only. No new dependencies.

**Changes summary:**

| # | Change | Effort |
|---|--------|--------|
| 1 | Embed `Confidence` in `Scorecard` struct | Near-free |
| 2 | Enforce signing (require `Signer`, fail without it) | Near-free |
| 3 | Add `session_start`, `session_end`, `annotation` entry types | Small |
| 4 | Add `operation_id` + `attempt` fields to `EvidenceEntry` | Medium |
| 5 | Add `tool_version` to `FindingPayload` | Small |
| 6 | Update normative docs to match all changes | Small |

---

## Task 1: Embed Confidence in Scorecard

**Files:**
- Modify: `internal/score/score.go:17-26` — add `Confidence` field to `Scorecard`
- Modify: `internal/score/score.go:29-76` — call `ComputeConfidence` inside `Compute`
- Modify: `internal/score/score_test.go` — update test assertions
- Modify: `cmd/evidra/main.go:117-135` — remove manual confidence wiring if any
- Modify: `cmd/evidra/main_test.go:304-314` — update scorecard assertions
- Modify: `tests/e2e/session_scoring_test.go:93-150` — update E2E assertions

### Step 1: Write the failing test

Add a test in `internal/score/score_test.go` that asserts `Confidence` is populated on the returned `Scorecard`:

```go
func TestCompute_IncludesConfidence(t *testing.T) {
	t.Parallel()
	results := []signal.SignalResult{
		{Name: "protocol_violation", Count: 1},
	}
	sc := Compute(results, 200, 0.0) // 0.0 = externalPct
	if sc.Confidence.Level == "" {
		t.Fatal("expected confidence to be set on scorecard")
	}
	if sc.Confidence.Level != "high" {
		t.Errorf("expected confidence level high, got %s", sc.Confidence.Level)
	}
}
```

**Run:** `go test ./internal/score/ -run TestCompute_IncludesConfidence -v`
**Expected:** FAIL — `Compute` does not accept `externalPct` parameter yet.

### Step 2: Update Scorecard struct and Compute signature

Modify `internal/score/score.go`:

1. Add `Confidence` field to `Scorecard`:

```go
type Scorecard struct {
	TotalOperations int                `json:"total_operations"`
	Signals         map[string]int     `json:"signals"`
	Rates           map[string]float64 `json:"rates"`
	Penalty         float64            `json:"penalty"`
	Score           float64            `json:"score"`
	Band            string             `json:"band"`
	Sufficient      bool               `json:"sufficient"`
	Confidence      Confidence         `json:"confidence"`
}
```

2. Change `Compute` signature to accept `externalPct`:

```go
func Compute(results []signal.SignalResult, totalOps int, externalPct float64) Scorecard {
```

3. At the end of `Compute`, before return, compute and embed confidence:

```go
	violationRate := sc.Rates["protocol_violation"]
	sc.Confidence = ComputeConfidence(externalPct, violationRate)
	return sc
```

### Step 3: Fix all callers of Compute

Every call to `score.Compute(results, totalOps)` must now pass `externalPct`.

**File: `cmd/evidra/main.go:115`**

Currently: `sc := score.Compute(results, totalOps)`

Add `externalPct` calculation before the call. Count entries with `canon_source=external` from the prescription payloads:

```go
	externalPct := computeExternalPct(signalEntries)
	sc := score.Compute(results, totalOps, externalPct)
```

Add helper near the existing `countPrescriptions` function:

```go
func computeExternalPct(entries []signal.Entry) float64 {
	var total, external int
	for _, e := range entries {
		if e.IsPrescription {
			total++
			if e.CanonSource == "external" {
				external++
			}
		}
	}
	if total == 0 {
		return 0
	}
	return float64(external) / float64(total)
}
```

Note: `signal.Entry` does not yet have `CanonSource`. For now, pass `0.0` as `externalPct` — all current entries are adapter-canonicalized. Add a `// TODO: extract canon_source from prescription payload` comment. The field can be threaded through in a follow-up.

Simpler approach — just pass `0.0`:

```go
	sc := score.Compute(results, totalOps, 0.0)
```

**File: `cmd/evidra/main.go` (compare command)** — find all other `score.Compute` calls and add `0.0` third argument.

**File: `tests/e2e/session_scoring_test.go`** — find `score.Compute` calls and add `0.0`.

### Step 4: Fix all existing tests

Update every test in `internal/score/score_test.go` that calls `Compute` to pass the third argument `0.0`:

```go
sc := Compute(results, totalOps, 0.0)
```

Update assertions that check struct equality to include the `Confidence` field.

### Step 5: Run all tests

**Run:** `go test ./internal/score/ -v -count=1`
**Expected:** All PASS, including the new `TestCompute_IncludesConfidence`.

**Run:** `go test ./... -count=1`
**Expected:** All PASS (no broken callers).

### Step 6: Format and commit

```bash
gofmt -w internal/score/score.go internal/score/score_test.go cmd/evidra/main.go
git add internal/score/score.go internal/score/score_test.go cmd/evidra/main.go cmd/evidra/main_test.go tests/e2e/session_scoring_test.go
git commit -m "$(cat <<'EOF'
feat: embed Confidence in Scorecard struct

Closes normative gap — EVIDRA_CORE_DATA_MODEL.md §7 requires confidence
as a MUST field. Compute now accepts externalPct and embeds confidence
in the returned Scorecard.
EOF
)"
```

---

## Task 2: Enforce signing (require Signer)

**Files:**
- Modify: `cmd/evidra/main.go:707-727` — `resolveSigner` fails if no key configured
- Modify: `cmd/evidra-mcp/main.go:50-54` — same pattern
- Modify: `pkg/evidence/entry_builder.go:57,95-98` — make Signer required in EntryBuildParams
- Modify: `pkg/evidence/entry_builder.go:106-118` — RehashEntry requires signer
- Modify: `pkg/evidence/entry_builder_test.go` — all tests must provide a signer
- Modify: `pkg/evidence/entry_store_test.go` — tests that build entries must provide a signer
- Modify: `pkg/mcpserver/integration_test.go` — tests must provide a signer
- Modify: `tests/e2e/*.go` — E2E tests must configure a signer

### Step 1: Create a test helper signer

Add to `pkg/evidence/entry_builder_test.go` (or reuse if exists) a deterministic test signer:

```go
func testSigner(t *testing.T) Signer {
	t.Helper()
	pub, priv, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatalf("generate test key: %v", err)
	}
	return &testSignerImpl{priv: priv, pub: pub}
}

type testSignerImpl struct {
	priv ed25519.PrivateKey
	pub  ed25519.PublicKey
}

func (s *testSignerImpl) Sign(payload []byte) []byte {
	return ed25519.Sign(s.priv, payload)
}

func (s *testSignerImpl) Verify(payload, sig []byte) bool {
	return ed25519.Verify(s.pub, payload, sig)
}

func (s *testSignerImpl) PublicKey() ed25519.PublicKey {
	return s.pub
}
```

Check if a similar helper already exists in the test files — reuse it if so.

### Step 2: Write failing test for BuildEntry without Signer

```go
func TestBuildEntry_RequiresSigner(t *testing.T) {
	t.Parallel()
	_, err := BuildEntry(EntryBuildParams{
		Type:    EntryTypePrescribe,
		TraceID: "trace-1",
		Payload: json.RawMessage(`{}`),
		// Signer intentionally nil
	})
	if err == nil {
		t.Fatal("expected error when Signer is nil")
	}
}
```

**Run:** `go test ./pkg/evidence/ -run TestBuildEntry_RequiresSigner -v`
**Expected:** FAIL — BuildEntry currently allows nil Signer.

### Step 3: Make Signer required in BuildEntry

In `pkg/evidence/entry_builder.go`, at the start of `BuildEntry`, add:

```go
	if p.Signer == nil {
		return EvidenceEntry{}, fmt.Errorf("evidence.BuildEntry: Signer is required")
	}
```

Remove the nil check around signing (lines 95-98), make it unconditional:

```go
	sig := p.Signer.Sign([]byte(hash))
	entry.Signature = base64.StdEncoding.EncodeToString(sig)
```

### Step 4: Make Signer required in RehashEntry

Change `RehashEntry` signature — `signer` is no longer a pointer/optional:

```go
func RehashEntry(entry *EvidenceEntry, signer Signer) error {
	if signer == nil {
		return fmt.Errorf("evidence.RehashEntry: Signer is required")
	}
```

Remove the nil check around signing (lines 114-117), make it unconditional.

### Step 5: Update resolveSigner in CLI

In `cmd/evidra/main.go:707-727`, change `resolveSigner` to return an error when no key is found:

```go
func resolveSigner(keyBase64, keyPath string) (evidence.Signer, error) {
	if keyBase64 == "" {
		keyBase64 = strings.TrimSpace(os.Getenv("EVIDRA_SIGNING_KEY"))
	}
	if keyPath == "" {
		keyPath = strings.TrimSpace(os.Getenv("EVIDRA_SIGNING_KEY_PATH"))
	}
	if keyBase64 == "" && keyPath == "" {
		return nil, fmt.Errorf("signing key required: set --signing-key, --signing-key-path, EVIDRA_SIGNING_KEY, or EVIDRA_SIGNING_KEY_PATH")
	}
	// ... rest unchanged
}
```

### Step 6: Update resolveSigner in MCP server

Same change in `cmd/evidra-mcp/main.go` — `resolveSigner` returns error if no key.

### Step 7: Fix all tests

Every test that calls `BuildEntry` or creates a `BenchmarkService` must now provide a signer. Use the `testSigner(t)` helper. This includes:

- `pkg/evidence/entry_builder_test.go`
- `pkg/evidence/entry_store_test.go`
- `pkg/evidence/chain_test.go`
- `pkg/mcpserver/integration_test.go`
- `pkg/mcpserver/e2e_test.go`
- `tests/e2e/findings_test.go`
- `tests/e2e/session_scoring_test.go`
- `cmd/evidra/main_test.go`

For test files in different packages that can't access the helper, create a shared `internal/testutil/signer.go`:

```go
package testutil

import (
	"crypto/ed25519"
	"testing"

	"samebits.com/evidra-benchmark/pkg/evidence"
)

// TestSigner returns a deterministic Ed25519 signer for testing.
func TestSigner(t *testing.T) evidence.Signer {
	t.Helper()
	_, priv, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatalf("generate test key: %v", err)
	}
	return &testSigner{priv: priv}
}

type testSigner struct {
	priv ed25519.PrivateKey
}

func (s *testSigner) Sign(payload []byte) []byte {
	return ed25519.Sign(s.priv, payload)
}

func (s *testSigner) Verify(payload, sig []byte) bool {
	pub := s.priv.Public().(ed25519.PublicKey)
	return ed25519.Verify(pub, payload, sig)
}

func (s *testSigner) PublicKey() ed25519.PublicKey {
	return s.priv.Public().(ed25519.PublicKey)
}
```

### Step 8: Run all tests

**Run:** `go test ./... -v -count=1`
**Expected:** All PASS.

### Step 9: Format and commit

```bash
gofmt -w pkg/evidence/entry_builder.go internal/testutil/signer.go
git add pkg/evidence/entry_builder.go pkg/evidence/entry_builder_test.go \
       pkg/evidence/entry_store_test.go pkg/evidence/chain_test.go \
       pkg/mcpserver/integration_test.go pkg/mcpserver/e2e_test.go \
       tests/e2e/ cmd/evidra/main.go cmd/evidra/main_test.go \
       cmd/evidra-mcp/main.go internal/testutil/
git commit -m "$(cat <<'EOF'
feat: enforce signing — Signer is now required

BuildEntry and RehashEntry return error if Signer is nil.
CLI and MCP server fail early if no signing key is configured.
Closes normative gap — EVIDRA_CORE_DATA_MODEL.md §5 requires
signature as MUST field on every EvidenceEntry.
EOF
)"
```

---

## Task 3: Add session_start, session_end, annotation entry types

**Files:**
- Modify: `pkg/evidence/entry.go:11-34` — add three new EntryType constants and register in validEntryTypes
- Modify: `pkg/evidence/payloads.go` — add `SessionStartPayload`, `SessionEndPayload`, `AnnotationPayload`
- Create: `pkg/evidence/entry_test.go` (if needed) — test new types are valid
- Modify: `internal/pipeline/bridge.go:61-63` — update skip comment to list new types

### Step 1: Write the failing test

In `pkg/evidence/entry_test.go` (create if needed):

```go
func TestEntryType_NewTypesAreValid(t *testing.T) {
	t.Parallel()
	newTypes := []evidence.EntryType{
		evidence.EntryTypeSessionStart,
		evidence.EntryTypeSessionEnd,
		evidence.EntryTypeAnnotation,
	}
	for _, et := range newTypes {
		if !et.Valid() {
			t.Errorf("expected %q to be valid", et)
		}
	}
}
```

**Run:** `go test ./pkg/evidence/ -run TestEntryType_NewTypesAreValid -v`
**Expected:** FAIL — constants don't exist yet.

### Step 2: Add entry type constants

In `pkg/evidence/entry.go`, add after `EntryTypeCanonFailure`:

```go
	// EntryTypeSessionStart marks the beginning of a session.
	EntryTypeSessionStart EntryType = "session_start"
	// EntryTypeSessionEnd marks the end of a session.
	EntryTypeSessionEnd EntryType = "session_end"
	// EntryTypeAnnotation is a human or system annotation on a session.
	EntryTypeAnnotation EntryType = "annotation"
```

Add to `validEntryTypes` map:

```go
	EntryTypeSessionStart: true,
	EntryTypeSessionEnd:   true,
	EntryTypeAnnotation:   true,
```

### Step 3: Add payload structs

In `pkg/evidence/payloads.go`, add:

```go
// SessionStartPayload is the typed payload for EntryTypeSessionStart entries.
type SessionStartPayload struct {
	Labels map[string]string `json:"labels,omitempty"`
}

// SessionEndPayload is the typed payload for EntryTypeSessionEnd entries.
type SessionEndPayload struct {
	Status string `json:"status"` // "completed", "aborted", "error"
}

// AnnotationPayload is the typed payload for EntryTypeAnnotation entries.
type AnnotationPayload struct {
	Key     string `json:"key"`
	Value   string `json:"value"`
	Message string `json:"message,omitempty"`
}
```

### Step 4: Update bridge.go skip comment

In `internal/pipeline/bridge.go:61-63`, update the comment:

```go
		default:
			// Skip finding, signal, receipt, canonicalization_failure, session_start, session_end, annotation entries
			continue
```

### Step 5: Run all tests

**Run:** `go test ./... -v -count=1`
**Expected:** All PASS.

### Step 6: Format and commit

```bash
gofmt -w pkg/evidence/entry.go pkg/evidence/payloads.go internal/pipeline/bridge.go
git add pkg/evidence/entry.go pkg/evidence/entry_test.go pkg/evidence/payloads.go internal/pipeline/bridge.go
git commit -m "$(cat <<'EOF'
feat: add session_start, session_end, annotation entry types

Extends the entry type enum with three new types required by the
session/operation event model. Adds corresponding payload structs.
Enables CloudEvents evidra.session.start/end without spec version caveat.
EOF
)"
```

---

## Task 4: Add operation_id and attempt to EvidenceEntry

**Files:**
- Modify: `pkg/evidence/entry.go:52-73` — add `OperationID` and `Attempt` fields
- Modify: `pkg/evidence/entry_builder.go:40-58` — add fields to `EntryBuildParams`
- Modify: `pkg/evidence/entry_builder.go:60-101` — thread fields into `BuildEntry`
- Modify: `pkg/evidence/entry_builder.go:122-142` — add to `hashableEntry`
- Modify: `pkg/mcpserver/server.go` — thread `operation_id` through prescribe/report
- Modify: `pkg/mcpserver/schemas/prescribe.json` — add `operation_id` and `attempt` to schema
- Modify: `pkg/mcpserver/schemas/report.json` — add `operation_id` to schema
- Modify: `cmd/evidra/main.go` — add `--operation-id` flag to prescribe and report commands

### Step 1: Write failing test

In `pkg/evidence/entry_builder_test.go`:

```go
func TestBuildEntry_OperationIDAndAttempt(t *testing.T) {
	t.Parallel()
	entry, err := BuildEntry(EntryBuildParams{
		Type:        EntryTypePrescribe,
		TraceID:     "trace-1",
		OperationID: "op-123",
		Attempt:     2,
		Payload:     json.RawMessage(`{}`),
		Signer:      testSigner(t),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if entry.OperationID != "op-123" {
		t.Errorf("expected OperationID op-123, got %s", entry.OperationID)
	}
	if entry.Attempt != 2 {
		t.Errorf("expected Attempt 2, got %d", entry.Attempt)
	}
}
```

**Run:** `go test ./pkg/evidence/ -run TestBuildEntry_OperationIDAndAttempt -v`
**Expected:** FAIL — fields don't exist.

### Step 2: Add fields to EvidenceEntry

In `pkg/evidence/entry.go`, add after `SessionID`:

```go
	OperationID  string            `json:"operation_id,omitempty"`
	Attempt      int               `json:"attempt,omitempty"`
```

### Step 3: Add fields to EntryBuildParams

In `pkg/evidence/entry_builder.go`, add after `SessionID`:

```go
	OperationID  string
	Attempt      int
```

### Step 4: Thread through BuildEntry

In `pkg/evidence/entry_builder.go`, in the `BuildEntry` function, add to the entry construction:

```go
		OperationID:     p.OperationID,
		Attempt:         p.Attempt,
```

### Step 5: Add to hashableEntry

In `pkg/evidence/entry_builder.go`, add to `hashableEntry` struct:

```go
	OperationID  string            `json:"operation_id,omitempty"`
	Attempt      int               `json:"attempt,omitempty"`
```

And in `computeEntryHash`, add:

```go
		OperationID:     e.OperationID,
		Attempt:         e.Attempt,
```

### Step 6: Run tests

**Run:** `go test ./pkg/evidence/ -v -count=1`
**Expected:** All PASS.

**Important:** Adding fields to `hashableEntry` changes hash computation. This will break any golden files or tests that assert specific hash values. Check:
- `pkg/evidence/chain_test.go` — may have hardcoded hashes
- `pkg/evidence/entry_builder_test.go` — may assert specific hash values

If tests compare exact hashes, update them. Since the fields are `omitempty`, entries without `operation_id`/`attempt` will produce identical hashes as before.

### Step 7: Thread through MCP server prescribe input

In `pkg/mcpserver/server.go`, find the `PrescribeInput` struct and add:

```go
	OperationID  string `json:"operation_id,omitempty"`
	Attempt      int    `json:"attempt,omitempty"`
```

Thread to `EntryBuildParams` where `BuildEntry` is called for prescribe entries.

Do the same for the report path — `ReportInput` should accept `operation_id`.

### Step 8: Update JSON schemas

In `pkg/mcpserver/schemas/prescribe.json`, add to `properties`:

```json
"operation_id": {
  "type": "string",
  "description": "Operation identifier, unique within a session"
},
"attempt": {
  "type": "integer",
  "description": "Retry attempt counter for this operation",
  "minimum": 0
}
```

In `pkg/mcpserver/schemas/report.json`, add `operation_id` similarly.

### Step 9: Add CLI flags

In `cmd/evidra/main.go`, add `--operation-id` flag to the prescribe and report subcommands. Thread to `EntryBuildParams`.

### Step 10: Run full test suite

**Run:** `go test ./... -v -count=1`
**Expected:** All PASS.

### Step 11: Format and commit

```bash
gofmt -w pkg/evidence/entry.go pkg/evidence/entry_builder.go pkg/mcpserver/server.go cmd/evidra/main.go
git add pkg/evidence/entry.go pkg/evidence/entry_builder.go pkg/evidence/entry_builder_test.go \
       pkg/mcpserver/server.go pkg/mcpserver/schemas/ cmd/evidra/main.go
git commit -m "$(cat <<'EOF'
feat: add operation_id and attempt to EvidenceEntry

Session/operation model requires operation_id (MUST) and attempt (SHOULD)
on operation-scoped events. Fields are omitempty — existing entries
without these fields remain hash-compatible.
EOF
)"
```

---

## Task 5: Add tool_version to FindingPayload

**Files:**
- Modify: `pkg/evidence/payloads.go:60-66` — add `ToolVersion` field
- Modify: `cmd/evidra/main.go` — thread `--tool-version` flag in ingest-findings
- Modify: `pkg/mcpserver/server.go` — thread in MCP findings path if applicable
- Modify: `cmd/evidra/main_test.go` — update finding test assertions

### Step 1: Write failing test

In a test file for payloads (or `entry_builder_test.go`):

```go
func TestFindingPayload_ToolVersion(t *testing.T) {
	t.Parallel()
	p := evidence.FindingPayload{
		Tool:        "trivy",
		ToolVersion: "0.50.0",
		RuleID:      "CVE-2023-1234",
		Severity:    "high",
		Resource:    "container:nginx",
		Message:     "vulnerable image",
	}
	data, err := json.Marshal(p)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), `"tool_version":"0.50.0"`) {
		t.Errorf("expected tool_version in JSON, got %s", data)
	}
}
```

**Run:** `go test ./pkg/evidence/ -run TestFindingPayload_ToolVersion -v`
**Expected:** FAIL — `ToolVersion` field doesn't exist.

### Step 2: Add field

In `pkg/evidence/payloads.go`, update `FindingPayload`:

```go
type FindingPayload struct {
	Tool        string `json:"tool"`
	ToolVersion string `json:"tool_version,omitempty"`
	RuleID      string `json:"rule_id"`
	Severity    string `json:"severity"`
	Resource    string `json:"resource"`
	Message     string `json:"message"`
}
```

### Step 3: Thread through CLI

In `cmd/evidra/main.go`, find the `ingest-findings` command. Add `--tool-version` flag and thread it to `FindingPayload.ToolVersion`.

### Step 4: Thread through SARIF ingestion

In the SARIF ingestion path (if it exists in `cmd/evidra/main.go`), extract `run.tool.driver.version` and map to `ToolVersion`. This aligns with the SARIF mapping table in the CNCF alignment doc.

### Step 5: Run tests

**Run:** `go test ./... -v -count=1`
**Expected:** All PASS.

### Step 6: Format and commit

```bash
gofmt -w pkg/evidence/payloads.go cmd/evidra/main.go
git add pkg/evidence/payloads.go cmd/evidra/main.go cmd/evidra/main_test.go
git commit -m "$(cat <<'EOF'
feat: add tool_version to FindingPayload

SARIF mapping requires run.tool.driver.version -> tool_version.
Field is omitempty — backward compatible with existing findings.
EOF
)"
```

---

## Task 6: Update normative docs

**Files:**
- Modify: `docs/system-design/EVIDRA_CORE_DATA_MODEL.md` — update §5 entry types table, §7 Scorecard, §4 FindingPayload
- Modify: `docs/system-design/EVIDRA_CNCF_STANDARDS_ALIGNMENT.md` — remove gap notes, update confidence and signing sections
- Modify: `docs/system-design/EVIDRA_PROTOCOL.md` — add `operation_id` and `attempt` to event fields
- Modify: `docs/system-design/evidra-session-operation-event-model-v1.md` — note that session_start/end/annotation are now implemented entry types

### Step 1: Update EVIDRA_CORE_DATA_MODEL.md

**§5 Entry Types table** — add three rows:

```markdown
| `session_start` | Session begins | Labels |
| `session_end` | Session ends | Status |
| `annotation` | Human or system annotation | Key, value, message |
```

**§5 EvidenceEntry table** — add:

```markdown
| operation_id | string | MAY | Operation identifier, unique within session |
| attempt | integer | MAY | Retry attempt counter |
```

**§4 ValidatorFinding table** — add:

```markdown
| tool_version | string | MAY | Scanner version |
```

**§7 Scorecard** — note that `confidence` is now embedded (MUST, implemented).

### Step 2: Update EVIDRA_CNCF_STANDARDS_ALIGNMENT.md

Remove or update the following gap notes:

1. **Confidence gap** (around line 314) — change to: "The normative model and runtime are now aligned — `Confidence` is embedded in the `Scorecard` struct."

2. **Signing gap** (around line 240) — change to: "Ed25519 signing is required. `BuildEntry` fails if no `Signer` is configured. This satisfies both the normative model and in-toto export requirements."

3. **Session lifecycle types** (around line 76) — update the table to remove "No internal entry type exists yet" for `evidra.session.start`, `evidra.session.end`, `evidra.annotation`:

```markdown
| `evidra.session.start` | `session_start` | Maps 1:1 |
| `evidra.session.end` | `session_end` | Maps 1:1 |
| `evidra.annotation` | `annotation` | Maps 1:1 |
```

Remove the "Important" note about these not having internal types.

### Step 3: Update EVIDRA_PROTOCOL.md

In §2 Event types list, add:

```
session_start
session_end
annotation
```

In §3 Correlation Model table, add `operation_id` and `attempt`.

### Step 4: Update session/operation event model

In §3.1 IDs table, add a note that `operation_id` and `attempt` are now fields on `EvidenceEntry`.

### Step 5: Commit

```bash
git add docs/system-design/EVIDRA_CORE_DATA_MODEL.md \
       docs/system-design/EVIDRA_CNCF_STANDARDS_ALIGNMENT.md \
       docs/system-design/EVIDRA_PROTOCOL.md \
       docs/system-design/evidra-session-operation-event-model-v1.md
git commit -m "$(cat <<'EOF'
docs: update normative docs for domain model alignment

Reflects all pre-release changes: embedded confidence, required signing,
new entry types (session_start, session_end, annotation), operation_id,
attempt, tool_version. Removes gap notes from CNCF alignment doc.
EOF
)"
```

---

## Task 7: Final verification

### Step 1: Build all binaries

```bash
go build ./cmd/evidra/ && go build ./cmd/evidra-mcp/
```

**Expected:** Both build without errors.

### Step 2: Run full test suite

```bash
go test ./... -v -count=1
```

**Expected:** All tests pass.

### Step 3: Race detector

```bash
go test -race ./...
```

**Expected:** No races.

### Step 4: Format check

```bash
gofmt -l .
```

**Expected:** No output (all files formatted).

### Step 5: Lint

```bash
golangci-lint run
```

**Expected:** No new issues.
