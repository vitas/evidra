package analytics

import (
	"testing"

	"samebits.com/evidra/internal/score"
)

func TestPublicSignalNames_UsesRegisteredSignals(t *testing.T) {
	t.Parallel()

	profile, err := score.ResolveProfile("")
	if err != nil {
		t.Fatalf("ResolveProfile: %v", err)
	}

	names := PublicSignalNames(profile)
	if len(names) == 0 {
		t.Fatal("expected public signal names")
	}
	for _, want := range []string{
		"protocol_violation",
		"artifact_drift",
		"retry_loop",
		"thrashing",
		"blast_radius",
		"risk_escalation",
		"new_scope",
		"repair_loop",
	} {
		found := false
		for _, got := range names {
			if got == want {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("missing signal %q in %v", want, names)
		}
	}
}
