package api

import (
	"crypto/ed25519"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
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

func TestRouter_ScorecardExperimentalReturns501(t *testing.T) {
	t.Parallel()

	router := NewRouter(RouterConfig{
		APIKey:        "test-key",
		DefaultTenant: "t1",
		Scorecard:     ExperimentalAnalytics{},
	})

	req := httptest.NewRequest("GET", "/v1/evidence/scorecard", nil)
	req.Header.Set("Authorization", "Bearer test-key")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotImplemented {
		t.Fatalf("expected 501, got %d", rec.Code)
	}

	var body map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if body["error"] == "" {
		t.Fatalf("missing error body: %q", rec.Body.String())
	}
	if !strings.Contains(body["error"], "experimental") || !strings.Contains(body["error"], "CLI/MCP") {
		t.Fatalf("unexpected error body: %q", body["error"])
	}
}

func TestRouter_ExplainExperimentalReturns501(t *testing.T) {
	t.Parallel()

	router := NewRouter(RouterConfig{
		APIKey:        "test-key",
		DefaultTenant: "t1",
		Explain:       ExperimentalAnalytics{},
	})

	req := httptest.NewRequest("GET", "/v1/evidence/explain", nil)
	req.Header.Set("Authorization", "Bearer test-key")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotImplemented {
		t.Fatalf("expected 501, got %d", rec.Code)
	}

	var body map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if body["error"] == "" {
		t.Fatalf("missing error body: %q", rec.Body.String())
	}
	if !strings.Contains(body["error"], "experimental") || !strings.Contains(body["error"], "CLI/MCP") {
		t.Fatalf("unexpected error body: %q", body["error"])
	}
}
