package gitlab

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestMergeRequestTransitions(t *testing.T) {
	tests := []struct {
		fixture      string
		eventID      string
		expectedType string
		expectedPRID string
		expectedSHA  string
	}{
		{"gitlab_mr_opened_webhook_payload.json", "gl-mr-opened", "merge_request_opened", "51", "def456"},
		{"gitlab_mr_merged_webhook_payload.json", "gl-mr-merged", "merge_request_merged", "51", "mrmerge789"},
		{"gitlab_mr_closed_webhook_payload.json", "gl-mr-closed", "merge_request_closed", "51", "def456"},
	}

	p := Parser{}
	for _, tt := range tests {
		t.Run(tt.fixture, func(t *testing.T) {
			body := readFixture(t, tt.fixture)
			events, err := p.Parse("Merge Request Hook", tt.eventID, body)
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

func TestMergeRequestExtractsTicketKey(t *testing.T) {
	body := []byte(`{
		"project":{"name":"payments","path_with_namespace":"acme/payments"},
		"user":{"username":"dev1"},
		"object_attributes":{
			"action":"open",
			"iid":51,
			"title":"OPS-42 improve rollout",
			"source_branch":"feature/OPS-42-rollout",
			"last_commit":{"id":"def456","message":"OPS-42 implement"}
		}
	}`)
	events, err := (Parser{}).Parse("Merge Request Hook", "gl-ticket", body)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
	if got := extensionString(events[0].Extensions, "ticket_key_primary"); got != "OPS-42" {
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
