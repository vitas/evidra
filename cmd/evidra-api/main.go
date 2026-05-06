package main

import (
	"context"
	"embed"
	"flag"
	"fmt"
	"io/fs"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	evidrabenchmark "samebits.com/evidra"
	"samebits.com/evidra/internal/analyticsvc"
	"samebits.com/evidra/internal/api"
	"samebits.com/evidra/internal/db"
	ievsigner "samebits.com/evidra/internal/evidence"
	argocdgitops "samebits.com/evidra/internal/gitops/argocd"
	"samebits.com/evidra/internal/store"
	pkevidence "samebits.com/evidra/pkg/evidence"
	"samebits.com/evidra/pkg/version"

	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

//go:embed static
var staticFS embed.FS

type signerConfig = ievsigner.SignerConfig

type controllerRunner interface {
	Run(context.Context) error
}

type argocdControllerConfig struct {
	Enabled              bool
	ApplicationNamespace string
	TenantID             string
	KubeconfigPath       string
}

type persistenceResources struct {
	Pinger     api.Pinger
	EntryStore *store.EntryStore
	KeyStore   *store.KeyStore
}

type runDeps struct {
	newSigner         func(signerConfig) (pkevidence.Signer, error)
	setupPersistence  func(string) (persistenceResources, func(), error)
	controllerFactory func(argocdControllerConfig, *store.EntryStore, pkevidence.Signer) (controllerRunner, error)
	uiFS              func() (fs.FS, error)
	signalChan        func() <-chan os.Signal
	listenAndServe    func(*http.Server) error
	shutdownServer    func(*http.Server, context.Context) error
	logf              func(string, ...any)
}

type runtimeSetup struct {
	listenAddr       string
	controllerCfg    argocdControllerConfig
	signer           pkevidence.Signer
	routerConfig     api.RouterConfig
	resources        persistenceResources
	closePersistence func()
}

func main() {
	os.Exit(run(os.Args[1:]))
}

func run(args []string) int {
	return runWithDeps(args, defaultRunDeps())
}

func defaultRunDeps() runDeps {
	return runDeps{}
}

func runWithDeps(args []string, deps runDeps) int {
	deps = deps.withDefaults()

	flags := flag.NewFlagSet("evidra-api", flag.ContinueOnError)
	versionFlag := flags.Bool("version", false, "print version and exit")
	if err := flags.Parse(args); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		return 1
	}

	if *versionFlag {
		fmt.Println(version.BuildString("evidra-api"))
		return 0
	}

	setup, code := initializeRuntime(deps)
	if code != 0 {
		return code
	}
	if setup.closePersistence != nil {
		defer setup.closePersistence()
	}

	handler := api.NewRouter(setup.routerConfig)
	srv := &http.Server{
		Addr:         setup.listenAddr,
		Handler:      handler,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if setup.controllerCfg.Enabled {
		controller, err := deps.controllerFactory(setup.controllerCfg, setup.resources.EntryStore, setup.signer)
		if err != nil {
			deps.logf("argocd controller startup failed: %v", err)
			return 1
		}
		go func() {
			if err := controller.Run(ctx); err != nil && ctx.Err() == nil {
				deps.logf("argocd controller error: %v", err)
			}
		}()
	}

	serverErrCh := make(chan error, 1)
	go func() {
		deps.logf("evidra-api %s listening on %s", version.Version, setup.listenAddr)
		serverErrCh <- deps.listenAndServe(srv)
	}()

	select {
	case <-deps.signalChan():
	case err := <-serverErrCh:
		if err != nil && err != http.ErrServerClosed {
			deps.logf("listen: %v", err)
			cancel()
			return 1
		}
	}

	deps.logf("shutting down...")
	cancel()

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()
	if err := deps.shutdownServer(srv, shutdownCtx); err != nil {
		deps.logf("shutdown error: %v", err)
		return 1
	}

	return 0
}

func initializeRuntime(deps runDeps) (runtimeSetup, int) {
	apiKey := os.Getenv("EVIDRA_API_KEY")
	if apiKey == "" {
		deps.logf("EVIDRA_API_KEY is required")
		return runtimeSetup{}, 1
	}

	signer, err := loadSigner(deps)
	if err != nil {
		deps.logf("warning: signer not configured: %v", err)
	}

	controllerCfg := loadArgoCDControllerConfigFromEnv()
	databaseURL := os.Getenv("DATABASE_URL")
	if err := validateControllerPrerequisites(controllerCfg, databaseURL, signer); err != nil {
		deps.logf("%v", err)
		return runtimeSetup{}, 1
	}

	cfg := buildRouterConfig(apiKey, signer)
	resources, closePersistence, ok := configurePersistence(deps, databaseURL, signer, &cfg)
	if !ok {
		return runtimeSetup{}, 1
	}
	if controllerCfg.Enabled && resources.EntryStore == nil {
		deps.logf("argocd controller requires database-backed entry store")
		return runtimeSetup{}, 1
	}
	if err := attachUIFS(deps, &cfg); err != nil {
		deps.logf("embed static: %v", err)
		return runtimeSetup{}, 1
	}

	return runtimeSetup{
		listenAddr:       envOr("LISTEN_ADDR", ":8080"),
		controllerCfg:    controllerCfg,
		signer:           signer,
		routerConfig:     cfg,
		resources:        resources,
		closePersistence: closePersistence,
	}, 0
}

func (d runDeps) withDefaults() runDeps {
	if d.newSigner == nil {
		d.newSigner = func(cfg signerConfig) (pkevidence.Signer, error) {
			return ievsigner.NewSigner(ievsigner.SignerConfig(cfg))
		}
	}
	if d.setupPersistence == nil {
		d.setupPersistence = defaultSetupPersistence
	}
	if d.controllerFactory == nil {
		d.controllerFactory = defaultControllerFactory
	}
	if d.uiFS == nil {
		d.uiFS = defaultUIFS
	}
	if d.signalChan == nil {
		d.signalChan = defaultSignalChan
	}
	if d.listenAndServe == nil {
		d.listenAndServe = func(srv *http.Server) error {
			return srv.ListenAndServe()
		}
	}
	if d.shutdownServer == nil {
		d.shutdownServer = func(srv *http.Server, ctx context.Context) error {
			return srv.Shutdown(ctx)
		}
	}
	if d.logf == nil {
		d.logf = log.Printf
	}
	return d
}

func loadSigner(deps runDeps) (pkevidence.Signer, error) {
	return deps.newSigner(signerConfig{
		KeyBase64: os.Getenv("EVIDRA_SIGNING_KEY"),
		KeyPath:   os.Getenv("EVIDRA_SIGNING_KEY_PATH"),
		DevMode:   os.Getenv("EVIDRA_SIGNING_MODE") == "optional",
	})
}

func validateControllerPrerequisites(controllerCfg argocdControllerConfig, databaseURL string, signer pkevidence.Signer) error {
	if !controllerCfg.Enabled {
		return nil
	}
	if strings.TrimSpace(databaseURL) == "" {
		return fmt.Errorf("argocd controller requires DATABASE_URL")
	}
	if signer == nil {
		return fmt.Errorf("argocd controller requires signing")
	}
	return nil
}

func buildRouterConfig(apiKey string, signer pkevidence.Signer) api.RouterConfig {
	cfg := api.RouterConfig{
		APIKey:        apiKey,
		DefaultTenant: "default",
	}
	if signer != nil {
		cfg.PublicKey = signer.PublicKey()
	}
	return cfg
}

func configurePersistence(deps runDeps, databaseURL string, signer pkevidence.Signer, cfg *api.RouterConfig) (persistenceResources, func(), bool) {
	if strings.TrimSpace(databaseURL) == "" {
		deps.logf("no DATABASE_URL — running without persistence")
		return persistenceResources{}, nil, true
	}

	resources, closePersistence, err := deps.setupPersistence(databaseURL)
	if err != nil {
		deps.logf("database connection failed: %v", err)
		return persistenceResources{}, nil, false
	}

	cfg.Pinger = resources.Pinger
	cfg.EntryStore = resources.EntryStore
	cfg.RawStore = resources.EntryStore
	cfg.KeyStore = resources.KeyStore
	cfg.InviteSecret = os.Getenv("EVIDRA_INVITE_SECRET")
	analyticsSvc := analyticsvc.NewService(resources.EntryStore)
	cfg.Scorecard = analyticsSvc
	cfg.Explain = analyticsSvc
	cfg.WebhookStore = resources.EntryStore
	cfg.WebhookSigner = signer
	cfg.ArgoCDSecret = os.Getenv("EVIDRA_WEBHOOK_SECRET_ARGOCD")
	cfg.GenericSecret = os.Getenv("EVIDRA_WEBHOOK_SECRET_GENERIC")

	deps.logf("database connected, migrations applied")
	return resources, closePersistence, true
}

func attachUIFS(deps runDeps, cfg *api.RouterConfig) error {
	uiFS, err := deps.uiFS()
	if err != nil {
		return err
	}
	cfg.UIFS = uiFS
	return nil
}

func defaultSetupPersistence(databaseURL string) (persistenceResources, func(), error) {
	pool, err := db.Connect(databaseURL)
	if err != nil {
		return persistenceResources{}, nil, err
	}

	es := store.NewEntryStore(pool)
	return persistenceResources{
			Pinger:     pool,
			EntryStore: es,
			KeyStore:   store.NewKeyStore(pool),
		}, func() {
			pool.Close()
		}, nil
}

func defaultControllerFactory(cfg argocdControllerConfig, entryStore *store.EntryStore, signer pkevidence.Signer) (controllerRunner, error) {
	client, err := newDynamicClient(cfg.KubeconfigPath)
	if err != nil {
		return nil, err
	}
	return argocdgitops.NewController(client, entryStore, signer, argocdgitops.ControllerConfig{
		ApplicationNamespace: cfg.ApplicationNamespace,
		TenantID:             cfg.TenantID,
	}), nil
}

func defaultUIFS() (fs.FS, error) {
	// Prefer the built React bundle when present. The checked-in static/ tree is a
	// compatibility fallback and should mirror the public product wording, not
	// grow into a separate product surface.
	if evidrabenchmark.UIDistFS != nil {
		return evidrabenchmark.UIDistFS, nil
	}
	return fs.Sub(staticFS, "static")
}

func defaultSignalChan() <-chan os.Signal {
	done := make(chan os.Signal, 1)
	signal.Notify(done, os.Interrupt, syscall.SIGTERM)
	return done
}

func loadArgoCDControllerConfigFromEnv() argocdControllerConfig {
	return argocdControllerConfig{
		Enabled:              strings.EqualFold(strings.TrimSpace(os.Getenv("EVIDRA_ARGOCD_CONTROLLER_ENABLED")), "true"),
		ApplicationNamespace: envOr("EVIDRA_ARGOCD_APPLICATION_NAMESPACE", "argocd"),
		TenantID:             envOr("EVIDRA_ARGOCD_TENANT_ID", "default"),
		KubeconfigPath:       strings.TrimSpace(os.Getenv("EVIDRA_KUBECONFIG")),
	}
}

func newDynamicClient(kubeconfigPath string) (dynamic.Interface, error) {
	var (
		cfg *rest.Config
		err error
	)
	if strings.TrimSpace(kubeconfigPath) != "" {
		cfg, err = clientcmd.BuildConfigFromFlags("", kubeconfigPath)
	} else {
		cfg, err = rest.InClusterConfig()
	}
	if err != nil {
		return nil, err
	}
	return dynamic.NewForConfig(cfg)
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
