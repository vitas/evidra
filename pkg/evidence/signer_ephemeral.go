package evidence

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"fmt"
)

// EphemeralSigner is an in-memory Ed25519 signer intended for external
// evidence-bundle producers (for example, benchmark harnesses that record a
// local run and hand the bundle to a human to import).
//
// It satisfies the Signer interface consumed by BuildEntry, so entries are
// signed with the exact same payload encoding (base64 Ed25519 over the entry
// hash string) that ValidateChainWithSignatures verifies. No key management is
// required by the producer: the key exists only for the lifetime of the
// process and the public half is embedded in the bundle manifest as evidence
// that the chain was produced atomically and not edited afterwards.
//
// Trust model: ephemeral signatures prove integrity and atomic authorship, not
// identity. Consumers should treat bundles signed with an embedded key as
// trust level "ephemeral", never as authenticated producer evidence. See
// docs/external-evidence-bundle-v1.md.
type EphemeralSigner struct {
	private ed25519.PrivateKey
	public  ed25519.PublicKey
}

// NewEphemeralSigner generates a fresh Ed25519 key pair.
func NewEphemeralSigner() (*EphemeralSigner, error) {
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return nil, fmt.Errorf("evidence.NewEphemeralSigner: generate key: %w", err)
	}
	return &EphemeralSigner{private: priv, public: pub}, nil
}

// Sign returns the raw Ed25519 signature of payload.
func (s *EphemeralSigner) Sign(payload []byte) []byte {
	return ed25519.Sign(s.private, payload)
}

// Verify reports whether sig is a valid signature of payload.
func (s *EphemeralSigner) Verify(payload, sig []byte) bool {
	return ed25519.Verify(s.public, payload, sig)
}

// PublicKey returns the ephemeral public key.
func (s *EphemeralSigner) PublicKey() ed25519.PublicKey {
	return s.public
}

// PublicKeyBase64 returns the public key as standard base64, the encoding
// stored in BundleManifest.PublicKey.
func (s *EphemeralSigner) PublicKeyBase64() string {
	return base64.StdEncoding.EncodeToString(s.public)
}
