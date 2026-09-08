package evidence

import (
	"strings"
	"testing"
)

func TestBundleManifestSaveLoad(t *testing.T) {
	root := t.TempDir()
	s, err := NewEphemeralSigner()
	if err != nil {
		t.Fatalf("NewEphemeralSigner: %v", err)
	}
	in := BundleManifest{
		Spec:       BundleSpecV1,
		Producer:   BundleProducer{Name: "test-producer", Version: "0.0.1", URL: "https://example.test"},
		TrustLevel: TrustEphemeral,
		PublicKey:  s.PublicKeyBase64(),
		Notes:      "unit test bundle",
	}
	if err := SaveBundleManifest(root, in); err != nil {
		t.Fatalf("SaveBundleManifest: %v", err)
	}
	if in.CreatedAt != "" {
		t.Fatal("CreatedAt must not be mutated on the caller's copy")
	}
	out, found, err := LoadBundleManifest(root)
	if err != nil {
		t.Fatalf("LoadBundleManifest: %v", err)
	}
	if !found {
		t.Fatal("bundle manifest not found")
	}
	if out.Spec != BundleSpecV1 || out.Producer.Name != "test-producer" || out.TrustLevel != TrustEphemeral {
		t.Fatalf("round-trip mismatch: %+v", out)
	}
	if out.CreatedAt == "" {
		t.Fatal("default CreatedAt not populated")
	}
	if pub, err := out.PublicKeyBytes(); err != nil || string(pub) != string(s.PublicKey()) {
		t.Fatalf("PublicKeyBytes mismatch: %v", err)
	}
}

func TestBundleManifestRejectsBadKey(t *testing.T) {
	root := t.TempDir()
	err := SaveBundleManifest(root, BundleManifest{Spec: BundleSpecV1, PublicKey: "not-base64!!"})
	if err == nil || !strings.Contains(err.Error(), "public_key") {
		t.Fatalf("expected public_key validation error, got %v", err)
	}
}

func TestLoadBundleManifestMissing(t *testing.T) {
	_, found, err := LoadBundleManifest(t.TempDir())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if found {
		t.Fatal("found=true for empty directory")
	}
}

func TestValidateBundleRejectsWithoutManifest(t *testing.T) {
	if _, err := ValidateBundle(t.TempDir()); err == nil {
		t.Fatal("expected error for store without bundle.json")
	}
}
