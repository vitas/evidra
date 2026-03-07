package evidence

import (
	"encoding/json"
	"time"
)

// EntryType represents the kind of evidence entry in the append-only log.
type EntryType string

const (
	// EntryTypePrescribe is a pre-execution risk assessment.
	EntryTypePrescribe EntryType = "prescribe"
	// EntryTypeReport is a post-execution outcome report.
	EntryTypeReport EntryType = "report"
	// EntryTypeFinding is an inspector-generated finding.
	EntryTypeFinding EntryType = "finding"
	// EntryTypeSignal is a behavioral signal detection result.
	EntryTypeSignal EntryType = "signal"
	// EntryTypeReceipt is an acknowledgement from a remote system.
	EntryTypeReceipt EntryType = "receipt"
	// EntryTypeCanonFailure records a canonicalization failure.
	EntryTypeCanonFailure EntryType = "canonicalization_failure"
	// EntryTypeSessionStart marks the beginning of a session.
	EntryTypeSessionStart EntryType = "session_start"
	// EntryTypeSessionEnd marks the end of a session.
	EntryTypeSessionEnd EntryType = "session_end"
	// EntryTypeAnnotation is a human or system annotation on a session.
	EntryTypeAnnotation EntryType = "annotation"
)

// validEntryTypes enumerates all allowed EntryType values.
var validEntryTypes = map[EntryType]bool{
	EntryTypePrescribe:    true,
	EntryTypeReport:       true,
	EntryTypeFinding:      true,
	EntryTypeSignal:       true,
	EntryTypeReceipt:      true,
	EntryTypeCanonFailure: true,
	EntryTypeSessionStart: true,
	EntryTypeSessionEnd:   true,
	EntryTypeAnnotation:   true,
}

// Valid reports whether et is a recognised entry type.
func (et EntryType) Valid() bool {
	return validEntryTypes[et]
}

// Actor identifies who or what produced the evidence entry.
type Actor struct {
	Type         string `json:"type"`
	ID           string `json:"id"`
	Provenance   string `json:"provenance"`
	InstanceID   string `json:"instance_id,omitempty"`
	Version      string `json:"version,omitempty"`
	SkillVersion string `json:"skill_version,omitempty"`
}

// EvidenceEntry is an append-only event log entry. Every JSONL line in an
// evidence segment file is one EvidenceEntry.
type EvidenceEntry struct {
	EntryID      string    `json:"entry_id"`
	PreviousHash string    `json:"previous_hash"`
	Hash         string    `json:"hash"`
	Signature    string    `json:"signature"`
	Type         EntryType `json:"type"`
	TenantID     string    `json:"tenant_id,omitempty"`
	// SessionID groups operations from one automation attempt (for example:
	// one CI pipeline run, one AI agent task, or one operator workflow).
	// For meaningful signal detection and scorecards, callers should generate
	// one session_id at task start and reuse it across prescribe/report calls.
	SessionID       string            `json:"session_id,omitempty"`
	OperationID     string            `json:"operation_id,omitempty"`
	Attempt         int               `json:"attempt,omitempty"`
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
