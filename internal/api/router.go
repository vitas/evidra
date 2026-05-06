package api

import (
	"context"
	"crypto/ed25519"
	"crypto/subtle"
	"io/fs"
	"net/http"

	iauth "samebits.com/evidra/internal/auth"
	"samebits.com/evidra/internal/ingest"
	"samebits.com/evidra/internal/store"
	pkevidence "samebits.com/evidra/pkg/evidence"
)

// RouterConfig holds dependencies for the API router.
type RouterConfig struct {
	APIKey        string
	DefaultTenant string
	PublicKey     ed25519.PublicKey
	EntryStore    *store.EntryStore
	Ingest        IngestPort
	KeyStore      *store.KeyStore
	RawStore      RawEntryStore
	Scorecard     ScorecardComputer
	Explain       ExplainComputer
	InviteSecret  string
	Pinger        Pinger
	UIFS          fs.FS // Embedded landing page filesystem
	WebhookStore  WebhookStore
	WebhookSigner pkevidence.Signer
	ArgoCDSecret  string
	GenericSecret string
}

// NewRouter creates the HTTP handler with all routes and middleware.
func NewRouter(cfg RouterConfig) http.Handler {
	mux := http.NewServeMux()

	// Public routes (no auth).
	mux.HandleFunc("GET /healthz", handleHealthz)
	if cfg.Pinger != nil {
		mux.Handle("GET /readyz", handleReadyz(cfg.Pinger))
	}
	if cfg.PublicKey != nil {
		mux.Handle("GET /v1/evidence/pubkey", handlePubkey(cfg.PublicKey))
	}

	// Key issuance (gated, not behind standard auth).
	mux.Handle("POST /v1/keys", handleKeys(cfg.KeyStore, cfg.InviteSecret))
	if cfg.WebhookStore != nil && cfg.ArgoCDSecret != "" {
		if cfg.KeyStore != nil {
			mux.Handle("POST /v1/hooks/argocd", handleArgoCDWebhookWithTenantResolver(cfg.WebhookStore, cfg.WebhookSigner, cfg.ArgoCDSecret, tenantResolverFromKeyStore(cfg.KeyStore)))
		}
	}
	if cfg.WebhookStore != nil && cfg.GenericSecret != "" {
		if cfg.KeyStore != nil {
			mux.Handle("POST /v1/hooks/generic", handleGenericWebhookWithTenantResolver(cfg.WebhookStore, cfg.WebhookSigner, cfg.GenericSecret, tenantResolverFromKeyStore(cfg.KeyStore)))
		}
	}

	// Authenticated routes.
	authMw := iauth.StaticKeyMiddleware(cfg.APIKey, cfg.DefaultTenant)
	if cfg.KeyStore != nil {
		authMw = iauth.KeyStoreMiddleware(func(ctx context.Context, token string) (string, error) {
			// Keep static key valid for Phase 0 compatibility.
			if subtle.ConstantTimeCompare([]byte(token), []byte(cfg.APIKey)) == 1 {
				return cfg.DefaultTenant, nil
			}

			rec, err := cfg.KeyStore.LookupKey(ctx, token)
			if err != nil {
				return "", err
			}
			return rec.TenantID, nil
		})
	}

	// Auth check (forwardAuth target).
	mux.Handle("GET /auth/check", authMw(iauth.AuthCheckHandler()))
	mux.Handle("HEAD /auth/check", authMw(iauth.AuthCheckHandler()))

	// Evidence ingestion.
	if cfg.RawStore != nil {
		mux.Handle("POST /v1/evidence/forward", authMw(handleForward(cfg.RawStore)))
		mux.Handle("POST /v1/evidence/batch", authMw(handleBatch(cfg.RawStore)))
		mux.Handle("POST /v1/evidence/findings", authMw(handleFindings(cfg.RawStore)))
	}
	ingestSvc := cfg.Ingest
	if ingestSvc == nil && cfg.EntryStore != nil {
		ingestSvc = ingest.NewService(cfg.EntryStore, cfg.WebhookSigner)
	}
	if ingestSvc != nil {
		mux.Handle("POST /v1/evidence/ingest/prescribe", authMw(handleIngestPrescribe(ingestSvc)))
		mux.Handle("POST /v1/evidence/ingest/report", authMw(handleIngestReport(ingestSvc)))
	}

	// Evidence queries.
	if cfg.EntryStore != nil {
		mux.Handle("GET /v1/evidence/entries", authMw(handleListEntries(cfg.EntryStore)))
		mux.Handle("GET /v1/evidence/entries/{id}", authMw(handleGetEntry(cfg.EntryStore)))
	}

	// Analytics.
	if cfg.Scorecard != nil {
		mux.Handle("GET /v1/evidence/scorecard", authMw(handleScorecard(cfg.Scorecard)))
	}
	if cfg.Explain != nil {
		mux.Handle("GET /v1/evidence/explain", authMw(handleExplain(cfg.Explain)))
	}

	// Embedded landing page.
	if cfg.UIFS != nil {
		mux.Handle("/", uiHandler(cfg.UIFS))
	}

	return wrapMiddleware(mux)
}
