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
	"syscall"
	"time"

	evidrabenchmark "samebits.com/evidra"
	"samebits.com/evidra/internal/api"
	"samebits.com/evidra/internal/db"
	ievsigner "samebits.com/evidra/internal/evidence"
	"samebits.com/evidra/internal/store"
	"samebits.com/evidra/pkg/version"
)

//go:embed static
var staticFS embed.FS

func main() {
	os.Exit(run(os.Args[1:]))
}

func run(args []string) int {
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

	listenAddr := envOr("LISTEN_ADDR", ":8080")
	apiKey := os.Getenv("EVIDRA_API_KEY")
	if apiKey == "" {
		log.Fatal("EVIDRA_API_KEY is required")
	}

	// Signer (optional).
	signer, err := ievsigner.NewSigner(ievsigner.SignerConfig{
		KeyBase64: os.Getenv("EVIDRA_SIGNING_KEY"),
		KeyPath:   os.Getenv("EVIDRA_SIGNING_KEY_PATH"),
		DevMode:   os.Getenv("EVIDRA_SIGNING_MODE") == "optional",
	})
	if err != nil {
		log.Printf("warning: signer not configured: %v", err)
	}

	cfg := api.RouterConfig{
		APIKey:        apiKey,
		DefaultTenant: "default",
	}

	if signer != nil {
		cfg.PublicKey = signer.PublicKey()
	}

	// Database (optional).
	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL != "" {
		pool, err := db.Connect(databaseURL)
		if err != nil {
			log.Fatalf("database connection failed: %v", err)
		}
		defer pool.Close()

		cfg.Pinger = pool
		es := store.NewEntryStore(pool)
		cfg.EntryStore = es
		cfg.RawStore = es // EntryStore implements RawEntryStore
		cfg.KeyStore = store.NewKeyStore(pool)
		cfg.BenchmarkStore = store.NewBenchmarkStore(pool)
		cfg.InviteSecret = os.Getenv("EVIDRA_INVITE_SECRET")
		cfg.Scorecard = es
		cfg.Explain = es
		cfg.WebhookStore = es
		cfg.WebhookSigner = signer
		cfg.ArgoCDSecret = os.Getenv("EVIDRA_WEBHOOK_SECRET_ARGOCD")
		cfg.GenericSecret = os.Getenv("EVIDRA_WEBHOOK_SECRET_GENERIC")

		log.Printf("database connected, migrations applied")
	} else {
		log.Printf("no DATABASE_URL — running without persistence")
	}

	// UI: prefer embedded React build (embed_ui tag), fall back to static/.
	if evidrabenchmark.UIDistFS != nil {
		cfg.UIFS = evidrabenchmark.UIDistFS
	} else {
		uiFS, err := fs.Sub(staticFS, "static")
		if err != nil {
			log.Fatalf("embed static: %v", err)
		}
		cfg.UIFS = uiFS
	}

	handler := api.NewRouter(cfg)

	srv := &http.Server{
		Addr:         listenAddr,
		Handler:      handler,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	// Graceful shutdown.
	done := make(chan os.Signal, 1)
	signal.Notify(done, os.Interrupt, syscall.SIGTERM)

	go func() {
		log.Printf("evidra-api %s listening on %s", version.Version, listenAddr)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("listen: %v", err)
		}
	}()

	<-done
	log.Printf("shutting down...")

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		log.Printf("shutdown error: %v", err)
		return 1
	}

	return 0
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
