package signal

import "time"

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
	ExitCode       *int
	RiskTags       []string
}

// SignalResult holds the result of a single signal detection.
type SignalResult struct {
	Name     string
	Count    int
	EventIDs []string
}

// AllSignals runs all five signal detectors and returns their results.
func AllSignals(entries []Entry) []SignalResult {
	return []SignalResult{
		DetectProtocolViolations(entries),
		DetectArtifactDrift(entries),
		DetectRetryLoops(entries),
		DetectBlastRadius(entries),
		DetectNewScope(entries),
	}
}
