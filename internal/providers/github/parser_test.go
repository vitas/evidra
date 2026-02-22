package github

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestPullRequestTransitions(t *testing.T) {
	tests := []struct {
		fixture      string
		eventID      string
		expectedType string
		expectedPRID string
		expectedSHA  string
	}{
		{"github_pr_opened_webhook_payload.json", "gh-pr-opened", "pull_request_opened", "842", "abc123"},
		{"github_pr_merged_webhook_payload.json", "gh-pr-merged", "pull_request_merged", "842", "mergeabc123"},
		{"github_pr_closed_webhook_payload.json", "gh-pr-closed", "pull_request_closed", "842", "abc123"},
	}

	p := Parser{}
	for _, tt := range tests {
		t.Run(tt.fixture, func(t *testing.T) {
			body := readFixture(t, tt.fixture)
			events, err := p.Parse("pull_request", tt.eventID, body)
			if err != nil {
				t.Fatalf("parse: %v", err)
			}
			if len(events) != 1 {
				t.Fatalf("expected 1 event, got %d", len(events))
			}
			e := events[0]
			if e.Type != tt.expectedType {
				t.Fatalf("type got %s want %s", e.Type, tt.expectedType)
			}
			if got := extensionString(e.Extensions, "pr_id"); got != tt.expectedPRID {
				t.Fatalf("pr_id got %s want %s", got, tt.expectedPRID)
			}
			if got := extensionString(e.Extensions, "commit_sha"); got != tt.expectedSHA {
				t.Fatalf("commit_sha got %s want %s", got, tt.expectedSHA)
			}
		})
	}
}

func TestPullRequestExtractsTicketKey(t *testing.T) {
	body := []byte(`{
		"action":"opened",
		"repository":{"full_name":"acme/payments"},
		"sender":{"login":"dev1"},
		"pull_request":{
			"number":101,
			"title":"PAY-77 implement payment retry",
			"head":{"sha":"abc123","ref":"feature/PAY-77-retry"},
			"base":{"ref":"main"},
			"created_at":"2026-02-16T10:00:00Z",
			"updated_at":"2026-02-16T10:01:00Z"
		}
	}`)

	events, err := (Parser{}).Parse("pull_request", "gh-ticket", body)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
	if got := extensionString(events[0].Extensions, "ticket_key_primary"); got != "PAY-77" {
		t.Fatalf("ticket_key_primary got %q", got)
	}
}

func readFixture(t *testing.T, name string) []byte {
	t.Helper()
	path := filepath.Join("..", "..", "..", "testdata", "events", name)
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var v interface{}
	if err := json.Unmarshal(b, &v); err != nil {
		t.Fatalf("invalid json fixture %s: %v", name, err)
	}
	return b
}

func extensionString(extensions map[string]interface{}, key string) string {
	v, _ := extensions[key]
	s, _ := v.(string)
	return s
}
