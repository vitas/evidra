package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"evidra/internal/export"
	"evidra/internal/store"
)

func TestGoldenTimelineAndCorrelationFromFixtures(t *testing.T) {
	repo := store.NewMemoryRepository()
	exporter := export.NewFilesystemExporter(t.TempDir())
	h := NewServer(repo, exporter).Routes()

	for _, fixture := range []string{"git_event_valid.json", "argocd_event_valid.json"} {
		body := mustLoadEventFixture(t, fixture)
		req := httptest.NewRequest(http.MethodPost, "/v1/events", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/cloudevents+json")
		res := httptest.NewRecorder()
		h.ServeHTTP(res, req)
		if res.Code != http.StatusAccepted {
			t.Fatalf("ingest fixture %s failed: status=%d body=%s", fixture, res.Code, res.Body.String())
		}
	}

	timelineReq := httptest.NewRequest(http.MethodGet, "/v1/timeline?subject=payments-api:prod-eu:eu-1&from=2026-02-16T00:00:00Z&to=2026-02-16T23:59:59Z&limit=10", nil)
	timelineRes := httptest.NewRecorder()
	h.ServeHTTP(timelineRes, timelineReq)
	if timelineRes.Code != http.StatusOK {
		t.Fatalf("timeline failed: status=%d body=%s", timelineRes.Code, timelineRes.Body.String())
	}
	assertJSONMatchesGolden(t, timelineRes.Body.Bytes(), filepath.Join("..", "..", "testdata", "expected", "timeline_payments.json"))

	corrReq := httptest.NewRequest(http.MethodGet, "/v1/correlations/commit_sha?value=abc123", nil)
	corrRes := httptest.NewRecorder()
	h.ServeHTTP(corrRes, corrReq)
	if corrRes.Code != http.StatusOK {
		t.Fatalf("correlation failed: status=%d body=%s", corrRes.Code, corrRes.Body.String())
	}
	assertJSONMatchesGolden(t, corrRes.Body.Bytes(), filepath.Join("..", "..", "testdata", "expected", "correlation_commit_sha_abc123.json"))
}

func TestInvalidFixtureRejected(t *testing.T) {
	repo := store.NewMemoryRepository()
	exporter := export.NewFilesystemExporter(t.TempDir())
	h := NewServer(repo, exporter).Routes()

	body := mustLoadEventFixture(t, "event_invalid_missing_subject.json")
	req := httptest.NewRequest(http.MethodPost, "/v1/events", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/cloudevents+json")
	res := httptest.NewRecorder()
	h.ServeHTTP(res, req)
	if res.Code != http.StatusBadRequest {
		t.Fatalf("expected invalid fixture to fail with 400, got %d body=%s", res.Code, res.Body.String())
	}
}

func mustLoadEventFixture(t *testing.T, filename string) []byte {
	t.Helper()
	path := filepath.Join("..", "..", "testdata", "events", filename)
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var v interface{}
	if err := json.Unmarshal(b, &v); err != nil {
		t.Fatalf("invalid json fixture %s: %v", filename, err)
	}
	return b
}

func assertJSONMatchesGolden(t *testing.T, actual []byte, goldenPath string) {
	t.Helper()
	goldenBytes, err := os.ReadFile(goldenPath)
	if err != nil {
		t.Fatal(err)
	}
	var actualV interface{}
	if err := json.Unmarshal(actual, &actualV); err != nil {
		t.Fatalf("invalid actual json: %v", err)
	}
	var goldenV interface{}
	if err := json.Unmarshal(goldenBytes, &goldenV); err != nil {
		t.Fatalf("invalid golden json: %v", err)
	}
	removeDynamicFields(actualV)
	removeDynamicFields(goldenV)
	normalizedActual, _ := json.MarshalIndent(actualV, "", "  ")
	normalizedGolden, _ := json.MarshalIndent(goldenV, "", "  ")
	if !bytes.Equal(normalizedActual, normalizedGolden) {
		t.Fatalf("json mismatch\nactual:\n%s\n\nexpected:\n%s", string(normalizedActual), string(normalizedGolden))
	}
}

// removeDynamicFields strips fields that vary between runs (ingested_at, integrity_hash).
func removeDynamicFields(v interface{}) {
	switch data := v.(type) {
	case map[string]interface{}:
		delete(data, "ingested_at")
		delete(data, "integrity_hash")
		for _, val := range data {
			removeDynamicFields(val)
		}
	case []interface{}:
		for _, item := range data {
			removeDynamicFields(item)
		}
	}
}
