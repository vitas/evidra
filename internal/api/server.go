package api

import (
	"sync"

	"evidra/internal/app"
	"evidra/internal/ingest"
	"evidra/internal/store"
)

type Exporter interface {
	app.Exporter
}

type AuthConfig struct {
	Read   BearerPolicy
	Ingest IngestPolicy
	OIDC   OIDCPolicy
	JWT    JWTPolicy
	Audit  AuditPolicy
	Rate   RateLimitPolicy
}

type BearerPolicy struct {
	Token string
}

type HMACPolicy struct {
	Header string
	Secret string
}

type IngestPolicy struct {
	Bearer         BearerPolicy
	GenericWebhook HMACPolicy
}

type OIDCPolicy struct {
	Enabled     bool
	RolesHeader string
}

type JWTPolicy struct {
	Enabled           bool
	Issuer            string
	Audience          string
	RolesClaim        string
	HS256Secret       string
	RS256PublicKeyPEM string
	JWKSURL           string
	JWKSRefresh       string
}

type AuditPolicy struct {
	LogFile string
}

type RateLimitPolicy struct {
	Enabled         bool
	ReadPerMinute   int
	ExportPerMinute int
	IngestPerMinute int
}

type ServerOptions struct {
	Auth            AuthConfig
	WebhookRegistry *ingest.Registry
	ArgoOnlyMode    bool
}

type Server struct {
	service         *app.Service
	auth            AuthConfig
	webhookRegistry *ingest.Registry
	argoOnlyMode    bool
	rateLimiter     *authRateLimiter
	jwksCache       *jwksKeyCache
	jwksMu          sync.Mutex
}

func NewServer(repo store.Repository, exporter Exporter) *Server {
	return NewServerWithOptions(repo, exporter, ServerOptions{})
}

func NewServerWithOptions(repo store.Repository, exporter Exporter, opts ServerOptions) *Server {
	reg := opts.WebhookRegistry
	if reg == nil {
		reg = ingest.NewRegistry()
	}
	auth := withAuthDefaults(opts.Auth)
	svc := app.NewService(repo, exporter)
	return &Server{
		service:         svc,
		auth:            auth,
		webhookRegistry: reg,
		argoOnlyMode:    opts.ArgoOnlyMode,
		rateLimiter:     newAuthRateLimiter(auth.Rate),
		jwksCache:       newJWKSKeyCache(auth.JWT),
	}
}
