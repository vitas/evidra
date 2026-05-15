# Evidra Core Simplification Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Refactor Evidra into an intent/outcome evidence ledger where canonicalization and assessment are optional external enrichments.

**Architecture:** Core prescribe/report stays in Evidra. Core records declared intent, optional artifact digests, optional canonical identity, and optional assessment results, then signs/stores evidence and runs analytics. Built-in canonicalizers, risk matrix execution, native detectors, and SARIF risk mapping leave the core write path and are removed or moved to a companion repository later.

**Tech Stack:** Go, JSON evidence contracts, CLI/MCP/API handlers, repository docs, golangci-lint

---

### Task 1: Add The Core Prescribe Contract

**Files:**
- Modify: `pkg/evidence/payloads.go`
- Modify: `pkg/evidence/payloads_test.go`

**Step 1: Write failing payload tests**

Add tests covering a prescribe payload with only declared intent and no
canonical or assessment enrichment:

```go
func TestPrescriptionPayload_DeclaredIntentOnly(t *testing.T) {
	payload := PrescriptionPayload{
		PrescriptionID: "presc_1",
		Intent: &DeclaredIntent{
			Tool:           "kubectl",
			Operation:      "apply",
			Target:         "deployment/web",
			Command:        "kubectl apply -f deploy.yaml",
			ArtifactDigest: "sha256:" + strings.Repeat("a", 64),
		},
		Assessment: &AssessmentPayload{Status: AssessmentNotProvided},
		TTLMs:      DefaultTTLMs,
	}

	raw, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}

	var decoded PrescriptionPayload
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded.CanonicalAction != nil {
		t.Fatalf("canonical_action = %s, want nil", decoded.CanonicalAction)
	}
	if decoded.Assessment.Status != AssessmentNotProvided {
		t.Fatalf("assessment.status = %q", decoded.Assessment.Status)
	}
	if decoded.Intent.Tool != "kubectl" || decoded.Intent.Target != "deployment/web" {
		t.Fatalf("intent = %+v", decoded.Intent)
	}
}
```

Add a second test for optional assessment:

```go
func TestPrescriptionPayload_AssessmentProvided(t *testing.T) {
	payload := PrescriptionPayload{
		PrescriptionID: "presc_2",
		Intent: &DeclaredIntent{Tool: "trivy", Operation: "scan"},
		Assessment: &AssessmentPayload{
			Status:        AssessmentProvided,
			Provider:      "trivy",
			RiskInputs:    []RiskInput{{Source: "trivy", RiskLevel: "high"}},
			EffectiveRisk: "high",
		},
		TTLMs: DefaultTTLMs,
	}

	if got := payload.EffectiveRiskLevel(); got != "high" {
		t.Fatalf("EffectiveRiskLevel() = %q, want high", got)
	}
}
```

**Step 2: Run tests to verify they fail**

Run:

```bash
go test ./pkg/evidence -run 'TestPrescriptionPayload_(DeclaredIntentOnly|AssessmentProvided)' -count=1
```

Expected: compile failure for missing `DeclaredIntent`, `AssessmentPayload`,
and helper methods.

**Step 3: Add minimal contract types**

In `pkg/evidence/payloads.go`, add:

```go
type AssessmentStatus string

const (
	AssessmentProvided    AssessmentStatus = "provided"
	AssessmentNotProvided AssessmentStatus = "not_provided"
	AssessmentFailed      AssessmentStatus = "failed"
)

type DeclaredIntent struct {
	Tool           string `json:"tool,omitempty"`
	Operation      string `json:"operation,omitempty"`
	Target         string `json:"target,omitempty"`
	Command        string `json:"command,omitempty"`
	ArtifactDigest string `json:"artifact_digest,omitempty"`
}

type AssessmentPayload struct {
	Status        AssessmentStatus `json:"status"`
	Provider      string           `json:"provider,omitempty"`
	RiskInputs    []RiskInput      `json:"risk_inputs,omitempty"`
	EffectiveRisk string           `json:"effective_risk,omitempty"`
	Detail        string           `json:"detail,omitempty"`
}
```

Update `PrescriptionPayload`:

```go
Intent          *DeclaredIntent    `json:"intent,omitempty"`
CanonicalAction json.RawMessage    `json:"canonical_action,omitempty"`
Assessment      *AssessmentPayload `json:"assessment,omitempty"`
```

Keep existing top-level `RiskInputs` and `EffectiveRisk` temporarily as
compatibility fields while migrating readers. Add methods:

```go
func (p PrescriptionPayload) EffectiveRiskLevel() string {
	if p.Assessment != nil && p.Assessment.EffectiveRisk != "" {
		return p.Assessment.EffectiveRisk
	}
	return p.EffectiveRisk
}

func (p PrescriptionPayload) AssessmentRiskInputs() []RiskInput {
	if p.Assessment != nil && len(p.Assessment.RiskInputs) > 0 {
		return p.Assessment.RiskInputs
	}
	return p.RiskInputs
}
```

**Step 4: Run tests**

Run:

```bash
go test ./pkg/evidence -count=1
```

Expected: PASS.

**Step 5: Commit**

```bash
git add pkg/evidence/payloads.go pkg/evidence/payloads_test.go
git commit -s -m "feat: add intent-first prescribe payload"
```

### Task 2: Add Shared Validation Helpers

**Files:**
- Create: `pkg/evidence/validation.go`
- Create: `pkg/evidence/validation_test.go`
- Create: `pkg/evidence/digest.go`
- Create: `pkg/evidence/digest_test.go`

**Step 1: Write failing validation tests**

Cover digest, risk level, and assessment status validation:

```go
func TestValidateDigest(t *testing.T) {
	valid := "sha256:" + strings.Repeat("a", 64)
	if err := ValidateDigest(valid); err != nil {
		t.Fatalf("valid digest rejected: %v", err)
	}
	if err := ValidateDigest("sha256:not-hex"); err == nil {
		t.Fatal("invalid digest accepted")
	}
}

func TestValidateRiskLevel(t *testing.T) {
	for _, level := range []string{"low", "medium", "high", "critical", "unknown"} {
		if err := ValidateRiskLevel(level); err != nil {
			t.Fatalf("%q rejected: %v", level, err)
		}
	}
	if err := ValidateRiskLevel("severe"); err == nil {
		t.Fatal("invalid risk level accepted")
	}
}
```

**Step 2: Run tests to verify they fail**

Run:

```bash
go test ./pkg/evidence -run 'TestValidate(Digest|RiskLevel)' -count=1
```

Expected: compile failure for missing validation helpers.

**Step 3: Implement minimal helpers**

Add validation helpers:

```go
var digestPattern = regexp.MustCompile(`^sha256:[a-f0-9]{64}$`)

func ValidateDigest(v string) error {
	if strings.TrimSpace(v) == "" {
		return nil
	}
	if !digestPattern.MatchString(v) {
		return fmt.Errorf("digest must match sha256:<64 lowercase hex>")
	}
	return nil
}

func ValidateRiskLevel(level string) error {
	switch level {
	case "", "low", "medium", "high", "critical", "unknown":
		return nil
	default:
		return fmt.Errorf("invalid risk level %q", level)
	}
}

func ValidateAssessmentStatus(status AssessmentStatus) error {
	switch status {
	case "", AssessmentProvided, AssessmentNotProvided, AssessmentFailed:
		return nil
	default:
		return fmt.Errorf("invalid assessment status %q", status)
	}
}
```

Add digest helpers so core does not depend on `internal/canon`:

```go
func SHA256Hex(data []byte) string {
	sum := sha256.Sum256(data)
	return "sha256:" + hex.EncodeToString(sum[:])
}

func ComputeDeclaredIntentDigest(intent DeclaredIntent) string {
	identity := struct {
		Tool      string `json:"tool,omitempty"`
		Operation string `json:"operation,omitempty"`
		Target    string `json:"target,omitempty"`
		Command   string `json:"command,omitempty"`
	}{
		Tool:      strings.TrimSpace(intent.Tool),
		Operation: strings.TrimSpace(intent.Operation),
		Target:    strings.TrimSpace(intent.Target),
		Command:   strings.TrimSpace(intent.Command),
	}
	raw, _ := json.Marshal(identity)
	return SHA256Hex(raw)
}
```

Add tests:

```go
func TestSHA256Hex(t *testing.T) {
	got := SHA256Hex([]byte("hello"))
	if !strings.HasPrefix(got, "sha256:") || len(got) != len("sha256:")+64 {
		t.Fatalf("digest = %q", got)
	}
}

func TestComputeDeclaredIntentDigest_ExcludesArtifactDigest(t *testing.T) {
	a := DeclaredIntent{Tool: "kubectl", Operation: "apply", Target: "deployment/web", ArtifactDigest: "sha256:" + strings.Repeat("a", 64)}
	b := a
	b.ArtifactDigest = "sha256:" + strings.Repeat("b", 64)
	if ComputeDeclaredIntentDigest(a) != ComputeDeclaredIntentDigest(b) {
		t.Fatal("declared intent digest should exclude artifact digest")
	}
}
```

**Step 4: Run tests**

Run:

```bash
go test ./pkg/evidence -count=1
```

Expected: PASS.

**Step 5: Commit**

```bash
git add pkg/evidence/validation.go pkg/evidence/validation_test.go pkg/evidence/digest.go pkg/evidence/digest_test.go
git commit -s -m "feat: validate prescribe enrichments"
```

### Task 3: Refactor Lifecycle Prescribe Around Declared Intent

**Files:**
- Modify: `internal/lifecycle/types.go`
- Modify: `internal/lifecycle/service.go`
- Modify: `internal/lifecycle/service_test.go`

**Step 1: Write failing lifecycle tests**

Add tests proving core can prescribe without canonicalization or assessment:

```go
func TestPrescribe_DeclaredIntentWithoutAssessment(t *testing.T) {
	dir := t.TempDir()
	svc := NewService(Options{EvidencePath: dir, Signer: testutil.TestSigner(t)})

	out, err := svc.Prescribe(context.Background(), PrescribeInput{
		Actor: evidence.Actor{Type: "agent", ID: "agent-1", Provenance: "test"},
		Intent: evidence.DeclaredIntent{
			Tool:      "kubectl",
			Operation: "apply",
			Target:    "deployment/web",
			Command:   "kubectl apply -f deploy.yaml",
		},
	})
	if err != nil {
		t.Fatalf("Prescribe: %v", err)
	}

	var payload evidence.PrescriptionPayload
	if err := json.Unmarshal(out.Entry.Payload, &payload); err != nil {
		t.Fatal(err)
	}
	if payload.CanonicalAction != nil {
		t.Fatalf("canonical_action = %s, want nil", payload.CanonicalAction)
	}
	if payload.Assessment == nil || payload.Assessment.Status != evidence.AssessmentNotProvided {
		t.Fatalf("assessment = %+v", payload.Assessment)
	}
}
```

Add a second test that raw artifact bytes are used only to compute digest:

```go
func TestPrescribe_RawArtifactOnlyComputesDigest(t *testing.T) {
	svc := NewService(Options{EvidencePath: t.TempDir(), Signer: testutil.TestSigner(t)})
	out, err := svc.Prescribe(context.Background(), PrescribeInput{
		Actor:      evidence.Actor{Type: "agent", ID: "agent-1", Provenance: "test"},
		Intent:     evidence.DeclaredIntent{Tool: "kubectl", Operation: "apply"},
		RawArtifact: []byte("kind: ConfigMap\n"),
	})
	if err != nil {
		t.Fatal(err)
	}
	if out.ArtifactDigest == "" {
		t.Fatal("missing artifact digest")
	}
	if out.CanonVersion != "" {
		t.Fatalf("CanonVersion = %q, want empty", out.CanonVersion)
	}
}
```

**Step 2: Run tests to verify they fail**

Run:

```bash
go test ./internal/lifecycle -run 'TestPrescribe_(DeclaredIntentWithoutAssessment|RawArtifactOnlyComputesDigest)' -count=1
```

Expected: compile failure or runtime failure because lifecycle still expects
canonicalization.

**Step 3: Update lifecycle input/output types**

In `internal/lifecycle/types.go`:

- Add `Intent evidence.DeclaredIntent`
- Add `CanonicalAction json.RawMessage`
- Add `Assessment *evidence.AssessmentPayload`
- Keep `Tool`, `Operation`, and `RawArtifact` temporarily as adapter-facing
  convenience fields, but map them into `Intent`
- Remove `Pipeline *assess.Pipeline`
- Remove `CanonicalAction *canon.CanonicalAction`
- Remove `ExternalFindings []ExternalFindingsSource`

Output should keep stable response fields but allow empty canonical fields:

```go
type PrescribeOutput struct {
	PrescriptionID string
	SessionID      string
	TraceID        string
	Actor          evidence.Actor
	Intent         evidence.DeclaredIntent
	Assessment    *evidence.AssessmentPayload
	ArtifactDigest string
	Entry          evidence.EvidenceEntry
	RawEntry       json.RawMessage
	Persisted      bool
}
```

**Step 4: Replace canonicalization with intent assembly**

In `internal/lifecycle/service.go`, replace `canonicalizePrescribeInput` with
a helper shaped like:

```go
func buildDeclaredIntent(input PrescribeInput) (evidence.DeclaredIntent, string, error) {
	intent := input.Intent
	if intent.Tool == "" {
		intent.Tool = normalizeToken(input.Tool)
	}
	if intent.Operation == "" {
		intent.Operation = normalizeToken(input.Operation)
	}
	if intent.ArtifactDigest == "" && len(input.RawArtifact) > 0 {
		intent.ArtifactDigest = evidence.SHA256Hex(input.RawArtifact)
	}
	if strings.TrimSpace(intent.Tool) == "" && strings.TrimSpace(intent.Operation) == "" && strings.TrimSpace(intent.Command) == "" && strings.TrimSpace(intent.Target) == "" {
		return evidence.DeclaredIntent{}, "", wrapError(ErrCodeInvalidInput, "prescribe intent is required", nil)
	}
	if err := evidence.ValidateDigest(intent.ArtifactDigest); err != nil {
		return evidence.DeclaredIntent{}, "", wrapError(ErrCodeInvalidInput, err.Error(), err)
	}
	return intent, intent.ArtifactDigest, nil
}
```

Use `crypto/sha256` locally or add a digest helper to `pkg/evidence`; do not
import `internal/canon`.

Build `PrescriptionPayload` from intent plus optional enrichment:

```go
assessment := input.Assessment
if assessment == nil {
	assessment = &evidence.AssessmentPayload{Status: evidence.AssessmentNotProvided}
}

payload := evidence.PrescriptionPayload{
	PrescriptionID:  ulid.Make().String(),
	Intent:          &intent,
	CanonicalAction: input.CanonicalAction,
	Assessment:      assessment,
	TTLMs:           evidence.DefaultTTLMs,
	Flavor:          input.Flavor,
	Evidence:        payloadEvidenceMetadata(input.EvidenceKind),
	Source:          payloadSourceMetadata(input.SourceSystem),
}
```

Set `IntentDigest` in `BuildEntry` to a digest of declared intent, or leave it
empty until Task 7 defines fallback signal identity. Prefer adding a simple
deterministic `evidence.ComputeIntentDigest(intent)` helper in Task 2 if the
existing entry builder requires a digest.

**Step 5: Run lifecycle tests**

Run:

```bash
go test ./internal/lifecycle -count=1
```

Expected: PASS for lifecycle package.

**Step 6: Commit**

```bash
git add internal/lifecycle/types.go internal/lifecycle/service.go internal/lifecycle/service_test.go pkg/evidence/validation.go
git commit -s -m "refactor: prescribe declared intent in lifecycle"
```

### Task 4: Refactor Typed Ingest To Accept Intent And Optional Enrichment

**Files:**
- Modify: `internal/ingest/contracts.go`
- Modify: `internal/ingest/contracts_test.go`
- Modify: `internal/ingest/service.go`
- Modify: `internal/ingest/service_test.go`
- Modify: `internal/api/ingest_handler.go`
- Modify: `internal/api/ingest_handler_test.go`

**Step 1: Write failing contract tests**

Add a typed prescribe request with `intent` and no `canonical_action`:

```go
func TestValidatePrescribeRequest_IntentOnly(t *testing.T) {
	req := validPrescribeRequest()
	req.Intent = &evidence.DeclaredIntent{
		Tool:      "kubectl",
		Operation: "apply",
		Target:    "deployment/web",
	}
	req.CanonicalAction = nil
	req.SmartTarget = nil

	if err := ValidatePrescribeRequest(req); err != nil {
		t.Fatalf("ValidatePrescribeRequest: %v", err)
	}
}
```

Add a test that no intent/canonical/smart target fails.

**Step 2: Run tests to verify they fail**

Run:

```bash
go test ./internal/ingest -run TestValidatePrescribeRequest -count=1
```

Expected: failure because current contract requires `canonical_action` or
`smart_target`.

**Step 3: Update request contract**

In `internal/ingest/contracts.go`:

- Add `Intent *evidence.DeclaredIntent`
- Change `CanonicalAction` to `json.RawMessage` or a local JSON field
- Add `Assessment *evidence.AssessmentPayload`
- Remove dependency on `internal/canon`
- Remove `SmartTarget` if it only exists to build canonical identity in core

Validation rule:

```text
payload_override XOR explicit fields
explicit fields require at least one declared intent field
canonical_action is optional
assessment is optional
```

**Step 4: Update ingest service**

Map ingest request to lifecycle:

```go
out, err := lifecycleSvc.Prescribe(ctx, lifecycle.PrescribeInput{
	Actor:           in.Actor,
	Intent:          derefIntent(in.Intent),
	CanonicalAction: rawCanonicalAction(in.CanonicalAction),
	Assessment:      in.Assessment,
	SessionID:       in.SessionID,
	OperationID:     in.OperationID,
	TraceID:         in.TraceID,
	SpanID:          in.SpanID,
	ParentSpanID:    in.ParentSpanID,
	ScopeDimensions: in.ScopeDimensions,
	Flavor:          in.Flavor,
	EvidenceKind:    evidenceKind(in.Evidence),
	SourceSystem:    sourceSystem(in.Source),
})
```

Remove pipeline construction and SARIF mapping from ingest.

**Step 5: Run ingest and API tests**

Run:

```bash
go test ./internal/ingest ./internal/api -count=1
```

Expected: PASS after fixtures and expected response values are updated.

**Step 6: Commit**

```bash
git add internal/ingest internal/api
git commit -s -m "refactor: ingest intent-first prescribe requests"
```

### Task 5: Refactor CLI Prescribe And Record Paths

**Files:**
- Modify: `cmd/evidra/*.go`
- Modify: `cmd/evidra/*_test.go`
- Modify: `docs/integrations/cli-reference.md`

**Step 1: Write failing CLI tests**

Add or update tests so `record -f deploy.yaml -- kubectl apply -f deploy.yaml`
stores declared intent and an artifact digest but no canonical action by
default.

Expected assertion shape:

```go
if payload.Intent == nil {
	t.Fatal("missing intent")
}
if payload.CanonicalAction != nil {
	t.Fatalf("canonical_action = %s, want nil by default", payload.CanonicalAction)
}
if payload.Assessment == nil || payload.Assessment.Status != evidence.AssessmentNotProvided {
	t.Fatalf("assessment = %+v", payload.Assessment)
}
```

**Step 2: Run targeted CLI tests**

Run:

```bash
go test ./cmd/evidra -run 'TestRunRecord|TestRunPrescribe' -count=1
```

Expected: failures on old canonical/risk expectations.

**Step 3: Update command flags**

Keep low-friction flags:

- `--tool`
- `--operation`
- `--target`
- `--command` or command inferred from `record -- ...`
- `--artifact` / `-f` only for digest computation
- `--artifact-digest` for callers that already computed it
- `--canonical-action-json` or `--canonical-action-file` for optional
  enrichment
- `--assessment-json` or `--assessment-file` for optional enrichment

Remove or deprecate flags that imply core assessment:

- `--external-findings`
- any flag that asks core to classify raw artifacts

**Step 4: Implement CLI mapping**

Map command input into `lifecycle.PrescribeInput.Intent`. If `-f` is supplied,
read the file only to compute `artifact_digest`. Do not parse it.

**Step 5: Run CLI tests**

Run:

```bash
go test ./cmd/evidra -count=1
```

Expected: PASS.

**Step 6: Commit**

```bash
git add cmd/evidra docs/integrations/cli-reference.md
git commit -s -m "refactor: make cli prescribe intent-first"
```

### Task 6: Refactor MCP Server Prescribe Tools

**Files:**
- Modify: `pkg/mcpserver/*.go`
- Modify: `pkg/mcpserver/*_test.go`
- Modify: `prompts/mcpserver/tools/*prescribe*.txt`
- Modify: `prompts/skill/SKILL*.md`

**Step 1: Write failing MCP tests**

Update prescribe tests so `prescribe_smart` and `prescribe_full` return a
prescription with optional/unknown assessment rather than internally computed
risk.

Expected checks:

```go
if output.EffectiveRisk != "" && output.EffectiveRisk != "unknown" {
	t.Fatalf("effective risk = %q, want absent or unknown without assessment", output.EffectiveRisk)
}
if len(output.RiskInputs) != 0 {
	t.Fatalf("risk inputs = %+v, want none without assessment", output.RiskInputs)
}
```

**Step 2: Run targeted MCP tests**

Run:

```bash
go test ./pkg/mcpserver -run 'TestPrescribe|TestRunCommand' -count=1
```

Expected: failures on old canonical/risk behavior.

**Step 3: Update tool schemas and handlers**

MCP prescribe input should accept:

- declared intent fields
- optional artifact digest
- optional canonical action JSON
- optional assessment JSON

Remove assumptions that raw artifacts will be canonicalized by core.

**Step 4: Update prompts**

Prompt docs should say:

```text
prescribe records declared intent before execution.
assessment is optional and may be supplied by an external scanner or policy provider.
canonical_action is optional normalized identity when available.
```

**Step 5: Run MCP tests and prompt verification**

Run:

```bash
go test ./pkg/mcpserver -count=1
make prompts-verify
```

Expected: PASS.

**Step 6: Commit**

```bash
git add pkg/mcpserver prompts/mcpserver prompts/skill
git commit -s -m "refactor: simplify mcp prescribe intent model"
```

### Task 7: Update Analytics To Degrade Gracefully

**Files:**
- Modify: `internal/pipeline/bridge.go`
- Modify: `internal/pipeline/bridge_test.go`
- Modify: `internal/assessment/tracker.go`
- Modify: `internal/assessment/assessment_test.go`
- Modify: `internal/signal/*.go`
- Modify: `internal/signal/*_test.go`

**Step 1: Write failing bridge tests**

Add a prescribe payload with `intent` and no `canonical_action`; verify signal
entries still contain useful identity:

```go
func TestEvidenceToSignalEntries_UsesDeclaredIntentFallback(t *testing.T) {
	entry := prescribeEntryWithPayload(evidence.PrescriptionPayload{
		PrescriptionID: "presc_1",
		Intent: &evidence.DeclaredIntent{
			Tool:      "kubectl",
			Operation: "apply",
			Target:    "deployment/web",
		},
		Assessment: &evidence.AssessmentPayload{Status: evidence.AssessmentNotProvided},
	})

	got, err := EvidenceToSignalEntries([]evidence.EvidenceEntry{entry})
	if err != nil {
		t.Fatal(err)
	}
	if got[0].Tool != "kubectl" || got[0].Operation != "apply" {
		t.Fatalf("signal entry = %+v", got[0])
	}
}
```

**Step 2: Run tests to verify they fail**

Run:

```bash
go test ./internal/pipeline ./internal/assessment ./internal/signal -count=1
```

Expected: failures where canonical action is assumed.

**Step 3: Implement fallback extraction**

Update extraction order:

1. If `canonical_action` exists, use it for normalized identity.
2. Else use `payload.intent.tool`, `payload.intent.operation`,
   `payload.intent.target`, and `payload.intent.artifact_digest`.
3. If assessment exists, populate risk fields from it.
4. If assessment is absent, leave risk fields empty or `unknown`.

Do not import removed assessment/canon packages.

**Step 4: Run analytics tests**

Run:

```bash
go test ./internal/pipeline ./internal/assessment ./internal/signal -count=1
```

Expected: PASS.

**Step 5: Commit**

```bash
git add internal/pipeline internal/assessment internal/signal
git commit -s -m "refactor: let analytics use declared intent fallback"
```

### Task 8: Remove Core Assessment Packages From Write Path

**Files:**
- Delete or move: `internal/assess/`
- Delete or move: `internal/canon/`
- Delete or move: `internal/risk/`
- Delete or move: `internal/detectors/`
- Delete or move: `tests/canon_fixtures/`
- Modify: any remaining imports found by `rg`

**Step 1: Verify remaining imports**

Run:

```bash
rg -n 'internal/(assess|canon|risk|detectors)' --glob '*.go'
```

Expected: output still shows packages to clean up.

**Step 2: Remove or quarantine packages**

If the companion repo exists during implementation, move the packages there. If
not, delete them from core in this branch after all callers are migrated.

**Step 3: Remove remaining references**

Run after each cleanup pass:

```bash
rg -n 'internal/(assess|canon|risk|detectors)' --glob '*.go'
```

Expected: no write-path imports remain. Test-only imports should also be removed
unless the test is specifically validating deleted compatibility behavior.

**Step 4: Run compile test**

Run:

```bash
go test ./... -run TestDoesNotExist -count=0
```

Expected: all packages compile.

**Step 5: Commit**

```bash
git add -A
git commit -s -m "refactor: remove native assessment from core"
```

### Task 9: Update Public Docs And API Schemas

**Files:**
- Modify: `README.md`
- Modify: `docs/system-design/EVIDRA_ARCHITECTURE_V1.md`
- Modify: `docs/contracts/EVIDRA_CANONICALIZATION_CONTRACT_V1.md`
- Modify: `docs/api-reference.md`
- Modify: `cmd/evidra-api/static/openapi.yaml`
- Modify: `ui/public/openapi.yaml`
- Modify: related docs tests under `tests/`

**Step 1: Write or update docs tests**

Update shell tests that assert old canonicalization positioning. Add checks for:

```text
intent/outcome ledger
assessment is optional
canonical_action is optional
prescribe -> report
```

**Step 2: Run docs tests to verify failure first**

Run the most relevant tests, for example:

```bash
bash tests/test_supported_core_positioning.sh
bash tests/test_public_claims.sh
bash tests/test_protocol_docs.sh
```

Expected: failures until docs are updated.

**Step 3: Update docs**

Docs should consistently say:

- Evidra core records declared intent and outcome.
- `canonical_action` is optional enrichment.
- `assessment` is optional enrichment.
- External scanners/policy/canonicalizers can provide enrichment.
- Evidra core is not a scanner, policy engine, or mandatory canonicalization
  framework.

**Step 4: Regenerate or sync OpenAPI files if needed**

If there is no generator, update both OpenAPI copies consistently:

- `cmd/evidra-api/static/openapi.yaml`
- `ui/public/openapi.yaml`

**Step 5: Run docs tests**

Run:

```bash
bash tests/test_supported_core_positioning.sh
bash tests/test_public_claims.sh
bash tests/test_protocol_docs.sh
```

Expected: PASS.

**Step 6: Commit**

```bash
git add README.md docs cmd/evidra-api/static/openapi.yaml ui/public/openapi.yaml tests
git commit -s -m "docs: position evidra as intent outcome core"
```

### Task 10: Final Verification

**Files:**
- All changed files

**Step 1: Run full Go tests**

Run:

```bash
go test ./... -count=1
```

Expected: PASS.

**Step 2: Run repository linter**

Run:

```bash
make lint
```

Expected: `0 issues.`

**Step 3: Run prompt verification**

Run:

```bash
make prompts-verify
```

Expected: PASS.

**Step 4: Run focused shell docs tests**

Run:

```bash
bash tests/test_supported_core_positioning.sh
bash tests/test_public_claims.sh
bash tests/test_protocol_docs.sh
```

Expected: PASS.

**Step 5: Inspect final dependency boundary**

Run:

```bash
rg -n 'internal/(assess|canon|risk|detectors)' --glob '*.go'
```

Expected: no matches in core write path. If any matches remain, document why or
remove them before completion.

**Step 6: Final commit if verification changes files**

```bash
git status --short
git add -A
git commit -s -m "test: verify core simplification"
```

Only commit if verification produced intentional file changes.
