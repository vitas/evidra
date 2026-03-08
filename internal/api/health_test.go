package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http/httptest"
	"testing"
)

type fakePinger struct{ err error }

func (f *fakePinger) Ping(ctx context.Context) error { return f.err }

func TestHealthz(t *testing.T) {
	t.Parallel()
	req := httptest.NewRequest("GET", "/healthz", nil)
	rec := httptest.NewRecorder()
	handleHealthz(rec, req)

	if rec.Code != 200 {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	var body map[string]string
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if body["status"] != "ok" {
		t.Fatalf("expected status=ok, got %s", body["status"])
	}
}

func TestReadyz_Healthy(t *testing.T) {
	t.Parallel()
	handler := handleReadyz(&fakePinger{})
	req := httptest.NewRequest("GET", "/readyz", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != 200 {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
}

func TestReadyz_Unhealthy(t *testing.T) {
	t.Parallel()
	handler := handleReadyz(&fakePinger{err: errors.New("db down")})
	req := httptest.NewRequest("GET", "/readyz", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != 503 {
		t.Fatalf("expected 503, got %d", rec.Code)
	}
}
