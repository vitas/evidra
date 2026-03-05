package testutil

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"testing"

	"samebits.com/evidra-benchmark/pkg/evidence"
)

// TestSigner returns a Signer backed by a freshly generated Ed25519 key pair.
func TestSigner(t *testing.T) evidence.Signer {
	t.Helper()
	_, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("generate test key: %v", err)
	}
	return &testSignerImpl{priv: priv}
}

type testSignerImpl struct {
	priv ed25519.PrivateKey
}

func (s *testSignerImpl) Sign(payload []byte) []byte {
	return ed25519.Sign(s.priv, payload)
}

func (s *testSignerImpl) Verify(payload, sig []byte) bool {
	pub := s.priv.Public().(ed25519.PublicKey)
	return ed25519.Verify(pub, payload, sig)
}

func (s *testSignerImpl) PublicKey() ed25519.PublicKey {
	return s.priv.Public().(ed25519.PublicKey)
}

// TestSigningKeyBase64 returns a base64-encoded Ed25519 seed (32 bytes)
// suitable for passing as the --signing-key flag or EVIDRA_SIGNING_KEY
// environment variable in tests.
func TestSigningKeyBase64(t *testing.T) string {
	t.Helper()
	_, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("generate test key: %v", err)
	}
	seed := priv.Seed()
	return base64.StdEncoding.EncodeToString(seed)
}
