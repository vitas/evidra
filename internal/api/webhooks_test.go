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

type fakeWebhookStore struct {
	savedTenants []string
}

func (f *fakeWebhookStore) LastHash(context.Context, string) (string, error) {
	return "", nil
}

func (f *fakeWebhookStore) SaveRaw(_ context.Context, tenantID string, raw json.RawMessage) (string, error) {
	f.savedTenants = append(f.savedTenants, tenantID)
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
