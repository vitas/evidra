package evidence

import (
	"strings"
	"testing"

	"samebits.com/evidra-benchmark/pkg/invocation"
)

func testBuilderConfig() BuilderConfig {
	return BuilderConfig{
		ServerID: "test-server",
		TenantID: "test-tenant",
	}
}

func testInvocation() invocation.ToolInvocation {
	return invocation.ToolInvocation{
		Actor:       invocation.Actor{Type: "agent", ID: "claude", Origin: "mcp"},
		Tool:        "kubectl",
		Operation:   "apply",
		Params:      map[string]interface{}{"target": map[string]interface{}{"namespace": "default"}},
		Environment: "staging",
	}
}

func testDecision() Decision {
	return Decision{
		Allow:          false,
		RiskLevel:      "high",
		Reason:         "namespace.forbidden",
		PolicyRef:      "sha256:abc123",
		BundleRevision: "2026.1",
		ProfileName:    "ops-v0.1",
		Reasons:        []string{"namespace.forbidden"},
		Hints:          []string{"use namespace test-*"},
		Hits:           []string{"namespace.forbidden"},
	}
}

func TestBuildRecord_FieldMapping(t *testing.T) {
	t.Parallel()

	cfg := testBuilderConfig()
	dec := testDecision()
	inv := testInvocation()

	rec, err := BuildRecord(cfg, dec, inv)
	if err != nil {
		t.Fatalf("BuildRecord: %v", err)
	}

	// Server-level fields.
	if rec.ServerID != cfg.ServerID {
		t.Errorf("ServerID = %q, want %q", rec.ServerID, cfg.ServerID)
	}
	if rec.TenantID != cfg.TenantID {
		t.Errorf("TenantID = %q, want %q", rec.TenantID, cfg.TenantID)
	}

	// Invocation fields.
	if rec.Actor.Type != inv.Actor.Type {
		t.Errorf("Actor.Type = %q, want %q", rec.Actor.Type, inv.Actor.Type)
	}
	if rec.Actor.ID != inv.Actor.ID {
		t.Errorf("Actor.ID = %q, want %q", rec.Actor.ID, inv.Actor.ID)
	}
	if rec.Actor.Origin != inv.Actor.Origin {
		t.Errorf("Actor.Origin = %q, want %q", rec.Actor.Origin, inv.Actor.Origin)
	}
	if rec.Tool != inv.Tool {
		t.Errorf("Tool = %q, want %q", rec.Tool, inv.Tool)
	}
	if rec.Operation != inv.Operation {
		t.Errorf("Operation = %q, want %q", rec.Operation, inv.Operation)
	}
	if rec.Environment != inv.Environment {
		t.Errorf("Environment = %q, want %q", rec.Environment, inv.Environment)
	}

	// Decision fields.
	if rec.Decision.Allow != dec.Allow {
		t.Errorf("Decision.Allow = %v, want %v", rec.Decision.Allow, dec.Allow)
	}
	if rec.Decision.RiskLevel != dec.RiskLevel {
		t.Errorf("Decision.RiskLevel = %q, want %q", rec.Decision.RiskLevel, dec.RiskLevel)
	}
	if rec.Decision.Reason != dec.Reason {
		t.Errorf("Decision.Reason = %q, want %q", rec.Decision.Reason, dec.Reason)
	}

	// Policy metadata.
	if rec.PolicyRef != dec.PolicyRef {
		t.Errorf("PolicyRef = %q, want %q", rec.PolicyRef, dec.PolicyRef)
	}
	if rec.BundleRevision != dec.BundleRevision {
		t.Errorf("BundleRevision = %q, want %q", rec.BundleRevision, dec.BundleRevision)
	}
	if rec.ProfileName != dec.ProfileName {
		t.Errorf("ProfileName = %q, want %q", rec.ProfileName, dec.ProfileName)
	}
}

func TestBuildRecord_EventID(t *testing.T) {
	t.Parallel()

	rec, err := BuildRecord(testBuilderConfig(), testDecision(), testInvocation())
	if err != nil {
		t.Fatalf("BuildRecord: %v", err)
	}

	if !strings.HasPrefix(rec.EventID, "evt_") {
		t.Errorf("EventID = %q, want evt_ prefix", rec.EventID)
	}

	// ULID is 26 chars, so total should be 30 (4 prefix + 26 ULID).
	if len(rec.EventID) != 30 {
		t.Errorf("EventID length = %d, want 30 (evt_ + 26-char ULID)", len(rec.EventID))
	}
}

func TestBuildRecord_EventIDUnique(t *testing.T) {
	t.Parallel()

	cfg := testBuilderConfig()
	dec := testDecision()
	inv := testInvocation()

	seen := make(map[string]bool, 100)
	for i := 0; i < 100; i++ {
		rec, err := BuildRecord(cfg, dec, inv)
		if err != nil {
			t.Fatalf("BuildRecord iteration %d: %v", i, err)
		}
		if seen[rec.EventID] {
			t.Fatalf("duplicate EventID on iteration %d: %s", i, rec.EventID)
		}
		seen[rec.EventID] = true
	}
}

func TestBuildRecord_TimestampUTC(t *testing.T) {
	t.Parallel()

	rec, err := BuildRecord(testBuilderConfig(), testDecision(), testInvocation())
	if err != nil {
		t.Fatalf("BuildRecord: %v", err)
	}

	if rec.Timestamp.IsZero() {
		t.Fatal("Timestamp is zero")
	}
	if rec.Timestamp.Location().String() != "UTC" {
		t.Errorf("Timestamp location = %q, want UTC", rec.Timestamp.Location())
	}
}

func TestBuildRecord_InputHash(t *testing.T) {
	t.Parallel()

	rec, err := BuildRecord(testBuilderConfig(), testDecision(), testInvocation())
	if err != nil {
		t.Fatalf("BuildRecord: %v", err)
	}

	if !strings.HasPrefix(rec.InputHash, "sha256:") {
		t.Errorf("InputHash = %q, want sha256: prefix", rec.InputHash)
	}

	// sha256 hex = 64 chars, plus "sha256:" prefix = 71.
	if len(rec.InputHash) != 71 {
		t.Errorf("InputHash length = %d, want 71", len(rec.InputHash))
	}
}

func TestBuildRecord_InputHashDeterministic(t *testing.T) {
	t.Parallel()

	cfg := testBuilderConfig()
	dec := testDecision()
	inv := testInvocation()

	rec1, err := BuildRecord(cfg, dec, inv)
	if err != nil {
		t.Fatalf("BuildRecord 1: %v", err)
	}

	rec2, err := BuildRecord(cfg, dec, inv)
	if err != nil {
		t.Fatalf("BuildRecord 2: %v", err)
	}

	if rec1.InputHash != rec2.InputHash {
		t.Errorf("InputHash not deterministic:\n  first:  %s\n  second: %s", rec1.InputHash, rec2.InputHash)
	}
}

func TestBuildRecord_InputHashDiffers(t *testing.T) {
	t.Parallel()

	cfg := testBuilderConfig()
	dec := testDecision()

	inv1 := testInvocation()
	inv2 := testInvocation()
	inv2.Tool = "terraform"

	rec1, err := BuildRecord(cfg, dec, inv1)
	if err != nil {
		t.Fatalf("BuildRecord 1: %v", err)
	}
	rec2, err := BuildRecord(cfg, dec, inv2)
	if err != nil {
		t.Fatalf("BuildRecord 2: %v", err)
	}

	if rec1.InputHash == rec2.InputHash {
		t.Error("different invocations should produce different InputHash")
	}
}

func TestBuildRecord_UnsignedFields(t *testing.T) {
	t.Parallel()

	rec, err := BuildRecord(testBuilderConfig(), testDecision(), testInvocation())
	if err != nil {
		t.Fatalf("BuildRecord: %v", err)
	}

	if rec.Signature != "" {
		t.Errorf("Signature should be empty, got %q", rec.Signature)
	}
	if rec.SigningPayload != "" {
		t.Errorf("SigningPayload should be empty, got %q", rec.SigningPayload)
	}
}

func TestBuildRecord_RuleIDsFromHits(t *testing.T) {
	t.Parallel()

	dec := testDecision()
	dec.Hits = []string{"namespace.forbidden", "image.unsigned"}

	rec, err := BuildRecord(testBuilderConfig(), dec, testInvocation())
	if err != nil {
		t.Fatalf("BuildRecord: %v", err)
	}

	if len(rec.Decision.RuleIDs) != len(dec.Hits) {
		t.Fatalf("RuleIDs length = %d, want %d", len(rec.Decision.RuleIDs), len(dec.Hits))
	}
	for i, id := range rec.Decision.RuleIDs {
		if id != dec.Hits[i] {
			t.Errorf("RuleIDs[%d] = %q, want %q", i, id, dec.Hits[i])
		}
	}
}

func TestBuildRecord_AllowedDecision(t *testing.T) {
	t.Parallel()

	dec := Decision{
		Allow:     true,
		RiskLevel: "low",
		Reason:    "all clear",
		PolicyRef: "sha256:000",
	}

	rec, err := BuildRecord(testBuilderConfig(), dec, testInvocation())
	if err != nil {
		t.Fatalf("BuildRecord: %v", err)
	}

	if !rec.Decision.Allow {
		t.Error("Decision.Allow should be true")
	}
	if rec.Decision.RiskLevel != "low" {
		t.Errorf("Decision.RiskLevel = %q, want low", rec.Decision.RiskLevel)
	}
}

func TestBuildRecord_EmptySlices(t *testing.T) {
	t.Parallel()

	dec := Decision{
		Allow:     true,
		RiskLevel: "low",
		Reason:    "ok",
		PolicyRef: "sha256:000",
	}

	rec, err := BuildRecord(testBuilderConfig(), dec, testInvocation())
	if err != nil {
		t.Fatalf("BuildRecord: %v", err)
	}

	if rec.Decision.Reasons != nil {
		t.Errorf("Reasons should be nil, got %v", rec.Decision.Reasons)
	}
	if rec.Decision.Hints != nil {
		t.Errorf("Hints should be nil, got %v", rec.Decision.Hints)
	}
	if rec.Decision.Hits != nil {
		t.Errorf("Hits should be nil, got %v", rec.Decision.Hits)
	}
	if rec.Decision.RuleIDs != nil {
		t.Errorf("RuleIDs should be nil, got %v", rec.Decision.RuleIDs)
	}
}

func TestBuildRecord_NoEnvironment(t *testing.T) {
	t.Parallel()

	inv := testInvocation()
	inv.Environment = ""

	rec, err := BuildRecord(testBuilderConfig(), testDecision(), inv)
	if err != nil {
		t.Fatalf("BuildRecord: %v", err)
	}

	if rec.Environment != "" {
		t.Errorf("Environment = %q, want empty", rec.Environment)
	}
}
