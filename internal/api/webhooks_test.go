package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	testutil "samebits.com/evidra/internal/testutil"
	"samebits.com/evidra/pkg/evidence"
)

func TestHandleGenericWebhook_RequiresTenantAPIKey(t *testing.T) {
	t.Parallel()

	store := &fakeWebhookStore{}
	handler := handleGenericWebhookWithTenantResolver(store, testutil.TestSigner(t), "route-secret", func(context.Context, string) (string, error) {
		return "", nil
	})

	req := httptest.NewRequest("POST", "/v1/hooks/generic", strings.NewReader(`{
		"event_type":"operation_started",
		"tool":"kubectl",
		"operation":"apply",
		"environment":"production"
	}`))
	req.Header.Set("Authorization", "Bearer route-secret")
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rec.Code)
	}
}

func TestHandleGenericWebhook_UsesResolvedTenant(t *testing.T) {
	t.Parallel()

	store := &fakeWebhookStore{}
	handler := handleGenericWebhookWithTenantResolver(store, testutil.TestSigner(t), "route-secret", func(_ context.Context, token string) (string, error) {
		if token != "tenant-api-key" {
			return "", errors.New("unknown key")
		}
		return "tenant-123", nil
	})

	req := httptest.NewRequest("POST", "/v1/hooks/generic", strings.NewReader(`{
		"event_type":"operation_started",
		"tool":"kubectl",
		"operation":"apply",
		"operation_id":"op-1",
		"environment":"production",
		"actor":"ci"
	}`))
	req.Header.Set("Authorization", "Bearer route-secret")
	req.Header.Set("X-Evidra-API-Key", "tenant-api-key")
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusAccepted {
		t.Fatalf("expected 202, got %d", rec.Code)
	}
	if len(store.savedTenants) != 1 {
		t.Fatalf("saved tenants = %d, want 1", len(store.savedTenants))
	}
	if store.savedTenants[0] != "tenant-123" {
		t.Fatalf("saved tenant = %q, want tenant-123", store.savedTenants[0])
	}
}

func TestHandleGenericWebhook_RequiresOperationID(t *testing.T) {
	t.Parallel()

	store := &fakeWebhookStore{}
	handler := handleGenericWebhookWithTenantResolver(store, testutil.TestSigner(t), "route-secret", func(context.Context, string) (string, error) {
		return "tenant-123", nil
	})

	req := httptest.NewRequest("POST", "/v1/hooks/generic", strings.NewReader(`{
		"event_type":"operation_started",
		"tool":"kubectl",
		"operation":"apply",
		"environment":"production",
		"actor":"ci"
	}`))
	req.Header.Set("Authorization", "Bearer route-secret")
	req.Header.Set("X-Evidra-API-Key", "tenant-api-key")
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
}

func TestHandleGenericWebhook_UsesOperationIDForLifecycleCorrelation(t *testing.T) {
	t.Parallel()

	store := &fakeWebhookStore{}
	handler := handleGenericWebhookWithTenantResolver(store, testutil.TestSigner(t), "route-secret", func(context.Context, string) (string, error) {
		return "tenant-123", nil
	})

	startReq := httptest.NewRequest("POST", "/v1/hooks/generic", strings.NewReader(`{
		"event_type":"operation_started",
		"tool":"kubectl",
		"operation":"apply",
		"operation_id":"op-123",
		"environment":"production",
		"actor":"ci",
		"session_id":"sess-1"
	}`))
	startReq.Header.Set("Authorization", "Bearer route-secret")
	startReq.Header.Set("X-Evidra-API-Key", "tenant-api-key")
	startReq.Header.Set("Content-Type", "application/json")
	startRec := httptest.NewRecorder()

	handler.ServeHTTP(startRec, startReq)

	if startRec.Code != http.StatusAccepted {
		t.Fatalf("start expected 202, got %d", startRec.Code)
	}

	completeReq := httptest.NewRequest("POST", "/v1/hooks/generic", strings.NewReader(`{
		"event_type":"operation_completed",
		"tool":"kubectl",
		"operation":"apply",
		"operation_id":"op-123",
		"environment":"production",
		"actor":"ci",
		"idempotency_key":"evt-123",
		"verdict":"success",
		"exit_code":0
	}`))
	completeReq.Header.Set("Authorization", "Bearer route-secret")
	completeReq.Header.Set("X-Evidra-API-Key", "tenant-api-key")
	completeReq.Header.Set("Content-Type", "application/json")
	completeRec := httptest.NewRecorder()

	handler.ServeHTTP(completeRec, completeReq)

	if completeRec.Code != http.StatusAccepted {
		t.Fatalf("complete expected 202, got %d", completeRec.Code)
	}
	if len(store.savedRaw) != 2 {
		t.Fatalf("saved entries = %d, want 2", len(store.savedRaw))
	}

	var prescribe evidence.EvidenceEntry
	if err := json.Unmarshal(store.savedRaw[0], &prescribe); err != nil {
		t.Fatalf("decode prescribe entry: %v", err)
	}
	var report evidence.EvidenceEntry
	if err := json.Unmarshal(store.savedRaw[1], &report); err != nil {
		t.Fatalf("decode report entry: %v", err)
	}
	var payload evidence.ReportPayload
	if err := json.Unmarshal(report.Payload, &payload); err != nil {
		t.Fatalf("decode report payload: %v", err)
	}

	if payload.PrescriptionID != prescribe.EntryID {
		t.Fatalf("report prescription_id = %q, want %q", payload.PrescriptionID, prescribe.EntryID)
	}

	var prescribePayload evidence.PrescriptionPayload
	if err := json.Unmarshal(prescribe.Payload, &prescribePayload); err != nil {
		t.Fatalf("decode prescribe payload: %v", err)
	}
	if prescribePayload.EffectiveRisk == "" {
		t.Fatal("mapped prescribe payload missing effective_risk")
	}
	if len(prescribePayload.RiskInputs) != 1 {
		t.Fatalf("mapped prescribe risk_inputs len = %d, want 1", len(prescribePayload.RiskInputs))
	}
	if prescribePayload.RiskInputs[0].Source != "evidra/matrix" {
		t.Fatalf("mapped prescribe risk_inputs[0].source = %q, want evidra/matrix", prescribePayload.RiskInputs[0].Source)
	}
}

func TestHandleArgoCDWebhook_UsesOperationIDForLifecycleCorrelation(t *testing.T) {
	t.Parallel()

	store := &fakeWebhookStore{}
	handler := handleArgoCDWebhookWithTenantResolver(store, testutil.TestSigner(t), "route-secret", func(context.Context, string) (string, error) {
		return "tenant-123", nil
	})

	startReq := httptest.NewRequest("POST", "/v1/hooks/argocd", strings.NewReader(`{
		"event":"sync_started",
		"app_name":"demo-app",
		"app_namespace":"production",
		"initiated_by":"argocd-bot",
		"operation_id":"argo-op-123",
		"revision":"abc123"
	}`))
	startReq.Header.Set("Authorization", "Bearer route-secret")
	startReq.Header.Set("X-Evidra-API-Key", "tenant-api-key")
	startReq.Header.Set("Content-Type", "application/json")
	startRec := httptest.NewRecorder()

	handler.ServeHTTP(startRec, startReq)

	if startRec.Code != http.StatusAccepted {
		t.Fatalf("start expected 202, got %d", startRec.Code)
	}

	completeReq := httptest.NewRequest("POST", "/v1/hooks/argocd", strings.NewReader(`{
		"event":"sync_completed",
		"app_name":"demo-app",
		"app_namespace":"production",
		"initiated_by":"argocd-bot",
		"operation_id":"argo-op-123",
		"phase":"Succeeded",
		"revision":"abc123"
	}`))
	completeReq.Header.Set("Authorization", "Bearer route-secret")
	completeReq.Header.Set("X-Evidra-API-Key", "tenant-api-key")
	completeReq.Header.Set("Content-Type", "application/json")
	completeRec := httptest.NewRecorder()

	handler.ServeHTTP(completeRec, completeReq)

	if completeRec.Code != http.StatusAccepted {
		t.Fatalf("complete expected 202, got %d", completeRec.Code)
	}
	if len(store.savedRaw) != 2 {
		t.Fatalf("saved entries = %d, want 2", len(store.savedRaw))
	}

	var prescribe evidence.EvidenceEntry
	if err := json.Unmarshal(store.savedRaw[0], &prescribe); err != nil {
		t.Fatalf("decode prescribe entry: %v", err)
	}
	var report evidence.EvidenceEntry
	if err := json.Unmarshal(store.savedRaw[1], &report); err != nil {
		t.Fatalf("decode report entry: %v", err)
	}
	var payload evidence.ReportPayload
	if err := json.Unmarshal(report.Payload, &payload); err != nil {
		t.Fatalf("decode report payload: %v", err)
	}

	if payload.PrescriptionID != prescribe.EntryID {
		t.Fatalf("report prescription_id = %q, want %q", payload.PrescriptionID, prescribe.EntryID)
	}
}

type fakeWebhookStore struct {
	savedTenants []string
	savedRaw     []json.RawMessage
}

func (f *fakeWebhookStore) LastHash(context.Context, string) (string, error) {
	return "", nil
}

func (f *fakeWebhookStore) SaveRaw(_ context.Context, tenantID string, raw json.RawMessage) (string, error) {
	f.savedTenants = append(f.savedTenants, tenantID)
	f.savedRaw = append(f.savedRaw, append(json.RawMessage(nil), raw...))
	if len(raw) == 0 {
		return "", errors.New("empty payload")
	}
	return "receipt-1", nil
}

func (f *fakeWebhookStore) ClaimWebhookEvent(context.Context, string, string, string, json.RawMessage) (bool, error) {
	return false, nil
}

func (f *fakeWebhookStore) ReleaseWebhookEvent(context.Context, string, string, string) error {
	return nil
}
