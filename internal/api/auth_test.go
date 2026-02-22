package api

import (
	"bytes"
	"crypto/hmac"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"evidra/internal/export"
	"evidra/internal/store"

	jwt "github.com/golang-jwt/jwt/v4"
)

func TestReadTokenAuth(t *testing.T) {
	repo := store.NewMemoryRepository()
	exporter := export.NewFilesystemExporter(t.TempDir())
	h := NewServerWithOptions(repo, exporter, ServerOptions{Auth: AuthConfig{
		Read: BearerPolicy{Token: "read-token"},
	}}).Routes()

	req := httptest.NewRequest(http.MethodGet, "/v1/subjects", nil)
	res := httptest.NewRecorder()
	h.ServeHTTP(res, req)
	if res.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 without token, got %d", res.Code)
	}

	req = httptest.NewRequest(http.MethodGet, "/v1/subjects", nil)
	req.Header.Set("Authorization", "Bearer read-token")
	res = httptest.NewRecorder()
	h.ServeHTTP(res, req)
	if res.Code != http.StatusOK {
		t.Fatalf("expected 200 with token, got %d", res.Code)
	}
}

func TestIngestTokenAndWebhookSignatureAuth(t *testing.T) {
	repo := store.NewMemoryRepository()
	exporter := export.NewFilesystemExporter(t.TempDir())
	h := NewServerWithOptions(repo, exporter, ServerOptions{Auth: AuthConfig{
		Ingest: IngestPolicy{
			Bearer:         BearerPolicy{Token: "ingest-token"},
			GenericWebhook: HMACPolicy{Secret: "secret-123"},
		},
	}}).Routes()

	body, _ := json.Marshal(map[string]interface{}{
		"specversion": "1.0",
		"id":          "evt_auth_1",
		"source":      "git",
		"type":        "pull_request_merged",
		"time":        "2026-02-16T12:00:00Z",
		"subject":     "payments-api",
		"cluster":     "eu-1",
		"namespace":   "prod-eu",
		"initiator":   "jane.doe",
		"commit_sha":  "abc123",
		"data":        map[string]interface{}{"repo": "org/payments"},
	})

	req := httptest.NewRequest(http.MethodPost, "/v1/events", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/cloudevents+json")
	res := httptest.NewRecorder()
	h.ServeHTTP(res, req)
	if res.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 without auth, got %d", res.Code)
	}

	req = httptest.NewRequest(http.MethodPost, "/v1/events", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/cloudevents+json")
	req.Header.Set("Authorization", "Bearer ingest-token")
	req.Header.Set("X-Evidra-Signature", "sha256=deadbeef")
	res = httptest.NewRecorder()
	h.ServeHTTP(res, req)
	if res.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 with invalid signature, got %d", res.Code)
	}

	req = httptest.NewRequest(http.MethodPost, "/v1/events", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/cloudevents+json")
	req.Header.Set("Authorization", "Bearer ingest-token")
	req.Header.Set("X-Evidra-Signature", validSig("secret-123", body))
	res = httptest.NewRecorder()
	h.ServeHTTP(res, req)
	if res.Code != http.StatusAccepted {
		t.Fatalf("expected 202 with valid auth, got %d body=%s", res.Code, res.Body.String())
	}
}

func TestIngestFallsBackToReadTokenWhenIngestTokenUnset(t *testing.T) {
	repo := store.NewMemoryRepository()
	exporter := export.NewFilesystemExporter(t.TempDir())
	h := NewServerWithOptions(repo, exporter, ServerOptions{Auth: AuthConfig{
		Read: BearerPolicy{Token: "read-token"},
	}}).Routes()

	body, _ := json.Marshal(map[string]interface{}{
		"specversion": "1.0",
		"id":          "evt_auth_fallback_1",
		"source":      "argocd",
		"type":        "sync_finished",
		"time":        "2026-02-16T12:00:00Z",
		"subject":     "payments-api",
		"cluster":     "eu-1",
		"namespace":   "prod-eu",
		"initiator":   "jane.doe",
		"revision":    "abc123",
		"data":        map[string]interface{}{"phase": "Succeeded"},
	})

	req := httptest.NewRequest(http.MethodPost, "/v1/events", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/cloudevents+json")
	res := httptest.NewRecorder()
	h.ServeHTTP(res, req)
	if res.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 without bearer token, got %d", res.Code)
	}

	req = httptest.NewRequest(http.MethodPost, "/v1/events", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/cloudevents+json")
	req.Header.Set("Authorization", "Bearer read-token")
	res = httptest.NewRecorder()
	h.ServeHTTP(res, req)
	if res.Code != http.StatusAccepted {
		t.Fatalf("expected 202 with read token fallback, got %d", res.Code)
	}
}

func validSig(secret string, body []byte) string {
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write(body)
	return "sha256=" + hex.EncodeToString(mac.Sum(nil))
}

func TestOIDCRoleAuthReadAndExport(t *testing.T) {
	repo := store.NewMemoryRepository()
	exporter := export.NewFilesystemExporter(t.TempDir())
	h := NewServerWithOptions(repo, exporter, ServerOptions{Auth: AuthConfig{
		OIDC: OIDCPolicy{
			Enabled: true,
		},
	}}).Routes()

	req := httptest.NewRequest(http.MethodGet, "/v1/subjects", nil)
	res := httptest.NewRecorder()
	h.ServeHTTP(res, req)
	if res.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 without oidc roles, got %d", res.Code)
	}

	req = httptest.NewRequest(http.MethodGet, "/v1/subjects", nil)
	req.Header.Set("X-Auth-Roles", "reader")
	res = httptest.NewRecorder()
	h.ServeHTTP(res, req)
	if res.Code != http.StatusOK {
		t.Fatalf("expected 200 with reader role, got %d", res.Code)
	}

	req = httptest.NewRequest(http.MethodPost, "/v1/exports", strings.NewReader(`{"format":"json","filter":{}}`))
	req.Header.Set("X-Auth-Roles", "reader")
	res = httptest.NewRecorder()
	h.ServeHTTP(res, req)
	if res.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 for export with reader role, got %d", res.Code)
	}

	req = httptest.NewRequest(http.MethodPost, "/v1/exports", strings.NewReader(`{"format":"json","filter":{}}`))
	req.Header.Set("X-Auth-Roles", "exporter")
	res = httptest.NewRecorder()
	h.ServeHTTP(res, req)
	if res.Code != http.StatusAccepted {
		t.Fatalf("expected 202 for export with exporter role, got %d", res.Code)
	}
}

func TestOIDCRoleAuthIngestAdmin(t *testing.T) {
	repo := store.NewMemoryRepository()
	exporter := export.NewFilesystemExporter(t.TempDir())
	h := NewServerWithOptions(repo, exporter, ServerOptions{Auth: AuthConfig{
		OIDC: OIDCPolicy{
			Enabled: true,
		},
	}}).Routes()

	body, _ := json.Marshal(map[string]interface{}{
		"specversion": "1.0",
		"id":          "evt_auth_oidc_admin_1",
		"source":      "git",
		"type":        "pull_request_merged",
		"time":        "2026-02-16T12:00:00Z",
		"subject":     "payments-api",
		"cluster":     "eu-1",
		"namespace":   "prod-eu",
		"initiator":   "jane.doe",
		"commit_sha":  "abc123",
		"data":        map[string]interface{}{"repo": "org/payments"},
	})

	req := httptest.NewRequest(http.MethodPost, "/v1/events", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/cloudevents+json")
	req.Header.Set("X-Auth-Roles", "reader")
	res := httptest.NewRecorder()
	h.ServeHTTP(res, req)
	if res.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 for ingest with non-admin role, got %d", res.Code)
	}

	req = httptest.NewRequest(http.MethodPost, "/v1/events", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/cloudevents+json")
	req.Header.Set("X-Auth-Roles", "admin")
	res = httptest.NewRecorder()
	h.ServeHTTP(res, req)
	if res.Code != http.StatusAccepted {
		t.Fatalf("expected 202 for ingest with admin role, got %d", res.Code)
	}
}

func TestJWTRoleAuthReadAndExport(t *testing.T) {
	repo := store.NewMemoryRepository()
	exporter := export.NewFilesystemExporter(t.TempDir())
	h := NewServerWithOptions(repo, exporter, ServerOptions{Auth: AuthConfig{
		JWT: JWTPolicy{
			Enabled:     true,
			Issuer:      "https://issuer.local",
			Audience:    "evidra",
			RolesClaim:  "roles",
			HS256Secret: "jwt-secret",
		},
	}}).Routes()

	tokenReader := mustMakeJWT(t, "jwt-secret", "https://issuer.local", "evidra", "user-1", []string{"reader"})
	req := httptest.NewRequest(http.MethodGet, "/v1/subjects", nil)
	req.Header.Set("Authorization", "Bearer "+tokenReader)
	res := httptest.NewRecorder()
	h.ServeHTTP(res, req)
	if res.Code != http.StatusOK {
		t.Fatalf("expected 200 with reader jwt, got %d", res.Code)
	}

	req = httptest.NewRequest(http.MethodPost, "/v1/exports", strings.NewReader(`{"format":"json","filter":{}}`))
	req.Header.Set("Authorization", "Bearer "+tokenReader)
	res = httptest.NewRecorder()
	h.ServeHTTP(res, req)
	if res.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 export with reader jwt, got %d", res.Code)
	}

	tokenExporter := mustMakeJWT(t, "jwt-secret", "https://issuer.local", "evidra", "user-2", []string{"exporter"})
	req = httptest.NewRequest(http.MethodPost, "/v1/exports", strings.NewReader(`{"format":"json","filter":{}}`))
	req.Header.Set("Authorization", "Bearer "+tokenExporter)
	res = httptest.NewRecorder()
	h.ServeHTTP(res, req)
	if res.Code != http.StatusAccepted {
		t.Fatalf("expected 202 export with exporter jwt, got %d", res.Code)
	}
}

func TestJWTRoleAuthIngestAdmin(t *testing.T) {
	repo := store.NewMemoryRepository()
	exporter := export.NewFilesystemExporter(t.TempDir())
	h := NewServerWithOptions(repo, exporter, ServerOptions{Auth: AuthConfig{
		JWT: JWTPolicy{
			Enabled:     true,
			Issuer:      "https://issuer.local",
			Audience:    "evidra",
			RolesClaim:  "roles",
			HS256Secret: "jwt-secret",
		},
	}}).Routes()

	body, _ := json.Marshal(map[string]interface{}{
		"specversion": "1.0",
		"id":          "evt_auth_jwt_admin_1",
		"source":      "git",
		"type":        "pull_request_merged",
		"time":        "2026-02-16T12:00:00Z",
		"subject":     "payments-api",
		"cluster":     "eu-1",
		"namespace":   "prod-eu",
		"initiator":   "jane.doe",
		"commit_sha":  "abc123",
		"data":        map[string]interface{}{"repo": "org/payments"},
	})

	tokenReader := mustMakeJWT(t, "jwt-secret", "https://issuer.local", "evidra", "user-1", []string{"reader"})
	req := httptest.NewRequest(http.MethodPost, "/v1/events", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/cloudevents+json")
	req.Header.Set("Authorization", "Bearer "+tokenReader)
	res := httptest.NewRecorder()
	h.ServeHTTP(res, req)
	if res.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 ingest with non-admin jwt, got %d", res.Code)
	}

	tokenAdmin := mustMakeJWT(t, "jwt-secret", "https://issuer.local", "evidra", "admin-1", []string{"admin"})
	req = httptest.NewRequest(http.MethodPost, "/v1/events", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/cloudevents+json")
	req.Header.Set("Authorization", "Bearer "+tokenAdmin)
	res = httptest.NewRecorder()
	h.ServeHTTP(res, req)
	if res.Code != http.StatusAccepted {
		t.Fatalf("expected 202 ingest with admin jwt, got %d", res.Code)
	}
}

func TestAuthRateLimitRead(t *testing.T) {
	repo := store.NewMemoryRepository()
	exporter := export.NewFilesystemExporter(t.TempDir())
	h := NewServerWithOptions(repo, exporter, ServerOptions{Auth: AuthConfig{
		Rate: RateLimitPolicy{
			Enabled:       true,
			ReadPerMinute: 1,
		},
	}}).Routes()

	req := httptest.NewRequest(http.MethodGet, "/v1/subjects", nil)
	req.RemoteAddr = "10.0.0.10:1234"
	res := httptest.NewRecorder()
	h.ServeHTTP(res, req)
	if res.Code != http.StatusOK {
		t.Fatalf("expected first request to pass, got %d", res.Code)
	}

	req = httptest.NewRequest(http.MethodGet, "/v1/subjects", nil)
	req.RemoteAddr = "10.0.0.10:1234"
	res = httptest.NewRecorder()
	h.ServeHTTP(res, req)
	if res.Code != http.StatusTooManyRequests {
		t.Fatalf("expected second request to be rate limited, got %d", res.Code)
	}
}

func TestJWTRoleAuthReadWithJWKS(t *testing.T) {
	repo := store.NewMemoryRepository()
	exporter := export.NewFilesystemExporter(t.TempDir())

	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate rsa key: %v", err)
	}
	pub := privateKey.PublicKey
	kid := "kid-1"
	jwks := fmt.Sprintf(`{"keys":[{"kty":"RSA","kid":"%s","n":"%s","e":"%s"}]}`,
		kid,
		base64.RawURLEncoding.EncodeToString(pub.N.Bytes()),
		base64.RawURLEncoding.EncodeToString(bigEndianBytes(pub.E)),
	)
	jwksServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(jwks))
	}))
	defer jwksServer.Close()

	h := NewServerWithOptions(repo, exporter, ServerOptions{Auth: AuthConfig{
		JWT: JWTPolicy{
			Enabled:     true,
			Issuer:      "https://issuer.local",
			Audience:    "evidra",
			RolesClaim:  "roles",
			JWKSURL:     jwksServer.URL,
			JWKSRefresh: "1m",
		},
	}}).Routes()

	token := jwt.NewWithClaims(jwt.SigningMethodRS256, jwt.MapClaims{
		"iss":   "https://issuer.local",
		"aud":   "evidra",
		"sub":   "user-1",
		"exp":   time.Now().Add(10 * time.Minute).Unix(),
		"iat":   time.Now().Add(-1 * time.Minute).Unix(),
		"roles": []string{"reader"},
	})
	token.Header["kid"] = kid
	raw, err := token.SignedString(privateKey)
	if err != nil {
		t.Fatalf("sign rs256 jwt: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/v1/subjects", nil)
	req.Header.Set("Authorization", "Bearer "+raw)
	res := httptest.NewRecorder()
	h.ServeHTTP(res, req)
	if res.Code != http.StatusOK {
		t.Fatalf("expected 200 with jwks-backed rs256 token, got %d", res.Code)
	}
}

func mustMakeJWT(t *testing.T, secret, iss, aud, sub string, roles []string) string {
	t.Helper()
	claims := jwt.MapClaims{
		"iss":   iss,
		"aud":   aud,
		"sub":   sub,
		"exp":   time.Now().Add(10 * time.Minute).Unix(),
		"iat":   time.Now().Add(-1 * time.Minute).Unix(),
		"roles": roles,
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	raw, err := token.SignedString([]byte(secret))
	if err != nil {
		t.Fatalf("sign jwt: %v", err)
	}
	return raw
}

func bigEndianBytes(v int) []byte {
	if v <= 0 {
		return []byte{0}
	}
	buf := make([]byte, 0, 8)
	for n := v; n > 0; n >>= 8 {
		buf = append([]byte{byte(n & 0xff)}, buf...)
	}
	return buf
}
