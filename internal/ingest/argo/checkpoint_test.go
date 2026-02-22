package argo

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	ce "evidra/internal/cloudevents"
	"evidra/internal/store"
)

func TestFileCheckpointStoreRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state", "argo.json")
	s := FileCheckpointStore{Path: path}
	in := Checkpoint{
		Apps: map[string]AppCheckpoint{
			"uid-1": {
				LastHistoryID:   11,
				LastHistoryAt:   time.Date(2026, 2, 17, 10, 0, 0, 0, time.UTC),
				LastStartKey:    "rev-1:start",
				LastTerminalKey: "rev-1:finish",
				LastHealth:      "Healthy",
			},
		},
	}
	if err := s.Save(in); err != nil {
		t.Fatalf("save checkpoint: %v", err)
	}
	out, err := s.Load()
	if err != nil {
		t.Fatalf("load checkpoint: %v", err)
	}
	if len(out.Apps) != 1 {
		t.Fatalf("unexpected apps checkpoint: %+v", out.Apps)
	}
	if out.Apps["uid-1"].LastHistoryID != 11 {
		t.Fatalf("unexpected history id: %d", out.Apps["uid-1"].LastHistoryID)
	}
}

func TestCollectorSkipsAlreadyCheckpointedEvents(t *testing.T) {
	repo := store.NewMemoryRepository()
	path := filepath.Join(t.TempDir(), "argo.json")
	cpStore := FileCheckpointStore{Path: path}
	if err := cpStore.Save(Checkpoint{
		Apps: map[string]AppCheckpoint{
			"uid-1": {
				LastHistoryID: 1,
				LastHistoryAt: time.Date(2026, 2, 17, 10, 0, 0, 0, time.UTC),
			},
		},
	}); err != nil {
		t.Fatalf("seed checkpoint: %v", err)
	}

	c := &Collector{
		Normalize: func(se SourceEvent) (ce.StoredEvent, error) {
			return NormalizeSourceEvent(se, "prod-eu")
		},
		Sink:       repo,
		Checkpoint: cpStore,
	}

	c.loadCheckpoint() // explicitly initialize the appCursors

	// First event should be skipped because history ID matches checkpoint (1) and time is older/equal.
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
						"id":         int64(1),
						"revision":   "abc1234",
						"deployedAt": "2026-02-17T10:00:00Z",
					},
				},
			},
		},
	}
	c.handleAppEvent(context.Background(), obj1)

	// Verify nothing was ingested
	res1, _ := repo.QueryTimeline(context.Background(), store.TimelineQuery{
		Limit: 10,
		To:    time.Now().Add(1000 * time.Hour),
	})
	if len(res1.Items) > 0 {
		t.Fatalf("expected obj1 to be skipped by checkpoint")
	}

	// Second event should be ingested because History ID makes it newer
	obj2 := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"metadata": map[string]interface{}{
				"name":      "payments-api",
				"namespace": "prod-eu",
				"uid":       "uid-1",
			},
			"status": map[string]interface{}{
				"sync": map[string]interface{}{
					"status":   "Synced",
					"revision": "abc999",
				},
				"history": []interface{}{
					map[string]interface{}{
						"id":         int64(2),
						"revision":   "abc999",
						"deployedAt": "2026-02-17T10:01:00Z",
					},
				},
			},
		},
	}
	c.handleAppEvent(context.Background(), obj2)

	res2, _ := repo.QueryTimeline(context.Background(), store.TimelineQuery{
		Limit: 10,
		To:    time.Now().Add(1000 * time.Hour),
	})
	if len(res2.Items) != 1 {
		t.Fatalf("expected obj2 to be ingested, got %d items", len(res2.Items))
	}
}
