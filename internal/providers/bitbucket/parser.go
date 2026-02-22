package bitbucket

import (
	"encoding/json"
	"fmt"

	ce "evidra/internal/cloudevents"
	"evidra/internal/providers/shared"
)

type Parser struct{}

func (Parser) Parse(eventType, eventID string, body []byte) ([]ce.StoredEvent, error) {
	eventType = shared.NormalizeEventType(eventType)
	eventID = shared.FallbackDelivery(eventID, body)

	switch eventType {
	case "repo:push":
		var p pushPayload
		if err := json.Unmarshal(body, &p); err != nil {
			return nil, err
		}
		repo := shared.NonEmpty(p.Repository.FullName, p.Repository.Name)
		sha := ""
		ref := ""
		tsRaw := ""
		if len(p.Push.Changes) > 0 {
			sha = p.Push.Changes[0].New.Target.Hash
			ref = p.Push.Changes[0].New.Name
			tsRaw = p.Push.Changes[0].New.Target.Date
		}
		ticketKeys := shared.ExtractIssueKeys(bitbucketPushTicketSources(p)...)
		extensions := map[string]interface{}{
			"cluster":    "unknown",
			"namespace":  "unknown",
			"initiator":  shared.NonEmpty(p.Actor.DisplayName, "unknown"),
			"repo":       repo,
			"commit_sha": sha,
			"ref":        ref,
			"provider":   "bitbucket",
		}
		if len(ticketKeys) > 0 {
			extensions["ticket_keys"] = ticketKeys
			extensions["ticket_key_primary"] = ticketKeys[0]
		}
		data, _ := json.Marshal(map[string]interface{}{
			"repo":       repo,
			"commit_sha": sha,
			"ref":        ref,
		})
		e := ce.StoredEvent{
			ID:         shared.MakeID("bitbucket", eventID, 0),
			Source:     "bitbucket",
			Type:       "push",
			Time:       shared.ParseTimeOrNow(tsRaw),
			Subject:    shared.RepoApp(repo),
			Extensions: extensions,
			Data:       json.RawMessage(data),
		}
		return []ce.StoredEvent{e}, nil
	case "pullrequest:created", "pullrequest:fulfilled", "pullrequest:rejected", "pullrequest:updated":
		var p pullRequestPayload
		if err := json.Unmarshal(body, &p); err != nil {
			return nil, err
		}
		repo := shared.NonEmpty(p.Repository.FullName, p.Repository.Name)
		if repo == "" {
			repo = shared.NonEmpty(p.PullRequest.Source.Repository.FullName, p.PullRequest.Source.Repository.Name)
		}
		eType := "pull_request_updated"
		switch eventType {
		case "pullrequest:created":
			eType = "pull_request_opened"
		case "pullrequest:fulfilled":
			eType = "pull_request_merged"
		case "pullrequest:rejected":
			eType = "pull_request_closed"
		}
		sha := p.PullRequest.Source.Commit.Hash
		ticketKeys := shared.ExtractIssueKeys(
			p.PullRequest.Title,
			p.PullRequest.Description,
			p.PullRequest.Source.Branch.Name,
		)
		extensions := map[string]interface{}{
			"cluster":    "unknown",
			"namespace":  "unknown",
			"initiator":  shared.NonEmpty(p.Actor.DisplayName, "unknown"),
			"repo":       repo,
			"commit_sha": sha,
			"pr_id":      fmt.Sprintf("%d", p.PullRequest.ID),
			"provider":   "bitbucket",
		}
		if len(ticketKeys) > 0 {
			extensions["ticket_keys"] = ticketKeys
			extensions["ticket_key_primary"] = ticketKeys[0]
		}
		data, _ := json.Marshal(map[string]interface{}{
			"repo":       repo,
			"commit_sha": sha,
			"pr_id":      fmt.Sprintf("%d", p.PullRequest.ID),
		})
		e := ce.StoredEvent{
			ID:         shared.MakeID("bitbucket", eventID, 0),
			Source:     "bitbucket",
			Type:       eType,
			Time:       shared.ParseTimeOrNow(shared.NonEmpty(p.PullRequest.UpdatedOn, p.PullRequest.CreatedOn)),
			Subject:    shared.RepoApp(repo),
			Extensions: extensions,
			Data:       json.RawMessage(data),
		}
		return []ce.StoredEvent{e}, nil
	default:
		return nil, fmt.Errorf("unsupported bitbucket event: %s", eventType)
	}
}

type pushPayload struct {
	Repository struct {
		Name     string `json:"name"`
		FullName string `json:"full_name"`
	} `json:"repository"`
	Actor struct {
		DisplayName string `json:"display_name"`
	} `json:"actor"`
	Push struct {
		Changes []struct {
			New struct {
				Name   string `json:"name"`
				Target struct {
					Hash    string `json:"hash"`
					Date    string `json:"date"`
					Message string `json:"message"`
				} `json:"target"`
			} `json:"new"`
		} `json:"changes"`
	} `json:"push"`
}

type pullRequestPayload struct {
	Repository struct {
		Name     string `json:"name"`
		FullName string `json:"full_name"`
	} `json:"repository"`
	Actor struct {
		DisplayName string `json:"display_name"`
	} `json:"actor"`
	PullRequest struct {
		ID          int    `json:"id"`
		CreatedOn   string `json:"created_on"`
		UpdatedOn   string `json:"updated_on"`
		Title       string `json:"title"`
		Description string `json:"description"`
		Source      struct {
			Branch struct {
				Name string `json:"name"`
			} `json:"branch"`
			Commit struct {
				Hash string `json:"hash"`
			} `json:"commit"`
			Repository struct {
				Name     string `json:"name"`
				FullName string `json:"full_name"`
			} `json:"repository"`
		} `json:"source"`
	} `json:"pullrequest"`
}

func bitbucketPushTicketSources(payload pushPayload) []string {
	out := make([]string, 0, len(payload.Push.Changes)+1)
	for _, change := range payload.Push.Changes {
		out = append(out, change.New.Name, change.New.Target.Message)
	}
	return out
}
