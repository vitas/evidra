//go:build integration

package api_test

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"samebits.com/evidra-benchmark/internal/api"
	"samebits.com/evidra-benchmark/internal/db"
	"samebits.com/evidra-benchmark/internal/store"
)

func TestIntegration_FullLifecycle(t *testing.T) {
	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		t.Skip("DATABASE_URL not set — skipping integration test")
	}

	pool, err := db.Connect(databaseURL)
	if err != nil {
		t.Fatalf("db.Connect: %v", err)
	}
	defer pool.Close()

	es := store.NewEntryStore(pool)
	ks := store.NewKeyStore(pool)

	const testAPIKey = "integration-test-key"

	router := api.NewRouter(api.RouterConfig{
		APIKey:        testAPIKey,
		DefaultTenant: "test-tenant",
		EntryStore:    es,
		RawStore:      es,
		KeyStore:      ks,
		Pinger:        pool,
	})

	srv := httptest.NewServer(router)
	defer srv.Close()

	t.Run("healthz", func(t *testing.T) {
		resp, err := http.Get(srv.URL + "/healthz")
		if err != nil {
			t.Fatalf("GET /healthz: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			t.Fatalf("expected 200, got %d", resp.StatusCode)
		}

		var body map[string]string
		if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
			t.Fatalf("decode: %v", err)
		}
		if body["status"] != "ok" {
			t.Fatalf("expected status ok, got %q", body["status"])
		}
	})

	t.Run("readyz", func(t *testing.T) {
		resp, err := http.Get(srv.URL + "/readyz")
		if err != nil {
			t.Fatalf("GET /readyz: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			t.Fatalf("expected 200, got %d", resp.StatusCode)
		}

		var body map[string]string
		if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
			t.Fatalf("decode: %v", err)
		}
		if body["status"] != "ok" {
			t.Fatalf("expected status ok, got %q", body["status"])
		}
	})

	t.Run("forward_requires_auth", func(t *testing.T) {
		payload := `{"type":"prescription","tool":"kubectl","operation":"apply"}`
		resp, err := http.Post(srv.URL+"/v1/evidence/forward", "application/json", strings.NewReader(payload))
		if err != nil {
			t.Fatalf("POST /v1/evidence/forward: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusUnauthorized {
			body, _ := io.ReadAll(resp.Body)
			t.Fatalf("expected 401, got %d: %s", resp.StatusCode, body)
		}
	})

	t.Run("forward_with_auth", func(t *testing.T) {
		payload := `{"type":"prescription","tool":"kubectl","operation":"apply"}`
		req, err := http.NewRequest("POST", srv.URL+"/v1/evidence/forward", strings.NewReader(payload))
		if err != nil {
			t.Fatalf("new request: %v", err)
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+testAPIKey)

		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("POST /v1/evidence/forward: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			body, _ := io.ReadAll(resp.Body)
			t.Fatalf("expected 200, got %d: %s", resp.StatusCode, body)
		}

		var body map[string]interface{}
		if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
			t.Fatalf("decode: %v", err)
		}

		if body["receipt_id"] == nil || body["receipt_id"] == "" {
			t.Fatal("expected non-empty receipt_id")
		}
		if body["status"] != "accepted" {
			t.Fatalf("expected status accepted, got %v", body["status"])
		}
	})
}
