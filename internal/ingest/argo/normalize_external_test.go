package argo

import (
	"testing"
	"time"

	ce "evidra/internal/cloudevents"
)

func TestNormalizeSourceEventExternalAnnotations(t *testing.T) {
	se := SourceEvent{
		ID:        "app-1:hist:42",
		App:       "payments-api",
		Cluster:   "eu-1",
		Namespace: "prod-eu",
		Revision:  "abc123",
		Occurred:  time.Date(2026, 2, 16, 12, 0, 0, 0, time.UTC),
		Actor:     "argocd",
		EventType: "argo.sync.finished",
		Payload: map[string]interface{}{
			"annotations": map[string]interface{}{
				"evidra.io/change-id":      "CHG123456",
				"evidra.io/ticket":         "JIRA-42",
				"evidra.io/approvals-ref":  "https://itsm.local/change/CHG123456",
				"evidra.io/approvals-json": `[{"source":"itsm","identity":"cab","timestamp":"2026-02-16T11:50:00Z","reference":"APR-7","summary":"approved"}]`,
			},
		},
	}

	ev, err := NormalizeSourceEvent(se, "prod-eu")
	if err != nil {
		t.Fatalf("normalize failed: %v", err)
	}
	if got := ce.ExtensionString(ev.Extensions, "external_change_id"); got != "CHG123456" {
		t.Fatalf("expected external_change_id CHG123456, got %#v", got)
	}
	if got := ce.ExtensionString(ev.Extensions, "ticket_id"); got != "JIRA-42" {
		t.Fatalf("expected ticket_id JIRA-42, got %#v", got)
	}
	if got := ce.ExtensionString(ev.Extensions, "approval_reference"); got != "https://itsm.local/change/CHG123456" {
		t.Fatalf("expected approval_reference set, got %#v", got)
	}
	// approvals should be in data["approvals"]
	if len(ev.Data) == 0 {
		t.Fatalf("expected data payload")
	}
}

func TestNormalizeSourceEventPreservesHistoryRecordedType(t *testing.T) {
	se := SourceEvent{
		ID:        "app-1:hist:43",
		App:       "payments-api",
		Cluster:   "eu-1",
		Namespace: "prod-eu",
		Revision:  "abc124",
		Occurred:  time.Date(2026, 2, 16, 12, 1, 0, 0, time.UTC),
		Actor:     "argocd",
		EventType: "argo.deployment.recorded",
		HistoryID: 43,
		Result:    "Recorded",
	}
	ev, err := NormalizeSourceEvent(se, "prod-eu")
	if err != nil {
		t.Fatalf("normalize failed: %v", err)
	}
	if ev.Type != "argo.deployment.recorded" {
		t.Fatalf("expected preserved event type, got %s", ev.Type)
	}
	if histID, ok := ev.Extensions["history_id"].(int64); !ok || histID != 43 {
		t.Fatalf("expected history_id=43 in extensions, got %v", ev.Extensions["history_id"])
	}
}
