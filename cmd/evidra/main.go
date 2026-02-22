package main

import (
	"context"
	"log"
	"net/http"
	"time"

	"evidra/internal/bootstrap"
	"evidra/internal/config"

	"github.com/go-logr/zapr"
	"go.uber.org/zap"
)

func main() {
	cfg := config.LoadFromEnv()
	if err := cfg.Validate(); err != nil {
		log.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	zapLogger, err := zap.NewProduction()
	if err != nil {
		log.Fatal(err)
	}
	defer func() { _ = zapLogger.Sync() }()
	logger := zapr.NewLogger(zapLogger)

	rt := bootstrap.NewRuntime(ctx, cfg, logger)
	defer rt.Cleanup()

	summary := cfg.Summary()
	logger.Info("startup config",
		"repository_mode", summary.RepositoryMode,
		"providers", summary.WebhookProviders,
		"dev_insecure", summary.DevInsecure,
		"argo_collector", summary.ArgoCollector,
		"argo_backend", summary.ArgoBackend,
		"argo_checkpoint_file", summary.ArgoCheckpointFile,
		"k8s_collector", summary.K8sCollector,
		"k8s_namespace", summary.K8sNamespace,
		"oidc_enabled", summary.OIDCEnabled,
		"jwt_enabled", summary.JWTEnabled,
		"tls_enabled", summary.TLSEnabled,
	)
	logger.Info("evidra listening", "addr", cfg.Addr)
	server := &http.Server{
		Addr:              cfg.Addr,
		Handler:           rt.Handler,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      60 * time.Second,
		IdleTimeout:       120 * time.Second,
	}
	var serveErr error
	if cfg.TLS.Enabled {
		serveErr = server.ListenAndServeTLS(cfg.TLS.CertFile, cfg.TLS.KeyFile)
	} else {
		serveErr = server.ListenAndServe()
	}
	if serveErr != nil {
		logger.Error(serveErr, "http server failed")
		log.Fatal(serveErr)
	}
}
