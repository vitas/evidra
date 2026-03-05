# v0.3.1 — E2E Tests, Signing Activation, Session Filtering & GitHub Action

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Activate signing in CLI/MCP, add `--session-id` filtering to scorecard/explain, build E2E tests that prove it all works, and ship a GitHub Action for CI integration.

**Architecture:** CLI and MCP server get `--signing-key` / `EVIDRA_SIGNING_KEY` support so every evidence entry is signed. Scorecard and explain commands get `--session-id` filtering. E2E tests are Go integration tests (build tag `e2e`) that invoke the CLI binary and verify evidence output. GitHub Action is a composite action that wraps the CLI binary.

**Tech Stack:** Go 1.24, Ed25519 signing, SARIF, GitHub Actions composite

**Current state (v0.3.0):**
- `BuildEntry` accepts `Signer` interface but CLI/MCP never pass one
- `--session-id` is accepted by prescribe/report/ingest-findings but scorecard/explain don't filter by it
- `evidra validate --public-key` works for signature verification
- `evidra ingest-findings --sarif` works standalone
- No E2E tests exist (only unit + MCP e2e_test.go)
- No GitHub Action exists

---

## Task 1: Wire signing into CLI commands

**Files:**
- Modify: `cmd/evidra/main.go:344-555` (prescribe), `cmd/evidra/main.go:557-673` (report), `cmd/evidra/main.go:772-856` (ingest-findings)
- Test: `cmd/evidra/main_test.go`

### Step 1: Write the failing test

Add to `cmd/evidra/main_test.go`:

```go
func TestRunPrescribe_WithSigningKey(t *testing.T) {
	t.Parallel()

	tmp := t.TempDir()
	artifact := filepath.Join(tmp, "artifact.yaml")
	if err := os.WriteFile(artifact, []byte("apiVersion: v1\nkind: ConfigMap\nmetadata:\n  name: test\ndata: {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Generate ephemeral key pair, write public key PEM
	pub, priv, _ := ed25519.GenerateKey(rand.Reader)
	der, _ := x509.MarshalPKCS8PrivateKey(priv)
	keyPath := filepath.Join(tmp, "key.pem")
	os.WriteFile(keyPath, pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der}), 0o600)

	pubDer, _ := x509.MarshalPKIXPublicKey(pub)
	pubPath := filepath.Join(tmp, "pub.pem")
	os.WriteFile(pubPath, pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: pubDer}), 0o600)

	evidenceDir := filepath.Join(tmp, "evidence")

	// Prescribe with signing key
	var out, errBuf bytes.Buffer
	code := run([]string{
		"prescribe",
		"--tool", "kubectl",
		"--artifact", artifact,
		"--evidence-dir", evidenceDir,
		"--signing-key-path", keyPath,
	}, &out, &errBuf)
	if code != 0 {
		t.Fatalf("prescribe exit %d: %s", code, errBuf.String())
	}

	// Verify entry has signature
	entries, err := evidence.ReadAllEntriesAtPath(evidenceDir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) == 0 {
		t.Fatal("no entries")
	}
	if entries[0].Signature == "" {
		t.Fatal("expected non-empty signature")
	}

	// Validate chain with signatures
	var vOut, vErr bytes.Buffer
	vCode := run([]string{
		"validate",
		"--evidence-dir", evidenceDir,
		"--public-key", pubPath,
	}, &vOut, &vErr)
	if vCode != 0 {
		t.Fatalf("validate exit %d: %s", vCode, vErr.String())
	}
}
```

### Step 2: Run test to verify it fails

Run: `go test -run TestRunPrescribe_WithSigningKey ./cmd/evidra/ -v -count=1`
Expected: FAIL — `--signing-key-path` flag not recognized

### Step 3: Implement signing key flags in CLI

Add `--signing-key` (base64) and `--signing-key-path` (PEM) flags to `cmdPrescribe`, `cmdReport`, and `cmdIngestFindings`. Also support `EVIDRA_SIGNING_KEY` and `EVIDRA_SIGNING_KEY_PATH` env vars as fallback.

In `cmd/evidra/main.go`, add a helper function:

```go
func resolveSigner(keyBase64, keyPath string) (evidence.Signer, error) {
	cfg := ievsigner.SignerConfig{}

	// Flag values take priority
	if keyBase64 != "" {
		cfg.KeyBase64 = keyBase64
	} else if v := strings.TrimSpace(os.Getenv("EVIDRA_SIGNING_KEY")); v != "" {
		cfg.KeyBase64 = v
	}

	if keyPath != "" {
		cfg.KeyPath = keyPath
	} else if v := strings.TrimSpace(os.Getenv("EVIDRA_SIGNING_KEY_PATH")); v != "" {
		cfg.KeyPath = v
	}

	// No key configured — signing is optional
	if cfg.KeyBase64 == "" && cfg.KeyPath == "" {
		return nil, nil
	}

	s, err := ievsigner.NewSigner(cfg)
	if err != nil {
		return nil, err
	}
	return s, nil
}
```

Add to each command's flag set:
```go
signingKeyFlag := fs.String("signing-key", "", "Base64-encoded Ed25519 signing key")
signingKeyPathFlag := fs.String("signing-key-path", "", "Path to PEM-encoded Ed25519 private key")
```

After flag parsing, resolve signer:
```go
signer, err := resolveSigner(*signingKeyFlag, *signingKeyPathFlag)
if err != nil {
	fmt.Fprintf(stderr, "resolve signing key: %v\n", err)
	return 1
}
```

Pass signer into `BuildEntry`:
```go
Signer: signer,
```

**Important:** The `evidence.Signer` interface is in `pkg/evidence/entry_builder.go`. The `internal/evidence.Signer` struct implements it. The `resolveSigner` helper returns `evidence.Signer` (the interface, nil-safe).

### Step 4: Run test to verify it passes

Run: `go test -run TestRunPrescribe_WithSigningKey ./cmd/evidra/ -v -count=1`
Expected: PASS

### Step 5: Run all tests

Run: `go test ./... -count=1`
Expected: All PASS

### Step 6: Commit

```bash
git add cmd/evidra/main.go cmd/evidra/main_test.go
git commit -m "feat: wire signing key into CLI prescribe/report/ingest-findings"
```

---

## Task 2: Add `--session-id` filter to scorecard and explain

**Files:**
- Modify: `cmd/evidra/main.go:63-141` (cmdScorecard), `cmd/evidra/main.go:143-253` (cmdExplain)
- Modify: `cmd/evidra/main.go:689-702` (filterEntries)
- Test: `cmd/evidra/main_test.go`

### Step 1: Write the failing test

Add to `cmd/evidra/main_test.go`:

```go
func TestScorecard_SessionIDFilter(t *testing.T) {
	t.Parallel()

	tmp := t.TempDir()
	evidenceDir := filepath.Join(tmp, "evidence")
	artifact := filepath.Join(tmp, "artifact.yaml")
	os.WriteFile(artifact, []byte("apiVersion: v1\nkind: ConfigMap\nmetadata:\n  name: test\ndata: {}\n"), 0o644)

	// Create entries in two different sessions
	for _, sid := range []string{"session-A", "session-B"} {
		var out, errBuf bytes.Buffer
		code := run([]string{
			"prescribe",
			"--tool", "kubectl",
			"--artifact", artifact,
			"--evidence-dir", evidenceDir,
			"--session-id", sid,
		}, &out, &errBuf)
		if code != 0 {
			t.Fatalf("prescribe %s: exit %d: %s", sid, code, errBuf.String())
		}

		var result map[string]interface{}
		json.Unmarshal(out.Bytes(), &result)
		prescriptionID := result["prescription_id"].(string)

		out.Reset()
		errBuf.Reset()
		code = run([]string{
			"report",
			"--prescription", prescriptionID,
			"--exit-code", "0",
			"--evidence-dir", evidenceDir,
			"--session-id", sid,
		}, &out, &errBuf)
		if code != 0 {
			t.Fatalf("report %s: exit %d: %s", sid, code, errBuf.String())
		}
	}

	// Scorecard for session-A only
	var out, errBuf bytes.Buffer
	code := run([]string{
		"scorecard",
		"--evidence-dir", evidenceDir,
		"--session-id", "session-A",
	}, &out, &errBuf)
	if code != 0 {
		t.Fatalf("scorecard exit %d: %s", code, errBuf.String())
	}

	var sc map[string]interface{}
	if err := json.Unmarshal(out.Bytes(), &sc); err != nil {
		t.Fatal(err)
	}
	// Should have 1 operation (session-A only), not 2
	totalOps := int(sc["total_operations"].(float64))
	if totalOps != 1 {
		t.Fatalf("total_operations = %d, want 1 (session-A only)", totalOps)
	}
}
```

### Step 2: Run test to verify it fails

Run: `go test -run TestScorecard_SessionIDFilter ./cmd/evidra/ -v -count=1`
Expected: FAIL — `--session-id` flag not recognized

### Step 3: Implement session-id filter

Add `--session-id` flag to `cmdScorecard` and `cmdExplain`:
```go
sessionIDFlag := fs.String("session-id", "", "Filter by session ID")
```

Modify `filterEntries` to accept session filter:
```go
func filterEntries(entries []evidence.EvidenceEntry, actor, period, sessionID string) []evidence.EvidenceEntry {
	cutoff := parsePeriodCutoff(period)
	var filtered []evidence.EvidenceEntry
	for _, e := range entries {
		if actor != "" && e.Actor.ID != actor {
			continue
		}
		if sessionID != "" && e.SessionID != sessionID {
			continue
		}
		if !cutoff.IsZero() && e.Timestamp.Before(cutoff) {
			continue
		}
		filtered = append(filtered, e)
	}
	return filtered
}
```

Update all callers of `filterEntries` (scorecard, explain, compare) to pass session ID. For compare, add the flag too.

Add `session_id` to scorecard JSON output:
```go
SessionID string `json:"session_id,omitempty"`
```

### Step 4: Run test to verify it passes

Run: `go test -run TestScorecard_SessionIDFilter ./cmd/evidra/ -v -count=1`
Expected: PASS

### Step 5: Run all tests

Run: `go test ./... -count=1`
Expected: All PASS

### Step 6: Commit

```bash
git add cmd/evidra/main.go cmd/evidra/main_test.go
git commit -m "feat: add --session-id filter to scorecard, explain, compare"
```

---

## Task 3: Wire signing into MCP server

**Files:**
- Modify: `pkg/mcpserver/server.go` (Options struct, tool handlers)
- Modify: `cmd/evidra-mcp/main.go` (pass signing config)
- Test: `pkg/mcpserver/e2e_test.go`

### Step 1: Write the failing test

Add to `pkg/mcpserver/e2e_test.go`:

```go
func TestE2E_SignedEntries(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	pub, priv, _ := ed25519.GenerateKey(rand.Reader)
	signer := &testEd25519Signer{priv: priv, pub: pub}

	server, serverErr := NewServer(Options{
		Name:         "test",
		Version:      "0.0.1",
		EvidencePath: dir,
		Environment:  "test",
		Signer:       signer,
	})
	if serverErr != nil {
		t.Fatalf("NewServer: %v", serverErr)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	st, ct := mcp.NewInMemoryTransports()
	if _, err := server.Connect(ctx, st, nil); err != nil {
		t.Fatalf("server connect: %v", err)
	}

	client := mcp.NewClient(
		&mcp.Implementation{Name: "test-client", Version: "0.0.1"},
		nil,
	)
	session, err := client.Connect(ctx, ct, nil)
	if err != nil {
		t.Fatalf("client connect: %v", err)
	}
	defer func() { _ = session.Close() }()

	// Prescribe
	prescribeResult, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name: "prescribe",
		Arguments: map[string]any{
			"actor": map[string]any{
				"type": "agent", "id": "test-agent", "origin": "e2e-test",
			},
			"tool":         "kubectl",
			"operation":    "apply",
			"raw_artifact": "apiVersion: v1\nkind: ConfigMap\nmetadata:\n  name: test\ndata: {}\n",
		},
	})
	if err != nil {
		t.Fatalf("prescribe: %v", err)
	}

	var prescribeOut PrescribeOutput
	if err := extractStructuredContent(prescribeResult, &prescribeOut); err != nil {
		t.Fatalf("parse: %v", err)
	}
	if !prescribeOut.OK {
		t.Fatalf("prescribe not ok: %+v", prescribeOut)
	}

	// Verify entry is signed
	entries, readErr := evidence.ReadAllEntriesAtPath(dir)
	if readErr != nil {
		t.Fatalf("read entries: %v", readErr)
	}
	if len(entries) == 0 {
		t.Fatal("no entries")
	}
	if entries[0].Signature == "" {
		t.Fatal("expected non-empty signature")
	}

	// Validate chain with signatures
	if err := evidence.ValidateChainWithSignatures(dir, pub); err != nil {
		t.Fatalf("validate signatures: %v", err)
	}
}

type testEd25519Signer struct {
	priv ed25519.PrivateKey
	pub  ed25519.PublicKey
}
func (s *testEd25519Signer) Sign(payload []byte) []byte { return ed25519.Sign(s.priv, payload) }
func (s *testEd25519Signer) Verify(payload, sig []byte) bool { return ed25519.Verify(s.pub, payload, sig) }
func (s *testEd25519Signer) PublicKey() ed25519.PublicKey { return s.pub }
```

### Step 2: Run test to verify it fails

Run: `go test -run TestE2E_SignedEntries ./pkg/mcpserver/ -v -count=1`
Expected: FAIL — `Options.Signer` field does not exist

### Step 3: Implement

Add `Signer evidence.Signer` field to `Options` struct in `pkg/mcpserver/server.go`.

Pass `opts.Signer` into `evidence.EntryBuildParams{Signer: opts.Signer}` in the prescribe, report, and findings tool handlers.

In `cmd/evidra-mcp/main.go`, resolve signer from env vars and pass to `Options`:
```go
var signer evidence.Signer
signerCfg := ievsigner.SignerConfig{
	KeyBase64: strings.TrimSpace(os.Getenv("EVIDRA_SIGNING_KEY")),
	KeyPath:   strings.TrimSpace(os.Getenv("EVIDRA_SIGNING_KEY_PATH")),
}
if signerCfg.KeyBase64 != "" || signerCfg.KeyPath != "" {
	s, err := ievsigner.NewSigner(signerCfg)
	if err != nil {
		fmt.Fprintf(stderr, "signing key: %v\n", err)
		return 1
	}
	signer = s
}
```

### Step 4: Run test to verify it passes

Run: `go test -run TestE2E_SignedEntries ./pkg/mcpserver/ -v -count=1`
Expected: PASS

### Step 5: Run all tests

Run: `go test ./... -count=1`
Expected: All PASS

### Step 6: Commit

```bash
git add pkg/mcpserver/server.go cmd/evidra-mcp/main.go pkg/mcpserver/e2e_test.go
git commit -m "feat: wire signing into MCP server"
```

---

## Task 4: Create SARIF test fixtures

**Files:**
- Create: `tests/e2e/fixtures/trivy.sarif`
- Create: `tests/e2e/fixtures/kubescape.sarif`
- Create: `tests/e2e/fixtures/k8s_deployment.yaml`

### Step 1: Create fixtures directory and files

Create `tests/e2e/fixtures/trivy.sarif` — minimal valid SARIF from Trivy with 2 findings:
```json
{
  "$schema": "https://raw.githubusercontent.com/oasis-tcs/sarif-spec/master/Schemata/sarif-schema-2.1.0.json",
  "version": "2.1.0",
  "runs": [{
    "tool": {
      "driver": {
        "name": "Trivy",
        "version": "0.50.0",
        "rules": [
          {"id": "KSV001", "defaultConfiguration": {"level": "error"}},
          {"id": "KSV012", "defaultConfiguration": {"level": "warning"}}
        ]
      }
    },
    "results": [
      {
        "ruleId": "KSV001",
        "level": "error",
        "message": {"text": "Process can elevate its own privileges"},
        "locations": [{"physicalLocation": {"artifactLocation": {"uri": "deployment.yaml"}, "region": {"startLine": 15}}}]
      },
      {
        "ruleId": "KSV012",
        "level": "warning",
        "message": {"text": "Runs with a root primary or supplementary GID"},
        "locations": [{"physicalLocation": {"artifactLocation": {"uri": "deployment.yaml"}, "region": {"startLine": 18}}}]
      }
    ]
  }]
}
```

Create `tests/e2e/fixtures/kubescape.sarif` — minimal SARIF from Kubescape with 1 finding:
```json
{
  "$schema": "https://raw.githubusercontent.com/oasis-tcs/sarif-spec/master/Schemata/sarif-schema-2.1.0.json",
  "version": "2.1.0",
  "runs": [{
    "tool": {
      "driver": {
        "name": "Kubescape",
        "version": "3.0.0",
        "rules": [
          {"id": "C-0034", "defaultConfiguration": {"level": "error"}}
        ]
      }
    },
    "results": [
      {
        "ruleId": "C-0034",
        "level": "error",
        "message": {"text": "Automatic mapping of service account token"},
        "locations": [{"physicalLocation": {"artifactLocation": {"uri": "deployment.yaml"}, "region": {"startLine": 1}}}]
      }
    ]
  }]
}
```

Create `tests/e2e/fixtures/k8s_deployment.yaml`:
```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: nginx
  namespace: default
spec:
  replicas: 1
  selector:
    matchLabels:
      app: nginx
  template:
    metadata:
      labels:
        app: nginx
    spec:
      containers:
      - name: nginx
        image: nginx:1.21
        ports:
        - containerPort: 80
```

### Step 2: Verify fixtures parse correctly

Run: `go test -run TestSarifParse ./internal/sarif/ -v -count=1`
Expected: Existing tests PASS (fixtures are for E2E, not unit tests)

### Step 3: Commit

```bash
git add tests/e2e/fixtures/
git commit -m "feat: add SARIF and K8s fixture files for E2E tests"
```

---

## Task 5: E2E-1 — Signing end-to-end test

**Files:**
- Create: `tests/e2e/signing_test.go`

This is a Go integration test with `//go:build e2e` tag. It invokes the `evidra` CLI binary (must be built first) and verifies signing works end-to-end.

### Step 1: Write the E2E test

Create `tests/e2e/signing_test.go`:

```go
//go:build e2e

package e2e

import (
	"bytes"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"samebits.com/evidra-benchmark/pkg/evidence"
)

func evidraBinary(t *testing.T) string {
	t.Helper()
	// Build the binary if not already built
	bin := filepath.Join(os.TempDir(), "evidra-e2e-test")
	cmd := exec.Command("go", "build", "-o", bin, "../../cmd/evidra")
	cmd.Dir = filepath.Join("..", "..")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("build evidra: %v\n%s", err, out)
	}
	return bin
}

func generateKeyPair(t *testing.T, dir string) (privPath, pubPath string) {
	t.Helper()
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}

	der, _ := x509.MarshalPKCS8PrivateKey(priv)
	privPath = filepath.Join(dir, "signing.pem")
	os.WriteFile(privPath, pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der}), 0o600)

	pubDer, _ := x509.MarshalPKIXPublicKey(pub)
	pubPath = filepath.Join(dir, "signing.pub.pem")
	os.WriteFile(pubPath, pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: pubDer}), 0o600)
	return
}

func runEvidra(t *testing.T, bin string, args ...string) (stdout, stderr string, exitCode int) {
	t.Helper()
	var outBuf, errBuf bytes.Buffer
	cmd := exec.Command(bin, args...)
	cmd.Stdout = &outBuf
	cmd.Stderr = &errBuf
	err := cmd.Run()
	exitCode = 0
	if err != nil {
		if ee, ok := err.(*exec.ExitError); ok {
			exitCode = ee.ExitCode()
		} else {
			t.Fatalf("exec %v: %v", args, err)
		}
	}
	return outBuf.String(), errBuf.String(), exitCode
}

func TestE2E_SigningEndToEnd(t *testing.T) {
	bin := evidraBinary(t)
	tmp := t.TempDir()
	privPath, pubPath := generateKeyPair(t, tmp)
	evidenceDir := filepath.Join(tmp, "evidence")
	artifactPath := filepath.Join("..", "..", "tests", "e2e", "fixtures", "k8s_deployment.yaml")

	// Step 1: Prescribe with signing
	stdout, stderr, code := runEvidra(t, bin,
		"prescribe",
		"--tool", "kubectl",
		"--operation", "apply",
		"--artifact", artifactPath,
		"--evidence-dir", evidenceDir,
		"--session-id", "e2e-signing-001",
		"--signing-key-path", privPath,
	)
	if code != 0 {
		t.Fatalf("prescribe exit %d: %s", code, stderr)
	}

	var prescribeResult map[string]interface{}
	if err := json.Unmarshal([]byte(stdout), &prescribeResult); err != nil {
		t.Fatalf("parse prescribe output: %v", err)
	}
	prescriptionID := prescribeResult["prescription_id"].(string)

	// Step 2: Report with signing
	_, stderr, code = runEvidra(t, bin,
		"report",
		"--prescription", prescriptionID,
		"--exit-code", "0",
		"--evidence-dir", evidenceDir,
		"--session-id", "e2e-signing-001",
		"--signing-key-path", privPath,
	)
	if code != 0 {
		t.Fatalf("report exit %d: %s", code, stderr)
	}

	// Step 3: Validate — chain + signatures
	stdout, stderr, code = runEvidra(t, bin,
		"validate",
		"--evidence-dir", evidenceDir,
		"--public-key", pubPath,
	)
	if code != 0 {
		t.Fatalf("validate exit %d: %s", code, stderr)
	}
	if !strings.Contains(stdout, "signatures verified") {
		t.Fatalf("validate output missing 'signatures verified': %s", stdout)
	}

	// Step 4: Verify all entries have signatures
	entries, err := evidence.ReadAllEntriesAtPath(evidenceDir)
	if err != nil {
		t.Fatal(err)
	}
	for i, e := range entries {
		if e.Signature == "" {
			t.Fatalf("entry %d (%s) has empty signature", i, e.EntryID)
		}
		if e.SessionID != "e2e-signing-001" {
			t.Fatalf("entry %d session_id = %q, want e2e-signing-001", i, e.SessionID)
		}
	}

	// Step 5: Tamper detection — modify 1 byte in segment file
	segFiles, _ := filepath.Glob(filepath.Join(evidenceDir, "segments", "*.jsonl"))
	if len(segFiles) == 0 {
		t.Fatal("no segment files found")
	}
	data, _ := os.ReadFile(segFiles[0])
	tampered := bytes.Replace(data, []byte(`"apply"`), []byte(`"delet"`), 1)
	os.WriteFile(segFiles[0], tampered, 0o644)

	_, stderr, code = runEvidra(t, bin,
		"validate",
		"--evidence-dir", evidenceDir,
		"--public-key", pubPath,
	)
	if code == 0 {
		t.Fatal("validate should fail after tampering")
	}
	if !strings.Contains(stderr, "hash mismatch") && !strings.Contains(stderr, "validation failed") {
		t.Fatalf("expected hash/validation error, got: %s", stderr)
	}
}
```

### Step 2: Build and run the test

Run: `go test -tags e2e -run TestE2E_SigningEndToEnd ./tests/e2e/ -v -count=1 -timeout=60s`
Expected: PASS (after Task 1 is completed)

### Step 3: Commit

```bash
git add tests/e2e/signing_test.go
git commit -m "test: E2E-1 signing end-to-end test"
```

---

## Task 6: E2E-2 — Session-filtered scoring

**Files:**
- Create: `tests/e2e/session_scoring_test.go`

### Step 1: Write the E2E test

Create `tests/e2e/session_scoring_test.go`:

```go
//go:build e2e

package e2e

import (
	"encoding/json"
	"path/filepath"
	"testing"
)

func TestE2E_SessionFilteredScoring(t *testing.T) {
	bin := evidraBinary(t)
	tmp := t.TempDir()
	evidenceDir := filepath.Join(tmp, "evidence")
	artifactPath := filepath.Join("..", "..", "tests", "e2e", "fixtures", "k8s_deployment.yaml")

	// Session A: clean run (prescribe + successful report)
	stdout, stderr, code := runEvidra(t, bin,
		"prescribe",
		"--tool", "kubectl",
		"--operation", "apply",
		"--artifact", artifactPath,
		"--evidence-dir", evidenceDir,
		"--session-id", "session-A",
		"--actor", "agent-clean",
	)
	if code != 0 {
		t.Fatalf("prescribe A: exit %d: %s", code, stderr)
	}
	var pA map[string]interface{}
	json.Unmarshal([]byte(stdout), &pA)

	runEvidra(t, bin,
		"report",
		"--prescription", pA["prescription_id"].(string),
		"--exit-code", "0",
		"--evidence-dir", evidenceDir,
		"--session-id", "session-A",
		"--actor", "agent-clean",
	)

	// Session B: bad run (prescribe + failed report with drift)
	stdout, stderr, code = runEvidra(t, bin,
		"prescribe",
		"--tool", "kubectl",
		"--operation", "apply",
		"--artifact", artifactPath,
		"--evidence-dir", evidenceDir,
		"--session-id", "session-B",
		"--actor", "agent-clean",
	)
	if code != 0 {
		t.Fatalf("prescribe B: exit %d: %s", code, stderr)
	}
	var pB map[string]interface{}
	json.Unmarshal([]byte(stdout), &pB)

	// Report with exit code 1 (failure) and different artifact digest (drift)
	runEvidra(t, bin,
		"report",
		"--prescription", pB["prescription_id"].(string),
		"--exit-code", "1",
		"--evidence-dir", evidenceDir,
		"--session-id", "session-B",
		"--actor", "agent-clean",
		"--artifact-digest", "sha256:different_from_prescription",
	)

	// Scorecard for session-A only
	stdout, stderr, code = runEvidra(t, bin,
		"scorecard",
		"--evidence-dir", evidenceDir,
		"--session-id", "session-A",
	)
	if code != 0 {
		t.Fatalf("scorecard A: exit %d: %s", code, stderr)
	}
	var scA map[string]interface{}
	json.Unmarshal([]byte(stdout), &scA)

	// Scorecard for session-B only
	stdout, _, code = runEvidra(t, bin,
		"scorecard",
		"--evidence-dir", evidenceDir,
		"--session-id", "session-B",
	)
	if code != 0 {
		t.Fatalf("scorecard B: exit %d", code)
	}
	var scB map[string]interface{}
	json.Unmarshal([]byte(stdout), &scB)

	// Session A should have higher or equal score than session B
	scoreA := scA["score"].(float64)
	scoreB := scB["score"].(float64)
	if scoreA < scoreB {
		t.Fatalf("session-A score (%f) should be >= session-B score (%f)", scoreA, scoreB)
	}

	// Each session should have 1 operation
	opsA := int(scA["total_operations"].(float64))
	opsB := int(scB["total_operations"].(float64))
	if opsA != 1 {
		t.Fatalf("session-A ops = %d, want 1", opsA)
	}
	if opsB != 1 {
		t.Fatalf("session-B ops = %d, want 1", opsB)
	}

	// Full scorecard (no session filter) should have 2 operations
	stdout, _, code = runEvidra(t, bin,
		"scorecard",
		"--evidence-dir", evidenceDir,
	)
	if code != 0 {
		t.Fatalf("scorecard all: exit %d", code)
	}
	var scAll map[string]interface{}
	json.Unmarshal([]byte(stdout), &scAll)
	opsAll := int(scAll["total_operations"].(float64))
	if opsAll != 2 {
		t.Fatalf("total ops (all) = %d, want 2", opsAll)
	}
}
```

### Step 2: Run

Run: `go test -tags e2e -run TestE2E_SessionFilteredScoring ./tests/e2e/ -v -count=1 -timeout=60s`
Expected: PASS

### Step 3: Commit

```bash
git add tests/e2e/session_scoring_test.go
git commit -m "test: E2E-2 session-filtered scoring test"
```

---

## Task 7: E2E-3 — Standalone findings ingestion

**Files:**
- Create: `tests/e2e/findings_test.go`

### Step 1: Write the E2E test

Create `tests/e2e/findings_test.go`:

```go
//go:build e2e

package e2e

import (
	"encoding/json"
	"path/filepath"
	"testing"

	"samebits.com/evidra-benchmark/pkg/evidence"
)

func TestE2E_FindingsIngestion(t *testing.T) {
	bin := evidraBinary(t)
	tmp := t.TempDir()
	evidenceDir := filepath.Join(tmp, "evidence")
	artifactPath := filepath.Join("..", "..", "tests", "e2e", "fixtures", "k8s_deployment.yaml")
	trivySarif := filepath.Join("..", "..", "tests", "e2e", "fixtures", "trivy.sarif")
	kubescapeSarif := filepath.Join("..", "..", "tests", "e2e", "fixtures", "kubescape.sarif")

	sessionID := "e2e-findings-001"

	// Step 1: Ingest pre-prescribe findings (trivy)
	stdout, stderr, code := runEvidra(t, bin,
		"ingest-findings",
		"--sarif", trivySarif,
		"--artifact", artifactPath,
		"--evidence-dir", evidenceDir,
		"--session-id", sessionID,
		"--actor", "scanner-trivy",
	)
	if code != 0 {
		t.Fatalf("ingest trivy: exit %d: %s", code, stderr)
	}
	var trivyResult map[string]interface{}
	json.Unmarshal([]byte(stdout), &trivyResult)
	trivyCount := int(trivyResult["findings_count"].(float64))
	if trivyCount != 2 {
		t.Fatalf("trivy findings = %d, want 2", trivyCount)
	}

	// Step 2: Prescribe
	stdout, stderr, code = runEvidra(t, bin,
		"prescribe",
		"--tool", "kubectl",
		"--operation", "apply",
		"--artifact", artifactPath,
		"--evidence-dir", evidenceDir,
		"--session-id", sessionID,
	)
	if code != 0 {
		t.Fatalf("prescribe: exit %d: %s", code, stderr)
	}
	var prescribeResult map[string]interface{}
	json.Unmarshal([]byte(stdout), &prescribeResult)
	prescriptionID := prescribeResult["prescription_id"].(string)

	// Step 3: Report
	_, stderr, code = runEvidra(t, bin,
		"report",
		"--prescription", prescriptionID,
		"--exit-code", "0",
		"--evidence-dir", evidenceDir,
		"--session-id", sessionID,
	)
	if code != 0 {
		t.Fatalf("report: exit %d: %s", code, stderr)
	}

	// Step 4: Ingest post-report findings (kubescape)
	stdout, stderr, code = runEvidra(t, bin,
		"ingest-findings",
		"--sarif", kubescapeSarif,
		"--artifact", artifactPath,
		"--evidence-dir", evidenceDir,
		"--session-id", sessionID,
		"--actor", "scanner-kubescape",
	)
	if code != 0 {
		t.Fatalf("ingest kubescape: exit %d: %s", code, stderr)
	}
	var kubeResult map[string]interface{}
	json.Unmarshal([]byte(stdout), &kubeResult)
	kubeCount := int(kubeResult["findings_count"].(float64))
	if kubeCount != 1 {
		t.Fatalf("kubescape findings = %d, want 1", kubeCount)
	}

	// Step 5: Verify evidence chain
	entries, err := evidence.ReadAllEntriesAtPath(evidenceDir)
	if err != nil {
		t.Fatal(err)
	}

	// Count by type
	counts := make(map[evidence.EntryType]int)
	sessionEntries := 0
	for _, e := range entries {
		counts[e.Type]++
		if e.SessionID == sessionID {
			sessionEntries++
		}
	}

	if counts[evidence.EntryTypeFinding] != 3 {
		t.Fatalf("finding entries = %d, want 3 (2 trivy + 1 kubescape)", counts[evidence.EntryTypeFinding])
	}
	if counts[evidence.EntryTypePrescribe] != 1 {
		t.Fatalf("prescribe entries = %d, want 1", counts[evidence.EntryTypePrescribe])
	}
	if counts[evidence.EntryTypeReport] != 1 {
		t.Fatalf("report entries = %d, want 1", counts[evidence.EntryTypeReport])
	}

	// All entries with session-id should match
	if sessionEntries != 5 {
		t.Fatalf("entries with session_id = %d, want 5", sessionEntries)
	}

	// Artifact digests should match between findings and prescription
	artifactDigest := trivyResult["artifact_digest"].(string)
	prescribeDigest := prescribeResult["artifact_digest"].(string)
	if artifactDigest != prescribeDigest {
		t.Fatalf("artifact digest mismatch: findings=%q, prescribe=%q", artifactDigest, prescribeDigest)
	}

	// Chain integrity
	if err := evidence.ValidateChainAtPath(evidenceDir); err != nil {
		t.Fatalf("chain validation failed: %v", err)
	}
}
```

### Step 2: Run

Run: `go test -tags e2e -run TestE2E_FindingsIngestion ./tests/e2e/ -v -count=1 -timeout=60s`
Expected: PASS

### Step 3: Commit

```bash
git add tests/e2e/findings_test.go
git commit -m "test: E2E-3 findings ingestion lifecycle test"
```

---

## Task 8: Add E2E tests to CI

**Files:**
- Modify: `.github/workflows/ci.yml`
- Modify: `Makefile`

### Step 1: Add e2e target to Makefile

Check the current Makefile first. Add:

```makefile
.PHONY: e2e
e2e: build
	go test -tags e2e ./tests/e2e/ -v -count=1 -timeout=120s
```

### Step 2: Add E2E job to CI

Add to `.github/workflows/ci.yml`:

```yaml
  e2e:
    runs-on: ubuntu-latest
    timeout-minutes: 10
    needs: test  # run after unit tests pass
    steps:
      - name: Checkout
        uses: actions/checkout@v6

      - name: Setup Go
        uses: actions/setup-go@v5
        with:
          go-version-file: go.mod
          cache: true

      - name: Run E2E tests
        run: make e2e
```

### Step 3: Run locally

Run: `make e2e`
Expected: All 3 E2E tests PASS

### Step 4: Commit

```bash
git add .github/workflows/ci.yml Makefile
git commit -m "ci: add E2E test job to CI pipeline"
```

---

## Task 9: GitHub Action — composite action

**Files:**
- Create: `.github/actions/evidra/action.yml`

### Step 1: Create the composite action

Create `.github/actions/evidra/action.yml`:

```yaml
name: 'Evidra Benchmark'
description: 'Run evidra benchmark — validate evidence, score reliability, ingest findings'

inputs:
  evidence-dir:
    description: 'Evidence directory'
    required: false
    default: './evidence'
  session-id:
    description: 'Session ID for grouping'
    required: false
    default: ${{ github.run_id }}
  sarif-path:
    description: 'SARIF scanner report to ingest'
    required: false
  public-key:
    description: 'Path to Ed25519 public key PEM for signature verification'
    required: false
  fail-on-risk:
    description: 'Fail if risk level exceeds threshold (low, medium, high, critical)'
    required: false
  evidra-version:
    description: 'Evidra version to download'
    required: false
    default: 'latest'

outputs:
  score:
    description: 'Reliability score (0-100)'
    value: ${{ steps.scorecard.outputs.score }}
  risk_level:
    description: 'Risk level from evidence'
    value: ${{ steps.scorecard.outputs.risk_level }}
  evidence-path:
    description: 'Evidence directory path'
    value: ${{ inputs.evidence-dir }}

runs:
  using: 'composite'
  steps:
    - name: Download evidra CLI
      shell: bash
      run: |
        VERSION="${{ inputs.evidra-version }}"
        if [ "$VERSION" = "latest" ]; then
          VERSION=$(gh release view --repo ${{ github.repository }} --json tagName -q .tagName 2>/dev/null || echo "v0.3.1")
        fi
        ARCHIVE="evidra_${VERSION#v}_linux_amd64.tar.gz"
        URL="https://github.com/${{ github.repository }}/releases/download/${VERSION}/${ARCHIVE}"
        echo "Downloading evidra ${VERSION}..."
        curl -sL "$URL" -o /tmp/evidra.tar.gz || {
          echo "::warning::Could not download release, building from source"
          go build -o /tmp/evidra ./cmd/evidra
          exit 0
        }
        tar -xzf /tmp/evidra.tar.gz -C /tmp/
        chmod +x /tmp/evidra

    - name: Ingest SARIF findings
      if: inputs.sarif-path != ''
      shell: bash
      run: |
        /tmp/evidra ingest-findings \
          --sarif "${{ inputs.sarif-path }}" \
          --evidence-dir "${{ inputs.evidence-dir }}" \
          --session-id "${{ inputs.session-id }}"

    - name: Validate evidence chain
      shell: bash
      run: |
        ARGS="--evidence-dir ${{ inputs.evidence-dir }}"
        if [ -n "${{ inputs.public-key }}" ]; then
          ARGS="$ARGS --public-key ${{ inputs.public-key }}"
        fi
        /tmp/evidra validate $ARGS

    - name: Generate scorecard
      id: scorecard
      shell: bash
      run: |
        OUTPUT=$(/tmp/evidra scorecard \
          --evidence-dir "${{ inputs.evidence-dir }}" \
          --session-id "${{ inputs.session-id }}" 2>/dev/null || echo '{"score":0,"band":"unknown"}')
        SCORE=$(echo "$OUTPUT" | jq -r '.score // 0')
        BAND=$(echo "$OUTPUT" | jq -r '.band // "unknown"')
        echo "score=$SCORE" >> $GITHUB_OUTPUT
        echo "risk_level=$BAND" >> $GITHUB_OUTPUT

        # Write job summary
        echo "## Evidra Benchmark Results" >> $GITHUB_STEP_SUMMARY
        echo "" >> $GITHUB_STEP_SUMMARY
        echo "| Metric | Value |" >> $GITHUB_STEP_SUMMARY
        echo "|--------|-------|" >> $GITHUB_STEP_SUMMARY
        echo "| Score | **${SCORE}** |" >> $GITHUB_STEP_SUMMARY
        echo "| Band | ${BAND} |" >> $GITHUB_STEP_SUMMARY
        echo "| Session | \`${{ inputs.session-id }}\` |" >> $GITHUB_STEP_SUMMARY
        echo "" >> $GITHUB_STEP_SUMMARY
        echo "<details><summary>Full scorecard</summary>" >> $GITHUB_STEP_SUMMARY
        echo "" >> $GITHUB_STEP_SUMMARY
        echo '```json' >> $GITHUB_STEP_SUMMARY
        echo "$OUTPUT" | jq . >> $GITHUB_STEP_SUMMARY
        echo '```' >> $GITHUB_STEP_SUMMARY
        echo "</details>" >> $GITHUB_STEP_SUMMARY

    - name: Check risk threshold
      if: inputs.fail-on-risk != ''
      shell: bash
      run: |
        BAND="${{ steps.scorecard.outputs.risk_level }}"
        THRESHOLD="${{ inputs.fail-on-risk }}"

        declare -A LEVELS=([excellent]=0 [good]=1 [fair]=2 [poor]=3)
        declare -A THRESHOLDS=([low]=1 [medium]=2 [high]=3 [critical]=3)

        BAND_LEVEL=${LEVELS[$BAND]:-4}
        THRESHOLD_LEVEL=${THRESHOLDS[$THRESHOLD]:-4}

        if [ "$BAND_LEVEL" -ge "$THRESHOLD_LEVEL" ]; then
          echo "::error::Risk threshold exceeded: band=$BAND, threshold=$THRESHOLD"
          exit 1
        fi

    - name: Upload evidence artifact
      uses: actions/upload-artifact@v4
      with:
        name: evidra-evidence
        path: ${{ inputs.evidence-dir }}
        if-no-files-found: warn
```

### Step 2: Verify action syntax

Run: `cat .github/actions/evidra/action.yml | head -5`
Expected: Valid YAML

### Step 3: Commit

```bash
git add .github/actions/evidra/action.yml
git commit -m "feat: add evidra GitHub Action (composite)"
```

---

## Task 10: Update documentation

**Files:**
- Modify: `README.md` — add CLI signing flags, session-id filter, GitHub Action usage
- Modify: `docs/system-design/EVIDRA_ARCHITECTURE_OVERVIEW.md` — update signing section
- Move: `docs/plans/plan_for_v.0.3.x.md` → `docs/plans/done/`

### Step 1: Update README

Add to README.md under CLI section:

**Signing:**
```
# Sign evidence with Ed25519 key
evidra prescribe --signing-key-path key.pem --artifact deploy.yaml --tool kubectl
evidra report --signing-key-path key.pem --prescription <ID> --exit-code 0

# Or via environment variable
export EVIDRA_SIGNING_KEY_PATH=/path/to/key.pem
evidra prescribe ...

# Validate chain + signatures
evidra validate --evidence-dir ./evidence --public-key pub.pem
```

**Session filtering:**
```
# Score a specific session
evidra scorecard --session-id $GITHUB_RUN_ID

# Explain signals for a session
evidra explain --session-id $GITHUB_RUN_ID
```

**GitHub Action:**
```yaml
- name: Run Evidra Benchmark
  uses: ./.github/actions/evidra
  with:
    evidence-dir: ./evidence
    session-id: ${{ github.run_id }}
    sarif-path: trivy-results.sarif
    public-key: signing.pub.pem
    fail-on-risk: high
```

### Step 2: Update architecture overview

In `docs/system-design/EVIDRA_ARCHITECTURE_OVERVIEW.md`, update "Signing Is Wired" section to note that CLI and MCP server both accept signing keys.

### Step 3: Move plan to done

```bash
git mv docs/plans/plan_for_v.0.3.x.md docs/plans/done/
```

### Step 4: Commit

```bash
git add README.md docs/system-design/EVIDRA_ARCHITECTURE_OVERVIEW.md docs/plans/done/
git commit -m "docs: update README with signing, session-id, and GitHub Action usage"
```

---

## Dependency Graph

```
Task 1 (wire signing CLI) ─────────┐
Task 2 (session-id filter) ────────┤
Task 3 (wire signing MCP) ─────────┤
Task 4 (fixtures) ─────────────────┤
                                    ├──► Task 5 (E2E-1 signing)
                                    ├──► Task 6 (E2E-2 sessions)
                                    ├──► Task 7 (E2E-3 findings)
                                    │
                                    ├──► Task 8 (E2E in CI)
                                    ├──► Task 9 (GitHub Action)
                                    └──► Task 10 (docs)
```

Tasks 1–4 can be done in parallel. Tasks 5–7 depend on 1–4. Tasks 8–10 depend on 5–7.

---

## Running Everything

```bash
# Unit tests (always)
go test ./... -count=1

# E2E tests (requires build)
make e2e

# Race detector
go test -race ./... -count=1

# Full CI simulation
make test && make e2e && make lint
```
