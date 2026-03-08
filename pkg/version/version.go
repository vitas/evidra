package version

// SpecVersion is the evidence specification version used by all entries.
const SpecVersion = "0.3.1"

var (
	// Version is the build/runtime version string for Evidra Benchmark binaries.
	Version = "0.4.1"
	// Commit describes the revision or commit hash used to build the binary.
	Commit = "dev"
	// Date stores the build timestamp.
	Date = "dev"
)

// String returns the version string.
func String() string {
	return Version
}
