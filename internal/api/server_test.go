package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"evidra/internal/export"
	"evidra/internal/model"
	"evidra/internal/store"
)

func TestEndpointsFlow(t *testing.T) {
	repo := store.NewMemoryRepository()
	exporter := export.NewFilesystemExporter(t.TempDir())
	srv := NewServer(repo, exporter)
	h := srv.Routes()

	// Post a CloudEvent
	cePayload := map[string]interface{}{
		"specversion":     "1.0",
		"id":              "evt_1",
		"source":          "git",
		"type":            "pull_request_merged",
		"time":            "2026-02-16T12:00:00Z",
		"subject":         "payments-api",
		"cluster":         "eu-1",
		"namespace":       "prod-eu",
		"initiator":       "jane.doe",
		"commit_sha":      "abc123",
		"datacontenttype": "application/json",
		"data":            map[string]interface{}{"repo": "org/payments"},
	}
	b, _ := json.Marshal(cePayload)
	req := httptest.NewRequest(http.MethodPost, "/v1/events", bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/cloudevents+json")
	res := httptest.NewRecorder()
	h.ServeHTTP(res, req)
	if res.Code != http.StatusAccepted {
		t.Fatalf("expected 202, got %d: %s", res.Code, res.Body.String())
	}

	qReq := httptest.NewRequest(http.MethodGet, "/v1/timeline?subject=payments-api:prod-eu:eu-1&from=2026-02-16T00:00:00Z&to=2026-02-16T23:59:59Z&limit=10", nil)
	qRes := httptest.NewRecorder()
	h.ServeHTTP(qRes, qReq)
	if qRes.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", qRes.Code)
	}

	cReq := httptest.NewRequest(http.MethodGet, "/v1/correlations/commit_sha?value=abc123", nil)
	cRes := httptest.NewRecorder()
	h.ServeHTTP(cRes, cReq)
	if cRes.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", cRes.Code)
	}

	chReq := httptest.NewRequest(http.MethodGet, "/v1/changes?subject=payments-api:prod-eu:eu-1&from=2026-02-16T00:00:00Z&to=2026-02-16T23:59:59Z", nil)
	chRes := httptest.NewRecorder()
	h.ServeHTTP(chRes, chReq)
	if chRes.Code != http.StatusOK {
		t.Fatalf("expected 200 for changes, got %d body=%s", chRes.Code, chRes.Body.String())
	}
	var changesResp struct {
		Items []struct {
			ID       string `json:"id"`
			ChangeID string `json:"change_id"`
		} `json:"items"`
	}
	if err := json.Unmarshal(chRes.Body.Bytes(), &changesResp); err != nil {
		t.Fatalf("failed to decode changes response: %v", err)
	}
	if len(changesResp.Items) == 0 || changesResp.Items[0].ID == "" || changesResp.Items[0].ChangeID == "" {
		t.Fatalf("expected non-empty changes list, got: %s", chRes.Body.String())
	}

	chdReq := httptest.NewRequest(http.MethodGet, "/v1/changes/"+changesResp.Items[0].ID+"?subject=payments-api:prod-eu:eu-1&from=2026-02-16T00:00:00Z&to=2026-02-16T23:59:59Z", nil)
	chdRes := httptest.NewRecorder()
	h.ServeHTTP(chdRes, chdReq)
	if chdRes.Code != http.StatusOK {
		t.Fatalf("expected 200 for change detail, got %d body=%s", chdRes.Code, chdRes.Body.String())
	}
	var detailResp struct {
		ID       string `json:"id"`
		ChangeID string `json:"change_id"`
	}
	if err := json.Unmarshal(chdRes.Body.Bytes(), &detailResp); err != nil {
		t.Fatalf("decode detail response: %v", err)
	}
	if detailResp.ID != changesResp.Items[0].ID || detailResp.ChangeID != changesResp.Items[0].ChangeID {
		t.Fatalf("expected detail/list id consistency, list=%+v detail=%+v", changesResp.Items[0], detailResp)
	}

	exportPayload := map[string]interface{}{
		"format": "json",
		"filter": map[string]interface{}{
			"subject": "payments-api:prod-eu:eu-1",
			"from":    "2026-02-16T00:00:00Z",
			"to":      "2026-02-16T23:59:59Z",
		},
	}
	eb, _ := json.Marshal(exportPayload)
	exReq := httptest.NewRequest(http.MethodPost, "/v1/exports", bytes.NewReader(eb))
	exRes := httptest.NewRecorder()
	h.ServeHTTP(exRes, exReq)
	if exRes.Code != http.StatusAccepted {
		t.Fatalf("expected 202, got %d: %s", exRes.Code, exRes.Body.String())
	}

	var job model.ExportJob
	if err := json.Unmarshal(exRes.Body.Bytes(), &job); err != nil {
		t.Fatal(err)
	}
	if job.ID == "" || job.ArtifactURI == "" || job.Status != "completed" {
		t.Fatalf("unexpected export job: %+v", job)
	}

	dReq := httptest.NewRequest(http.MethodGet, "/v1/exports/"+job.ID+"/download", nil)
	dRes := httptest.NewRecorder()
	h.ServeHTTP(dRes, dReq)
	if dRes.Code != http.StatusOK {
		t.Fatalf("expected 200 for download, got %d", dRes.Code)
	}
	var exportArtifact struct {
		ChangeID          string `json:"change_id"`
		GeneratedAt       string `json:"generated_at"`
		Source            string `json:"source"`
		Application       string `json:"application"`
		Cluster           string `json:"cluster"`
		Namespace         string `json:"namespace"`
		DeterministicHash string `json:"deterministic_hash_sha256"`
		Timeline          []interface{} `json:"timeline"`
		PostDeployDegradation struct {
			Observed bool `json:"observed"`
		} `json:"post_deploy_degradation"`
	}
	if err := json.Unmarshal(dRes.Body.Bytes(), &exportArtifact); err != nil {
		t.Fatalf("decode export artifact: %v", err)
	}
	if exportArtifact.ChangeID == "" || exportArtifact.GeneratedAt == "" || exportArtifact.Source != "argocd" {
		t.Fatalf("missing top-level export metadata: %+v", exportArtifact)
	}
	if exportArtifact.Application == "" || exportArtifact.Cluster == "" || exportArtifact.Namespace == "" {
		t.Fatalf("missing target metadata: %+v", exportArtifact)
	}
	if len(exportArtifact.DeterministicHash) != 64 || len(exportArtifact.Timeline) == 0 {
		t.Fatalf("missing deterministic hash or timeline: %+v", exportArtifact)
	}
}

func TestEventsRejectOversizedPayload(t *testing.T) {
	repo := store.NewMemoryRepository()
	exporter := export.NewFilesystemExporter(t.TempDir())
	h := NewServer(repo, exporter).Routes()

	oversized := bytes.Repeat([]byte("a"), int(maxIngestBodyBytes)+1)

	req := httptest.NewRequest(http.MethodPost, "/v1/events", bytes.NewReader(oversized))
	res := httptest.NewRecorder()
	h.ServeHTTP(res, req)
	if res.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("expected 413 for /v1/events, got %d body=%s", res.Code, res.Body.String())
	}
}
