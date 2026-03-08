package client

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestNew_DefaultTimeout(t *testing.T) {
	t.Parallel()
	c := New(Config{URL: "http://localhost", APIKey: "key"})
	if c.URL() != "http://localhost" {
		t.Fatalf("expected URL=http://localhost, got %s", c.URL())
	}
}

func TestForward_Success(t *testing.T) {
	t.Parallel()
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer test-key" {
			t.Error("missing auth header")
		}
		if r.URL.Path != "/v1/evidence/forward" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"receipt_id": "r1",
			"status":     "accepted",
		})
	}))
	defer ts.Close()

	c := New(Config{URL: ts.URL, APIKey: "test-key"})
	resp, err := c.Forward(context.Background(), json.RawMessage(`{"type":"prescribe"}`))
	if err != nil {
		t.Fatalf("Forward: %v", err)
	}
	if resp.ReceiptID != "r1" {
		t.Fatalf("expected receipt_id=r1, got %s", resp.ReceiptID)
	}
}

func TestForward_Unauthorized(t *testing.T) {
	t.Parallel()
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(401)
	}))
	defer ts.Close()

	c := New(Config{URL: ts.URL, APIKey: "bad"})
	_, err := c.Forward(context.Background(), json.RawMessage(`{}`))
	if !errors.Is(err, ErrUnauthorized) {
		t.Fatalf("expected ErrUnauthorized, got %v", err)
	}
}

func TestPing_Success(t *testing.T) {
	t.Parallel()
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
	}))
	defer ts.Close()

	c := New(Config{URL: ts.URL, APIKey: "key"})
	if err := c.Ping(context.Background()); err != nil {
		t.Fatalf("Ping: %v", err)
	}
}

func TestIsReachabilityError(t *testing.T) {
	t.Parallel()
	if !IsReachabilityError(ErrUnreachable) {
		t.Error("ErrUnreachable should be reachability error")
	}
	if !IsReachabilityError(ErrServerError) {
		t.Error("ErrServerError should be reachability error")
	}
	if IsReachabilityError(ErrUnauthorized) {
		t.Error("ErrUnauthorized should NOT be reachability error")
	}
}
