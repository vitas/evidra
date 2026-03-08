package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http/httptest"
	"testing"

	"samebits.com/evidra-benchmark/internal/auth"
)

type fakeEntryStore struct {
	lastTenant string
	entries    []json.RawMessage
}

func (f *fakeEntryStore) SaveRaw(ctx context.Context, tenantID string, raw json.RawMessage) (string, error) {
	f.lastTenant = tenantID
	f.entries = append(f.entries, raw)
	return "receipt-1", nil
}

func TestForwardHandler_Success(t *testing.T) {
	t.Parallel()
	store := &fakeEntryStore{}
	handler := handleForward(store)

	body := []byte(`{"type":"prescribe","hash":"sha256:abc"}`)
	req := httptest.NewRequest("POST", "/v1/evidence/forward", bytes.NewReader(body))
	req = req.WithContext(auth.WithTenantID(req.Context(), "t1"))
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != 200 {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	if store.lastTenant != "t1" {
		t.Fatalf("expected tenant=t1, got %s", store.lastTenant)
	}
}

func TestForwardHandler_EmptyBody(t *testing.T) {
	t.Parallel()
	store := &fakeEntryStore{}
	handler := handleForward(store)

	req := httptest.NewRequest("POST", "/v1/evidence/forward", nil)
	req = req.WithContext(auth.WithTenantID(req.Context(), "t1"))
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != 400 {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
}

func TestBatchHandler_Success(t *testing.T) {
	t.Parallel()
	store := &fakeEntryStore{}
	handler := handleBatch(store)

	body := []byte(`{"entries":[{"type":"prescribe"},{"type":"report"}]}`)
	req := httptest.NewRequest("POST", "/v1/evidence/batch", bytes.NewReader(body))
	req = req.WithContext(auth.WithTenantID(req.Context(), "t1"))
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != 200 {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	var resp map[string]interface{}
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if int(resp["accepted"].(float64)) != 2 {
		t.Fatalf("expected accepted=2, got %v", resp["accepted"])
	}
}
