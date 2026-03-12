package analytics

import (
	"samebits.com/evidra/internal/score"
)

var stablePublicSignalNames = []string{
	"protocol_violation",
	"artifact_drift",
	"retry_loop",
	"blast_radius",
	"new_scope",
	"repair_loop",
	"thrashing",
	"risk_escalation",
}

// PublicSignalNames returns the stable public signal order used by analytics views and API responses.
func PublicSignalNames(profile score.Profile) []string {
	_ = profile

	names := make([]string, len(stablePublicSignalNames))
	copy(names, stablePublicSignalNames)
	return names
}
