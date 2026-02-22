package gitlab

import (
	"encoding/json"
	"fmt"
	"strings"

	ce "evidra/internal/cloudevents"
	"evidra/internal/providers/shared"
)

type Parser struct{}

func (Parser) Parse(eventType, eventID string, body []byte) ([]ce.StoredEvent, error) {
	eventType = shared.NormalizeEventType(eventType)
	eventID = shared.FallbackDelivery(eventID, body)

	switch eventType {
	case "push hook":
		var p pushPayload
		if err := json.Unmarshal(body, &p); err != nil {
			return nil, err
		}
		repo := shared.NonEmpty(p.Project.PathWithNamespace, p.Project.Name)
		ticketKeys := shared.ExtractIssueKeys(gitlabPushTicketSources(p)...)
		extensions := map[string]interface{}{
			"cluster":    "unknown",
			"namespace":  "unknown",
			"initiator":  shared.NonEmpty(p.UserUsername, "unknown"),
			"repo":       repo,
			"commit_sha": p.CheckoutSHA,
			"ref":        p.Ref,
			"provider":   "gitlab",
		}
		if len(ticketKeys) > 0 {
			extensions["ticket_keys"] = ticketKeys
			extensions["ticket_key_primary"] = ticketKeys[0]
		}
		data, _ := json.Marshal(map[string]interface{}{
			"repo":       repo,
			"commit_sha": p.CheckoutSHA,
			"ref":        p.Ref,
		})
		e := ce.StoredEvent{
			ID:         shared.MakeID("gitlab", eventID, 0),
			Source:     "gitlab",
			Type:       "push",
			Time:       shared.ParseTimeOrNow(lastCommitTime(p.Commits, p.EventCreatedAt)),
			Subject:    shared.RepoApp(repo),
			Extensions: extensions,
			Data:       json.RawMessage(data),
		}
		return []ce.StoredEvent{e}, nil
	case "merge request hook":
		var p mergeRequestPayload
		if err := json.Unmarshal(body, &p); err != nil {
			return nil, err
		}
		repo := shared.NonEmpty(p.Project.PathWithNamespace, p.Project.Name)
		action := strings.ToLower(p.ObjectAttributes.Action)
		eType := "merge_request_updated"
		switch action {
		case "open", "opened":
			eType = "merge_request_opened"
		case "merge", "merged":
			eType = "merge_request_merged"
		case "close", "closed":
			eType = "merge_request_closed"
		}
		commit := shared.NonEmpty(p.ObjectAttributes.MergeCommitSHA, p.ObjectAttributes.LastCommit.ID)
		ticketKeys := shared.ExtractIssueKeys(
			p.ObjectAttributes.Title,
			p.ObjectAttributes.Description,
			p.ObjectAttributes.SourceBranch,
			p.ObjectAttributes.TargetBranch,
			p.ObjectAttributes.LastCommit.Message,
		)
		extensions := map[string]interface{}{
			"cluster":    "unknown",
			"namespace":  "unknown",
			"initiator":  shared.NonEmpty(p.User.Username, "unknown"),
			"repo":       repo,
			"commit_sha": commit,
			"pr_id":      fmt.Sprintf("%d", p.ObjectAttributes.IID),
			"provider":   "gitlab",
		}
		if len(ticketKeys) > 0 {
			extensions["ticket_keys"] = ticketKeys
			extensions["ticket_key_primary"] = ticketKeys[0]
		}
		data, _ := json.Marshal(map[string]interface{}{
			"repo":       repo,
			"commit_sha": commit,
			"pr_id":      fmt.Sprintf("%d", p.ObjectAttributes.IID),
		})
		e := ce.StoredEvent{
			ID:         shared.MakeID("gitlab", eventID, 0),
			Source:     "gitlab",
			Type:       eType,
			Time:       shared.ParseTimeOrNow(shared.NonEmpty(p.ObjectAttributes.UpdatedAt, p.ObjectAttributes.CreatedAt)),
			Subject:    shared.RepoApp(repo),
			Extensions: extensions,
			Data:       json.RawMessage(data),
		}
		return []ce.StoredEvent{e}, nil
	default:
		return nil, fmt.Errorf("unsupported gitlab event: %s", eventType)
	}
}

func lastCommitTime(commits []commit, fallback string) string {
	if len(commits) == 0 {
		return fallback
	}
	for i := len(commits) - 1; i >= 0; i-- {
		if strings.TrimSpace(commits[i].Timestamp) != "" {
			return commits[i].Timestamp
		}
	}
	return fallback
}

type pushPayload struct {
	Ref            string `json:"ref"`
	CheckoutSHA    string `json:"checkout_sha"`
	EventCreatedAt string `json:"event_created_at"`
	UserUsername   string `json:"user_username"`
	Project        struct {
		Name              string `json:"name"`
		PathWithNamespace string `json:"path_with_namespace"`
	} `json:"project"`
	Commits []commit `json:"commits"`
}

type commit struct {
	Timestamp string `json:"timestamp"`
	Message   string `json:"message"`
}

type mergeRequestPayload struct {
	Project struct {
		Name              string `json:"name"`
		PathWithNamespace string `json:"path_with_namespace"`
	} `json:"project"`
	User struct {
		Username string `json:"username"`
	} `json:"user"`
	ObjectAttributes struct {
		Action         string `json:"action"`
		IID            int    `json:"iid"`
		MergeCommitSHA string `json:"merge_commit_sha"`
		CreatedAt      string `json:"created_at"`
		UpdatedAt      string `json:"updated_at"`
		Title          string `json:"title"`
		Description    string `json:"description"`
		SourceBranch   string `json:"source_branch"`
		TargetBranch   string `json:"target_branch"`
		LastCommit     struct {
			ID      string `json:"id"`
			Message string `json:"message"`
		} `json:"last_commit"`
	} `json:"object_attributes"`
}

func gitlabPushTicketSources(payload pushPayload) []string {
	out := make([]string, 0, len(payload.Commits)+1)
	out = append(out, payload.Ref)
	for _, commit := range payload.Commits {
		out = append(out, commit.Message)
	}
	return out
}
