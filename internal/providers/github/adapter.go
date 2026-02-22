package github

import (
	"fmt"
	"strings"

	ce "evidra/internal/cloudevents"
	"evidra/internal/ingest"
	"evidra/internal/providers/shared"
)

const (
	defaultSignatureHeader = "X-Hub-Signature-256"
	eventTypeHeader        = "X-GitHub-Event"
	eventIDHeader          = "X-GitHub-Delivery"
)

type Adapter struct {
	Parser          ingest.Parser
	SignatureHeader string
	Secret          string
}

func NewAdapter(secret string) Adapter {
	return Adapter{
		Parser:          Parser{},
		SignatureHeader: defaultSignatureHeader,
		Secret:          secret,
	}
}

func (a Adapter) Provider() string { return "github" }
func (a Adapter) EventTypeHeader() string {
	return eventTypeHeader
}
func (a Adapter) EventIDHeader() string {
	return eventIDHeader
}

func (a Adapter) Authorize(headers ingest.HeaderReader, body []byte) error {
	secret := strings.TrimSpace(a.Secret)
	if secret == "" {
		return nil
	}
	header := strings.TrimSpace(headers.Get(nonEmpty(a.SignatureHeader, defaultSignatureHeader)))
	if !shared.ValidSHA256Signature(secret, body, header) {
		return fmt.Errorf("invalid github webhook signature")
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
