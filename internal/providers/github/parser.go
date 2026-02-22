package github

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	ce "evidra/internal/cloudevents"
	"evidra/internal/providers/shared"

	githubv53 "github.com/google/go-github/v53/github"
)

type Parser struct{}

func (Parser) Parse(eventType, deliveryID string, body []byte) ([]ce.StoredEvent, error) {
	eventType = shared.NormalizeEventType(eventType)
	deliveryID = shared.FallbackDelivery(deliveryID, body)

	switch eventType {
	case "push":
		var p githubv53.PushEvent
		if err := json.Unmarshal(body, &p); err != nil {
			return nil, err
		}
		repo := repoFullNameFromPush(p.GetRepo())
		pusher := ""
		if p.Pusher != nil {
			pusher = p.Pusher.GetName()
		}
		timestamp := headCommitTimestamp(p.GetHeadCommit())
		ticketKeys := shared.ExtractIssueKeys(pushTicketSources(p)...)
		extensions := map[string]interface{}{
			"cluster":    "unknown",
			"namespace":  "unknown",
			"initiator":  shared.NonEmpty(pusher, "unknown"),
			"repo":       repo,
			"commit_sha": p.GetAfter(),
			"ref":        p.GetRef(),
			"provider":   "github",
		}
		if len(ticketKeys) > 0 {
			extensions["ticket_keys"] = ticketKeys
			extensions["ticket_key_primary"] = ticketKeys[0]
		}
		data, _ := json.Marshal(map[string]interface{}{
			"repo":       repo,
			"commit_sha": p.GetAfter(),
			"ref":        p.GetRef(),
		})
		e := ce.StoredEvent{
			ID:         shared.MakeID("github", deliveryID, 0),
			Source:     "github",
			Type:       "push",
			Time:       shared.ParseTimeOrNow(timestamp),
			Subject:    shared.RepoApp(repo),
			Extensions: extensions,
			Data:       json.RawMessage(data),
		}
		return []ce.StoredEvent{e}, nil
	case "pull_request":
		var p githubv53.PullRequestEvent
		if err := json.Unmarshal(body, &p); err != nil {
			return nil, err
		}
		repo := repoFullNameFromRepo(p.GetRepo())
		eType := "pull_request_updated"
		switch strings.ToLower(p.GetAction()) {
		case "opened":
			eType = "pull_request_opened"
		case "closed":
			if p.PullRequest != nil && p.PullRequest.GetMerged() {
				eType = "pull_request_merged"
			} else {
				eType = "pull_request_closed"
			}
		}
		commit := ""
		if p.GetPullRequest() != nil {
			commit = shared.NonEmpty(p.GetPullRequest().GetMergeCommitSHA(), p.GetPullRequest().GetHead().GetSHA())
		}
		updated := ""
		created := ""
		if p.GetPullRequest() != nil {
			if p.GetPullRequest().UpdatedAt != nil && p.GetPullRequest().UpdatedAt.GetTime() != nil {
				updated = p.GetPullRequest().UpdatedAt.GetTime().Format(time.RFC3339)
			}
			if p.GetPullRequest().CreatedAt != nil && p.GetPullRequest().CreatedAt.GetTime() != nil {
				created = p.GetPullRequest().CreatedAt.GetTime().Format(time.RFC3339)
			}
		}
		prID := 0
		if p.GetPullRequest() != nil {
			prID = p.GetPullRequest().GetNumber()
		}
		ticketKeys := shared.ExtractIssueKeys(prTicketSources(&p)...)
		extensions := map[string]interface{}{
			"cluster":    "unknown",
			"namespace":  "unknown",
			"initiator":  shared.NonEmpty(p.GetSender().GetLogin(), "unknown"),
			"repo":       repo,
			"commit_sha": commit,
			"pr_id":      fmt.Sprintf("%d", prID),
			"provider":   "github",
		}
		if len(ticketKeys) > 0 {
			extensions["ticket_keys"] = ticketKeys
			extensions["ticket_key_primary"] = ticketKeys[0]
		}
		data, _ := json.Marshal(map[string]interface{}{
			"repo":       repo,
			"commit_sha": commit,
			"pr_id":      fmt.Sprintf("%d", prID),
		})
		e := ce.StoredEvent{
			ID:         shared.MakeID("github", deliveryID, 0),
			Source:     "github",
			Type:       eType,
			Time:       shared.ParseTimeOrNow(shared.NonEmpty(updated, created)),
			Subject:    shared.RepoApp(repo),
			Extensions: extensions,
			Data:       json.RawMessage(data),
		}
		return []ce.StoredEvent{e}, nil
	default:
		return nil, fmt.Errorf("unsupported github event: %s", eventType)
	}
}

func repoFullNameFromRepo(repo *githubv53.Repository) string {
	if repo == nil {
		return "unknown"
	}
	return shared.NonEmpty(repo.GetFullName(), repo.GetName())
}

func repoFullNameFromPush(repo *githubv53.PushEventRepository) string {
	if repo == nil {
		return "unknown"
	}
	return shared.NonEmpty(repo.GetFullName(), repo.GetName())
}

func headCommitTimestamp(commit *githubv53.HeadCommit) string {
	if commit == nil {
		return ""
	}
	ts := commit.GetTimestamp()
	if ts.GetTime() == nil {
		return ""
	}
	return ts.GetTime().Format(time.RFC3339)
}

func pushTicketSources(event githubv53.PushEvent) []string {
	out := make([]string, 0, len(event.GetCommits())+2)
	out = append(out, event.GetRef())
	if hc := event.GetHeadCommit(); hc != nil {
		out = append(out, hc.GetMessage())
	}
	for _, commit := range event.GetCommits() {
		if commit == nil {
			continue
		}
		out = append(out, commit.GetMessage())
	}
	return out
}

func prTicketSources(event *githubv53.PullRequestEvent) []string {
	if event == nil || event.GetPullRequest() == nil {
		return nil
	}
	pr := event.GetPullRequest()
	out := []string{
		pr.GetTitle(),
		pr.GetBody(),
		pr.GetHead().GetRef(),
		pr.GetBase().GetRef(),
	}
	return out
}
