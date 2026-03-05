# v0.3.1 Review Fixes Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Fix remaining high-priority issues from the v0.3.1 architecture review: version drift, session ID generation, findings correlation bug, protocol taxonomy cleanup, GitHub Action fixes, and `evidra keygen` command.

**Architecture:** Single source of truth for versions via `pkg/version`. Auto-generate session IDs when omitted. Fix finding entries to carry proper correlation fields. Add `evidra keygen` for signing key generation.

**Tech Stack:** Go 1.24, Ed25519 (crypto/ed25519), ULID (oklog/ulid/v2)

---

### Task 1: Bump all versions to 0.3.1 (single source of truth)

**Files:**
- Modify: `pkg/version/version.go:5`
- Modify: `cmd/evidra/main.go` (lines 131-132, 439, 496, 557, 639, 683, 909)
- Modify: `pkg/mcpserver/server.go` (lines 291, 373, 465, 527)

**Context:**
- `pkg/version/version.go` has `Version = "0.3.0"` — this is the single source of truth
- `cmd/evidra/main.go` has 8 hardcoded `"0.3.0"` strings for `SpecVersion` and `ScoringVersion`
- `pkg/mcpserver/server.go` has 4 hardcoded `"0.3.0"` strings for `SpecVersion`
- The scorecard output struct has separate `ScoringVersion` and `SpecVersion` fields hardcoded to `"0.3.0"`

**Step 1: Add SpecVersion constant to `pkg/version/version.go`**

```go
package version

var (
	Version = "0.3.1"
	Commit  = "dev"
	Date    = "dev"
)

// SpecVersion is the protocol/spec version for evidence entries.
const SpecVersion = "0.3.1"

func String() string {
	return Version
}
```

**Step 2: Replace all hardcoded `"0.3.0"` in `cmd/evidra/main.go`**

Replace every `SpecVersion: "0.3.0"` with `SpecVersion: version.SpecVersion`.
Replace `ScoringVersion: "0.3.0"` with `ScoringVersion: version.SpecVersion`.

There are 8 occurrences in `cmd/evidra/main.go`:
- Line 131: `ScoringVersion: "0.3.0"` → `ScoringVersion: version.SpecVersion`
- Line 132: `SpecVersion: "0.3.0"` → `SpecVersion: version.SpecVersion`
- Line 439: `SpecVersion: "0.3.0"` → `SpecVersion: version.SpecVersion`
- Line 496: `SpecVersion: "0.3.0"` → `SpecVersion: version.SpecVersion`
- Line 557: `SpecVersion: "0.3.0"` → `SpecVersion: version.SpecVersion`
- Line 639: `SpecVersion: "0.3.0"` → `SpecVersion: version.SpecVersion`
- Line 683: `SpecVersion: "0.3.0"` → `SpecVersion: version.SpecVersion`
- Line 909: `SpecVersion: "0.3.0"` → `SpecVersion: version.SpecVersion`

**Step 3: Replace all hardcoded `"0.3.0"` in `pkg/mcpserver/server.go`**

Replace every `SpecVersion: "0.3.0"` with `SpecVersion: version.SpecVersion`.

There are 4 occurrences:
- Line 291: `SpecVersion: "0.3.0"` → `SpecVersion: version.SpecVersion`
- Line 373: `SpecVersion: "0.3.0"` → `SpecVersion: version.SpecVersion`
- Line 465: `SpecVersion: "0.3.0"` → `SpecVersion: version.SpecVersion`
- Line 527: `SpecVersion: "0.3.0"` → `SpecVersion: version.SpecVersion`

Add import: `"samebits.com/evidra-benchmark/pkg/version"` if not already present.

**Step 4: Update test files to use the constant**

Test files (`pkg/evidence/chain_test.go`, `pkg/evidence/entry_builder_test.go`, `pkg/evidence/entry_test.go`, `pkg/evidence/entry_store_test.go`) use hardcoded `"0.3.0"`. These are test fixtures — they can stay as-is since they test specific version strings in evidence entries. Do NOT change test files — they represent stored evidence data.

**Step 5: Run tests**

```bash
go test ./cmd/evidra/... ./pkg/mcpserver/... ./pkg/version/... -v -count=1
```

**Step 6: Verify no remaining hardcoded versions in production code**

```bash
grep -rn '"0\.3\.0"' cmd/ pkg/ internal/ --include='*.go' | grep -v '_test.go'
```

Expected: zero matches.

**Step 7: Commit**

```bash
git add pkg/version/version.go cmd/evidra/main.go pkg/mcpserver/server.go
git commit -m "fix(H1): bump versions to 0.3.1, single source of truth via version.SpecVersion"
```

---

### Task 2: Add `GenerateSessionID()` and auto-generate in CLI when omitted

**Files:**
- Modify: `pkg/evidence/trace.go`
- Modify: `cmd/evidra/main.go` (cmdPrescribe, cmdReport, cmdIngestFindings)

**Context:**
- `pkg/evidence/trace.go` already has `GenerateTraceID()` returning a ULID
- CLI flag help says `(generated if omitted)` for `--session-id` in prescribe
- But code just passes empty string when omitted — no generation
- Session ID should be auto-generated so findings/reports can correlate
- The prescribe command should print the generated session_id in output so callers can reuse it

**Step 1: Add `GenerateSessionID` to `pkg/evidence/trace.go`**

```go
// GenerateSessionID creates a new session_id as a ULID.
func GenerateSessionID() string {
	return ulid.Make().String()
}
```

**Step 2: Auto-generate session_id in `cmdPrescribe` when omitted**

After flag parsing (around line 367), add:

```go
sessionID := *sessionIDFlag
if sessionID == "" {
	sessionID = evidence.GenerateSessionID()
}
```

Then replace all `*sessionIDFlag` references in cmdPrescribe with `sessionID`.

Also add `"session_id": sessionID` to the result map (around line 522) so callers can capture it:

```go
result["session_id"] = sessionID
```

**Step 3: Auto-generate session_id in `cmdReport` when omitted**

After flag parsing in cmdReport, add the same pattern:

```go
sessionID := *sessionIDFlag
if sessionID == "" {
	sessionID = evidence.GenerateSessionID()
}
```

Replace `*sessionIDFlag` with `sessionID` in the BuildEntry call.

**Step 4: Auto-generate session_id in `cmdIngestFindings` when omitted**

Same pattern in cmdIngestFindings.

**Step 5: Run tests**

```bash
go test ./cmd/evidra/... ./pkg/evidence/... -v -count=1
```

**Step 6: Commit**

```bash
git add pkg/evidence/trace.go cmd/evidra/main.go
git commit -m "fix(H2): auto-generate session_id in CLI when omitted"
```

---

### Task 3: Fix findings correlation bug (TraceID, SessionID, OperationID)

**Files:**
- Modify: `cmd/evidra/main.go` (scanner findings in cmdPrescribe, lines 547-570)

**Context:**
- When cmdPrescribe writes scanner findings, it sets `TraceID: cr.ArtifactDigest` — this is an artifact digest, NOT a trace ID
- Finding entries are missing `SessionID`, `OperationID`, and `Attempt`
- Findings should share the same trace_id and session_id as the parent prescribe entry
- The `traceID` variable (line 462) is already generated for the prescribe entry — reuse it

**Step 1: Fix the scanner findings loop in cmdPrescribe**

Current code (around line 550):
```go
findingEntry, err := evidence.BuildEntry(evidence.EntryBuildParams{
    Type:           evidence.EntryTypeFinding,
    TraceID:        cr.ArtifactDigest,  // BUG: digest, not trace ID
    Actor:          actor,
    ArtifactDigest: cr.ArtifactDigest,
    Payload:        findingPayload,
    PreviousHash:   lastHash,
    SpecVersion:    "0.3.0",
    AdapterVersion: version.Version,
    Signer:         signer,
})
```

Replace with:
```go
findingEntry, err := evidence.BuildEntry(evidence.EntryBuildParams{
    Type:           evidence.EntryTypeFinding,
    SessionID:      sessionID,
    OperationID:    *operationIDFlag,
    Attempt:        *attemptFlag,
    TraceID:        traceID,
    Actor:          actor,
    ArtifactDigest: cr.ArtifactDigest,
    Payload:        findingPayload,
    PreviousHash:   lastHash,
    SpecVersion:    version.SpecVersion,
    AdapterVersion: version.Version,
    Signer:         signer,
})
```

Note: `sessionID` comes from Task 2 (the auto-generated or flag value). `traceID` is the same one used for the prescribe entry (line 462).

**Step 2: Fix findings in cmdIngestFindings too**

The `cmdIngestFindings` also uses `TraceID: artifactDigest` (line 904). Fix it to use a proper trace ID:

```go
traceID := evidence.GenerateTraceID()
```

Add this before the findings loop, then use `TraceID: traceID` in the BuildEntry call.

**Step 3: Run tests**

```bash
go test ./cmd/evidra/... -v -count=1
```

**Step 4: Commit**

```bash
git add cmd/evidra/main.go
git commit -m "fix(H3): findings correlation — correct TraceID, attach SessionID/OperationID/Attempt"
```

---

### Task 4: Fix GitHub Action (M4 from review)

**Files:**
- Modify: `.github/actions/evidra/action.yml`

**Context:**
- Download URL uses `${{ github.repository }}` — points to consumer's repo, not Evidra's
- Uses `gh` CLI which may not be available
- Uses `jq` without guaranteeing availability
- Fallback builds from source assuming Go toolchain exists

**Step 1: Fix the download URL to use a fixed repo**

Replace `${{ github.repository }}` with `samebits/evidra-benchmark` (or whatever the canonical repo is).

Replace the `gh release view` call with a direct curl to the GitHub API:

```yaml
    - name: Download evidra CLI
      shell: bash
      run: |
        VERSION="${{ inputs.evidra-version }}"
        REPO="samebits/evidra-benchmark"
        if [ "$VERSION" = "latest" ]; then
          VERSION=$(curl -sL "https://api.github.com/repos/${REPO}/releases/latest" | grep -o '"tag_name": *"[^"]*"' | head -1 | cut -d'"' -f4 || echo "v0.3.1")
        fi
        ARCHIVE="evidra_${VERSION#v}_linux_amd64.tar.gz"
        URL="https://github.com/${REPO}/releases/download/${VERSION}/${ARCHIVE}"
        echo "Downloading evidra ${VERSION}..."
        curl -sL "$URL" -o /tmp/evidra.tar.gz && tar -xzf /tmp/evidra.tar.gz -C /tmp/ && chmod +x /tmp/evidra || {
          echo "::warning::Could not download release, building from source"
          go build -o /tmp/evidra ./cmd/evidra
        }
```

**Step 2: Remove jq dependency in scorecard step**

Replace the `jq` calls with `grep`/`sed` or use `python3 -c` (available on all GitHub runners):

```yaml
    - name: Generate scorecard
      id: scorecard
      shell: bash
      run: |
        OUTPUT=$(/tmp/evidra scorecard \
          --evidence-dir "${{ inputs.evidence-dir }}" \
          --session-id "${{ inputs.session-id }}" 2>/dev/null || echo '{"score":0,"band":"unknown"}')
        SCORE=$(echo "$OUTPUT" | python3 -c "import sys,json; print(json.load(sys.stdin).get('score',0))" 2>/dev/null || echo "0")
        BAND=$(echo "$OUTPUT" | python3 -c "import sys,json; print(json.load(sys.stdin).get('band','unknown'))" 2>/dev/null || echo "unknown")
        echo "score=$SCORE" >> "$GITHUB_OUTPUT"
        echo "band=$BAND" >> "$GITHUB_OUTPUT"
```

Keep the rest of the step (summary generation) as-is — summary doesn't need machine parsing.

**Step 3: Commit**

```bash
git add .github/actions/evidra/action.yml
git commit -m "fix(M4): GitHub Action uses fixed repo URL, removes gh/jq dependencies"
```

---

### Task 5: Add `evidra keygen` command

**Files:**
- Create: `cmd/evidra/keygen.go`
- Create: `cmd/evidra/keygen_test.go`
- Modify: `cmd/evidra/main.go` (add case to switch)

**Context:**
- Signing is now required for all evidence entries
- Users need a way to generate Ed25519 keypairs without external tools
- `internal/evidence/signer.go` already has `signerEphemeral()` that generates keys and `PublicKeyPEM()` for export
- The command should output the private key (base64, for `EVIDRA_SIGNING_KEY`) and the public key PEM (for `--public-key` in validate)

**Step 1: Write the test `cmd/evidra/keygen_test.go`**

```go
package main

import (
	"bytes"
	"encoding/base64"
	"strings"
	"testing"
)

func TestCmdKeygen_OutputsKeyPair(t *testing.T) {
	t.Parallel()
	var stdout, stderr bytes.Buffer
	code := cmdKeygen(nil, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("expected exit 0, got %d; stderr: %s", code, stderr.String())
	}
	out := stdout.String()
	if !strings.Contains(out, "EVIDRA_SIGNING_KEY=") {
		t.Error("expected EVIDRA_SIGNING_KEY= in output")
	}
	if !strings.Contains(out, "BEGIN PUBLIC KEY") {
		t.Error("expected PEM public key in output")
	}
	// Extract base64 key and validate it decodes
	for _, line := range strings.Split(out, "\n") {
		if strings.HasPrefix(line, "EVIDRA_SIGNING_KEY=") {
			b64 := strings.TrimPrefix(line, "EVIDRA_SIGNING_KEY=")
			raw, err := base64.StdEncoding.DecodeString(b64)
			if err != nil {
				t.Errorf("base64 decode: %v", err)
			}
			if len(raw) != 32 && len(raw) != 64 {
				t.Errorf("unexpected key length: %d", len(raw))
			}
			break
		}
	}
}
```

**Step 2: Run test to verify it fails**

```bash
go test -run TestCmdKeygen ./cmd/evidra/ -v -count=1
```
Expected: FAIL (cmdKeygen not defined)

**Step 3: Implement `cmd/evidra/keygen.go`**

```go
package main

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"fmt"
	"io"
)

func cmdKeygen(_ []string, stdout, stderr io.Writer) int {
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		fmt.Fprintf(stderr, "generate key: %v\n", err)
		return 1
	}

	// Output private key seed as base64 (32 bytes)
	seed := priv.Seed()
	fmt.Fprintf(stdout, "EVIDRA_SIGNING_KEY=%s\n\n", base64.StdEncoding.EncodeToString(seed))

	// Output public key as PEM
	der, err := x509.MarshalPKIXPublicKey(pub)
	if err != nil {
		fmt.Fprintf(stderr, "marshal public key: %v\n", err)
		return 1
	}
	pemBlock := pem.EncodeToMemory(&pem.Block{
		Type:  "PUBLIC KEY",
		Bytes: der,
	})
	fmt.Fprintf(stdout, "%s", pemBlock)

	return 0
}
```

**Step 4: Wire into `cmd/evidra/main.go`**

Add to the switch in `run()`:

```go
case "keygen":
    return cmdKeygen(args[1:], stdout, stderr)
```

Add to `printUsage()`:

```go
fmt.Fprintln(w, "  keygen            Generate Ed25519 signing keypair")
```

**Step 5: Run test to verify it passes**

```bash
go test -run TestCmdKeygen ./cmd/evidra/ -v -count=1
```

**Step 6: Run all tests**

```bash
go test ./cmd/evidra/... -v -count=1
```

**Step 7: Run gofmt**

```bash
gofmt -w cmd/evidra/keygen.go cmd/evidra/keygen_test.go
```

**Step 8: Commit**

```bash
git add cmd/evidra/keygen.go cmd/evidra/keygen_test.go cmd/evidra/main.go
git commit -m "feat: add evidra keygen command for Ed25519 keypair generation"
```

---

### Task 6: Protocol taxonomy verification (H4 cleanup)

**Files:**
- Read: `docs/system-design/EVIDRA_PROTOCOL.md`
- Read: `docs/system-design/EVIDRA_CORE_DATA_MODEL.md`
- Read: `docs/system-design/EVIDRA_SESSION_OPERATION_EVENT_MODEL.md`
- Read: `docs/system-design/EVIDRA_CNCF_STANDARDS_ALIGNMENT.md`
- Modify: `docs/system-design/EVIDRA_PROTOCOL.md` (if needed)

**Context:**
- The review noted protocol doc lists `tool_start/tool_end/tool_error/validator_findings/...` while code uses `prescribe/report/finding/signal/...`
- We already added `session_start/session_end/annotation` entry types in the previous round
- The CNCF alignment doc has the correct CloudEvents↔EntryType mapping table
- The session/operation event model doc defines `operation.start/end/error` → `prescribe/report` mapping
- Need to verify EVIDRA_PROTOCOL.md references the correct entry types and doesn't have stale taxonomy

**Step 1: Read `EVIDRA_PROTOCOL.md` fully and identify taxonomy mismatches**

Look for any references to `tool_start`, `tool_end`, `tool_error` or other event types that don't match the actual `EntryType` enum in code.

**Step 2: If mismatches exist, update EVIDRA_PROTOCOL.md**

Ensure the protocol doc references the canonical entry types:
- `prescribe`, `report`, `finding`, `signal`, `receipt`, `canonicalization_failure`, `session_start`, `session_end`, `annotation`

If the protocol uses a conceptual taxonomy (`tool_start/end/error`), add a normative mapping table or reference the session/operation event model doc.

**Step 3: Commit**

```bash
git add docs/system-design/EVIDRA_PROTOCOL.md
git commit -m "docs(H4): align protocol taxonomy with actual EntryType enum"
```

---

### Task 7: Final verification

**Step 1: Run full test suite**

```bash
make test
```

**Step 2: Run linter**

```bash
make lint
```

**Step 3: Verify no hardcoded 0.3.0 in production code**

```bash
grep -rn '"0\.3\.0"' cmd/ pkg/ internal/ --include='*.go' | grep -v '_test.go'
```

Expected: zero matches.

**Step 4: Verify version output**

```bash
go run ./cmd/evidra version
```

Expected: `evidra-benchmark 0.3.1 (commit: dev, built: dev)`

**Step 5: Test keygen end-to-end**

```bash
go run ./cmd/evidra keygen
```

Expected: outputs `EVIDRA_SIGNING_KEY=<base64>` and a PEM public key block.

---

## Summary of changes by review finding

| Finding | Task | Status |
|---------|------|--------|
| H1: Version drift | Task 1 | Fix |
| H2: Session ID generation | Task 2 | Fix |
| H3: Findings correlation bug | Task 3 | Fix |
| H4: Protocol taxonomy | Task 6 | Verify/Fix |
| M4: GitHub Action fixes | Task 4 | Fix |
| M4 (related): `evidra keygen` | Task 5 | New feature |
