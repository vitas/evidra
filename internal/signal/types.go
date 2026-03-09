package signal

import "time"

// DefaultTTL is the default time-to-live for prescription matching.
// Prescriptions without a report within this window are flagged.
const DefaultTTL = 10 * time.Minute

// Entry represents a single evidence record for signal analysis.
type Entry struct {
	EventID        string
	Timestamp      time.Time
	Tool           string
	Operation      string
	ActorID        string
	Environment    string
	IsPrescription bool
	IsReport       bool
	PrescriptionID string // for reports: which prescription this reports on
	ArtifactDigest string
	IntentDigest   string
	ShapeHash      string
	ResourceCount  int
	OperationClass string
	ScopeClass     string
	ExitCode       *int
	RiskTags       []string
}

// SignalResult holds the result of a single signal detection.
type SignalResult struct {
	Name     string
	Count    int
	EventIDs []string
}

// SignalEvent is a detailed signal occurrence with sub-signal classification.
type SignalEvent struct {
	Signal    string    `json:"signal"`
	SubSignal string    `json:"sub_signal"`
	Timestamp time.Time `json:"ts"`
	EntryRef  string    `json:"entry_ref"`
	Details   string    `json:"details"`
}

// AllSignals runs all signal detectors and returns their results.
// TTL controls the window for unreported prescription detection. Use
// DefaultTTL if no override is needed.
func AllSignals(entries []Entry, ttl time.Duration) []SignalResult {
	return []SignalResult{
		DetectProtocolViolations(entries, ttl),
		DetectArtifactDrift(entries),
		DetectRetryLoops(entries),
		DetectBlastRadius(entries),
		DetectNewScope(entries),
		DetectRepairLoop(entries),
		DetectThrashing(entries),
		DetectRiskEscalation(entries),
	}
}
