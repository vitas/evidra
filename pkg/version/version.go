package version

import "fmt"

const (
	// SpecVersion is the evidence/signal specification version written into entries and score outputs.
	SpecVersion = "v1.1.0"
	// ScoringVersion is the scoring model version written into entries and score outputs.
	ScoringVersion = "v1.1.0"
	// BaseVersion is the semantic release version before build metadata is injected.
	BaseVersion = "0.4.10"
)

var (
	// Version is the build/runtime version string for Evidra Benchmark binaries.
	Version = BaseVersion
	// Commit describes the revision or commit hash used to build the binary.
	Commit = "dev"
	// Date stores the build timestamp.
	Date = "dev"
)

// String returns the version string.
func String() string {
	return Version
}

// BuildString returns the human-readable build identifier used by CLI version output.
func BuildString(binaryName string) string {
	commit := Commit
	if commit == "" {
		commit = "unknown"
	}
	date := Date
	if date == "" {
		date = "unknown"
	}
	return fmt.Sprintf("%s %s (commit: %s, built: %s)", binaryName, Version, commit, date)
}
