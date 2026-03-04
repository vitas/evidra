package evidence

import (
	"errors"
	"fmt"
)

// StoreManifest describes the state of a segmented evidence store.
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

const (
	defaultSegmentMaxBytes int64 = 5_000_000
	segmentMaxBytesEnv           = "EVIDRA_EVIDENCE_SEGMENT_MAX_BYTES"
	manifestFileName             = "manifest.json"
	segmentsDirName              = "segments"
	lockFileName                 = ".evidra.lock"
	defaultLockTimeoutMS         = 2000
	lockTimeoutEnv               = "EVIDRA_EVIDENCE_LOCK_TIMEOUT_MS"
)

var ErrChainInvalid = errors.New("evidence_chain_invalid")
var ErrCursorSegmentNotFound = errors.New("cursor_segment_not_found")
var ErrCursorLineOutOfRange = errors.New("cursor_line_out_of_range")

const (
	ErrorCodeStoreBusy               = "evidence_store_busy"
	ErrorCodeLockNotSupportedWindows = "evidence_lock_not_supported_on_windows"
)

// StoreError represents an error from the evidence store with an error code.
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

// ErrorCode extracts the error code from a StoreError, or returns empty string.
func ErrorCode(err error) string {
	var se *StoreError
	if errors.As(err, &se) {
		return se.Code
	}
	return ""
}

// IsStoreBusyError reports whether err is a store-busy error.
func IsStoreBusyError(err error) bool {
	return ErrorCode(err) == ErrorCodeStoreBusy
}

// ChainValidationError describes a hash-chain validation failure at a specific record index.
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
