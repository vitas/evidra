package gitlab

import (
	"fmt"
	"strings"

	ce "evidra/internal/cloudevents"
	"evidra/internal/ingest"
)

const (
	defaultTokenHeader = "X-Gitlab-Token"
	eventTypeHeader    = "X-Gitlab-Event"
	eventIDHeader      = "X-Gitlab-Event-UUID"
)

type Adapter struct {
	Parser      ingest.Parser
	TokenHeader string
	Token       string
}

func NewAdapter(token string) Adapter {
	return Adapter{
		Parser:      Parser{},
		TokenHeader: defaultTokenHeader,
		Token:       token,
	}
}

func (a Adapter) Provider() string { return "gitlab" }
func (a Adapter) EventTypeHeader() string {
	return eventTypeHeader
}
func (a Adapter) EventIDHeader() string {
	return eventIDHeader
}

func (a Adapter) Authorize(headers ingest.HeaderReader, _ []byte) error {
	token := strings.TrimSpace(a.Token)
	if token == "" {
		return nil
	}
	if strings.TrimSpace(headers.Get(nonEmpty(a.TokenHeader, defaultTokenHeader))) != token {
		return fmt.Errorf("invalid gitlab webhook token")
	}
	return nil
}

func (a Adapter) Parse(eventType, eventID string, body []byte) ([]ce.StoredEvent, error) {
	p := a.Parser
	if p == nil {
		p = Parser{}
	}
	return p.Parse(eventType, eventID, body)
}

func nonEmpty(v, fallback string) string {
	if strings.TrimSpace(v) == "" {
		return fallback
	}
	return v
}
