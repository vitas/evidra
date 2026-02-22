package store

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	ce "evidra/internal/cloudevents"
)

func sampleEvent(id string, ts time.Time, extensions map[string]interface{}) ce.StoredEvent {
	return ce.StoredEvent{
		ID:      id,
		Source:  "git",
		Type:    "pull_request_merged",
		Time:    ts.UTC(),
		Subject: "payments-api",
		Extensions: mergeExtensions(map[string]interface{}{
			"cluster":   "eu-1",
			"namespace": "prod-eu",
			"initiator": "jane.doe",
		}, extensions),
		Data: json.RawMessage(`{"repo":"org/payments"}`),
	}
}

func mergeExtensions(base, extra map[string]interface{}) map[string]interface{} {
	out := make(map[string]interface{}, len(base)+len(extra))
	for k, v := range base {
		out[k] = v
	}
	for k, v := range extra {
		out[k] = v
	}
	return out
}

func TestIngestIdempotencyAndConflict(t *testing.T) {
	repo := NewMemoryRepository()
	ctx := context.Background()
	e := sampleEvent("evt1", time.Date(2026, 2, 16, 10, 0, 0, 0, time.UTC), map[string]interface{}{"commit_sha": "a"})

	status, _, err := repo.IngestEvent(ctx, e)
	if err != nil || status != IngestAccepted {
		t.Fatalf("expected first ingest accepted, got status=%s err=%v", status, err)
	}

	status, _, err = repo.IngestEvent(ctx, e)
	if err != nil || status != IngestDuplicate {
		t.Fatalf("expected duplicate ingest, got status=%s err=%v", status, err)
	}

	// Same id, different data => conflict
	e2 := e
	e2.Extensions = mergeExtensions(e.Extensions, map[string]interface{}{"commit_sha": "b"})
	e2.Data = json.RawMessage(`{"repo":"org/other"}`)
	// Reset hash so it's recomputed
	e2.IntegrityHash = ""
	_, _, err = repo.IngestEvent(ctx, e2)
	if err == nil {
		t.Fatalf("expected conflict on same id with different payload")
	}
}

func TestTimelineOrderingAndPagination(t *testing.T) {
	repo := NewMemoryRepository()
	ctx := context.Background()
	base := time.Date(2026, 2, 16, 10, 0, 0, 0, time.UTC)

	_ = mustIngest(t, repo, ctx, sampleEvent("evt2", base.Add(2*time.Minute), map[string]interface{}{"commit_sha": "c2"}))
	_ = mustIngest(t, repo, ctx, sampleEvent("evt1", base.Add(1*time.Minute), map[string]interface{}{"commit_sha": "c1"}))
	_ = mustIngest(t, repo, ctx, sampleEvent("evt3", base.Add(3*time.Minute), map[string]interface{}{"commit_sha": "c3"}))

	q := TimelineQuery{
		Subject:   "payments-api",
		Namespace: "prod-eu",
		Cluster:   "eu-1",
		From:      base,
		To:        base.Add(10 * time.Minute),
		Limit:     2,
	}
	page1, err := repo.QueryTimeline(ctx, q)
	if err != nil {
		t.Fatal(err)
	}
	if len(page1.Items) != 2 || page1.Items[0].ID != "evt1" || page1.Items[1].ID != "evt2" {
		t.Fatalf("unexpected first page order: %+v", page1.Items)
	}
	if page1.NextCursor == "" {
		t.Fatalf("expected next cursor")
	}

	q.Cursor = page1.NextCursor
	page2, err := repo.QueryTimeline(ctx, q)
	if err != nil {
		t.Fatal(err)
	}
	if len(page2.Items) != 1 || page2.Items[0].ID != "evt3" {
		t.Fatalf("unexpected second page: %+v", page2.Items)
	}
}

func TestExtensionQueryAndAppendOnly(t *testing.T) {
	repo := NewMemoryRepository()
	ctx := context.Background()
	e := sampleEvent("evt1", time.Date(2026, 2, 16, 10, 0, 0, 0, time.UTC), map[string]interface{}{"commit_sha": "abc123"})
	_ = mustIngest(t, repo, ctx, e)

	items, err := repo.EventsByExtension(ctx, "commit_sha", "abc123", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || items[0].ID != "evt1" {
		t.Fatalf("unexpected extension query result")
	}

	if err := repo.DeleteEvent(ctx, "evt1"); err != ErrAppendOnly {
		t.Fatalf("expected append-only error, got %v", err)
	}
}

func TestTimelineExcludesSupportingByDefault(t *testing.T) {
	repo := NewMemoryRepository()
	ctx := context.Background()
	base := time.Date(2026, 2, 16, 10, 0, 0, 0, time.UTC)

	_ = mustIngest(t, repo, ctx, sampleEvent("evt-primary", base.Add(time.Minute), map[string]interface{}{
		"commit_sha": "p1",
	}))
	_ = mustIngest(t, repo, ctx, ce.StoredEvent{
		ID:      "evt-supporting",
		Source:  "kubernetes",
		Type:    "k8s.supporting.observation",
		Time:    base.Add(2 * time.Minute).UTC(),
		Subject: "payments-api",
		Extensions: map[string]interface{}{
			"cluster":                "eu-1",
			"namespace":              "prod-eu",
			"initiator":              "kubelet",
			"supporting_observation": true,
		},
		Data: json.RawMessage(`{"event":"pod_restart"}`),
	})

	q := TimelineQuery{
		Subject:   "payments-api",
		Namespace: "prod-eu",
		Cluster:   "eu-1",
		From:      base,
		To:        base.Add(10 * time.Minute),
		Limit:     10,
	}
	res, err := repo.QueryTimeline(ctx, q)
	if err != nil {
		t.Fatalf("query timeline: %v", err)
	}
	if len(res.Items) != 1 || res.Items[0].ID != "evt-primary" {
		t.Fatalf("expected only primary events by default, got %+v", res.Items)
	}

	q.IncludeSupporting = true
	res, err = repo.QueryTimeline(ctx, q)
	if err != nil {
		t.Fatalf("query timeline with supporting: %v", err)
	}
	if len(res.Items) != 2 {
		t.Fatalf("expected supporting event included, got %+v", res.Items)
	}
}

func mustIngest(t *testing.T, repo *MemoryRepository, ctx context.Context, e ce.StoredEvent) IngestStatus {
	t.Helper()
	status, _, err := repo.IngestEvent(ctx, e)
	if err != nil {
		t.Fatalf("ingest failed: %v", err)
	}
	return status
}
