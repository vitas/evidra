package bootstrap

import (
	"context"
	"database/sql"
	"log"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	ce "evidra/internal/cloudevents"
	"evidra/internal/api"
	"evidra/internal/config"
	"evidra/internal/export"
	"evidra/internal/ingest"
	"evidra/internal/ingest/argo"
	"evidra/internal/migrate"
	"evidra/internal/observability"
	"evidra/internal/store"

	"github.com/go-logr/logr"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"

	_ "github.com/jackc/pgx/v5/stdlib"

	_ "modernc.org/sqlite"
)

type Runtime struct {
	Handler http.Handler
	Cleanup func()
}

func NewRuntime(ctx context.Context, cfg config.Config, logger logr.Logger) *Runtime {
	repo, cleanup := buildRepository(ctx, cfg)
	startArgoCollector(ctx, logger, cfg, repo)
	if cfg.K8s.CollectorEnabled {
		logger.Info("kubernetes collector configuration ignored: out of scope in Argo CD-first v1")
	}

	exporter := export.NewFilesystemExporter(cfg.ExportDir)
	server := api.NewServerWithOptions(repo, exporter, api.ServerOptions{
		Auth: api.AuthConfig{
			Read: api.BearerPolicy{
				Token: cfg.Auth.Read.Token,
			},
			Ingest: api.IngestPolicy{
				Bearer: api.BearerPolicy{
					Token: cfg.Auth.Ingest.Bearer.Token,
				},
				GenericWebhook: api.HMACPolicy{
					Secret: cfg.Auth.Ingest.GenericWebhook.Secret,
				},
			},
			OIDC: api.OIDCPolicy{
				Enabled:     cfg.Auth.OIDC.Enabled,
				RolesHeader: cfg.Auth.OIDC.RolesHeader,
			},
			JWT: api.JWTPolicy{
				Enabled:           cfg.Auth.JWT.Enabled,
				Issuer:            cfg.Auth.JWT.Issuer,
				Audience:          cfg.Auth.JWT.Audience,
				RolesClaim:        cfg.Auth.JWT.RolesClaim,
				HS256Secret:       cfg.Auth.JWT.HS256Secret,
				RS256PublicKeyPEM: cfg.Auth.JWT.RS256PublicKeyPEM,
				JWKSURL:           cfg.Auth.JWT.JWKSURL,
				JWKSRefresh:       cfg.Auth.JWT.JWKSRefresh.String(),
			},
			Audit: api.AuditPolicy{
				LogFile: cfg.Auth.Audit.LogFile,
			},
			Rate: api.RateLimitPolicy{
				Enabled:         cfg.Auth.RateLimit.Enabled,
				ReadPerMinute:   cfg.Auth.RateLimit.ReadPerMinute,
				ExportPerMinute: cfg.Auth.RateLimit.ExportPerMinute,
				IngestPerMinute: cfg.Auth.RateLimit.IngestPerMinute,
			},
		},
		WebhookRegistry: buildWebhookRegistry(cfg),
		ArgoOnlyMode:    true,
	})

	metrics := observability.NewHTTPMetrics()
	rootMux := http.NewServeMux()
	rootMux.Handle("/metrics", promhttp.Handler())
	rootMux.Handle("/", metrics.Wrap(server.Routes()))

	return &Runtime{
		Handler: rootMux,
		Cleanup: cleanup,
	}
}

func buildWebhookRegistry(cfg config.Config) *ingest.Registry {
	_ = cfg
	return ingest.NewRegistry()
}

func buildRepository(ctx context.Context, cfg config.Config) (store.Repository, func()) {
	if cfg.DBDriver == "" || cfg.DBDSN == "" {
		log.Printf("running with in-memory repository")
		return store.NewMemoryRepository(), func() {}
	}

	dsn := applyPostgresTLS(cfg)
	db, err := sql.Open(cfg.DBDriver, dsn)
	if err != nil {
		log.Printf("db open failed (%v), falling back to in-memory repository", err)
		return store.NewMemoryRepository(), func() {}
	}

	pingCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	if err := db.PingContext(pingCtx); err != nil {
		log.Printf("db ping failed (%v), falling back to in-memory repository", err)
		_ = db.Close()
		return store.NewMemoryRepository(), func() {}
	}

	if cfg.DBMigrate {
		runner := migrate.NewRunner(os.DirFS("."))
		if err := runner.Apply(ctx, db, cfg.DBDialect); err != nil {
			log.Printf("migration apply failed (%v), falling back to in-memory repository", err)
			_ = db.Close()
			return store.NewMemoryRepository(), func() {}
		}
	}

	repo, err := store.NewSQLRepository(db, cfg.DBDialect)
	if err != nil {
		log.Printf("sql repository init failed (%v), falling back to in-memory repository", err)
		_ = db.Close()
		return store.NewMemoryRepository(), func() {}
	}
	log.Printf("running with SQL repository: dialect=%s", cfg.DBDialect)
	return repo, func() { _ = db.Close() }
}

func applyPostgresTLS(cfg config.Config) string {
	driver := strings.ToLower(strings.TrimSpace(cfg.DBDriver))
	if driver != "pgx" {
		return cfg.DBDSN
	}
	if strings.TrimSpace(cfg.DB.SSLMode) == "" &&
		strings.TrimSpace(cfg.DB.SSLRootCert) == "" &&
		strings.TrimSpace(cfg.DB.SSLCert) == "" &&
		strings.TrimSpace(cfg.DB.SSLKey) == "" {
		return cfg.DBDSN
	}
	u, err := url.Parse(cfg.DBDSN)
	if err != nil || u.Scheme == "" || u.Host == "" {
		return cfg.DBDSN
	}
	q := u.Query()
	if strings.TrimSpace(cfg.DB.SSLMode) != "" {
		q.Set("sslmode", strings.TrimSpace(cfg.DB.SSLMode))
	}
	if strings.TrimSpace(cfg.DB.SSLRootCert) != "" {
		q.Set("sslrootcert", strings.TrimSpace(cfg.DB.SSLRootCert))
	}
	if strings.TrimSpace(cfg.DB.SSLCert) != "" {
		q.Set("sslcert", strings.TrimSpace(cfg.DB.SSLCert))
	}
	if strings.TrimSpace(cfg.DB.SSLKey) != "" {
		q.Set("sslkey", strings.TrimSpace(cfg.DB.SSLKey))
	}
	u.RawQuery = q.Encode()
	return u.String()
}

func startArgoCollector(ctx context.Context, logger logr.Logger, cfg config.Config, repo store.Repository) {
	if !cfg.Argo.CollectorEnabled {
		return
	}
	if strings.TrimSpace(cfg.Argo.APIURL) == "" {
		logger.Info("argo collector enabled but EVIDRA_ARGO_API_URL is empty; collector not started")
		return
	}

	// Attempt to build Kubernetes dynamic client
	var dynClient dynamic.Interface
	kubeConfig, err := rest.InClusterConfig()
	if err != nil {
		kubeconfigPath := os.Getenv("KUBECONFIG")
		if kubeconfigPath == "" {
			kubeconfigPath = os.ExpandEnv("$HOME/.kube/config")
		}
		kubeConfig, err = clientcmd.BuildConfigFromFlags("", kubeconfigPath)
		if err != nil {
			logger.Error(err, "argo collector could not construct kubernetes config, dynamic watch will fail")
		}
	}
	if kubeConfig != nil {
		dynClient, err = dynamic.NewForConfig(kubeConfig)
		if err != nil {
			logger.Error(err, "argo collector failed to create dynamic client")
		}
	}

	fetch, err := argo.NewFetchFunc(argo.BackendOptions{
		Backend: cfg.Argo.Backend,
		URL:     cfg.Argo.APIURL,
		Token:   cfg.Argo.APIToken,
	})
	if err != nil {
		logger.Error(err, "argo collector backend init failed")
		return
	}
	collector := &argo.Collector{
		Interval:      cfg.Argo.CollectorInterval,
		Fetch:         fetch,
		DynamicClient: dynClient,
		Namespace:     "argocd", // Provide default if not configured
		Normalize: func(se argo.SourceEvent) (ce.StoredEvent, error) {
			return argo.NormalizeSourceEvent(se, cfg.Argo.DefaultEnv)
		},
		Sink:   repo,
		Logger: logger.WithName("argo-collector"),
		Checkpoint: argo.FileCheckpointStore{
			Path: cfg.Argo.CheckpointFile,
		},
	}
	go collector.Start(ctx)
	logger.Info("argo collector started", "interval", cfg.Argo.CollectorInterval.String(), "api_url", cfg.Argo.APIURL, "dynamic_client_ready", dynClient != nil)
}
