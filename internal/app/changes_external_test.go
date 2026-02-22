package app

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	ce "evidra/internal/cloudevents"
	"evidra/internal/store"
)

func TestChangesExternalCorrelationAndApprovals(t *testing.T) {
	repo := store.NewMemoryRepository()
	svc := NewService(repo, nil)
	ctx := context.Background()

	events := []ce.StoredEvent{
		{
			ID:      "evt_argocd_1",
			Source:  "argocd",
			Type:    "argo.sync.started",
			Time:    time.Date(2026, 2, 16, 12, 0, 0, 0, time.UTC),
			Subject: "payments-api",
			Extensions: map[string]interface{}{
				"cluster":            "eu-1",
				"namespace":          "prod-eu",
				"initiator":          "argocd",
				"operation_id":       "op-1",
				"sync_revision":      "abc123",
				"external_change_id": "CHG123456",
				"ticket_id":          "JIRA-42",
				"approval_reference": "APR-7",
			},
			Data: json.RawMessage(`{"argocd_app":"payments-api","approvals":[{"source":"itsm","identity":"cab","timestamp":"2026-02-16T11:50:00Z","reference":"APR-7","summary":"approved"}]}`),
		},
		{
			ID:      "evt_argocd_2",
			Source:  "argocd",
			Type:    "argo.sync.finished",
			Time:    time.Date(2026, 2, 16, 12, 2, 0, 0, time.UTC),
			Subject: "payments-api",
			Extensions: map[string]interface{}{
				"cluster":       "eu-1",
				"namespace":     "prod-eu",
				"initiator":     "argocd",
				"operation_id":  "op-1",
				"sync_revision": "abc123",
			},
			Data: json.RawMessage(`{"argocd_app":"payments-api","status":"Succeeded"}`),
		},
	}
	for _, e := range events {
		if _, _, err := repo.IngestEvent(ctx, e); err != nil {
			t.Fatalf("seed ingest %s failed: %v", e.ID, err)
		}
	}

	q := ChangeQuery{
		Subject: ParsedSubject{App: "payments-api", Environment: "prod-eu", Cluster: "eu-1"},
		From:    time.Date(2026, 2, 16, 0, 0, 0, 0, time.UTC),
		To:      time.Date(2026, 2, 16, 23, 59, 59, 0, time.UTC),
	}
	list, err := svc.ListChanges(ctx, q)
	if err != nil {
		t.Fatalf("list changes failed: %v", err)
	}
	if len(list.Items) != 1 {
		t.Fatalf("expected one change, got %d", len(list.Items))
	}
	ch := list.Items[0]
	if ch.ExternalChangeID != "CHG123456" {
		t.Fatalf("expected external change id, got %q", ch.ExternalChangeID)
	}
	if ch.TicketID != "JIRA-42" {
		t.Fatalf("expected ticket id, got %q", ch.TicketID)
	}
	if ch.ApprovalReference != "APR-7" {
		t.Fatalf("expected approval reference, got %q", ch.ApprovalReference)
	}
	if !ch.HasApprovals {
		t.Fatalf("expected has_approvals=true")
	}

	filtered, err := svc.ListChanges(ctx, ChangeQuery{
		Subject:               q.Subject,
		From:                  q.From,
		To:                    q.To,
		ExternalChangeID:      "CHG123456",
		ExternalChangeIDState: "set",
		TicketID:              "JIRA-42",
		TicketIDState:         "set",
		ApprovalReference:     "APR-7",
		HasApprovals:          "yes",
	})
	if err != nil {
		t.Fatalf("filtered list changes failed: %v", err)
	}
	if len(filtered.Items) != 1 {
		t.Fatalf("expected one filtered change, got %d", len(filtered.Items))
	}

	ev, err := svc.GetChangeEvidence(ctx, ch.ID, q)
	if err != nil {
		t.Fatalf("get evidence failed: %v", err)
	}
	if len(ev.Approvals) != 1 {
		t.Fatalf("expected one approval, got %d", len(ev.Approvals))
	}
	if ev.Approvals[0].Identity != "cab" {
		t.Fatalf("expected approval identity cab, got %q", ev.Approvals[0].Identity)
	}
}
