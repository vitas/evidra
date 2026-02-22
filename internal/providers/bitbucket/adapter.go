package bitbucket

import (
	"fmt"
	"strings"

	ce "evidra/internal/cloudevents"
	"evidra/internal/ingest"
	"evidra/internal/providers/shared"
)

const (
	eventTypeHeader = "X-Event-Key"
	eventIDHeader   = "X-Request-UUID"
)

type HMACPolicy struct {
	Header string
	Secret string
}

type Adapter struct {
	Parser   ingest.Parser
	Policies []HMACPolicy
}

func NewAdapter(secret string) Adapter {
	return Adapter{
		Parser: Parser{},
		Policies: []HMACPolicy{
			{Header: "X-Hub-Signature", Secret: secret},
			{Header: "X-Hub-Signature-256", Secret: secret},
		},
	}
}

func (a Adapter) Provider() string { return "bitbucket" }
func (a Adapter) EventTypeHeader() string {
	return eventTypeHeader
}
func (a Adapter) EventIDHeader() string {
	return eventIDHeader
}

func (a Adapter) Authorize(headers ingest.HeaderReader, body []byte) error {
	policies := a.Policies
	if len(policies) == 0 {
		return nil
	}
	hasSecret := false
	for _, policy := range policies {
		secret := strings.TrimSpace(policy.Secret)
		if secret == "" {
			continue
		}
		hasSecret = true
		header := strings.TrimSpace(headers.Get(strings.TrimSpace(policy.Header)))
		if header == "" {
			continue
		}
		if shared.ValidSHA256Signature(secret, body, header) {
			return nil
		}
		return fmt.Errorf("invalid bitbucket webhook signature")
	}
	if hasSecret {
		return fmt.Errorf("invalid bitbucket webhook signature")
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
