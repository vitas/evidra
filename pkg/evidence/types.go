package evidence

import (
	"errors"
	"fmt"
	"time"

	"samebits.com/evidra-benchmark/pkg/invocation"
)

type PolicyDecision struct {
	Allow     bool     `json:"allow"`
	RiskLevel string   `json:"risk_level"`
	Reason    string   `json:"reason"`
	Reasons   []string `json:"reasons,omitempty"`
	Hints     []string `json:"hints,omitempty"`
	RuleIDs   []string `json:"rule_ids,omitempty"`
	Advisory  bool     `json:"advisory"`
}

type ExecutionResult struct {
	Status          string `json:"status"`
	ExitCode        *int   `json:"exit_code"`
	Stdout          string `json:"stdout,omitempty"`
	Stderr          string `json:"stderr,omitempty"`
	StdoutTruncated bool   `json:"stdout_truncated,omitempty"`
	StderrTruncated bool   `json:"stderr_truncated,omitempty"`
}

type EvidenceRecord struct {
	EventID          string                 `json:"event_id"`
	Timestamp        time.Time              `json:"timestamp"`
	PolicyRef        string                 `json:"policy_ref"`
	BundleRevision   string                 `json:"bundle_revision,omitempty"`
	ProfileName      string                 `json:"profile_name,omitempty"`
	EnvironmentLabel string                 `json:"environment_label,omitempty"`
	InputHash        string                 `json:"input_hash,omitempty"`
	Source           string                 `json:"source,omitempty"`
	Actor            invocation.Actor       `json:"actor"`
	Tool             string                 `json:"tool"`
	Operation        string                 `json:"operation"`
	Params           map[string]interface{} `json:"params"`
	PolicyDecision   PolicyDecision         `json:"policy_decision"`
	ExecutionResult  ExecutionResult        `json:"execution_result"`
	PreviousHash     string                 `json:"previous_hash"`
	Hash             string                 `json:"hash"`
}

type canonicalEvidenceRecord struct {
	EventID          string                 `json:"event_id"`
	Timestamp        time.Time              `json:"timestamp"`
	PolicyRef        string                 `json:"policy_ref"`
	BundleRevision   string                 `json:"bundle_revision,omitempty"`
	ProfileName      string                 `json:"profile_name,omitempty"`
	EnvironmentLabel string                 `json:"environment_label,omitempty"`
	InputHash        string                 `json:"input_hash,omitempty"`
	Source           string                 `json:"source,omitempty"`
	Actor            invocation.Actor       `json:"actor"`
	Tool             string                 `json:"tool"`
	Operation        string                 `json:"operation"`
	Params           map[string]interface{} `json:"params"`
	PolicyDecision   PolicyDecision         `json:"policy_decision"`
	ExecutionResult  ExecutionResult        `json:"execution_result"`
	PreviousHash     string                 `json:"previous_hash"`
}

type StoreManifest struct {
	Format          string   `json:"format"`
	CreatedAt       string   `json:"created_at"`
	UpdatedAt       string   `json:"updated_at"`
	SegmentsDir     string   `json:"segments_dir"`
	CurrentSegment  string   `json:"current_segment"`
	SealedSegments  []string `json:"sealed_segments"`
	SegmentMaxBytes int64    `json:"segment_max_bytes"`
	RecordsTotal    int      `json:"records_total"`
	LastHash        string   `json:"last_hash"`
	PolicyRef       string   `json:"policy_ref"`
	Notes           string   `json:"notes"`
}

type ForwarderCursor struct {
	Segment string `json:"segment"`
	Line    int    `json:"line"`
}

type ForwarderDestination struct {
	Type string `json:"type"`
	ID   string `json:"id"`
}

type ForwarderState struct {
	Format      string               `json:"format"`
	UpdatedAt   string               `json:"updated_at"`
	Cursor      ForwarderCursor      `json:"cursor"`
	LastAckHash string               `json:"last_ack_hash"`
	Destination ForwarderDestination `json:"destination"`
	Notes       string               `json:"notes"`
}

type Metadata struct {
	Records   int
	LastHash  string
	PolicyRef string
}

type Record = EvidenceRecord

const (
	defaultSegmentMaxBytes int64 = 5_000_000
	segmentMaxBytesEnv           = "EVIDRA_EVIDENCE_SEGMENT_MAX_BYTES"
	manifestFileName             = "manifest.json"
	segmentsDirName              = "segments"
	forwarderStateFileName       = "forwarder_state.json"
	lockFileName                 = ".evidra.lock"
	defaultLockTimeoutMS         = 2000
	lockTimeoutEnv               = "EVIDRA_EVIDENCE_LOCK_TIMEOUT_MS"
)

var ErrChainInvalid = errors.New("evidence_chain_invalid")
var ErrCursorSegmentNotFound = errors.New("cursor_segment_not_found")
var ErrCursorLineOutOfRange = errors.New("cursor_line_out_of_range")
var errCursorResolved = errors.New("cursor_resolved")

const (
	ErrorCodeStoreBusy               = "evidence_store_busy"
	ErrorCodeLockNotSupportedWindows = "evidence_lock_not_supported_on_windows"
)

type StoreError struct {
	Code    string
	Message string
	Err     error
}

func (e *StoreError) Error() string {
	if e == nil {
		return ""
	}
	return e.Message
}

func (e *StoreError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}

func ErrorCode(err error) string {
	var se *StoreError
	if errors.As(err, &se) {
		return se.Code
	}
	return ""
}

func IsStoreBusyError(err error) bool {
	return ErrorCode(err) == ErrorCodeStoreBusy
}

type ChainValidationError struct {
	Index   int
	EventID string
	Message string
}

func (e *ChainValidationError) Error() string {
	if e == nil {
		return ""
	}
	if e.EventID != "" {
		return fmt.Sprintf("record %d (%s): %s", e.Index, e.EventID, e.Message)
	}
	return fmt.Sprintf("record %d: %s", e.Index, e.Message)
}
