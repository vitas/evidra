package config

import "testing"

func TestResolveEvidenceWriteMode_DefaultStrict(t *testing.T) {
	t.Setenv("EVIDRA_EVIDENCE_WRITE_MODE", "")
	mode, err := ResolveEvidenceWriteMode("")
	if err != nil {
		t.Fatalf("ResolveEvidenceWriteMode: %v", err)
	}
	if mode != EvidenceWriteModeStrict {
		t.Fatalf("mode = %q, want %q", mode, EvidenceWriteModeStrict)
	}
}

func TestResolveEvidenceWriteMode_ExplicitBestEffort(t *testing.T) {
	t.Setenv("EVIDRA_EVIDENCE_WRITE_MODE", "")
	mode, err := ResolveEvidenceWriteMode("best_effort")
	if err != nil {
		t.Fatalf("ResolveEvidenceWriteMode: %v", err)
	}
	if mode != EvidenceWriteModeBestEffort {
		t.Fatalf("mode = %q, want %q", mode, EvidenceWriteModeBestEffort)
	}
}

func TestResolveEvidenceWriteMode_FromEnv(t *testing.T) {
	t.Setenv("EVIDRA_EVIDENCE_WRITE_MODE", "best_effort")
	mode, err := ResolveEvidenceWriteMode("")
	if err != nil {
		t.Fatalf("ResolveEvidenceWriteMode: %v", err)
	}
	if mode != EvidenceWriteModeBestEffort {
		t.Fatalf("mode = %q, want %q", mode, EvidenceWriteModeBestEffort)
	}
}

func TestResolveEvidenceWriteMode_Invalid(t *testing.T) {
	t.Setenv("EVIDRA_EVIDENCE_WRITE_MODE", "")
	if _, err := ResolveEvidenceWriteMode("invalid"); err == nil {
		t.Fatal("expected error for invalid mode")
	}
}
