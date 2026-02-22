package app

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	ce "evidra/internal/cloudevents"
	"evidra/internal/store"
)

func TestChangeSummaryFieldsPresent(t *testing.T) {
	repo := store.NewMemoryRepository()
	svc := NewService(repo, nil)
	ctx := context.Background()

	events := []ce.StoredEvent{
		{
			ID:      "evt_1",
			Source:  "argocd",
			Type:    "argo.sync.started",
			Time:    time.Date(2026, 2, 16, 12, 0, 0, 0, time.UTC),
			Subject: "payments-api",
			Extensions: map[string]interface{}{
				"cluster":            "eu-1",
				"namespace":          "prod-eu",
				"initiator":          "alice",
				"operation_id":       "op-1",
				"sync_revision":      "abc123",
				"external_change_id": "CHG-1",
				"ticket_id":          "JIRA-1",
				"health_status":      "Healthy",
			},
			Data: json.RawMessage(`{"argocd_app":"payments-api"}`),
		},
		{
			ID:      "evt_2",
			Source:  "argocd",
			Type:    "argo.sync.finished",
			Time:    time.Date(2026, 2, 16, 12, 1, 0, 0, time.UTC),
			Subject: "payments-api",
			Extensions: map[string]interface{}{
				"cluster":       "eu-1",
				"namespace":     "prod-eu",
				"initiator":     "alice",
				"operation_id":  "op-1",
				"sync_revision": "abc123",
			},
			Data: json.RawMessage(`{"argocd_app":"payments-api","status":"Succeeded"}`),
		},
	}
	for _, e := range events {
		if _, _, err := repo.IngestEvent(ctx, e); err != nil {
			t.Fatalf("ingest: %v", err)
		}
	}

	res, err := svc.ListChanges(ctx, ChangeQuery{
		Subject: ParsedSubject{App: "payments-api", Environment: "prod-eu", Cluster: "eu-1"},
		From:    time.Date(2026, 2, 16, 0, 0, 0, 0, time.UTC),
		To:      time.Date(2026, 2, 16, 23, 59, 59, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("list changes: %v", err)
	}
	if len(res.Items) != 1 {
		t.Fatalf("expected one change, got %d", len(res.Items))
	}
	ch := res.Items[0]
	if ch.ChangeID == "" || ch.Permalink == "" {
		t.Fatalf("expected stable id and permalink")
	}
	if ch.Application == "" || ch.TargetCluster == "" || ch.Namespace == "" {
		t.Fatalf("expected summary target fields, got %+v", ch)
	}
	if ch.Initiator != "alice" {
		t.Fatalf("expected initiator alice, got %q", ch.Initiator)
	}
	if ch.HealthAtOperationStart == "" || ch.HealthAfterDeploy == "" {
		t.Fatalf("expected health summary fields")
	}
	if ch.EvidenceLastUpdatedAt.IsZero() || ch.EvidenceWindowSeconds == 0 {
		t.Fatalf("expected freshness fields")
	}
}

func TestPostDeployDegradationObservedOnlyFromHealthyStart(t *testing.T) {
	repo := store.NewMemoryRepository()
	svc := NewService(repo, nil)
	ctx := context.Background()

	cases := []struct {
		name      string
		start     string
		expectObs bool
	}{
		{name: "healthy_start_then_degraded", start: "Healthy", expectObs: true},
		{name: "already_degraded", start: "Degraded", expectObs: false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			op := "op-" + tc.name
			evs := []ce.StoredEvent{
				{
					ID:      "evt-start-" + tc.name,
					Source:  "argocd",
					Type:    "argo.sync.started",
					Time:    time.Date(2026, 2, 16, 10, 0, 0, 0, time.UTC),
					Subject: "payments-api",
					Extensions: map[string]interface{}{
						"cluster":       "eu-1",
						"namespace":     "prod-eu",
						"initiator":     "argocd",
						"operation_id":  op,
						"sync_revision": tc.name,
						"health_status": tc.start,
					},
					Data: json.RawMessage(`{"argocd_app":"payments-api"}`),
				},
				{
					ID:      "evt-mid-" + tc.name,
					Source:  "argocd",
					Type:    "argo.health.changed",
					Time:    time.Date(2026, 2, 16, 10, 1, 0, 0, time.UTC),
					Subject: "payments-api",
					Extensions: map[string]interface{}{
						"cluster":       "eu-1",
						"namespace":     "prod-eu",
						"initiator":     "argocd",
						"operation_id":  op,
						"sync_revision": tc.name,
						"health_status": "Degraded",
					},
					Data: json.RawMessage(`{"argocd_app":"payments-api"}`),
				},
				{
					ID:      "evt-end-" + tc.name,
					Source:  "argocd",
					Type:    "argo.sync.finished",
					Time:    time.Date(2026, 2, 16, 10, 2, 0, 0, time.UTC),
					Subject: "payments-api",
					Extensions: map[string]interface{}{
						"cluster":       "eu-1",
						"namespace":     "prod-eu",
						"initiator":     "argocd",
						"operation_id":  op,
						"sync_revision": tc.name,
					},
					Data: json.RawMessage(`{"argocd_app":"payments-api","status":"Succeeded"}`),
				},
			}
			for _, e := range evs {
				if _, _, err := repo.IngestEvent(ctx, e); err != nil {
					t.Fatalf("ingest %s: %v", e.ID, err)
				}
			}
		})
	}

	res, err := svc.ListChanges(ctx, ChangeQuery{
		Subject: ParsedSubject{App: "payments-api", Environment: "prod-eu", Cluster: "eu-1"},
		From:    time.Date(2026, 2, 16, 0, 0, 0, 0, time.UTC),
		To:      time.Date(2026, 2, 16, 23, 59, 59, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("list changes: %v", err)
	}
	if len(res.Items) < 2 {
		t.Fatalf("expected at least two changes, got %d", len(res.Items))
	}
	foundObserved := false
	foundNotObserved := false
	for _, ch := range res.Items {
		if ch.Revision == "healthy_start_then_degraded" && ch.PostDeployDegradation.Observed {
			foundObserved = true
		}
		if ch.Revision == "already_degraded" && !ch.PostDeployDegradation.Observed {
			foundNotObserved = true
		}
	}
	if !foundObserved {
		t.Fatalf("expected observed post-deploy degradation for healthy start case")
	}
	if !foundNotObserved {
		t.Fatalf("expected no post-deploy degradation observation for already degraded case")
	}
}

func TestEvidenceFreshnessMayBeIncompleteForRecentChange(t *testing.T) {
	repo := store.NewMemoryRepository()
	svc := NewService(repo, nil)
	ctx := context.Background()
	now := time.Now().UTC()
	e := ce.StoredEvent{
		ID:      "evt_recent",
		Source:  "argocd",
		Type:    "argo.sync.finished",
		Time:    now,
		Subject: "payments-api",
		Extensions: map[string]interface{}{
			"cluster":       "eu-1",
			"namespace":     "prod-eu",
			"initiator":     "argocd",
			"operation_id":  "op-recent",
			"sync_revision": "recent",
		},
		Data: json.RawMessage(`{"argocd_app":"payments-api","status":"Succeeded"}`),
	}
	if _, _, err := repo.IngestEvent(ctx, e); err != nil {
		t.Fatalf("ingest recent event: %v", err)
	}
	res, err := svc.ListChanges(ctx, ChangeQuery{
		Subject: ParsedSubject{App: "payments-api", Environment: "prod-eu", Cluster: "eu-1"},
		From:    now.Add(-1 * time.Hour),
		To:      now.Add(1 * time.Hour),
	})
	if err != nil {
		t.Fatalf("list changes: %v", err)
	}
	if len(res.Items) != 1 {
		t.Fatalf("expected one change, got %d", len(res.Items))
	}
	if !res.Items[0].EvidenceMayBeIncomplete {
		t.Fatalf("expected evidence_may_be_incomplete=true for recent change")
	}
}
