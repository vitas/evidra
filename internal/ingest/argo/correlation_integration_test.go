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

func TestCollectorCorrelatesWithGitByCommitSHA(t *testing.T) {
	ctx := context.Background()
	repo := store.NewMemoryRepository()

	gitEvent := ce.StoredEvent{
		ID:      "evt_git_1",
		Source:  "git",
		Type:    "push",
		Time:    time.Date(2026, 2, 16, 12, 0, 0, 0, time.UTC),
		Subject: "payments-api",
		Extensions: map[string]interface{}{
			"cluster":    "eu-1",
			"namespace":  "prod-eu",
			"initiator":  "dev-1",
			"commit_sha": "abc123",
		},
		Data: json.RawMessage(`{"repo":"org/payments","commit_sha":"abc123"}`),
	}
	if _, _, err := repo.IngestEvent(ctx, gitEvent); err != nil {
		t.Fatalf("ingest git event: %v", err)
	}

	c := &Collector{
		Normalize: func(se SourceEvent) (ce.StoredEvent, error) {
			return NormalizeSourceEvent(se, "prod-eu")
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
					"status":   "Synced",
					"revision": "abc123",
				},
			},
		},
	}

	c.handleAppEvent(ctx, obj1)

	items, err := repo.EventsByExtension(ctx, "commit_sha", "abc123", 10)
	if err != nil {
		t.Fatalf("query extension: %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("expected 2 correlated events, got %d", len(items))
	}
	if items[0].Source != "git" || items[1].Source != "argocd" {
		t.Fatalf("expected ordered sources [git,argocd], got [%s,%s]", items[0].Source, items[1].Source)
	}
}
