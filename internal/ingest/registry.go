package ingest

import (
	"fmt"
	"strings"

	ce "evidra/internal/cloudevents"
)

type Registry struct {
	providers map[string]WebhookAdapter
}

func NewRegistry() *Registry {
	return &Registry{providers: map[string]WebhookAdapter{}}
}

func (r *Registry) Register(adapter WebhookAdapter) {
	if r == nil || adapter == nil {
		return
	}
	if r.providers == nil {
		r.providers = map[string]WebhookAdapter{}
	}
	r.providers[strings.ToLower(strings.TrimSpace(adapter.Provider()))] = adapter
}

func (r *Registry) Adapter(provider string) (WebhookAdapter, error) {
	if r == nil {
		return nil, fmt.Errorf("nil registry")
	}
	p, ok := r.providers[strings.ToLower(strings.TrimSpace(provider))]
	if !ok {
		return nil, fmt.Errorf("unsupported provider: %s", provider)
	}
	return p, nil
}

func (r *Registry) Parse(provider, eventType, eventID string, body []byte) ([]ce.StoredEvent, error) {
	p, err := r.Adapter(provider)
	if err != nil {
		return nil, err
	}
	return p.Parse(eventType, eventID, body)
}

func (r *Registry) ParseFromHeaders(provider string, headers HeaderReader, body []byte) ([]ce.StoredEvent, error) {
	p, err := r.Adapter(provider)
	if err != nil {
		return nil, err
	}
	return p.Parse(headers.Get(p.EventTypeHeader()), headers.Get(p.EventIDHeader()), body)
}

func (r *Registry) Authorize(provider string, headers HeaderReader, body []byte) error {
	p, err := r.Adapter(provider)
	if err != nil {
		return err
	}
	return p.Authorize(headers, body)
}

func (r *Registry) Headers(provider string) (string, string, error) {
	p, err := r.Adapter(provider)
	if err != nil {
		return "", "", err
	}
	return p.EventTypeHeader(), p.EventIDHeader(), nil
}

func (r *Registry) MustHaveProviders() error {
	if r == nil {
		return fmt.Errorf("nil registry")
	}
	if len(r.providers) == 0 {
		return fmt.Errorf("empty provider registry")
	}
	return nil
}
