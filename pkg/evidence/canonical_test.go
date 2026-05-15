package evidence

import "testing"

func TestComputeCanonicalActionDigestExcludesShapeHash(t *testing.T) {
	action := CanonicalAction{
		Tool:              "kubectl",
		Operation:         "apply",
		OperationClass:    "mutate",
		ScopeClass:        "production",
		ResourceCount:     1,
		ResourceShapeHash: "sha256:shape-a",
		ResourceIdentity: []ResourceID{
			{APIVersion: "apps/v1", Kind: "Deployment", Namespace: "prod", Name: "web"},
		},
	}
	changedShape := action
	changedShape.ResourceShapeHash = "sha256:shape-b"

	if ComputeCanonicalActionDigest(action) != ComputeCanonicalActionDigest(changedShape) {
		t.Fatal("canonical action digest should exclude resource_shape_hash")
	}
}

func TestResolveScopeClass(t *testing.T) {
	if got := ResolveScopeClass("prod", nil); got != "production" {
		t.Fatalf("scope = %q, want production", got)
	}
	if got := ResolveScopeClass("", []ResourceID{{Namespace: "staging-system"}}); got != "staging" {
		t.Fatalf("scope = %q, want staging", got)
	}
	if got := ResolveScopeClass("", nil); got != "unknown" {
		t.Fatalf("scope = %q, want unknown", got)
	}
}
