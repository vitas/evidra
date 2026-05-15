package evidence

import (
	"strings"
	"testing"
)

func TestSHA256Hex(t *testing.T) {
	t.Parallel()

	got := SHA256Hex([]byte("hello"))
	if !strings.HasPrefix(got, "sha256:") || len(got) != len("sha256:")+64 {
		t.Fatalf("digest = %q", got)
	}
}

func TestComputeDeclaredIntentDigest_ExcludesArtifactDigest(t *testing.T) {
	t.Parallel()

	a := DeclaredIntent{
		Tool:           "kubectl",
		Operation:      "apply",
		Target:         "deployment/web",
		ArtifactDigest: "sha256:" + strings.Repeat("a", 64),
	}
	b := a
	b.ArtifactDigest = "sha256:" + strings.Repeat("b", 64)

	if ComputeDeclaredIntentDigest(a) != ComputeDeclaredIntentDigest(b) {
		t.Fatal("declared intent digest should exclude artifact digest")
	}
}
