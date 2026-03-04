package evidence

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/oklog/ulid/v2"
)

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
	Type           EntryType
	TenantID       string
	TraceID        string
	Actor          Actor
	IntentDigest   string
	ArtifactDigest string
	Payload        json.RawMessage
	PreviousHash   string
	SpecVersion    string
	CanonVersion   string
	AdapterVersion string
	ScoringVersion string
}

// BuildEntry constructs a complete EvidenceEntry from the given parameters.
// It generates a ULID entry_id, timestamps the entry, formats digests with
// sha256: prefix, and computes the hash chain.
func BuildEntry(p EntryBuildParams) (EvidenceEntry, error) {
	if !p.Type.Valid() {
		return EvidenceEntry{}, fmt.Errorf("evidence.BuildEntry: invalid entry type %q", p.Type)
	}

	entry := EvidenceEntry{
		EntryID:        ulid.Make().String(),
		Type:           p.Type,
		TenantID:       p.TenantID,
		TraceID:        p.TraceID,
		Actor:          p.Actor,
		Timestamp:      time.Now().UTC(),
		IntentDigest:   FormatDigest(p.IntentDigest),
		ArtifactDigest: FormatDigest(p.ArtifactDigest),
		Payload:        p.Payload,
		PreviousHash:   p.PreviousHash,
		SpecVersion:    p.SpecVersion,
		CanonVersion:   p.CanonVersion,
		AdapterVersion: p.AdapterVersion,
		ScoringVersion: p.ScoringVersion,
	}

	hash, err := computeEntryHash(entry)
	if err != nil {
		return EvidenceEntry{}, fmt.Errorf("evidence.BuildEntry: %w", err)
	}
	entry.Hash = hash

	return entry, nil
}

// hashableEntry is a projection of EvidenceEntry that excludes Hash and
// Signature so they do not participate in hash computation.
type hashableEntry struct {
	EntryID        string          `json:"entry_id"`
	PreviousHash   string          `json:"previous_hash"`
	Type           EntryType       `json:"type"`
	TenantID       string          `json:"tenant_id,omitempty"`
	TraceID        string          `json:"trace_id"`
	Actor          Actor           `json:"actor"`
	Timestamp      time.Time       `json:"timestamp"`
	IntentDigest   string          `json:"intent_digest,omitempty"`
	ArtifactDigest string          `json:"artifact_digest,omitempty"`
	Payload        json.RawMessage `json:"payload"`
	SpecVersion    string          `json:"spec_version"`
	CanonVersion   string          `json:"canonical_version"`
	AdapterVersion string          `json:"adapter_version"`
	ScoringVersion string          `json:"scoring_version,omitempty"`
}

// computeEntryHash computes sha256:<hex> over all entry fields except hash and signature.
func computeEntryHash(e EvidenceEntry) (string, error) {
	h := hashableEntry{
		EntryID:        e.EntryID,
		PreviousHash:   e.PreviousHash,
		Type:           e.Type,
		TenantID:       e.TenantID,
		TraceID:        e.TraceID,
		Actor:          e.Actor,
		Timestamp:      e.Timestamp,
		IntentDigest:   e.IntentDigest,
		ArtifactDigest: e.ArtifactDigest,
		Payload:        e.Payload,
		SpecVersion:    e.SpecVersion,
		CanonVersion:   e.CanonVersion,
		AdapterVersion: e.AdapterVersion,
		ScoringVersion: e.ScoringVersion,
	}

	data, err := json.Marshal(h)
	if err != nil {
		return "", fmt.Errorf("computeEntryHash: marshal: %w", err)
	}

	sum := sha256.Sum256(data)
	return "sha256:" + hex.EncodeToString(sum[:]), nil
}
