package api

import (
	"net/http/httptest"
	"testing"
)

func TestClientIP_UsesForwardedForOnlyFromTrustedProxy(t *testing.T) {
	t.Parallel()

	req := httptest.NewRequest("POST", "/v1/keys", nil)
	req.RemoteAddr = "127.0.0.1:1234"
	req.Header.Set("X-Forwarded-For", "198.51.100.20, 10.0.0.5")

	if got := clientIP(req); got != "198.51.100.20" {
		t.Fatalf("clientIP = %q, want 198.51.100.20", got)
	}
}

func TestClientIP_IgnoresForwardedForFromUntrustedPeer(t *testing.T) {
	t.Parallel()

	req := httptest.NewRequest("POST", "/v1/keys", nil)
	req.RemoteAddr = "203.0.113.10:1234"
	req.Header.Set("X-Forwarded-For", "198.51.100.20")

	if got := clientIP(req); got != "203.0.113.10" {
		t.Fatalf("clientIP = %q, want 203.0.113.10", got)
	}
}
