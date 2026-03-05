package lifecycle

import (
	"errors"
	"fmt"

	"samebits.com/evidra-benchmark/internal/canon"
	"samebits.com/evidra-benchmark/pkg/evidence"
)

// RetryRecorder tracks repeated operations by intent digest and shape hash.
type RetryRecorder interface {
	Record(intentDigest, resourceShapeHash string) int
}

// Options configures the shared lifecycle service.
type Options struct {
	EvidencePath string
	Signer       evidence.Signer
	RetryTracker RetryRecorder
}

// Service is the shared prescribe/report business logic used by CLI and MCP.
type Service struct {
	evidencePath string
	signer       evidence.Signer
	retryTracker RetryRecorder
}

// NewService creates a lifecycle service from options.
func NewService(opts Options) *Service {
	return &Service{
		evidencePath: opts.EvidencePath,
		signer:       opts.Signer,
		retryTracker: opts.RetryTracker,
	}
}

// PrescribeInput captures pre-execution operation context.
type PrescribeInput struct {
	Actor           evidence.Actor
	Tool            string
	Operation       string
	RawArtifact     []byte
	Environment     string
	CanonicalAction *canon.CanonicalAction
	SessionID       string
	OperationID     string
	Attempt         int
	TraceID         string
	SpanID          string
	ParentSpanID    string
	ScopeDimensions map[string]string
}

// PrescribeOutput contains stable data adapters need to render responses.
type PrescribeOutput struct {
	PrescriptionID string
	SessionID      string
	TraceID        string
	Actor          evidence.Actor
	RiskLevel      string
	RiskTags       []string
	ArtifactDigest string
	IntentDigest   string
	ShapeHash      string
	ResourceCount  int
	OperationClass string
	ScopeClass     string
	CanonVersion   string
	RetryCount     int
}

// ReportInput captures post-execution operation context.
type ReportInput struct {
	PrescriptionID string
	ExitCode       int
	ArtifactDigest string
	Actor          evidence.Actor
	ExternalRefs   []evidence.ExternalRef
	SessionID      string
	OperationID    string
	SpanID         string
	ParentSpanID   string
}

// ReportOutput contains identifiers/correlation for written report entries.
type ReportOutput struct {
	ReportID       string
	SessionID      string
	TraceID        string
	Actor          evidence.Actor
	PrescriptionID string
}

// Code is a stable adapter-facing lifecycle error code.
type Code string

const (
	ErrCodeInvalidInput       Code = "invalid_input"
	ErrCodeParseError         Code = "parse_error"
	ErrCodeNotFound           Code = "not_found"
	ErrCodeInternal           Code = "internal_error"
	ErrCodeEvidenceRead       Code = "evidence_read_failed"
	ErrCodeEvidenceWrite      Code = "evidence_write_failed"
	ErrCodeNoSignerConfigured Code = "no_signer_configured"
)

// Error is a typed lifecycle error used by adapter layers.
type Error struct {
	Code    Code
	Message string
	Err     error
}

func (e *Error) Error() string {
	if e == nil {
		return ""
	}
	return e.Message
}

func (e *Error) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}

// ErrorCode extracts the lifecycle error code from err.
func ErrorCode(err error) Code {
	var le *Error
	if errors.As(err, &le) {
		return le.Code
	}
	return ""
}

func wrapError(code Code, message string, err error) error {
	return &Error{Code: code, Message: message, Err: err}
}

func requiredSigner(signer evidence.Signer) error {
	if signer == nil {
		return wrapError(ErrCodeNoSignerConfigured, "evidence signer is required", fmt.Errorf("nil signer"))
	}
	return nil
}
