package auth

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestExtractBearerToken(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		header string
		want   string
	}{
		{"valid", "Bearer abc123", "abc123"},
		{"no prefix", "abc123", ""},
		{"empty", "", ""},
		{"lowercase", "bearer abc123", ""},
		{"extra spaces", "Bearer  abc123", " abc123"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := extractBearerToken(tt.header)
			if got != tt.want {
				t.Errorf("extractBearerToken(%q) = %q, want %q", tt.header, got, tt.want)
			}
		})
	}
}

func TestStaticKeyMiddleware_ValidKey(t *testing.T) {
	t.Parallel()
	handler := StaticKeyMiddleware("test-key", "default-tenant")(
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			tid := TenantID(r.Context())
			w.Write([]byte(tid))
		}),
	)

	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("Authorization", "Bearer test-key")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != 200 {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	if rec.Body.String() != "default-tenant" {
		t.Fatalf("expected tenant=default-tenant, got %s", rec.Body.String())
	}
}

func TestStaticKeyMiddleware_NoAuth(t *testing.T) {
	t.Parallel()
	handler := StaticKeyMiddleware("test-key", "t1")(
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}),
	)

	req := httptest.NewRequest("GET", "/", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != 401 {
		t.Fatalf("expected 401, got %d", rec.Code)
	}
}

func TestStaticKeyMiddleware_WrongKey(t *testing.T) {
	t.Parallel()
	handler := StaticKeyMiddleware("correct-key", "t1")(
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}),
	)

	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("Authorization", "Bearer wrong-key")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != 401 {
		t.Fatalf("expected 401, got %d", rec.Code)
	}
}
