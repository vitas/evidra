package auth

import (
	"context"
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
		{"lowercase", "bearer abc123", "abc123"},
		{"mixed case", "bEaReR abc123", "abc123"},
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
			_, _ = w.Write([]byte(tid))
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

func TestKeyStoreMiddleware_PassesRequestContext(t *testing.T) {
	t.Parallel()

	type ctxKey string

	handler := KeyStoreMiddleware(func(ctx context.Context, plaintext string) (string, error) {
		if plaintext != "lookup-key" {
			t.Fatalf("lookup token = %q, want %q", plaintext, "lookup-key")
		}
		if got, _ := ctx.Value(ctxKey("request_id")).(string); got != "req-123" {
			t.Fatalf("lookup context value = %q, want %q", got, "req-123")
		}
		return "tenant-123", nil
	})(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(TenantID(r.Context())))
	}))

	req := httptest.NewRequest("GET", "/", nil)
	req = req.WithContext(context.WithValue(req.Context(), ctxKey("request_id"), "req-123"))
	req.Header.Set("Authorization", "Bearer lookup-key")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	if rec.Body.String() != "tenant-123" {
		t.Fatalf("tenant = %q, want %q", rec.Body.String(), "tenant-123")
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
