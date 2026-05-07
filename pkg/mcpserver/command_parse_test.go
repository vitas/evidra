package mcpserver

import "testing"

func TestParseCommand_PreservesQuotedPatchPayload(t *testing.T) {
	t.Parallel()

	args, err := parseCommand(`kubectl patch deployment/web -n demo --type=merge -p '{"spec":{"replicas":2}}'`)
	if err != nil {
		t.Fatalf("parseCommand: %v", err)
	}
	if got := args[len(args)-1]; got != `{"spec":{"replicas":2}}` {
		t.Fatalf("payload = %q, want %q", got, `{"spec":{"replicas":2}}`)
	}
}

func TestParseCommand_RejectsUnterminatedQuotes(t *testing.T) {
	t.Parallel()

	if _, err := parseCommand(`kubectl patch deployment/web -p '{"spec":{"replicas":2}}`); err == nil {
		t.Fatal("expected parseCommand to reject unterminated quotes")
	}
}
