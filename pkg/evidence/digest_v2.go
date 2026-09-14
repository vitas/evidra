package evidence

// Operational fingerprints (§23-§24).
//
// Tool arguments and results are not stored: they carry whatever the caller put
// in them. What is stored is a keyed HMAC, which allows "was this the same call
// again?" to be answered locally without publishing the payload. Two properties
// matter and only two are claimed: equality under the same key, and non-reversal
// for anything with real entropy. Low-entropy values are not protected — an
// attacker who can read the store can brute-force `{"service":"payments"}`.

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
)

// DigestKeyLen is 32 bytes, one SHA-256 block.
const DigestKeyLen = 32

// DefaultMaxResultBytes bounds what a result fingerprint may cost (§24). Reading
// a hundred megabyte tool result into memory just to hash it would make the
// recorder the least scalable part of the pipeline.
const DefaultMaxResultBytes = 4 << 20

// NewDigestKey generates the per-store HMAC key.
func NewDigestKey() ([]byte, error) {
	key := make([]byte, DigestKeyLen)
	if _, err := rand.Read(key); err != nil {
		return nil, fmt.Errorf("generate digest key: %w", err)
	}
	return key, nil
}

// DigestKeyID is the stable non-secret name of a comparison domain. Fingerprints
// are only comparable inside one domain, so aggregation groups by this id rather
// than pretending keys are global (§20).
func DigestKeyID(key []byte) string {
	sum := sha256.Sum256(key)
	return "dk1:" + hex.EncodeToString(sum[:8])
}

// LoadOrCreateDigestKey reads digest.key, creating it with 0600 when absent.
func LoadOrCreateDigestKey(path string) ([]byte, error) {
	raw, err := readIfExists(path)
	if err != nil {
		return nil, err
	}
	if len(raw) > 0 {
		key, err := hex.DecodeString(trimOneSpace(raw))
		if err != nil || len(key) != DigestKeyLen {
			return nil, fmt.Errorf("%s: not a %d-byte hex key", path, DigestKeyLen)
		}
		return key, nil
	}
	key, err := NewDigestKey()
	if err != nil {
		return nil, err
	}
	return key, writeExclusive(path, []byte(hex.EncodeToString(key)+"\n"), 0o600)
}

// HMACHex is the fingerprint primitive: keyed, hex-encoded SHA-256.
func HMACHex(key, data []byte) string {
	mac := hmac.New(sha256.New, key)
	mac.Write(data)
	return "sha256:" + hex.EncodeToString(mac.Sum(nil))
}

// ArgumentsHMAC canonicalizes arguments before hashing them, so the same object
// serialized with different key order or whitespace fingerprints identically.
func ArgumentsHMAC(key []byte, args any) (string, error) {
	canonical, err := CanonicalizeJCS(args)
	if err != nil {
		return "", fmt.Errorf("arguments fingerprint: %w", err)
	}
	return HMACHex(key, canonical), nil
}

// ResultFingerprint hashes the exact wire representation of a result, not a
// semantic normalization of it: the point is "the same bytes came back", and
// claiming more than that is how a fingerprint becomes a lie.
//
// Results above maxBytes are reported as omitted rather than hashed partially;
// a truncated hash would look like a real fingerprint in every downstream count.
func ResultFingerprint(key, raw []byte, maxBytes int) (hmacStr, status, format string) {
	if maxBytes <= 0 {
		maxBytes = DefaultMaxResultBytes
	}
	if len(raw) == 0 {
		return "", FingerprintUnavailable, ""
	}
	if len(raw) > maxBytes {
		return "", FingerprintOmittedOversize, ""
	}
	return HMACHex(key, raw), FingerprintPresent, "wire_json"
}

// ErrorFingerprint keys a message the recorder must keep the shape of without
// keeping the text, which is often where credentials leak into evidence.
func ErrorFingerprint(key []byte, message string) string {
	if message == "" {
		return ""
	}
	return HMACHex(key, []byte(message))
}
