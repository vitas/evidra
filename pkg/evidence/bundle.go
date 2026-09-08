package evidence

import (
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// External evidence bundle spec. A bundle is a regular segmented evidence
// store (manifest.json + segments/) produced by an external tool, carrying a
// sidecar bundle.json that names the producer and the signing key. See
// docs/external-evidence-bundle-v1.md.
const (
	// BundleSpecV1 is the bundle.json schema identifier.
	BundleSpecV1 = "evidra-external-bundle/v1"

	// BundleFileName is the sidecar manifest file at the store root.
	BundleFileName = "bundle.json"

	// TrustEphemeral marks a bundle signed with a key generated per run and
	// embedded in the bundle itself: integrity without identity.
	TrustEphemeral = "ephemeral"

	// TrustVerified marks a bundle signed with a long-lived producer key.
	TrustVerified = "verified"
)

// BundleProducer identifies the tool that created an external bundle.
type BundleProducer struct {
	Name    string `json:"name"`
	Version string `json:"version,omitempty"`
	URL     string `json:"url,omitempty"`
}

// BundleManifest is the bundle.json sidecar describing an external bundle.
//
// PublicKey is the standard-base64 Ed25519 public key whose EphemeralSigner
// (or an equivalent Signer implementation) produced every entry signature in
// the store. Verifiers must reject a bundle whose entries carry signatures
// that do not validate against this key.
type BundleManifest struct {
	Spec       string         `json:"spec"`
	Producer   BundleProducer `json:"producer"`
	TrustLevel string         `json:"trust_level"`
	PublicKey  string         `json:"public_key"`
	CreatedAt  string         `json:"created_at"`
	Notes      string         `json:"notes,omitempty"`
}

// BundleManifestPath returns the bundle.json path for a store root.
func BundleManifestPath(root string) string {
	return filepath.Join(root, BundleFileName)
}

// SaveBundleManifest writes bundle.json into the store root atomically enough
// for the append-only model (single JSON file, no locking required before the
// store is published).
func SaveBundleManifest(root string, m BundleManifest) error {
	if m.Spec == "" {
		return fmt.Errorf("evidence.SaveBundleManifest: spec is required")
	}
	if m.CreatedAt == "" {
		m.CreatedAt = time.Now().UTC().Format(time.RFC3339)
	}
	if _, err := parseBundlePublicKey(m.PublicKey); err != nil {
		return fmt.Errorf("evidence.SaveBundleManifest: public_key must be base64 Ed25519: %w", err)
	}
	data, err := json.MarshalIndent(&m, "", "  ")
	if err != nil {
		return fmt.Errorf("evidence.SaveBundleManifest: marshal: %w", err)
	}
	if err := os.MkdirAll(root, 0o755); err != nil {
		return fmt.Errorf("evidence.SaveBundleManifest: mkdir: %w", err)
	}
	tmp := BundleManifestPath(root) + ".tmp"
	if err := os.WriteFile(tmp, append(data, '\n'), 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, BundleManifestPath(root))
}

// LoadBundleManifest reads bundle.json from a store root. It returns
// found=false (with a nil error) when the file does not exist, meaning the
// store is not an external bundle or predates the bundle spec.
func LoadBundleManifest(root string) (BundleManifest, bool, error) {
	var m BundleManifest
	raw, err := os.ReadFile(BundleManifestPath(root))
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return m, false, nil
		}
		return m, false, fmt.Errorf("evidence.LoadBundleManifest: %w", err)
	}
	if err := json.Unmarshal(raw, &m); err != nil {
		return m, true, fmt.Errorf("evidence.LoadBundleManifest: parse: %w", err)
	}
	return m, true, nil
}

// PublicKeyBytes decodes the manifest public key.
func (m BundleManifest) PublicKeyBytes() (ed25519.PublicKey, error) {
	key, err := parseBundlePublicKey(m.PublicKey)
	if err != nil {
		return nil, fmt.Errorf("evidence.BundleManifest: invalid public_key: %w", err)
	}
	return key, nil
}

func parseBundlePublicKey(b64 string) (ed25519.PublicKey, error) {
	raw, err := base64.StdEncoding.DecodeString(b64)
	if err != nil {
		return nil, err
	}
	if len(raw) != ed25519.PublicKeySize {
		return nil, fmt.Errorf("expected %d-byte ed25519 key, got %d", ed25519.PublicKeySize, len(raw))
	}
	return ed25519.PublicKey(raw), nil
}

// ValidateBundle performs the full external-bundle check: hash chain
// integrity plus signature verification against the embedded public key.
// Stores without a bundle.json are rejected as not being external bundles.
func ValidateBundle(root string) (BundleManifest, error) {
	m, found, err := LoadBundleManifest(root)
	if err != nil {
		return BundleManifest{}, err
	}
	if !found {
		return BundleManifest{}, fmt.Errorf("evidence.ValidateBundle: %s not found in %s (not an external bundle)", BundleFileName, root)
	}
	if m.Spec != BundleSpecV1 {
		return m, fmt.Errorf("evidence.ValidateBundle: unsupported bundle spec %q (want %s)", m.Spec, BundleSpecV1)
	}
	pub, err := m.PublicKeyBytes()
	if err != nil {
		return m, err
	}
	if err := ValidateChainWithSignatures(root, pub); err != nil {
		return m, fmt.Errorf("evidence.ValidateBundle: %w", err)
	}
	return m, nil
}
