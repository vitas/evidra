package version

const (
	// SpecVersion is the evidence/signal specification version written into entries and score outputs.
	SpecVersion = "v1.1.0"
	// ScoringVersion is the scoring model version written into entries and score outputs.
	ScoringVersion = "v1.1.0"
)

var (
	// Version is the build/runtime version string for Evidra Benchmark binaries.
	Version = "0.4.7"
	// Commit describes the revision or commit hash used to build the binary.
	Commit = "dev"
	// Date stores the build timestamp.
	Date = "dev"
)

// String returns the version string.
func String() string {
	return Version
}
