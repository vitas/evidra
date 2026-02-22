package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sort"
	"testing"

	"evidra/internal/export"
	"evidra/internal/store"
)

func TestChangesContractShape(t *testing.T) {
	repo := store.NewMemoryRepository()
	seedChangeEvents(t, repo)
	exporter := export.NewFilesystemExporter(t.TempDir())
	h := NewServer(repo, exporter).Routes()

	baseQuery := "?subject=payments-api:prod-eu:eu-1&from=2026-02-16T00:00:00Z&to=2026-02-16T23:59:59Z"

	listBody := requestJSONMap(t, h, http.MethodGet, "/v1/changes"+baseQuery, http.StatusOK)
	assertKeys(t, listBody, "items", "page")
	page, ok := listBody["page"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected object page, got %#v", listBody["page"])
	}
	assertKeys(t, page, "limit", "next_cursor")

	items, ok := listBody["items"].([]interface{})
	if !ok || len(items) == 0 {
		t.Fatalf("expected non-empty items, got %#v", listBody["items"])
	}
	first, ok := items[0].(map[string]interface{})
	if !ok {
		t.Fatalf("expected change item object, got %#v", items[0])
	}
	assertKeys(t, first,
		"id",
		"change_id",
		"permalink",
		"subject",
		"application",
		"target_cluster",
		"namespace",
		"primary_provider",
		"initiator",
		"result_status",
		"health_status",
		"health_at_operation_start",
		"health_after_deploy",
		"post_deploy_degradation",
		"evidence_last_updated_at",
		"evidence_window_seconds",
		"evidence_may_be_incomplete",
		"started_at",
		"completed_at",
		"event_count",
	)
	changeID, _ := first["id"].(string)
	if changeID == "" {
		t.Fatalf("expected non-empty change id")
	}

	detail := requestJSONMap(t, h, http.MethodGet, "/v1/changes/"+changeID+baseQuery, http.StatusOK)
	assertKeys(t, detail,
		"id",
		"change_id",
		"permalink",
		"subject",
		"application",
		"target_cluster",
		"namespace",
		"primary_provider",
		"initiator",
		"result_status",
		"health_status",
		"health_at_operation_start",
		"health_after_deploy",
		"post_deploy_degradation",
		"evidence_last_updated_at",
		"evidence_window_seconds",
		"evidence_may_be_incomplete",
		"started_at",
		"completed_at",
		"event_count",
		"events",
	)

	timeline := requestJSONMap(t, h, http.MethodGet, "/v1/changes/"+changeID+"/timeline"+baseQuery, http.StatusOK)
	assertKeys(t, timeline, "items")

	evidence := requestJSONMap(t, h, http.MethodGet, "/v1/changes/"+changeID+"/evidence"+baseQuery, http.StatusOK)
	assertKeys(t, evidence, "change", "supporting_observations")
}

func requestJSONMap(t *testing.T, h http.Handler, method, path string, expectedStatus int) map[string]interface{} {
	t.Helper()
	req := httptest.NewRequest(method, path, nil)
	res := httptest.NewRecorder()
	h.ServeHTTP(res, req)
	if res.Code != expectedStatus {
		t.Fatalf("unexpected status %d for %s %s body=%s", res.Code, method, path, res.Body.String())
	}
	var body map[string]interface{}
	if err := json.Unmarshal(res.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	return body
}

func assertKeys(t *testing.T, obj map[string]interface{}, want ...string) {
	t.Helper()
	got := make([]string, 0, len(obj))
	for k := range obj {
		got = append(got, k)
	}
	sort.Strings(got)
	sort.Strings(want)
	if len(got) < len(want) {
		t.Fatalf("missing keys: got=%v want at least %v", got, want)
	}
	for _, k := range want {
		if _, ok := obj[k]; !ok {
			t.Fatalf("missing key %q in %v", k, got)
		}
	}
}
