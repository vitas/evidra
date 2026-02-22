package model

import (
	"time"

	cloudevents "github.com/cloudevents/sdk-go/v2"
	"github.com/cloudevents/sdk-go/v2/event"
)

type Subject struct {
	App         string `json:"app"`
	Environment string `json:"environment"`
	Cluster     string `json:"cluster"`
	Resource    string `json:"resource,omitempty"`
}

type Actor struct {
	Kind string `json:"kind"`
	ID   string `json:"id"`
}

type RawRef struct {
	Kind string `json:"kind"`
	Ref  string `json:"ref"`
}

type Event struct {
	ID                 string                 `json:"id"`
	Source             string                 `json:"source"`
	Type               string                 `json:"type"`
	Timestamp          time.Time              `json:"timestamp"`
	Subject            Subject                `json:"subject"`
	Actor              Actor                  `json:"actor"`
	Metadata           map[string]interface{} `json:"metadata"`
	RawRef             RawRef                 `json:"raw_ref"`
	EventSchemaVersion int                    `json:"event_schema_version"`
}

func (e *Event) ToCloudEvent() (event.Event, error) {
	ce := cloudevents.NewEvent()
	ce.SetID(e.ID)
	ce.SetSource(e.Source)
	ce.SetType(e.Type)
	ce.SetTime(e.Timestamp)

	// Extension attributes for Evidra specific routing
	ce.SetExtension("subjectapp", e.Subject.App)
	ce.SetExtension("subjectenv", e.Subject.Environment)
	ce.SetExtension("subjectcluster", e.Subject.Cluster)

	if err := ce.SetData(cloudevents.ApplicationJSON, e); err != nil {
		return ce, err
	}
	return ce, nil
}

type ExportJob struct {
	ID          string                 `json:"id"`
	Status      string                 `json:"status"`
	Format      string                 `json:"format"`
	Filter      map[string]interface{} `json:"filter"`
	ArtifactURI string                 `json:"artifact_uri,omitempty"`
	Error       string                 `json:"error,omitempty"`
	CreatedAt   time.Time              `json:"created_at"`
	CompletedAt *time.Time             `json:"completed_at,omitempty"`
}
