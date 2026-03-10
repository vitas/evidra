package evidence

import "encoding/json"

// Verdict represents the terminal outcome classification of a prescribed action.
type Verdict string

const (
	// VerdictSuccess indicates the action completed successfully (exit code 0).
	VerdictSuccess Verdict = "success"
	// VerdictFailure indicates the action failed (exit code > 0).
	VerdictFailure Verdict = "failure"
	// VerdictError indicates the action could not be executed (exit code < 0).
	VerdictError Verdict = "error"
	// VerdictDeclined indicates execution was intentionally not started.
	VerdictDeclined Verdict = "declined"
)

// VerdictFromExitCode maps a process exit code to a Verdict.
// Zero means success, negative means error (could not execute), positive means failure.
func VerdictFromExitCode(code int) Verdict {
	switch {
	case code == 0:
		return VerdictSuccess
	case code < 0:
		return VerdictError
	default:
		return VerdictFailure
	}
}

// Valid reports whether v is a supported verdict value.
func (v Verdict) Valid() bool {
	switch v {
	case VerdictSuccess, VerdictFailure, VerdictError, VerdictDeclined:
		return true
	default:
		return false
	}
}

// DecisionContext records why an actor intentionally declined execution.
type DecisionContext struct {
	Trigger string `json:"trigger"`
	Reason  string `json:"reason"`
}

// PrescriptionPayload is the typed payload for EntryTypePrescribe entries.
// It captures the pre-execution risk assessment for a canonical action.
type PrescriptionPayload struct {
	PrescriptionID  string          `json:"prescription_id"`
	CanonicalAction json.RawMessage `json:"canonical_action"`
	RiskLevel       string          `json:"risk_level"`
	// RiskDetails is the canonical risk field used by benchmark validators.
	// Preferred field since v0.3.1.
	RiskDetails []string `json:"risk_details,omitempty"`
	// RiskTags is kept for backward compatibility with older readers.
	// Deprecated: use RiskDetails. Planned removal in v0.5.0.
	RiskTags    []string `json:"risk_tags,omitempty"`
	TTLMs       int64    `json:"ttl_ms"`
	CanonSource string   `json:"canon_source"`
}

// EffectiveRiskDetails returns canonical risk details when present,
// otherwise falls back to legacy risk_tags for backward compatibility.
func (p PrescriptionPayload) EffectiveRiskDetails() []string {
	if len(p.RiskDetails) > 0 {
		return p.RiskDetails
	}
	return p.RiskTags
}

// ExternalRef is an external reference attached to a report entry.
type ExternalRef struct {
	Type string `json:"type"`
	ID   string `json:"id"`
}

// ReportPayload is the typed payload for EntryTypeReport entries.
// It records the post-execution outcome linked back to a prescription.
type ReportPayload struct {
	ReportID        string           `json:"report_id"`
	PrescriptionID  string           `json:"prescription_id"`
	ExitCode        *int             `json:"exit_code,omitempty"`
	Verdict         Verdict          `json:"verdict"`
	DecisionContext *DecisionContext `json:"decision_context,omitempty"`
	ExternalRefs    []ExternalRef    `json:"external_refs,omitempty"`
}

// FindingPayload is the typed payload for EntryTypeFinding entries.
// It captures a single finding from an external inspection tool.
type FindingPayload struct {
	Tool        string `json:"tool"`
	ToolVersion string `json:"tool_version,omitempty"`
	RuleID      string `json:"rule_id"`
	Severity    string `json:"severity"`
	Resource    string `json:"resource"`
	Message     string `json:"message"`
}

// SignalPayload is the typed payload for EntryTypeSignal entries.
// It records a behavioral signal detected across one or more evidence entries.
type SignalPayload struct {
	SignalName string   `json:"signal_name"`
	SubSignal  string   `json:"sub_signal,omitempty"`
	EntryRefs  []string `json:"entry_refs"`
	Details    string   `json:"details,omitempty"`
}

// CanonFailurePayload is the typed payload for EntryTypeCanonFailure entries.
// It records why canonicalization of a raw artifact failed.
type CanonFailurePayload struct {
	ErrorCode    string `json:"error_code"`
	ErrorMessage string `json:"error_message"`
	Adapter      string `json:"adapter"`
	RawDigest    string `json:"raw_digest"`
}

// SessionStartPayload is the typed payload for EntryTypeSessionStart entries.
type SessionStartPayload struct {
	Labels map[string]string `json:"labels,omitempty"`
}

// SessionEndPayload is the typed payload for EntryTypeSessionEnd entries.
type SessionEndPayload struct {
	Status string `json:"status"` // "completed", "aborted", "error"
}

// AnnotationPayload is the typed payload for EntryTypeAnnotation entries.
type AnnotationPayload struct {
	Key     string `json:"key"`
	Value   string `json:"value"`
	Message string `json:"message,omitempty"`
}
