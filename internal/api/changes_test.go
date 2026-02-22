package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	ce "evidra/internal/cloudevents"
	"evidra/internal/export"
	"evidra/internal/store"
)

func TestChangesEndpointsFiltersAndDetail(t *testing.T) {
	repo := store.NewMemoryRepository()
	seedChangeEvents(t, repo)
	exporter := export.NewFilesystemExporter(t.TempDir())
	h := NewServer(repo, exporter).Routes()

	req := httptest.NewRequest(
		http.MethodGet,
		"/v1/changes?subject=payments-api:prod-eu:eu-1&from=2026-02-16T00:00:00Z&to=2026-02-16T23:59:59Z&result_status=succeeded",
		nil,
	)
	res := httptest.NewRecorder()
	h.ServeHTTP(res, req)
	if res.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", res.Code, res.Body.String())
	}
	var listResp struct {
		Items []struct {
			ID           string `json:"id"`
			ResultStatus string `json:"result_status"`
		} `json:"items"`
	}
	if err := json.Unmarshal(res.Body.Bytes(), &listResp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(listResp.Items) != 1 || listResp.Items[0].ResultStatus != "succeeded" {
		t.Fatalf("expected one succeeded change, got %s", res.Body.String())
	}

	req = httptest.NewRequest(
		http.MethodGet,
		"/v1/changes?subject=payments-api:prod-eu:eu-1&from=2026-02-16T00:00:00Z&to=2026-02-16T23:59:59Z&health_status=degraded",
		nil,
	)
	res = httptest.NewRecorder()
	h.ServeHTTP(res, req)
	if res.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", res.Code, res.Body.String())
	}
	if err := json.Unmarshal(res.Body.Bytes(), &listResp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(listResp.Items) != 1 {
		t.Fatalf("expected one degraded change, got %s", res.Body.String())
	}
	changeID := listResp.Items[0].ID

	req = httptest.NewRequest(
		http.MethodGet,
		"/v1/changes?subject=payments-api:prod-eu:eu-1&from=2026-02-16T00:00:00Z&to=2026-02-16T23:59:59Z&q=op-2",
		nil,
	)
	res = httptest.NewRecorder()
	h.ServeHTTP(res, req)
	if res.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", res.Code, res.Body.String())
	}
	if err := json.Unmarshal(res.Body.Bytes(), &listResp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(listResp.Items) != 1 {
		t.Fatalf("expected one q-filtered change, got %s", res.Body.String())
	}

	req = httptest.NewRequest(
		http.MethodGet,
		"/v1/changes?subject=payments-api:prod-eu:eu-1&from=2026-02-16T00:00:00Z&to=2026-02-16T23:59:59Z&has_approvals=no",
		nil,
	)
	res = httptest.NewRecorder()
	h.ServeHTTP(res, req)
	if res.Code != http.StatusOK {
		t.Fatalf("expected 200 for has_approvals filter, got %d body=%s", res.Code, res.Body.String())
	}
	if err := json.Unmarshal(res.Body.Bytes(), &listResp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(listResp.Items) == 0 {
		t.Fatalf("expected non-empty changes for has_approvals=no, got %s", res.Body.String())
	}

	req = httptest.NewRequest(
		http.MethodGet,
		"/v1/changes/"+changeID+"?subject=payments-api:prod-eu:eu-1&from=2026-02-16T00:00:00Z&to=2026-02-16T23:59:59Z",
		nil,
	)
	res = httptest.NewRecorder()
	h.ServeHTTP(res, req)
	if res.Code != http.StatusOK {
		t.Fatalf("expected 200 for detail, got %d body=%s", res.Code, res.Body.String())
	}
	var detailResp struct {
		ID     string        `json:"id"`
		Events []interface{} `json:"events"`
	}
	if err := json.Unmarshal(res.Body.Bytes(), &detailResp); err != nil {
		t.Fatalf("decode detail response: %v", err)
	}
	if detailResp.ID == "" || len(detailResp.Events) == 0 {
		t.Fatalf("expected non-empty change detail, got %s", res.Body.String())
	}
}

func TestChangesPaginationAndSubresources(t *testing.T) {
	repo := store.NewMemoryRepository()
	seedChangeEvents(t, repo)
	exporter := export.NewFilesystemExporter(t.TempDir())
	h := NewServer(repo, exporter).Routes()

	req := httptest.NewRequest(
		http.MethodGet,
		"/v1/changes?subject=payments-api:prod-eu:eu-1&from=2026-02-16T00:00:00Z&to=2026-02-16T23:59:59Z&limit=1",
		nil,
	)
	res := httptest.NewRecorder()
	h.ServeHTTP(res, req)
	if res.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", res.Code, res.Body.String())
	}
	var pageResp struct {
		Items []struct {
			ID string `json:"id"`
		} `json:"items"`
		Page struct {
			NextCursor string `json:"next_cursor"`
		} `json:"page"`
	}
	if err := json.Unmarshal(res.Body.Bytes(), &pageResp); err != nil {
		t.Fatalf("decode page response: %v", err)
	}
	if len(pageResp.Items) != 1 || pageResp.Page.NextCursor == "" {
		t.Fatalf("expected one item and next cursor, got %s", res.Body.String())
	}
	firstID := pageResp.Items[0].ID

	req = httptest.NewRequest(
		http.MethodGet,
		"/v1/changes?subject=payments-api:prod-eu:eu-1&from=2026-02-16T00:00:00Z&to=2026-02-16T23:59:59Z&limit=1&cursor="+pageResp.Page.NextCursor,
		nil,
	)
	res = httptest.NewRecorder()
	h.ServeHTTP(res, req)
	if res.Code != http.StatusOK {
		t.Fatalf("expected 200 on second page, got %d body=%s", res.Code, res.Body.String())
	}
	if err := json.Unmarshal(res.Body.Bytes(), &pageResp); err != nil {
		t.Fatalf("decode second page response: %v", err)
	}
	if len(pageResp.Items) != 1 || pageResp.Items[0].ID == firstID {
		t.Fatalf("expected a different second-page change, got %s", res.Body.String())
	}

	req = httptest.NewRequest(
		http.MethodGet,
		"/v1/changes/"+pageResp.Items[0].ID+"/timeline?subject=payments-api:prod-eu:eu-1&from=2026-02-16T00:00:00Z&to=2026-02-16T23:59:59Z",
		nil,
	)
	res = httptest.NewRecorder()
	h.ServeHTTP(res, req)
	if res.Code != http.StatusOK {
		t.Fatalf("expected 200 for timeline subresource, got %d body=%s", res.Code, res.Body.String())
	}
	var timelineResp struct {
		Items []interface{} `json:"items"`
	}
	if err := json.Unmarshal(res.Body.Bytes(), &timelineResp); err != nil {
		t.Fatalf("decode timeline response: %v", err)
	}
	if len(timelineResp.Items) == 0 {
		t.Fatalf("expected timeline items, got %s", res.Body.String())
	}

	req = httptest.NewRequest(
		http.MethodGet,
		"/v1/changes/"+pageResp.Items[0].ID+"/evidence?subject=payments-api:prod-eu:eu-1&from=2026-02-16T00:00:00Z&to=2026-02-16T23:59:59Z",
		nil,
	)
	res = httptest.NewRecorder()
	h.ServeHTTP(res, req)
	if res.Code != http.StatusOK {
		t.Fatalf("expected 200 for evidence subresource, got %d body=%s", res.Code, res.Body.String())
	}
	var evidenceResp struct {
		Change struct {
			ID string `json:"id"`
		} `json:"change"`
	}
	if err := json.Unmarshal(res.Body.Bytes(), &evidenceResp); err != nil {
		t.Fatalf("decode evidence response: %v", err)
	}
	if evidenceResp.Change.ID == "" {
		t.Fatalf("expected evidence payload with change id, got %s", res.Body.String())
	}
}

func TestChangesEndpointsValidationAndNotFound(t *testing.T) {
	repo := store.NewMemoryRepository()
	seedChangeEvents(t, repo)
	exporter := export.NewFilesystemExporter(t.TempDir())
	h := NewServer(repo, exporter).Routes()

	req := httptest.NewRequest(http.MethodGet, "/v1/changes?from=2026-02-16T00:00:00Z&to=2026-02-16T23:59:59Z", nil)
	res := httptest.NewRecorder()
	h.ServeHTTP(res, req)
	if res.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for missing subject, got %d body=%s", res.Code, res.Body.String())
	}

	req = httptest.NewRequest(http.MethodGet, "/v1/changes?subject=payments-api:prod-eu:eu-1&from=bad&to=2026-02-16T23:59:59Z", nil)
	res = httptest.NewRecorder()
	h.ServeHTTP(res, req)
	if res.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for invalid from, got %d body=%s", res.Code, res.Body.String())
	}

	req = httptest.NewRequest(
		http.MethodGet,
		"/v1/changes?subject=payments-api:prod-eu:eu-1&from=2026-02-16T00:00:00Z&to=2026-02-16T23:59:59Z&limit=oops",
		nil,
	)
	res = httptest.NewRecorder()
	h.ServeHTTP(res, req)
	if res.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for invalid limit, got %d body=%s", res.Code, res.Body.String())
	}

	req = httptest.NewRequest(
		http.MethodGet,
		"/v1/changes?subject=payments-api:prod-eu:eu-1&from=2026-02-16T00:00:00Z&to=2026-02-16T23:59:59Z&cursor=bad_cursor",
		nil,
	)
	res = httptest.NewRecorder()
	h.ServeHTTP(res, req)
	if res.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for invalid cursor, got %d body=%s", res.Code, res.Body.String())
	}

	req = httptest.NewRequest(
		http.MethodGet,
		"/v1/changes?subject=payments-api:prod-eu:eu-1&from=2026-02-16T00:00:00Z&to=2026-02-16T23:59:59Z&has_approvals=maybe",
		nil,
	)
	res = httptest.NewRecorder()
	h.ServeHTTP(res, req)
	if res.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for invalid has_approvals, got %d body=%s", res.Code, res.Body.String())
	}

	req = httptest.NewRequest(
		http.MethodGet,
		"/v1/changes/chg_missing?subject=payments-api:prod-eu:eu-1&from=2026-02-16T00:00:00Z&to=2026-02-16T23:59:59Z",
		nil,
	)
	res = httptest.NewRecorder()
	h.ServeHTTP(res, req)
	if res.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for missing change id, got %d body=%s", res.Code, res.Body.String())
	}
}

func TestChangesEndpointsExternalCorrelationFields(t *testing.T) {
	repo := store.NewMemoryRepository()
	ctx := context.Background()
	e := ce.StoredEvent{
		ID:      "evt_external_1",
		Source:  "argocd",
		Type:    "argo.sync.finished",
		Time:    time.Date(2026, 2, 16, 12, 0, 0, 0, time.UTC),
		Subject: "payments-api",
		Extensions: map[string]interface{}{
			"cluster":            "eu-1",
			"namespace":          "prod-eu",
			"initiator":          "argocd",
			"operation_id":       "op-external-1",
			"sync_revision":      "abc123",
			"external_change_id": "CHG123456",
			"ticket_id":          "JIRA-42",
			"approval_reference": "APR-7",
		},
		Data: json.RawMessage(`{"argocd_app":"payments-api","approvals":[{"source":"itsm","identity":"cab","timestamp":"2026-02-16T11:50:00Z","reference":"APR-7","summary":"approved"}]}`),
	}
	if _, _, err := repo.IngestEvent(ctx, e); err != nil {
		t.Fatalf("seed ingest failed: %v", err)
	}
	exporter := export.NewFilesystemExporter(t.TempDir())
	h := NewServer(repo, exporter).Routes()

	req := httptest.NewRequest(
		http.MethodGet,
		"/v1/changes?subject=payments-api:prod-eu:eu-1&from=2026-02-16T00:00:00Z&to=2026-02-16T23:59:59Z",
		nil,
	)
	res := httptest.NewRecorder()
	h.ServeHTTP(res, req)
	if res.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", res.Code, res.Body.String())
	}
	var listResp struct {
		Items []struct {
			ID                string `json:"id"`
			ExternalChangeID  string `json:"external_change_id"`
			TicketID          string `json:"ticket_id"`
			ApprovalReference string `json:"approval_reference"`
			HasApprovals      bool   `json:"has_approvals"`
		} `json:"items"`
	}
	if err := json.Unmarshal(res.Body.Bytes(), &listResp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(listResp.Items) != 1 {
		t.Fatalf("expected one change, got %d", len(listResp.Items))
	}
	if listResp.Items[0].ExternalChangeID != "CHG123456" || listResp.Items[0].TicketID != "JIRA-42" || listResp.Items[0].ApprovalReference != "APR-7" {
		t.Fatalf("unexpected external correlation fields: %+v", listResp.Items[0])
	}
	if !listResp.Items[0].HasApprovals {
		t.Fatalf("expected has_approvals=true, got false")
	}

	req = httptest.NewRequest(
		http.MethodGet,
		"/v1/changes?subject=payments-api:prod-eu:eu-1&from=2026-02-16T00:00:00Z&to=2026-02-16T23:59:59Z&external_change_id=CHG123456&external_change_id_state=set&ticket_id=JIRA-42&ticket_id_state=set&approval_reference=APR-7&has_approvals=yes",
		nil,
	)
	res = httptest.NewRecorder()
	h.ServeHTTP(res, req)
	if res.Code != http.StatusOK {
		t.Fatalf("expected 200 on correlation filters, got %d body=%s", res.Code, res.Body.String())
	}
	if err := json.Unmarshal(res.Body.Bytes(), &listResp); err != nil {
		t.Fatalf("decode filtered response: %v", err)
	}
	if len(listResp.Items) != 1 {
		t.Fatalf("expected one filtered change, got %d body=%s", len(listResp.Items), res.Body.String())
	}

	req = httptest.NewRequest(
		http.MethodGet,
		"/v1/changes/"+listResp.Items[0].ID+"/evidence?subject=payments-api:prod-eu:eu-1&from=2026-02-16T00:00:00Z&to=2026-02-16T23:59:59Z",
		nil,
	)
	res = httptest.NewRecorder()
	h.ServeHTTP(res, req)
	if res.Code != http.StatusOK {
		t.Fatalf("expected 200 evidence, got %d body=%s", res.Code, res.Body.String())
	}
	var evidenceResp struct {
		Approvals []struct {
			Source    string `json:"source"`
			Identity  string `json:"identity"`
			Timestamp string `json:"timestamp"`
			Reference string `json:"reference"`
			Summary   string `json:"summary"`
		} `json:"approvals"`
	}
	if err := json.Unmarshal(res.Body.Bytes(), &evidenceResp); err != nil {
		t.Fatalf("decode evidence response: %v", err)
	}
	if len(evidenceResp.Approvals) != 1 || evidenceResp.Approvals[0].Identity != "cab" {
		t.Fatalf("unexpected approvals payload: %+v", evidenceResp.Approvals)
	}
}

func seedChangeEvents(t *testing.T, repo store.Repository) {
	t.Helper()
	ctx := context.Background()

	events := []ce.StoredEvent{
		{
			ID:      "evt_change_1_start",
			Source:  "argocd",
			Type:    "argo.sync.started",
			Time:    time.Date(2026, 2, 16, 12, 0, 0, 0, time.UTC),
			Subject: "payments-api",
			Extensions: map[string]interface{}{
				"cluster":       "eu-1",
				"namespace":     "prod-eu",
				"initiator":     "argocd",
				"operation_id":  "op-1",
				"sync_revision": "rev-1",
			},
			Data: json.RawMessage(`{"argocd_app":"payments-api"}`),
		},
		{
			ID:      "evt_change_1_finish",
			Source:  "argocd",
			Type:    "argo.sync.finished",
			Time:    time.Date(2026, 2, 16, 12, 2, 0, 0, time.UTC),
			Subject: "payments-api",
			Extensions: map[string]interface{}{
				"cluster":       "eu-1",
				"namespace":     "prod-eu",
				"initiator":     "argocd",
				"operation_id":  "op-1",
				"sync_revision": "rev-1",
				"health_status": "healthy",
			},
			Data: json.RawMessage(`{"argocd_app":"payments-api","status":"Succeeded"}`),
		},
		{
			ID:      "evt_change_2_finish",
			Source:  "argocd",
			Type:    "argo.sync.finished",
			Time:    time.Date(2026, 2, 16, 13, 2, 0, 0, time.UTC),
			Subject: "payments-api",
			Extensions: map[string]interface{}{
				"cluster":       "eu-1",
				"namespace":     "prod-eu",
				"initiator":     "argocd",
				"operation_id":  "op-2",
				"sync_revision": "rev-2",
				"health_status": "degraded",
			},
			Data: json.RawMessage(`{"argocd_app":"payments-api","status":"Failed"}`),
		},
	}
	for _, e := range events {
		if _, _, err := repo.IngestEvent(ctx, e); err != nil {
			t.Fatalf("seed ingest failed for %s: %v", e.ID, err)
		}
	}
}
