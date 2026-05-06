package api

import (
	"crypto/ed25519"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestRouter_HealthzNoAuth(t *testing.T) {
	t.Parallel()
	cfg := RouterConfig{
		APIKey:        "test-key",
		DefaultTenant: "t1",
	}
	router := NewRouter(cfg)

	req := httptest.NewRequest("GET", "/healthz", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
}

func TestRouter_PubkeyNoAuth(t *testing.T) {
	t.Parallel()
	pub, _, _ := ed25519.GenerateKey(nil)
	cfg := RouterConfig{
		APIKey:        "test-key",
		DefaultTenant: "t1",
		PublicKey:     pub,
	}
	router := NewRouter(cfg)

	req := httptest.NewRequest("GET", "/v1/evidence/pubkey", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
}

func TestRouter_ForwardRequiresAuth(t *testing.T) {
	t.Parallel()
	cfg := RouterConfig{
		APIKey:        "test-key",
		DefaultTenant: "t1",
		RawStore:      &fakeEntryStore{},
	}
	router := NewRouter(cfg)

	req := httptest.NewRequest("POST", "/v1/evidence/forward", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rec.Code)
	}
}
