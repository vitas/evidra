# v0.4.0 — Signing Integration + Scorecard Breakdowns

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Wire the existing Ed25519 signer into the evidence pipeline so every entry is signed, add signature verification to chain validation, and add BY TOOL/BY SCOPE scorecard breakdowns.

**Architecture:** The signer module (`internal/evidence/signer.go`, 149 LOC, 14 tests) is complete but unintegrated. This plan wires it into `BuildEntry()` → `AppendEntryAtPath()`, adds verification to `ValidateChainAtPath()`, and extends the scorecard with grouped breakdowns.

**Tech Stack:** Go 1.24, existing `internal/evidence/signer.go`, `pkg/evidence/entry_builder.go`, `internal/score/score.go`.

---

## Task 1: Add Signer to BuildEntry flow

**Files:**
- Modify: `pkg/evidence/entry_builder.go` — add optional Signer parameter
- Modify: `pkg/evidence/entry.go` — no struct changes needed (Signature field exists)

**Step 1: Add SignEntry function**

Do NOT modify `BuildEntry()` signature (would break all callers). Instead, add a `SignEntry()` function that signs an already-built entry:

```go
// SignEntry populates the Signature field of an entry using the given signer.
// The signing payload is the entry's computed Hash field.
func SignEntry(entry *EvidenceEntry, signer *evidence.Signer) error {
    if signer == nil {
        return nil // no-op when signing is disabled
    }
    sig := signer.Sign([]byte(entry.Hash))
    entry.Signature = "ed25519:" + base64.StdEncoding.EncodeToString(sig)
    return nil
}
```

Add import for `"encoding/base64"` and `"samebits.com/evidra-benchmark/internal/evidence"`.

Note: The signing payload is the entry's Hash (SHA256 of all fields except Hash and Signature). This means: hash covers content integrity, signature covers authorship.

**Step 2: Write test**

```go
func TestSignEntry_PopulatesSignature(t *testing.T) {
    t.Parallel()
    signer, err := evidence.NewSigner(evidence.SignerConfig{DevMode: true})
    if err != nil {
        t.Fatal(err)
    }
    entry, err := BuildEntry(EntryBuildParams{
        Type:        EntryTypePrescribe,
        TraceID:     "trace-1",
        Actor:       Actor{Type: "test", ID: "test"},
        Payload:     json.RawMessage(`{}`),
        SpecVersion: "0.4.0",
    })
    if err != nil {
        t.Fatal(err)
    }
    if entry.Signature != "" {
        t.Fatal("signature should be empty before signing")
    }
    if err := SignEntry(&entry, signer); err != nil {
        t.Fatal(err)
    }
    if !strings.HasPrefix(entry.Signature, "ed25519:") {
        t.Errorf("signature should have ed25519: prefix, got %q", entry.Signature)
    }
}

func TestSignEntry_NilSigner(t *testing.T) {
    t.Parallel()
    entry := EvidenceEntry{Hash: "sha256:abc"}
    if err := SignEntry(&entry, nil); err != nil {
        t.Fatal(err)
    }
    if entry.Signature != "" {
        t.Error("nil signer should leave signature empty")
    }
}
```

**Step 3: Run tests, gofmt, commit**

```bash
gofmt -w pkg/evidence/entry_builder.go pkg/evidence/entry_builder_test.go
go test ./pkg/evidence/ -v -count=1
git add pkg/evidence/
git commit -m "feat: add SignEntry for Ed25519 signing of evidence entries"
```

---

## Task 2: Wire signing into MCP server

**Files:**
- Modify: `cmd/evidra-mcp/main.go` — read signing key env vars, pass to server
- Modify: `pkg/mcpserver/server.go` — accept Signer in Options, sign entries

**Step 1: Add signing key config to MCP server startup**

In `cmd/evidra-mcp/main.go`, after reading existing env vars:

```go
signingKey := os.Getenv("EVIDRA_SIGNING_KEY")
signingKeyPath := os.Getenv("EVIDRA_SIGNING_KEY_PATH")
isDev := strings.EqualFold(environment, "development")
```

Initialize signer (optional — nil if no key and not dev mode):

```go
var signer *evidence.Signer
signerCfg := evidence.SignerConfig{
    KeyBase64: signingKey,
    KeyPath:   signingKeyPath,
    DevMode:   isDev,
}
if signingKey != "" || signingKeyPath != "" || isDev {
    s, err := evidence.NewSigner(signerCfg)
    if err != nil {
        fmt.Fprintf(os.Stderr, "warning: signing disabled: %v\n", err)
    } else {
        signer = s
    }
}
```

Pass signer to Options:

```go
opts := mcpserver.Options{
    // ... existing fields ...
    Signer: signer,
}
```

**Step 2: Accept Signer in mcpserver.Options**

```go
type Options struct {
    Name         string
    Version      string
    EvidencePath string
    Environment  string
    RetryTracker bool
    Signer       *evidence.Signer  // optional, nil disables signing
}
```

Store in `BenchmarkService`:

```go
type BenchmarkService struct {
    evidencePath string
    retryTracker *RetryTracker
    traceID      string
    lastActor    evidencePkg.Actor
    signer       *evidence.Signer
}
```

**Step 3: Sign entries in Prescribe() and Report()**

After `evidence.BuildEntry()` and before `evidence.AppendEntryAtPath()`, add:

```go
if err := evidencePkg.SignEntry(&entry, s.signer); err != nil {
    // log warning but don't fail — signing is advisory
}
```

Apply this pattern in both `Prescribe()` and `Report()` methods.

**Step 4: Run tests, commit**

```bash
gofmt -w cmd/evidra-mcp/main.go pkg/mcpserver/server.go
go build ./cmd/evidra-mcp/
go test ./pkg/mcpserver/ -v -count=1
git add cmd/evidra-mcp/main.go pkg/mcpserver/
git commit -m "feat: wire Ed25519 signing into MCP server evidence pipeline"
```

---

## Task 3: Wire signing into CLI prescribe and report

**Files:**
- Modify: `cmd/evidra/main.go` — add `--signing-key` flag, sign entries

**Step 1: Add signing key flag to cmdPrescribe and cmdReport**

```go
signingKeyFlag := fs.String("signing-key", "", "Base64-encoded Ed25519 signing key (or set EVIDRA_SIGNING_KEY)")
```

After `BuildEntry()` and before `AppendEntryAtPath()`:

```go
// Sign entry if signing key available
sigKey := *signingKeyFlag
if sigKey == "" {
    sigKey = os.Getenv("EVIDRA_SIGNING_KEY")
}
if sigKey != "" {
    signer, err := evidence.NewSigner(evidence.SignerConfig{KeyBase64: sigKey})
    if err != nil {
        fmt.Fprintf(stderr, "signing key error: %v\n", err)
        return 1
    }
    evidencePkg.SignEntry(&entry, signer)
}
```

Note: CLI creates a new Signer per invocation (stateless). MCP server keeps one Signer for the session.

**Step 2: Run tests, commit**

```bash
gofmt -w cmd/evidra/main.go
go build ./cmd/evidra/
go test ./... -count=1
git add cmd/evidra/main.go
git commit -m "feat: wire Ed25519 signing into CLI prescribe and report"
```

---

## Task 4: Add signature verification to ValidateChainAtPath

**Files:**
- Modify: `pkg/evidence/entry_store.go` — add signature verification
- Test: `pkg/evidence/chain_test.go` — add signature verification tests

**Step 1: Add VerifyEntrySignature function**

```go
// VerifyEntrySignature checks the Ed25519 signature on an entry.
// Returns nil if signature is empty (unsigned entry) or valid.
// Returns error if signature is present but invalid.
func VerifyEntrySignature(entry EvidenceEntry, pubKey ed25519.PublicKey) error {
    if entry.Signature == "" {
        return nil // unsigned entries are allowed
    }
    sigStr := strings.TrimPrefix(entry.Signature, "ed25519:")
    sig, err := base64.StdEncoding.DecodeString(sigStr)
    if err != nil {
        return fmt.Errorf("decode signature: %w", err)
    }
    if !ed25519.Verify(pubKey, []byte(entry.Hash), sig) {
        return fmt.Errorf("invalid signature")
    }
    return nil
}
```

**Step 2: Add ValidateChainWithSignatures**

```go
// ValidateChainWithSignatures validates hash chain AND verifies signatures.
// If pubKey is nil, signature verification is skipped.
func ValidateChainWithSignatures(root string, pubKey ed25519.PublicKey) error {
    // First validate hash chain
    if err := ValidateChainAtPath(root); err != nil {
        return err
    }
    if pubKey == nil {
        return nil
    }
    entries, err := ReadAllEntriesAtPath(root)
    if err != nil {
        return err
    }
    for i, entry := range entries {
        if err := VerifyEntrySignature(entry, pubKey); err != nil {
            return &ChainValidationError{
                Index:   i,
                EventID: entry.EntryID,
                Message: fmt.Sprintf("signature verification failed: %v", err),
            }
        }
    }
    return nil
}
```

**Step 3: Write tests**

```go
func TestChainIntegrity_SignedEntries(t *testing.T) {
    t.Parallel()
    dir := t.TempDir()
    signer, _ := evidence.NewSigner(evidence.SignerConfig{DevMode: true})

    // Append 3 signed entries
    for i := 0; i < 3; i++ {
        lastHash, _ := LastHashAtPath(dir)
        entry, _ := BuildEntry(EntryBuildParams{
            Type:         EntryTypePrescribe,
            TraceID:      fmt.Sprintf("trace-%d", i),
            Actor:        Actor{Type: "test", ID: "test"},
            Payload:      json.RawMessage(`{}`),
            PreviousHash: lastHash,
            SpecVersion:  "0.4.0",
        })
        SignEntry(&entry, signer)
        AppendEntryAtPath(dir, entry)
    }

    // Verify with correct key
    if err := ValidateChainWithSignatures(dir, signer.PublicKey()); err != nil {
        t.Fatalf("valid chain should pass: %v", err)
    }
}

func TestChainIntegrity_WrongKey(t *testing.T) {
    t.Parallel()
    dir := t.TempDir()
    signer1, _ := evidence.NewSigner(evidence.SignerConfig{DevMode: true})
    signer2, _ := evidence.NewSigner(evidence.SignerConfig{DevMode: true})

    lastHash, _ := LastHashAtPath(dir)
    entry, _ := BuildEntry(EntryBuildParams{
        Type:         EntryTypePrescribe,
        TraceID:      "trace-1",
        Actor:        Actor{Type: "test", ID: "test"},
        Payload:      json.RawMessage(`{}`),
        PreviousHash: lastHash,
        SpecVersion:  "0.4.0",
    })
    SignEntry(&entry, signer1)
    AppendEntryAtPath(dir, entry)

    // Verify with wrong key
    err := ValidateChainWithSignatures(dir, signer2.PublicKey())
    if err == nil {
        t.Fatal("wrong key should fail verification")
    }
}
```

**Step 4: Run tests, commit**

```bash
gofmt -w pkg/evidence/entry_store.go pkg/evidence/chain_test.go
go test ./pkg/evidence/ -v -count=1
git add pkg/evidence/
git commit -m "feat: add signature verification to evidence chain validation"
```

---

## Task 5: Add --verify-signatures flag to CLI scorecard

**Files:**
- Modify: `cmd/evidra/main.go` — add `--pubkey` flag to scorecard

**Step 1: Add pubkey flag**

In `cmdScorecard()`:

```go
pubkeyFlag := fs.String("pubkey", "", "Path to Ed25519 public key PEM for signature verification")
```

After reading evidence, before computing signals, if pubkey is set:

```go
if *pubkeyFlag != "" {
    pemData, err := os.ReadFile(*pubkeyFlag)
    if err != nil {
        fmt.Fprintf(stderr, "read pubkey: %v\n", err)
        return 1
    }
    pubKey, err := evidence.ParsePublicKeyPEM(pemData)
    if err != nil {
        fmt.Fprintf(stderr, "parse pubkey: %v\n", err)
        return 1
    }
    if err := evidence.ValidateChainWithSignatures(evidencePath, pubKey); err != nil {
        fmt.Fprintf(stderr, "signature verification failed: %v\n", err)
        return 1
    }
}
```

**Step 2: Add ParsePublicKeyPEM to evidence package**

```go
func ParsePublicKeyPEM(pemData []byte) (ed25519.PublicKey, error) {
    block, _ := pem.Decode(pemData)
    if block == nil {
        return nil, fmt.Errorf("no PEM block found")
    }
    pub, err := x509.ParsePKIXPublicKey(block.Bytes)
    if err != nil {
        return nil, fmt.Errorf("parse public key: %w", err)
    }
    edPub, ok := pub.(ed25519.PublicKey)
    if !ok {
        return nil, fmt.Errorf("not an Ed25519 public key")
    }
    return edPub, nil
}
```

**Step 3: Run tests, commit**

```bash
gofmt -w cmd/evidra/main.go pkg/evidence/entry_store.go
go build ./cmd/evidra/
go test ./... -count=1
git add cmd/evidra/main.go pkg/evidence/
git commit -m "feat: add --pubkey flag for signature verification in scorecard"
```

---

## Task 6: Scorecard BY TOOL / BY SCOPE breakdowns

**Files:**
- Modify: `internal/score/score.go` — add `ComputeGrouped` function
- Modify: `cmd/evidra/main.go` — include breakdowns in scorecard output

**Step 1: Add ComputeGrouped to score package**

```go
// GroupedScorecard includes per-tool and per-scope breakdowns.
type GroupedScorecard struct {
    Scorecard
    ByTool  map[string]Scorecard `json:"by_tool,omitempty"`
    ByScope map[string]Scorecard `json:"by_scope,omitempty"`
}

// ComputeGrouped computes aggregate score plus per-tool and per-scope breakdowns.
func ComputeGrouped(entries []signal.Entry, results []signal.SignalResult, totalOps int) GroupedScorecard {
    gs := GroupedScorecard{
        Scorecard: Compute(results, totalOps),
        ByTool:    make(map[string]Scorecard),
        ByScope:   make(map[string]Scorecard),
    }

    // Group entries by tool
    toolGroups := make(map[string][]signal.Entry)
    scopeGroups := make(map[string][]signal.Entry)
    for _, e := range entries {
        if e.Tool != "" {
            toolGroups[e.Tool] = append(toolGroups[e.Tool], e)
        }
        if e.ScopeClass != "" {
            scopeGroups[e.ScopeClass] = append(scopeGroups[e.ScopeClass], e)
        }
    }

    for tool, toolEntries := range toolGroups {
        toolResults := signal.AllSignals(toolEntries)
        toolOps := countPrescriptions(toolEntries)
        gs.ByTool[tool] = Compute(toolResults, toolOps)
    }

    for scope, scopeEntries := range scopeGroups {
        scopeResults := signal.AllSignals(scopeEntries)
        scopeOps := countPrescriptions(scopeEntries)
        gs.ByScope[scope] = Compute(scopeResults, scopeOps)
    }

    return gs
}

func countPrescriptions(entries []signal.Entry) int {
    n := 0
    for _, e := range entries {
        if e.IsPrescription {
            n++
        }
    }
    return n
}
```

**Step 2: Wire into cmdScorecard**

Replace `sc := score.Compute(results, totalOps)` with:

```go
gs := score.ComputeGrouped(signalEntries, results, totalOps)
```

Update the output struct to embed `GroupedScorecard` instead of `Scorecard`.

**Step 3: Write test for ComputeGrouped**

```go
func TestComputeGrouped_ByTool(t *testing.T) {
    t.Parallel()
    entries := []signal.Entry{
        {EventID: "1", IsPrescription: true, Tool: "kubectl", ScopeClass: "staging"},
        {EventID: "2", IsPrescription: true, Tool: "terraform", ScopeClass: "production"},
        {EventID: "3", IsPrescription: true, Tool: "kubectl", ScopeClass: "staging"},
    }
    results := signal.AllSignals(entries)
    gs := ComputeGrouped(entries, results, 3)

    if len(gs.ByTool) != 2 {
        t.Errorf("expected 2 tools, got %d", len(gs.ByTool))
    }
    if gs.ByTool["kubectl"].TotalOperations != 2 {
        t.Errorf("kubectl ops: got %d, want 2", gs.ByTool["kubectl"].TotalOperations)
    }
    if gs.ByTool["terraform"].TotalOperations != 1 {
        t.Errorf("terraform ops: got %d, want 1", gs.ByTool["terraform"].TotalOperations)
    }
}
```

**Step 4: Run tests, commit**

```bash
gofmt -w internal/score/score.go internal/score/score_test.go cmd/evidra/main.go
go test ./internal/score/ -v -count=1
go test ./... -count=1
git add internal/score/ cmd/evidra/main.go
git commit -m "feat: add BY TOOL and BY SCOPE scorecard breakdowns"
```

---

## Task 7: Final verification

**Step 1: Build**

```bash
go build ./cmd/evidra/ ./cmd/evidra-mcp/
```

**Step 2: All tests pass**

```bash
go test ./... -v -count=1
```

**Step 3: Race detector**

```bash
go test -race ./...
```

**Step 4: Lint**

```bash
gofmt -l .
```

**Step 5: Count tests**

```bash
go test ./... -v -count=1 2>&1 | grep -c 'PASS:'
```

Target: 215+ tests passing (208 current + new signing/scoring tests).
