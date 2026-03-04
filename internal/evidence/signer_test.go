package evidence

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"os"
	"path/filepath"
	"testing"
)

func testSigner(t *testing.T) *Signer {
	t.Helper()
	s, err := NewSigner(SignerConfig{DevMode: true})
	if err != nil {
		t.Fatalf("testSigner: %v", err)
	}
	return s
}

func TestNewSigner_Ephemeral(t *testing.T) {
	t.Parallel()
	s := testSigner(t)

	if s.privateKey == nil {
		t.Fatal("expected private key")
	}
	if s.publicKey == nil {
		t.Fatal("expected public key")
	}
	if len(s.publicKey) != ed25519.PublicKeySize {
		t.Fatalf("public key length = %d, want %d", len(s.publicKey), ed25519.PublicKeySize)
	}
}

func TestNewSigner_Base64Seed(t *testing.T) {
	t.Parallel()
	_, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}

	encoded := base64.StdEncoding.EncodeToString(priv.Seed())
	s, err := NewSigner(SignerConfig{KeyBase64: encoded})
	if err != nil {
		t.Fatalf("NewSigner: %v", err)
	}

	if !s.publicKey.Equal(priv.Public()) {
		t.Fatal("public key mismatch from seed")
	}
}

func TestNewSigner_Base64FullKey(t *testing.T) {
	t.Parallel()
	_, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}

	encoded := base64.StdEncoding.EncodeToString(priv)
	s, err := NewSigner(SignerConfig{KeyBase64: encoded})
	if err != nil {
		t.Fatalf("NewSigner: %v", err)
	}

	if !s.publicKey.Equal(priv.Public()) {
		t.Fatal("public key mismatch from full key")
	}
}

func TestNewSigner_Base64Invalid(t *testing.T) {
	t.Parallel()
	_, err := NewSigner(SignerConfig{KeyBase64: "not-valid-base64!!!"})
	if err == nil {
		t.Fatal("expected error for invalid base64")
	}
}

func TestNewSigner_Base64WrongLength(t *testing.T) {
	t.Parallel()
	encoded := base64.StdEncoding.EncodeToString([]byte("too-short"))
	_, err := NewSigner(SignerConfig{KeyBase64: encoded})
	if err != ErrInvalidKey {
		t.Fatalf("err = %v, want ErrInvalidKey", err)
	}
}

func TestNewSigner_PEMFile(t *testing.T) {
	t.Parallel()
	_, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}

	pemBytes := marshalPrivateKeyPEM(t, priv)
	path := filepath.Join(t.TempDir(), "key.pem")
	if err := os.WriteFile(path, pemBytes, 0600); err != nil {
		t.Fatal(err)
	}

	s, err := NewSigner(SignerConfig{KeyPath: path})
	if err != nil {
		t.Fatalf("NewSigner: %v", err)
	}

	if !s.publicKey.Equal(priv.Public()) {
		t.Fatal("public key mismatch from PEM")
	}
}

func TestNewSigner_PEMFileNotFound(t *testing.T) {
	t.Parallel()
	_, err := NewSigner(SignerConfig{KeyPath: "/nonexistent/key.pem"})
	if err == nil {
		t.Fatal("expected error for missing file")
	}
}

func TestNewSigner_PEMInvalidBlock(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "bad.pem")
	if err := os.WriteFile(path, []byte("not a pem file"), 0600); err != nil {
		t.Fatal(err)
	}

	_, err := NewSigner(SignerConfig{KeyPath: path})
	if err != ErrInvalidPEMBlock {
		t.Fatalf("err = %v, want ErrInvalidPEMBlock", err)
	}
}

func TestNewSigner_NoKeyProduction(t *testing.T) {
	t.Parallel()
	_, err := NewSigner(SignerConfig{})
	if err != ErrNoSigningKey {
		t.Fatalf("err = %v, want ErrNoSigningKey", err)
	}
}

func TestNewSigner_PriorityOrder(t *testing.T) {
	t.Parallel()

	// Generate two distinct keys.
	_, privA, _ := ed25519.GenerateKey(rand.Reader)
	_, privB, _ := ed25519.GenerateKey(rand.Reader)

	b64 := base64.StdEncoding.EncodeToString(privA.Seed())
	pemBytes := marshalPrivateKeyPEM(t, privB)
	path := filepath.Join(t.TempDir(), "key.pem")
	if err := os.WriteFile(path, pemBytes, 0600); err != nil {
		t.Fatal(err)
	}

	// KeyBase64 takes priority over KeyPath.
	s, err := NewSigner(SignerConfig{KeyBase64: b64, KeyPath: path, DevMode: true})
	if err != nil {
		t.Fatalf("NewSigner: %v", err)
	}
	if !s.publicKey.Equal(privA.Public()) {
		t.Fatal("expected base64 key to win over PEM")
	}
}

func TestSign_Verify_RoundTrip(t *testing.T) {
	t.Parallel()
	s := testSigner(t)

	payload := []byte("evidra.v1\nevent_id=abc123\n")
	sig := s.Sign(payload)

	if !s.Verify(payload, sig) {
		t.Fatal("valid signature rejected")
	}
}

func TestVerify_WrongPayload(t *testing.T) {
	t.Parallel()
	s := testSigner(t)

	sig := s.Sign([]byte("original"))
	if s.Verify([]byte("tampered"), sig) {
		t.Fatal("tampered payload should not verify")
	}
}

func TestVerify_WrongKey(t *testing.T) {
	t.Parallel()
	s1 := testSigner(t)
	s2 := testSigner(t)

	payload := []byte("test payload")
	sig := s1.Sign(payload)

	if s2.Verify(payload, sig) {
		t.Fatal("signature from different key should not verify")
	}
}

func TestVerify_EmptyPayload(t *testing.T) {
	t.Parallel()
	s := testSigner(t)

	sig := s.Sign([]byte{})
	if !s.Verify([]byte{}, sig) {
		t.Fatal("empty payload sign/verify should round-trip")
	}
}

func TestPublicKey(t *testing.T) {
	t.Parallel()
	s := testSigner(t)

	pub := s.PublicKey()
	if len(pub) != ed25519.PublicKeySize {
		t.Fatalf("public key length = %d, want %d", len(pub), ed25519.PublicKeySize)
	}
}

func TestPublicKeyPEM(t *testing.T) {
	t.Parallel()
	s := testSigner(t)

	pemData, err := s.PublicKeyPEM()
	if err != nil {
		t.Fatalf("PublicKeyPEM: %v", err)
	}

	block, _ := pem.Decode(pemData)
	if block == nil {
		t.Fatal("no PEM block found")
	}
	if block.Type != "PUBLIC KEY" {
		t.Fatalf("PEM type = %q, want PUBLIC KEY", block.Type)
	}

	pub, err := x509.ParsePKIXPublicKey(block.Bytes)
	if err != nil {
		t.Fatalf("parse public key: %v", err)
	}

	edPub, ok := pub.(ed25519.PublicKey)
	if !ok {
		t.Fatal("parsed key is not Ed25519")
	}
	if !edPub.Equal(s.PublicKey()) {
		t.Fatal("PEM-encoded public key does not match original")
	}
}

func marshalPrivateKeyPEM(t *testing.T, priv ed25519.PrivateKey) []byte {
	t.Helper()
	der, err := x509.MarshalPKCS8PrivateKey(priv)
	if err != nil {
		t.Fatalf("marshal PKCS8: %v", err)
	}
	return pem.EncodeToMemory(&pem.Block{
		Type:  "PRIVATE KEY",
		Bytes: der,
	})
}
