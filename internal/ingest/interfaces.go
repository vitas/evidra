package ingest

import ce "evidra/internal/cloudevents"

type Parser interface {
	Parse(eventType, eventID string, body []byte) ([]ce.StoredEvent, error)
}

type HeaderReader interface {
	Get(key string) string
}

type WebhookAdapter interface {
	Provider() string
	EventTypeHeader() string
	EventIDHeader() string
	Authorize(headers HeaderReader, body []byte) error
	Parse(eventType, eventID string, body []byte) ([]ce.StoredEvent, error)
}
