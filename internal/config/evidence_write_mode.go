package config

import (
	"fmt"
	"os"
	"strings"
)

const evidenceWriteModeEnv = "EVIDRA_EVIDENCE_WRITE_MODE"

// EvidenceWriteMode controls how write failures to the evidence store are handled.
type EvidenceWriteMode string

const (
	EvidenceWriteModeStrict     EvidenceWriteMode = "strict"
	EvidenceWriteModeBestEffort EvidenceWriteMode = "best_effort"
)

// ResolveEvidenceWriteMode returns write mode from explicit flag, then env, then default.
// Default is strict.
func ResolveEvidenceWriteMode(explicit string) (EvidenceWriteMode, error) {
	raw := strings.TrimSpace(explicit)
	if raw == "" {
		raw = strings.TrimSpace(os.Getenv(evidenceWriteModeEnv))
	}
	if raw == "" {
		return EvidenceWriteModeStrict, nil
	}
	switch strings.ToLower(raw) {
	case string(EvidenceWriteModeStrict):
		return EvidenceWriteModeStrict, nil
	case string(EvidenceWriteModeBestEffort):
		return EvidenceWriteModeBestEffort, nil
	default:
		return "", fmt.Errorf("invalid evidence write mode %q (expected strict|best_effort)", raw)
	}
}
