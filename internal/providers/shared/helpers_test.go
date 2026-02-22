package shared

import "testing"

func TestExtractIssueKeys(t *testing.T) {
	keys := ExtractIssueKeys(
		"feat(payments): PROJ-123 add flow",
		"refs/heads/bugfix/proj-123-and-OPS-9",
		"no key here",
		"OPS-9 repeated",
	)
	if len(keys) != 2 {
		t.Fatalf("expected 2 keys, got %d: %v", len(keys), keys)
	}
	if keys[0] != "PROJ-123" || keys[1] != "OPS-9" {
		t.Fatalf("unexpected keys order/content: %v", keys)
	}
}
