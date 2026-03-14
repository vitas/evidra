package lifecycle

import (
	"context"
	"crypto/ed25519"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"samebits.com/evidra/internal/canon"
	"samebits.com/evidra/internal/testutil"
	"samebits.com/evidra/pkg/evidence"
)

type countingSigner struct {
	evidence.Signer
	signCalls int
}

func (s *countingSigner) Sign(payload []byte) []byte {
	s.signCalls++
	return s.Signer.Sign(payload)
}

func (s *countingSigner) Verify(payload, sig []byte) bool {
	return s.Signer.Verify(payload, sig)
}

func (s *countingSigner) PublicKey() ed25519.PublicKey {
	return s.Signer.PublicKey()
}

func TestServicePrescribe_ParseErrorWritesCanonFailure(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	svc := NewService(Options{
		EvidencePath: dir,
		Signer:       testutil.TestSigner(t),
	})

	_, err := svc.Prescribe(context.Background(), PrescribeInput{
		Actor:       evidence.Actor{Type: "agent", ID: "agent-1", Provenance: "mcp"},
		Tool:        "terraform",
		Operation:   "apply",
		RawArtifact: []byte("not valid json {{{"),
		SessionID:   "session-1",
	})
	if err == nil {
		t.Fatal("expected parse error")
	}
	if ErrorCode(err) != ErrCodeParseError {
		t.Fatalf("error code = %q, want %q", ErrorCode(err), ErrCodeParseError)
	}

	entries, readErr := evidence.ReadAllEntriesAtPath(dir)
	if readErr != nil {
		t.Fatalf("ReadAllEntriesAtPath: %v", readErr)
	}
	if len(entries) != 1 {
		t.Fatalf("entry count = %d, want 1", len(entries))
	}
	if entries[0].Type != evidence.EntryTypeCanonFailure {
		t.Fatalf("entry type = %q, want %q", entries[0].Type, evidence.EntryTypeCanonFailure)
	}
}

func TestServicePrescribe_CanonicalActionNormalizesToolOperation(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	svc := NewService(Options{
		EvidencePath: dir,
		Signer:       testutil.TestSigner(t),
	})

	preCanon := &canon.CanonicalAction{
		OperationClass:    "mutate",
		ScopeClass:        "production",
		ResourceCount:     1,
		ResourceShapeHash: "sha256:test-shape",
		ResourceIdentity: []canon.ResourceID{
			{APIVersion: "apps/v1", Kind: "Deployment", Namespace: "default", Name: "demo"},
		},
	}

	out, err := svc.Prescribe(context.Background(), PrescribeInput{
		Actor:           evidence.Actor{Type: "agent", ID: "agent-1", Provenance: "mcp"},
		Tool:            "Kubectl",
		Operation:       "Apply",
		RawArtifact:     []byte(`{"noop":true}`),
		CanonicalAction: preCanon,
		SessionID:       "session-1",
	})
	if err != nil {
		t.Fatalf("Prescribe: %v", err)
	}

	entry, found, findErr := evidence.FindEntryByID(dir, out.PrescriptionID)
	if findErr != nil {
		t.Fatalf("FindEntryByID: %v", findErr)
	}
	if !found {
		t.Fatalf("prescription %s not found", out.PrescriptionID)
	}

	var payload evidence.PrescriptionPayload
	if err := json.Unmarshal(entry.Payload, &payload); err != nil {
		t.Fatalf("unmarshal prescription payload: %v", err)
	}

	var action canon.CanonicalAction
	if err := json.Unmarshal(payload.CanonicalAction, &action); err != nil {
		t.Fatalf("unmarshal canonical action: %v", err)
	}

	if action.Tool != "kubectl" {
		t.Fatalf("canonical_action.tool = %q, want kubectl", action.Tool)
	}
	if action.Operation != "apply" {
		t.Fatalf("canonical_action.operation = %q, want apply", action.Operation)
	}
}

func TestServicePrescribe_PopulatesRiskInputsAndEffectiveRisk(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	svc := NewService(Options{
		EvidencePath: dir,
		Signer:       testutil.TestSigner(t),
	})

	out, err := svc.Prescribe(context.Background(), PrescribeInput{
		Actor:       evidence.Actor{Type: "agent", ID: "agent-1", Provenance: "mcp"},
		Tool:        "kubectl",
		Operation:   "apply",
		RawArtifact: []byte("apiVersion: v1\nkind: ConfigMap\nmetadata:\n  name: cm1\n  namespace: default\n"),
		SessionID:   "session-risk-inputs",
	})
	if err != nil {
		t.Fatalf("Prescribe: %v", err)
	}
	if out.EffectiveRisk == "" {
		t.Fatal("expected effective_risk")
	}
	if len(out.RiskInputs) != 1 {
		t.Fatalf("risk_inputs len = %d, want 1", len(out.RiskInputs))
	}
	if out.RiskInputs[0].Source != "evidra/native" {
		t.Fatalf("risk_inputs[0].source = %q, want evidra/native", out.RiskInputs[0].Source)
	}

	var payload evidence.PrescriptionPayload
	if err := json.Unmarshal(out.Entry.Payload, &payload); err != nil {
		t.Fatalf("unmarshal prescription payload: %v", err)
	}
	if payload.EffectiveRisk != out.EffectiveRisk {
		t.Fatalf("payload effective_risk = %q, want %q", payload.EffectiveRisk, out.EffectiveRisk)
	}
	if len(payload.RiskInputs) != 1 {
		t.Fatalf("payload risk_inputs len = %d, want 1", len(payload.RiskInputs))
	}
	if payload.RiskInputs[0].Source != "evidra/native" {
		t.Fatalf("payload risk_inputs[0].source = %q, want evidra/native", payload.RiskInputs[0].Source)
	}
	if payload.RiskLevel != "" {
		t.Fatalf("legacy payload risk_level = %q, want empty", payload.RiskLevel)
	}
	if len(payload.RiskDetails) != 0 {
		t.Fatalf("legacy payload risk_details = %v, want empty", payload.RiskDetails)
	}
	if len(payload.RiskTags) != 0 {
		t.Fatalf("legacy payload risk_tags = %v, want empty", payload.RiskTags)
	}
}

func TestServicePrescribe_DefaultsTraceIDToSessionIDWhenOmitted(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	svc := NewService(Options{
		EvidencePath: dir,
		Signer:       testutil.TestSigner(t),
	})

	out, err := svc.Prescribe(context.Background(), PrescribeInput{
		Actor:       evidence.Actor{Type: "agent", ID: "agent-1", Provenance: "mcp"},
		Tool:        "kubectl",
		Operation:   "apply",
		RawArtifact: []byte("apiVersion: v1\nkind: ConfigMap\nmetadata:\n  name: cm1\n  namespace: default\n"),
		SessionID:   "session-trace-default",
	})
	if err != nil {
		t.Fatalf("Prescribe: %v", err)
	}
	if out.TraceID != out.SessionID {
		t.Fatalf("trace_id=%q, want session_id=%q", out.TraceID, out.SessionID)
	}
}

func TestServicePrescribe_RejectsMissingActorType(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	svc := NewService(Options{
		EvidencePath: dir,
		Signer:       testutil.TestSigner(t),
	})

	_, err := svc.Prescribe(context.Background(), PrescribeInput{
		Actor:       evidence.Actor{ID: "agent-1", Provenance: "mcp"},
		Tool:        "kubectl",
		Operation:   "apply",
		RawArtifact: []byte("apiVersion: v1\nkind: ConfigMap\nmetadata:\n  name: cm1\n  namespace: default\n"),
		SessionID:   "session-missing-type",
	})
	if err == nil {
		t.Fatal("expected invalid_input error")
	}
	if ErrorCode(err) != ErrCodeInvalidInput {
		t.Fatalf("error code=%q, want %q", ErrorCode(err), ErrCodeInvalidInput)
	}
}

func TestServicePrescribe_RejectsMissingActorProvenance(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	svc := NewService(Options{
		EvidencePath: dir,
		Signer:       testutil.TestSigner(t),
	})

	_, err := svc.Prescribe(context.Background(), PrescribeInput{
		Actor:       evidence.Actor{Type: "agent", ID: "agent-1"},
		Tool:        "kubectl",
		Operation:   "apply",
		RawArtifact: []byte("apiVersion: v1\nkind: ConfigMap\nmetadata:\n  name: cm1\n  namespace: default\n"),
		SessionID:   "session-missing-provenance",
	})
	if err == nil {
		t.Fatal("expected invalid_input error")
	}
	if ErrorCode(err) != ErrCodeInvalidInput {
		t.Fatalf("error code=%q, want %q", ErrorCode(err), ErrCodeInvalidInput)
	}
}

func TestServicePrescribe_CanonicalActionScopeAliasNormalizes(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	svc := NewService(Options{
		EvidencePath: dir,
		Signer:       testutil.TestSigner(t),
	})

	preCanon := &canon.CanonicalAction{
		OperationClass:    "mutate",
		ScopeClass:        "prod",
		ResourceCount:     1,
		ResourceShapeHash: "sha256:test-shape",
		ResourceIdentity: []canon.ResourceID{
			{APIVersion: "apps/v1", Kind: "Deployment", Namespace: "default", Name: "demo"},
		},
	}

	out, err := svc.Prescribe(context.Background(), PrescribeInput{
		Actor:           evidence.Actor{Type: "agent", ID: "agent-1", Provenance: "mcp"},
		Tool:            "kubectl",
		Operation:       "apply",
		RawArtifact:     []byte(`{"noop":true}`),
		CanonicalAction: preCanon,
		SessionID:       "session-1",
	})
	if err != nil {
		t.Fatalf("Prescribe: %v", err)
	}
	if out.ScopeClass != "production" {
		t.Fatalf("scope_class=%q, want production", out.ScopeClass)
	}
}

func TestServicePrescribe_CanonicalActionScopeRejectsInvalid(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	svc := NewService(Options{
		EvidencePath: dir,
		Signer:       testutil.TestSigner(t),
	})

	preCanon := &canon.CanonicalAction{
		OperationClass:    "mutate",
		ScopeClass:        "prod-east",
		ResourceCount:     1,
		ResourceShapeHash: "sha256:test-shape",
		ResourceIdentity: []canon.ResourceID{
			{APIVersion: "apps/v1", Kind: "Deployment", Namespace: "default", Name: "demo"},
		},
	}

	_, err := svc.Prescribe(context.Background(), PrescribeInput{
		Actor:           evidence.Actor{Type: "agent", ID: "agent-1", Provenance: "mcp"},
		Tool:            "kubectl",
		Operation:       "apply",
		RawArtifact:     []byte(`{"noop":true}`),
		CanonicalAction: preCanon,
		SessionID:       "session-1",
	})
	if err == nil {
		t.Fatal("expected invalid_input error")
	}
	if ErrorCode(err) != ErrCodeInvalidInput {
		t.Fatalf("error code=%q, want %q", ErrorCode(err), ErrCodeInvalidInput)
	}
}

func TestServicePrescribe_SignsEntryOnce(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	signer := &countingSigner{Signer: testutil.TestSigner(t)}
	svc := NewService(Options{
		EvidencePath: dir,
		Signer:       signer,
	})

	_, err := svc.Prescribe(context.Background(), PrescribeInput{
		Actor:       evidence.Actor{Type: "agent", ID: "agent-1", Provenance: "mcp"},
		Tool:        "kubectl",
		Operation:   "apply",
		RawArtifact: []byte("apiVersion: v1\nkind: ConfigMap\nmetadata:\n  name: cm1\n  namespace: default\n"),
		SessionID:   "session-single-sign",
	})
	if err != nil {
		t.Fatalf("Prescribe: %v", err)
	}
	if signer.signCalls != 1 {
		t.Fatalf("sign calls=%d, want 1", signer.signCalls)
	}
}

func TestServicePrescribe_StoreFailureReturnsErrorInStrictMode(t *testing.T) {
	t.Parallel()

	tmp := t.TempDir()
	evidencePath := filepath.Join(tmp, "legacy.log")
	if err := os.WriteFile(evidencePath, []byte("legacy"), 0o644); err != nil {
		t.Fatalf("write legacy file: %v", err)
	}

	svc := NewService(Options{
		EvidencePath: evidencePath,
		Signer:       testutil.TestSigner(t),
	})

	_, err := svc.Prescribe(context.Background(), PrescribeInput{
		Actor:       evidence.Actor{Type: "agent", ID: "agent-1", Provenance: "mcp"},
		Tool:        "kubectl",
		Operation:   "apply",
		RawArtifact: []byte("apiVersion: v1\nkind: ConfigMap\nmetadata:\n  name: cm1\n  namespace: default\n"),
		SessionID:   "session-strict-write-fail",
	})
	if err == nil {
		t.Fatal("expected error")
	}
	if ErrorCode(err) != ErrCodeEvidenceRead {
		t.Fatalf("error code=%q, want %q", ErrorCode(err), ErrCodeEvidenceRead)
	}
}

func TestServicePrescribe_BestEffortWriteModeSuppressesWriteError(t *testing.T) {
	t.Parallel()

	tmp := t.TempDir()
	evidencePath := filepath.Join(tmp, "legacy.log")
	if err := os.WriteFile(evidencePath, []byte("legacy"), 0o644); err != nil {
		t.Fatalf("write legacy file: %v", err)
	}

	svc := NewService(Options{
		EvidencePath:     evidencePath,
		Signer:           testutil.TestSigner(t),
		BestEffortWrites: true,
	})

	out, err := svc.Prescribe(context.Background(), PrescribeInput{
		Actor:       evidence.Actor{Type: "agent", ID: "agent-1", Provenance: "mcp"},
		Tool:        "kubectl",
		Operation:   "apply",
		RawArtifact: []byte("apiVersion: v1\nkind: ConfigMap\nmetadata:\n  name: cm1\n  namespace: default\n"),
		SessionID:   "session-best-effort",
	})
	if err != nil {
		t.Fatalf("Prescribe: %v", err)
	}
	if out.PrescriptionID == "" {
		t.Fatal("expected prescription_id")
	}
}

func TestServiceReport_UnknownPrescriptionWritesSignal(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	svc := NewService(Options{
		EvidencePath: dir,
		Signer:       testutil.TestSigner(t),
	})

	_, err := svc.Report(context.Background(), ReportInput{
		PrescriptionID: "NONEXISTENT",
		Verdict:        evidence.VerdictFailure,
		ExitCode:       intPtr(1),
		Actor:          evidence.Actor{Type: "agent", ID: "agent-1", Provenance: "mcp"},
	})
	if err == nil {
		t.Fatal("expected not_found error")
	}
	if ErrorCode(err) != ErrCodeNotFound {
		t.Fatalf("error code = %q, want %q", ErrorCode(err), ErrCodeNotFound)
	}

	entries, readErr := evidence.ReadAllEntriesAtPath(dir)
	if readErr != nil {
		t.Fatalf("ReadAllEntriesAtPath: %v", readErr)
	}
	if len(entries) != 1 {
		t.Fatalf("entry count = %d, want 1", len(entries))
	}
	if entries[0].Type != evidence.EntryTypeSignal {
		t.Fatalf("entry type = %q, want %q", entries[0].Type, evidence.EntryTypeSignal)
	}

	var payload evidence.SignalPayload
	if err := json.Unmarshal(entries[0].Payload, &payload); err != nil {
		t.Fatalf("unmarshal signal payload: %v", err)
	}
	if payload.SubSignal != "unprescribed_action" {
		t.Fatalf("sub_signal = %q, want unprescribed_action", payload.SubSignal)
	}
}

func TestServiceReport_KnownPrescriptionUsesPrescriptionCorrelation(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	svc := NewService(Options{
		EvidencePath: dir,
		Signer:       testutil.TestSigner(t),
	})

	prescribeOut, err := svc.Prescribe(context.Background(), PrescribeInput{
		Actor:       evidence.Actor{Type: "agent", ID: "agent-1", Provenance: "mcp"},
		Tool:        "kubectl",
		Operation:   "apply",
		RawArtifact: []byte("apiVersion: v1\nkind: ConfigMap\nmetadata:\n  name: cm1\n  namespace: default\n"),
		SessionID:   "session-1",
		TraceID:     "trace-1",
	})
	if err != nil {
		t.Fatalf("Prescribe: %v", err)
	}

	reportOut, err := svc.Report(context.Background(), ReportInput{
		PrescriptionID: prescribeOut.PrescriptionID,
		Verdict:        evidence.VerdictSuccess,
		ExitCode:       intPtr(0),
	})
	if err != nil {
		t.Fatalf("Report: %v", err)
	}
	if reportOut.ReportID == "" {
		t.Fatal("expected report ID")
	}

	entries, readErr := evidence.ReadAllEntriesAtPath(dir)
	if readErr != nil {
		t.Fatalf("ReadAllEntriesAtPath: %v", readErr)
	}
	if len(entries) != 2 {
		t.Fatalf("entry count = %d, want 2", len(entries))
	}
	if entries[1].Type != evidence.EntryTypeReport {
		t.Fatalf("entry type = %q, want %q", entries[1].Type, evidence.EntryTypeReport)
	}
	if entries[1].TraceID != entries[0].TraceID {
		t.Fatalf("report trace_id = %q, want %q", entries[1].TraceID, entries[0].TraceID)
	}
	if entries[1].Actor.ID != entries[0].Actor.ID {
		t.Fatalf("report actor = %q, want %q", entries[1].Actor.ID, entries[0].Actor.ID)
	}
}
