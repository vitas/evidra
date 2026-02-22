package config

import (
	"testing"
	"time"
)

func TestLoadFromEnvArgoDefaults(t *testing.T) {
	t.Setenv("EVIDRA_DEV_INSECURE", "")
	t.Setenv("EVIDRA_ARGO_COLLECTOR_ENABLED", "")
	t.Setenv("EVIDRA_ARGO_COLLECTOR_INTERVAL", "")
	t.Setenv("EVIDRA_ARGO_API_URL", "")
	t.Setenv("EVIDRA_ARGO_API_TOKEN", "")
	t.Setenv("EVIDRA_ARGO_DEFAULT_ENV", "")
	t.Setenv("EVIDRA_K8S_COLLECTOR_ENABLED", "")
	t.Setenv("EVIDRA_K8S_COLLECTOR_INTERVAL", "")
	t.Setenv("EVIDRA_K8S_NAMESPACE", "")
	t.Setenv("EVIDRA_K8S_CLUSTER", "")
	t.Setenv("EVIDRA_K8S_ENVIRONMENT", "")
	t.Setenv("EVIDRA_K8S_KUBECONFIG", "")
	t.Setenv("EVIDRA_K8S_CONTEXT", "")
	t.Setenv("EVIDRA_AUTH_OIDC_ENABLED", "")
	t.Setenv("EVIDRA_AUTH_OIDC_ROLES_HEADER", "")
	t.Setenv("EVIDRA_AUTH_JWT_ENABLED", "")
	t.Setenv("EVIDRA_AUTH_JWT_ISSUER", "")
	t.Setenv("EVIDRA_AUTH_JWT_AUDIENCE", "")
	t.Setenv("EVIDRA_AUTH_JWT_ROLES_CLAIM", "")
	t.Setenv("EVIDRA_AUTH_JWT_HS256_SECRET", "")
	t.Setenv("EVIDRA_AUTH_JWT_RS256_PUBLIC_KEY_PEM", "")
	t.Setenv("EVIDRA_AUTH_JWT_JWKS_URL", "")
	t.Setenv("EVIDRA_AUTH_JWT_JWKS_REFRESH", "")
	t.Setenv("EVIDRA_AUTH_AUDIT_LOG_FILE", "")
	t.Setenv("EVIDRA_AUTH_RATE_LIMIT_ENABLED", "")
	t.Setenv("EVIDRA_AUTH_RATE_LIMIT_READ_PER_MIN", "")
	t.Setenv("EVIDRA_AUTH_RATE_LIMIT_EXPORT_PER_MIN", "")
	t.Setenv("EVIDRA_AUTH_RATE_LIMIT_INGEST_PER_MIN", "")
	t.Setenv("EVIDRA_DB_SSLMODE", "")
	t.Setenv("EVIDRA_DB_SSLROOTCERT", "")
	t.Setenv("EVIDRA_DB_SSLCERT", "")
	t.Setenv("EVIDRA_DB_SSLKEY", "")
	t.Setenv("EVIDRA_TLS_ENABLED", "")
	t.Setenv("EVIDRA_TLS_CERT_FILE", "")
	t.Setenv("EVIDRA_TLS_KEY_FILE", "")

	cfg := LoadFromEnv()
	if cfg.Argo.CollectorEnabled {
		t.Fatalf("expected collector disabled by default")
	}
	if cfg.Argo.CollectorInterval != 30*time.Second {
		t.Fatalf("expected default interval 30s, got %s", cfg.Argo.CollectorInterval)
	}
	if cfg.Argo.DefaultEnv != "unknown" {
		t.Fatalf("expected default env unknown, got %q", cfg.Argo.DefaultEnv)
	}
	if cfg.Argo.CheckpointFile != "./var/argo_checkpoint.json" {
		t.Fatalf("expected default checkpoint path, got %q", cfg.Argo.CheckpointFile)
	}
	if cfg.Argo.Backend != "argocd-client" {
		t.Fatalf("expected default backend argocd-client, got %q", cfg.Argo.Backend)
	}
	if cfg.K8s.CollectorEnabled {
		t.Fatalf("expected k8s collector disabled by default")
	}
	if cfg.K8s.Cluster != "unknown" {
		t.Fatalf("expected default k8s cluster unknown, got %q", cfg.K8s.Cluster)
	}
	if cfg.Auth.OIDC.Enabled {
		t.Fatalf("expected oidc auth disabled by default")
	}
	if cfg.Auth.JWT.Enabled {
		t.Fatalf("expected jwt auth disabled by default")
	}
	if cfg.Auth.RateLimit.Enabled {
		t.Fatalf("expected auth rate limit disabled by default")
	}
	if cfg.TLS.Enabled {
		t.Fatalf("expected tls disabled by default")
	}
	if cfg.DevInsecure {
		t.Fatalf("expected dev insecure disabled by default")
	}
}

func TestLoadFromEnvArgoConfigured(t *testing.T) {
	t.Setenv("EVIDRA_DEV_INSECURE", "true")
	t.Setenv("EVIDRA_ARGO_COLLECTOR_ENABLED", "true")
	t.Setenv("EVIDRA_ARGO_COLLECTOR_INTERVAL", "45s")
	t.Setenv("EVIDRA_ARGO_API_URL", "http://argocd.local/events")
	t.Setenv("EVIDRA_ARGO_API_TOKEN", "token")
	t.Setenv("EVIDRA_ARGO_DEFAULT_ENV", "prod-eu")
	t.Setenv("EVIDRA_ARGO_CHECKPOINT_FILE", "/tmp/evidra-argo.json")
	t.Setenv("EVIDRA_ARGO_BACKEND", "argocd-client")
	t.Setenv("EVIDRA_K8S_COLLECTOR_ENABLED", "true")
	t.Setenv("EVIDRA_K8S_COLLECTOR_INTERVAL", "20s")
	t.Setenv("EVIDRA_K8S_NAMESPACE", "prod")
	t.Setenv("EVIDRA_K8S_CLUSTER", "eu-1")
	t.Setenv("EVIDRA_K8S_ENVIRONMENT", "prod-eu")
	t.Setenv("EVIDRA_K8S_KUBECONFIG", "/tmp/kubeconfig")
	t.Setenv("EVIDRA_K8S_CONTEXT", "prod")
	t.Setenv("EVIDRA_AUTH_OIDC_ENABLED", "true")
	t.Setenv("EVIDRA_AUTH_OIDC_ROLES_HEADER", "X-Forwarded-Roles")
	t.Setenv("EVIDRA_AUTH_JWT_ENABLED", "true")
	t.Setenv("EVIDRA_AUTH_JWT_ISSUER", "https://issuer.local")
	t.Setenv("EVIDRA_AUTH_JWT_AUDIENCE", "evidra")
	t.Setenv("EVIDRA_AUTH_JWT_ROLES_CLAIM", "groups")
	t.Setenv("EVIDRA_AUTH_JWT_HS256_SECRET", "dev-secret")
	t.Setenv("EVIDRA_AUTH_JWT_JWKS_URL", "https://issuer.local/.well-known/jwks.json")
	t.Setenv("EVIDRA_AUTH_JWT_JWKS_REFRESH", "2m")
	t.Setenv("EVIDRA_AUTH_AUDIT_LOG_FILE", "/tmp/evidra-audit.log")
	t.Setenv("EVIDRA_AUTH_RATE_LIMIT_ENABLED", "true")
	t.Setenv("EVIDRA_AUTH_RATE_LIMIT_READ_PER_MIN", "700")
	t.Setenv("EVIDRA_AUTH_RATE_LIMIT_EXPORT_PER_MIN", "140")
	t.Setenv("EVIDRA_AUTH_RATE_LIMIT_INGEST_PER_MIN", "280")
	t.Setenv("EVIDRA_DB_SSLMODE", "verify-full")
	t.Setenv("EVIDRA_DB_SSLROOTCERT", "/tmp/ca.crt")
	t.Setenv("EVIDRA_DB_SSLCERT", "/tmp/client.crt")
	t.Setenv("EVIDRA_DB_SSLKEY", "/tmp/client.key")
	t.Setenv("EVIDRA_TLS_ENABLED", "true")
	t.Setenv("EVIDRA_TLS_CERT_FILE", "/tmp/tls.crt")
	t.Setenv("EVIDRA_TLS_KEY_FILE", "/tmp/tls.key")

	cfg := LoadFromEnv()
	if !cfg.Argo.CollectorEnabled {
		t.Fatalf("expected collector enabled")
	}
	if cfg.Argo.CollectorInterval != 45*time.Second {
		t.Fatalf("expected interval 45s, got %s", cfg.Argo.CollectorInterval)
	}
	if cfg.Argo.APIURL != "http://argocd.local/events" {
		t.Fatalf("unexpected api url: %q", cfg.Argo.APIURL)
	}
	if cfg.Argo.APIToken != "token" {
		t.Fatalf("unexpected api token")
	}
	if cfg.Argo.DefaultEnv != "prod-eu" {
		t.Fatalf("unexpected default env: %q", cfg.Argo.DefaultEnv)
	}
	if cfg.Argo.CheckpointFile != "/tmp/evidra-argo.json" {
		t.Fatalf("unexpected checkpoint file: %q", cfg.Argo.CheckpointFile)
	}
	if cfg.Argo.Backend != "argocd-client" {
		t.Fatalf("unexpected backend: %q", cfg.Argo.Backend)
	}
	if !cfg.K8s.CollectorEnabled {
		t.Fatalf("expected k8s collector enabled")
	}
	if cfg.K8s.CollectorInterval != 20*time.Second {
		t.Fatalf("unexpected k8s interval: %s", cfg.K8s.CollectorInterval)
	}
	if cfg.K8s.Namespace != "prod" {
		t.Fatalf("unexpected k8s namespace: %q", cfg.K8s.Namespace)
	}
	if !cfg.Auth.OIDC.Enabled {
		t.Fatalf("expected oidc enabled")
	}
	if cfg.Auth.OIDC.RolesHeader != "X-Forwarded-Roles" {
		t.Fatalf("unexpected oidc roles header: %q", cfg.Auth.OIDC.RolesHeader)
	}
	if !cfg.Auth.JWT.Enabled || cfg.Auth.JWT.RolesClaim != "groups" {
		t.Fatalf("expected jwt enabled with groups claim")
	}
	if cfg.Auth.JWT.JWKSURL == "" || cfg.Auth.JWT.JWKSRefresh != 2*time.Minute {
		t.Fatalf("expected jwks configuration")
	}
	if cfg.Auth.Audit.LogFile != "/tmp/evidra-audit.log" {
		t.Fatalf("unexpected audit log file: %q", cfg.Auth.Audit.LogFile)
	}
	if !cfg.Auth.RateLimit.Enabled || cfg.Auth.RateLimit.ReadPerMinute != 700 {
		t.Fatalf("unexpected auth rate limit config")
	}
	if cfg.DB.SSLMode != "verify-full" || cfg.DB.SSLRootCert != "/tmp/ca.crt" {
		t.Fatalf("unexpected db tls settings")
	}
	if !cfg.TLS.Enabled {
		t.Fatalf("expected tls enabled")
	}
	if cfg.TLS.CertFile != "/tmp/tls.crt" || cfg.TLS.KeyFile != "/tmp/tls.key" {
		t.Fatalf("unexpected tls files")
	}
	if !cfg.DevInsecure {
		t.Fatalf("expected dev insecure enabled")
	}
}

func TestValidate(t *testing.T) {
	cfg := Config{
		Addr:        ":8080",
		ExportDir:   "./var/exports",
		DevInsecure: true,
	}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("expected valid config, got %v", err)
	}

	cfg.DBDriver = "pgx"
	if err := cfg.Validate(); err == nil {
		t.Fatalf("expected db validation error")
	}
}

func TestLoadFromEnvBuildsDSNFromSplitVars(t *testing.T) {
	t.Setenv("EVIDRA_DB_DSN", "")
	t.Setenv("EVIDRA_DB_DRIVER", "pgx")
	t.Setenv("EVIDRA_DB_HOST", "postgres.evidra.svc.cluster.local")
	t.Setenv("EVIDRA_DB_PORT", "5432")
	t.Setenv("EVIDRA_DB_NAME", "evidra")
	t.Setenv("EVIDRA_DB_USER", "evidra")
	t.Setenv("EVIDRA_DB_PASSWORD", "change-me")
	t.Setenv("EVIDRA_DB_SSLMODE", "disable")
	t.Setenv("EVIDRA_DEV_INSECURE", "true")

	cfg := LoadFromEnv()
	want := "postgres://evidra:change-me@postgres.evidra.svc.cluster.local:5432/evidra?sslmode=disable"
	if cfg.DBDSN != want {
		t.Fatalf("unexpected derived dsn: got %q want %q", cfg.DBDSN, want)
	}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("expected valid config with split db vars, got %v", err)
	}
}

func TestValidateRejectsOpenAuthWithoutDevInsecure(t *testing.T) {
	cfg := Config{
		Addr:      ":8080",
		ExportDir: "./var/exports",
	}
	if err := cfg.Validate(); err == nil {
		t.Fatalf("expected validation error when auth is not configured")
	}
}

func TestValidateAcceptsConfiguredTokensWithoutDevInsecure(t *testing.T) {
	cfg := Config{
		Addr:      ":8080",
		ExportDir: "./var/exports",
		Auth: AuthConfig{
			Read: BearerAuth{Token: "read-token"},
		},
	}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("expected valid config with explicit auth, got %v", err)
	}
}
