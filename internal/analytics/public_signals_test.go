package analytics

import (
	"reflect"
	"testing"

	"samebits.com/evidra/internal/score"
	"samebits.com/evidra/internal/signal"
)

func TestDecodePublicSignalManifest_RejectsEmptySignals(t *testing.T) {
	t.Parallel()

	_, err := decodePublicSignalManifest([]byte(`{"signals":[]}`))
	if err == nil {
		t.Fatal("expected empty manifest to fail")
	}
}

func TestDecodePublicSignalManifest_PreservesDeclaredOrder(t *testing.T) {
	t.Parallel()

	got, err := decodePublicSignalManifest([]byte(`{"signals":["b","a","c"]}`))
	if err != nil {
		t.Fatalf("decodePublicSignalManifest: %v", err)
	}
	want := []string{"b", "a", "c"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("decoded signals = %v, want %v", got, want)
	}
}

func TestPublicSignalNames_ReturnsStableContractOrder(t *testing.T) {
	t.Parallel()

	profile, err := score.ResolveProfile("")
	if err != nil {
		t.Fatalf("ResolveProfile: %v", err)
	}

	names := PublicSignalNames(profile)
	want := []string{
		"protocol_violation",
		"artifact_drift",
		"retry_loop",
		"blast_radius",
		"new_scope",
		"repair_loop",
		"thrashing",
		"risk_escalation",
	}
	if !reflect.DeepEqual(names, want) {
		t.Fatalf("public signal order = %v, want %v", names, want)
	}
}

func TestPublicSignalNames_IgnoresProfileWeightOrdering(t *testing.T) {
	t.Parallel()

	profile, err := score.ResolveProfile("")
	if err != nil {
		t.Fatalf("ResolveProfile: %v", err)
	}
	profile.Weights["repair_loop"] = 10.0
	profile.Weights["protocol_violation"] = 0.01

	got := PublicSignalNames(profile)
	want := PublicSignalNames(score.Profile{})
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("public signal order should ignore profile weights, got %v want %v", got, want)
	}
}

func TestPublicSignalNames_AreRegisteredAndWeighted(t *testing.T) {
	t.Parallel()

	profile, err := score.ResolveProfile("")
	if err != nil {
		t.Fatalf("ResolveProfile: %v", err)
	}

	registered := make(map[string]struct{}, len(signal.RegisteredSignalNames()))
	for _, name := range signal.RegisteredSignalNames() {
		registered[name] = struct{}{}
	}

	for _, name := range PublicSignalNames(profile) {
		if _, ok := registered[name]; !ok {
			t.Fatalf("public signal %q is not registered", name)
		}
		if _, ok := profile.Weights[name]; !ok {
			t.Fatalf("public signal %q is missing a scoring weight", name)
		}
	}
}
