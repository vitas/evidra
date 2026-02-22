package argo

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	ce "evidra/internal/cloudevents"
	"evidra/internal/store"
)

func TestCollectorIngestsStandardInformerEvents(t *testing.T) {
	repo := store.NewMemoryRepository()

	c := &Collector{
		Normalize: func(se SourceEvent) (ce.StoredEvent, error) {
			return ce.StoredEvent{
				ID:      "evt_from_" + se.App,
				Source:  "argocd",
				Type:    se.EventType,
				Time:    se.Occurred.UTC(),
				Subject: se.App,
				Extensions: map[string]interface{}{
					"cluster":      "eu-1",
					"namespace":    "prod-eu",
					"initiator":    "argocd",
					"operation_id": se.OperationKey,
				},
				Data: json.RawMessage(`{"argocd_app":"` + se.App + `"}`),
			}, nil
		},
		Sink: repo,
	}

	c.loadCheckpoint() // Ensure cursors map is initialized

	obj1 := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"metadata": map[string]interface{}{
				"name":      "payments-api",
				"namespace": "prod-eu",
				"uid":       "uid-1",
			},
			"status": map[string]interface{}{
				"sync": map[string]interface{}{
					"status":   "Synced",
					"revision": "abc1234",
				},
				"history": []interface{}{
					map[string]interface{}{
						"id":         int64(101),
						"revision":   "abc1234",
						"deployedAt": "2026-02-16T12:38:10Z",
					},
				},
			},
		},
	}

	c.handleAppEvent(context.Background(), obj1)

	e, err := repo.GetEvent(context.Background(), "evt_from_payments-api")
	if err != nil {
		t.Fatalf("expected ingested event, got error: %v", err)
	}
	if e.Source != "argocd" {
		t.Fatalf("expected argocd source")
	}
}

func TestCollectorDeduplicatesHistoryRecordedEventsViaInformer(t *testing.T) {
	repo := store.NewMemoryRepository()
	c := &Collector{
		Normalize: func(se SourceEvent) (ce.StoredEvent, error) {
			return ce.StoredEvent{
				ID:      "evt_from_" + se.ID,
				Source:  "argocd",
				Type:    "argo.sync.finished",
				Time:    se.Occurred.UTC(),
				Subject: se.App,
				Extensions: map[string]interface{}{
					"cluster":    "eu-1",
					"namespace":  "prod-eu",
					"initiator":  "argocd",
					"history_id": se.HistoryID,
				},
				Data: json.RawMessage(`{"argocd_app":"` + se.App + `"}`),
			}, nil
		},
		Sink: repo,
	}

	c.loadCheckpoint()

	obj1 := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"metadata": map[string]interface{}{
				"name":      "payments-api",
				"namespace": "prod-eu",
				"uid":       "uid-1",
			},
			"status": map[string]interface{}{
				"sync": map[string]interface{}{
					"status": "Synced",
				},
				"history": []interface{}{
					map[string]interface{}{
						"id":         int64(100),
						"revision":   "abc123",
						"deployedAt": "2026-02-16T12:38:10Z",
					},
				},
			},
		},
	}

	c.handleAppEvent(context.Background(), obj1)
	c.handleAppEvent(context.Background(), obj1) // Second event should be deduped

	res, err := repo.QueryTimeline(context.Background(), store.TimelineQuery{
		Subject:   "payments-api",
		Namespace: "prod-eu",
		Cluster:   "eu-1",
		From:      time.Date(2026, 2, 16, 0, 0, 0, 0, time.UTC),
		To:        time.Date(2026, 2, 17, 0, 0, 0, 0, time.UTC),
		Limit:     100,
	})
	if err != nil {
		t.Fatalf("query timeline failed: %v", err)
	}
	if len(res.Items) != 1 {
		t.Fatalf("expected exactly one recorded history event, got %d", len(res.Items))
	}
}
