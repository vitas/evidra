package bitbucket

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestPullRequestTransitions(t *testing.T) {
	tests := []struct {
		fixture      string
		eventKey     string
		eventID      string
		expectedType string
		expectedPRID string
		expectedSHA  string
	}{
		{"bitbucket_pr_created_webhook_payload.json", "pullrequest:created", "bb-pr-created", "pull_request_opened", "73", "bbprabc123"},
		{"bitbucket_pr_fulfilled_webhook_payload.json", "pullrequest:fulfilled", "bb-pr-fulfilled", "pull_request_merged", "73", "bbprmerge123"},
		{"bitbucket_pr_rejected_webhook_payload.json", "pullrequest:rejected", "bb-pr-rejected", "pull_request_closed", "73", "bbprabc123"},
	}

	p := Parser{}
	for _, tt := range tests {
		t.Run(tt.fixture, func(t *testing.T) {
			body := readFixture(t, tt.fixture)
			events, err := p.Parse(tt.eventKey, tt.eventID, body)
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
		"repository":{"full_name":"acme/payments"},
		"actor":{"display_name":"dev1"},
		"pullrequest":{
			"id":73,
			"title":"PLAT-19 update policy",
			"source":{
				"branch":{"name":"feature/PLAT-19-policy"},
				"commit":{"hash":"bbprabc123"}
			}
		}
	}`)
	events, err := (Parser{}).Parse("pullrequest:created", "bb-ticket", body)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
	if got := extensionString(events[0].Extensions, "ticket_key_primary"); got != "PLAT-19" {
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
