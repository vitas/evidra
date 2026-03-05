package evidence

import (
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/oklog/ulid/v2"
)

// Signer signs evidence entry hashes. When provided to BuildEntry,
// the entry's Signature field is populated with a base64-encoded Ed25519 signature.
type Signer interface {
	Sign(payload []byte) []byte
	Verify(payload, sig []byte) bool
	PublicKey() ed25519.PublicKey
}

// DefaultTTLMs is the default time-to-live for a prescription in milliseconds (5 minutes).
const DefaultTTLMs = 300000

// FormatDigest ensures a digest string has the "sha256:" prefix.
// Empty strings are returned as-is.
func FormatDigest(d string) string {
	if d == "" {
		return ""
	}
	if strings.HasPrefix(d, "sha256:") {
		return d
	}
	return "sha256:" + d
}

// EntryBuildParams holds all inputs needed to construct an EvidenceEntry.
type EntryBuildParams struct {
	Type            EntryType
	TenantID        string
	SessionID       string
	TraceID         string
	SpanID          string
	ParentSpanID    string
	Actor           Actor
	IntentDigest    string
	ArtifactDigest  string
	Payload         json.RawMessage
	PreviousHash    string
	ScopeDimensions map[string]string
	SpecVersion     string
	CanonVersion    string
	AdapterVersion  string
	ScoringVersion  string
	Signer          Signer // required: signs the entry hash
}

// BuildEntry constructs a complete EvidenceEntry from the given parameters.
// It generates a ULID entry_id, timestamps the entry, formats digests with
// sha256: prefix, and computes the hash chain.
func BuildEntry(p EntryBuildParams) (EvidenceEntry, error) {
	if p.Signer == nil {
		return EvidenceEntry{}, fmt.Errorf("evidence.BuildEntry: Signer is required")
	}
	if !p.Type.Valid() {
		return EvidenceEntry{}, fmt.Errorf("evidence.BuildEntry: invalid entry type %q", p.Type)
	}

	entry := EvidenceEntry{
		EntryID:         ulid.Make().String(),
		Type:            p.Type,
		TenantID:        p.TenantID,
		SessionID:       p.SessionID,
		TraceID:         p.TraceID,
		SpanID:          p.SpanID,
		ParentSpanID:    p.ParentSpanID,
		Actor:           p.Actor,
		Timestamp:       time.Now().UTC(),
		IntentDigest:    FormatDigest(p.IntentDigest),
		ArtifactDigest:  FormatDigest(p.ArtifactDigest),
		Payload:         p.Payload,
		PreviousHash:    p.PreviousHash,
		ScopeDimensions: p.ScopeDimensions,
		SpecVersion:     p.SpecVersion,
		CanonVersion:    p.CanonVersion,
		AdapterVersion:  p.AdapterVersion,
		ScoringVersion:  p.ScoringVersion,
	}

	hash, err := computeEntryHash(entry)
	if err != nil {
		return EvidenceEntry{}, fmt.Errorf("evidence.BuildEntry: %w", err)
	}
	entry.Hash = hash

	sig := p.Signer.Sign([]byte(hash))
	entry.Signature = base64.StdEncoding.EncodeToString(sig)

	return entry, nil
}

// RehashEntry recomputes the hash and signature of an entry after its payload
// has been mutated. This is needed when fields like prescription_id are set
// after initial BuildEntry.
func RehashEntry(entry *EvidenceEntry, signer Signer) error {
	if signer == nil {
		return fmt.Errorf("evidence.RehashEntry: Signer is required")
	}
	hash, err := computeEntryHash(*entry)
	if err != nil {
		return fmt.Errorf("evidence.RehashEntry: %w", err)
	}
	entry.Hash = hash

	sig := signer.Sign([]byte(hash))
	entry.Signature = base64.StdEncoding.EncodeToString(sig)
	return nil
}

// hashableEntry is a projection of EvidenceEntry that excludes Hash and
// Signature so they do not participate in hash computation.
type hashableEntry struct {
	EntryID         string            `json:"entry_id"`
	PreviousHash    string            `json:"previous_hash"`
	Type            EntryType         `json:"type"`
	TenantID        string            `json:"tenant_id,omitempty"`
	SessionID       string            `json:"session_id,omitempty"`
	TraceID         string            `json:"trace_id"`
	SpanID          string            `json:"span_id,omitempty"`
	ParentSpanID    string            `json:"parent_span_id,omitempty"`
	Actor           Actor             `json:"actor"`
	Timestamp       time.Time         `json:"timestamp"`
	IntentDigest    string            `json:"intent_digest,omitempty"`
	ArtifactDigest  string            `json:"artifact_digest,omitempty"`
	Payload         json.RawMessage   `json:"payload"`
	ScopeDimensions map[string]string `json:"scope_dimensions,omitempty"`
	SpecVersion     string            `json:"spec_version"`
	CanonVersion    string            `json:"canonical_version"`
	AdapterVersion  string            `json:"adapter_version"`
	ScoringVersion  string            `json:"scoring_version,omitempty"`
}

// computeEntryHash computes sha256:<hex> over all entry fields except hash and signature.
func computeEntryHash(e EvidenceEntry) (string, error) {
	h := hashableEntry{
		EntryID:         e.EntryID,
		PreviousHash:    e.PreviousHash,
		Type:            e.Type,
		TenantID:        e.TenantID,
		SessionID:       e.SessionID,
		TraceID:         e.TraceID,
		SpanID:          e.SpanID,
		ParentSpanID:    e.ParentSpanID,
		Actor:           e.Actor,
		Timestamp:       e.Timestamp,
		IntentDigest:    e.IntentDigest,
		ArtifactDigest:  e.ArtifactDigest,
		Payload:         e.Payload,
		ScopeDimensions: e.ScopeDimensions,
		SpecVersion:     e.SpecVersion,
		CanonVersion:    e.CanonVersion,
		AdapterVersion:  e.AdapterVersion,
		ScoringVersion:  e.ScoringVersion,
	}

	data, err := json.Marshal(h)
	if err != nil {
		return "", fmt.Errorf("computeEntryHash: marshal: %w", err)
	}

	sum := sha256.Sum256(data)
	return "sha256:" + hex.EncodeToString(sum[:]), nil
}
