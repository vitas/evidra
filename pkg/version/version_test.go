package version

import "testing"

func TestBuildString_UsesInjectedBuildMetadata(t *testing.T) {
	originalVersion, originalCommit, originalDate := Version, Commit, Date
	t.Cleanup(func() {
		Version = originalVersion
		Commit = originalCommit
		Date = originalDate
	})

	Version = "v0.4.9-2-g3c5b1fe"
	Commit = "3c5b1fe"
	Date = "2026-03-15T12:00:00Z"

	got := BuildString("evidra")
	want := "evidra v0.4.9-2-g3c5b1fe (commit: 3c5b1fe, built: 2026-03-15T12:00:00Z)"
	if got != want {
		t.Fatalf("BuildString() = %q, want %q", got, want)
	}
}

func TestBuildString_UsesDefaultsWhenBuildMetadataMissing(t *testing.T) {
	originalVersion, originalCommit, originalDate := Version, Commit, Date
	t.Cleanup(func() {
		Version = originalVersion
		Commit = originalCommit
		Date = originalDate
	})

	Version = BaseVersion
	Commit = ""
	Date = ""

	got := BuildString("evidra")
	want := "evidra " + BaseVersion + " (commit: unknown, built: unknown)"
	if got != want {
		t.Fatalf("BuildString() = %q, want %q", got, want)
	}
}
