package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	testutil "samebits.com/evidra-benchmark/internal/testutil"
	"samebits.com/evidra-benchmark/pkg/evidence"
)

type fakeWebhookStore struct {
	lastHash string
	claimed  map[string]bool
	saved    []json.RawMessage
	released []string
}

func (f *fakeWebhookStore) LastHash(_ context.Context, _ string) (string, error) {
	return f.lastHash, nil
}

func (f *fakeWebhookStore) SaveRaw(_ context.Context, _ string, raw json.RawMessage) (string, error) {
	f.saved = append(f.saved, raw)
	var entry evidence.EvidenceEntry
	if err := json.Unmarshal(raw, &entry); err == nil {
		f.lastHash = entry.Hash
	}
	return "receipt-1", nil
}

func (f *fakeWebhookStore) ClaimWebhookEvent(_ context.Context, _ string, source, key string, _ json.RawMessage) (bool, error) {
	if f.claimed == nil {
		f.claimed = make(map[string]bool)
	}
	composite := source + ":" + key
	if f.claimed[composite] {
		return true, nil
	}
	f.claimed[composite] = true
	return false, nil
}

func (f *fakeWebhookStore) ReleaseWebhookEvent(_ context.Context, _ string, source, key string) error {
	f.released = append(f.released, source+":"+key)
	delete(f.claimed, source+":"+key)
	return nil
}

func TestHandleGenericWebhook_RequiresSigner(t *testing.T) {
	t.Parallel()

	handler := handleGenericWebhook(&fakeWebhookStore{}, nil, "generic-secret")
	req := httptest.NewRequest("POST", "/v1/hooks/generic", bytes.NewBufferString(`{
		"event_type":"operation_started",
		"tool":"mycontroller",
		"operation":"reconcile",
		"environment":"production",
		"actor":"my-operator",
		"session_id":"sess-1",
		"idempotency_key":"evt-1"
	}`))
	req.Header.Set("Authorization", "Bearer generic-secret")
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestHandleGenericWebhook_CompletedRequiresIdempotencyKey(t *testing.T) {
	t.Parallel()

	handler := handleGenericWebhook(&fakeWebhookStore{}, testutil.TestSigner(t), "generic-secret")
	req := httptest.NewRequest("POST", "/v1/hooks/generic", bytes.NewBufferString(`{
		"event_type":"operation_completed",
		"tool":"mycontroller",
		"operation":"reconcile",
		"environment":"production",
		"actor":"my-operator",
		"session_id":"sess-1",
		"exit_code":0,
		"verdict":"success"
	}`))
	req.Header.Set("Authorization", "Bearer generic-secret")
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestHandleGenericWebhook_DuplicateReturns200WithoutNewEvidence(t *testing.T) {
	t.Parallel()

	store := &fakeWebhookStore{}
	handler := handleGenericWebhook(store, testutil.TestSigner(t), "generic-secret")
	body := `{
		"event_type":"operation_started",
		"tool":"mycontroller",
		"operation":"reconcile",
		"environment":"production",
		"actor":"my-operator",
		"session_id":"sess-1",
		"idempotency_key":"evt-1"
	}`

	req1 := httptest.NewRequest("POST", "/v1/hooks/generic", bytes.NewBufferString(body))
	req1.Header.Set("Authorization", "Bearer generic-secret")
	req1.Header.Set("Content-Type", "application/json")
	rec1 := httptest.NewRecorder()
	handler.ServeHTTP(rec1, req1)

	req2 := httptest.NewRequest("POST", "/v1/hooks/generic", bytes.NewBufferString(body))
	req2.Header.Set("Authorization", "Bearer generic-secret")
	req2.Header.Set("Content-Type", "application/json")
	rec2 := httptest.NewRecorder()
	handler.ServeHTTP(rec2, req2)

	if rec1.Code != http.StatusAccepted {
		t.Fatalf("first request expected 202, got %d body=%s", rec1.Code, rec1.Body.String())
	}
	if rec2.Code != http.StatusOK {
		t.Fatalf("second request expected 200 duplicate, got %d body=%s", rec2.Code, rec2.Body.String())
	}
	if len(store.saved) != 1 {
		t.Fatalf("saved entries = %d, want 1", len(store.saved))
	}
}

func TestHandleArgoCDWebhook_MapsStartAndCompletion(t *testing.T) {
	t.Parallel()

	store := &fakeWebhookStore{}
	handler := handleArgoCDWebhook(store, testutil.TestSigner(t), "argocd-secret")

	startReq := httptest.NewRequest("POST", "/v1/hooks/argocd", bytes.NewBufferString(`{
		"event":"sync_started",
		"app_name":"payments",
		"app_namespace":"prod",
		"revision":"abc123",
		"initiated_by":"argocd-bot",
		"operation_id":"op-1"
	}`))
	startReq.Header.Set("Authorization", "Bearer argocd-secret")
	startReq.Header.Set("Content-Type", "application/json")
	startRec := httptest.NewRecorder()
	handler.ServeHTTP(startRec, startReq)
	if startRec.Code != http.StatusAccepted {
		t.Fatalf("start expected 202, got %d body=%s", startRec.Code, startRec.Body.String())
	}

	var prescribe evidence.EvidenceEntry
	if err := json.Unmarshal(store.saved[0], &prescribe); err != nil {
		t.Fatalf("decode prescribe: %v", err)
	}
	if prescribe.Type != evidence.EntryTypePrescribe {
		t.Fatalf("type = %q, want prescribe", prescribe.Type)
	}
	if prescribe.Actor.ID != "argocd-bot" {
		t.Fatalf("actor = %q, want argocd-bot", prescribe.Actor.ID)
	}
	if prescribe.SessionID != "op-1" {
		t.Fatalf("session_id = %q, want op-1", prescribe.SessionID)
	}

	completeReq := httptest.NewRequest("POST", "/v1/hooks/argocd", bytes.NewBufferString(`{
		"event":"sync_completed",
		"app_name":"payments",
		"app_namespace":"prod",
		"phase":"Failed",
		"message":"sync failed",
		"operation_id":"op-1"
	}`))
	completeReq.Header.Set("Authorization", "Bearer argocd-secret")
	completeReq.Header.Set("Content-Type", "application/json")
	completeRec := httptest.NewRecorder()
	handler.ServeHTTP(completeRec, completeReq)
	if completeRec.Code != http.StatusAccepted {
		t.Fatalf("completion expected 202, got %d body=%s", completeRec.Code, completeRec.Body.String())
	}

	if len(store.saved) != 2 {
		t.Fatalf("saved entries = %d, want 2", len(store.saved))
	}
	var report evidence.EvidenceEntry
	if err := json.Unmarshal(store.saved[1], &report); err != nil {
		t.Fatalf("decode report: %v", err)
	}
	if report.Type != evidence.EntryTypeReport {
		t.Fatalf("type = %q, want report", report.Type)
	}
	if report.PreviousHash != prescribe.Hash {
		t.Fatalf("previous_hash = %q, want %q", report.PreviousHash, prescribe.Hash)
	}
}
