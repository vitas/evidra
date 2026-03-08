package auth

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/json"
	"math/big"
	"net/http"
	"strings"
	"time"
)

// StaticKeyMiddleware authenticates requests using a single static API key.
// Sets tenant ID to defaultTenant for all authenticated requests.
func StaticKeyMiddleware(apiKey, defaultTenant string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			token := extractBearerToken(r.Header.Get("Authorization"))
			if token == "" || subtle.ConstantTimeCompare([]byte(token), []byte(apiKey)) != 1 {
				authFail(w)
				return
			}
			ctx := WithTenantID(r.Context(), defaultTenant)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

func extractBearerToken(header string) string {
	const prefix = "Bearer "
	if !strings.HasPrefix(header, prefix) {
		return ""
	}
	return header[len(prefix):]
}

func authFail(w http.ResponseWriter) {
	jitterSleep()
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusUnauthorized)
	json.NewEncoder(w).Encode(map[string]string{"error": "unauthorized"})
}

// jitterSleep adds 50-100ms random delay to prevent timing attacks.
func jitterSleep() {
	n, err := rand.Int(rand.Reader, big.NewInt(50))
	if err != nil {
		time.Sleep(75 * time.Millisecond)
		return
	}
	time.Sleep(time.Duration(50+n.Int64()) * time.Millisecond)
}

// KeyStoreAuthFunc is a function that looks up a key and returns a tenant ID.
type KeyStoreAuthFunc func(plaintext string) (tenantID string, err error)

// KeyStoreMiddleware authenticates requests using a database-backed key store.
func KeyStoreMiddleware(lookup KeyStoreAuthFunc) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			token := extractBearerToken(r.Header.Get("Authorization"))
			if token == "" {
				authFail(w)
				return
			}
			tenantID, err := lookup(token)
			if err != nil {
				authFail(w)
				return
			}
			ctx := WithTenantID(r.Context(), tenantID)
			w.Header().Set("X-Evidra-Tenant", tenantID)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// AuthCheckHandler returns 200 with X-Evidra-Tenant for valid tokens (forwardAuth target).
func AuthCheckHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		tid := TenantID(r.Context())
		if tid == "" {
			authFail(w)
			return
		}
		w.Header().Set("X-Evidra-Tenant", tid)
		if r.Method == http.MethodHead {
			w.WriteHeader(http.StatusOK)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"ok": "true", "tenant_id": tid})
	}
}

// ExtractTenantHeader reads X-Evidra-Tenant for downstream use (e.g., after forwardAuth).
func ExtractTenantHeader(header string) string {
	return strings.TrimSpace(header)
}
