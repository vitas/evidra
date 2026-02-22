//go:build integration

package integration

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"evidra/internal/api"
	"evidra/internal/export"
	"evidra/internal/ingest"
	"evidra/internal/migrate"
	"evidra/internal/providers/bitbucket"
	"evidra/internal/providers/github"
	"evidra/internal/providers/gitlab"
	"evidra/internal/store"

	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"

	_ "github.com/jackc/pgx/v5/stdlib"
)

func TestE2EWebhookTimelineExportWithPostgres(t *testing.T) {
	ctx := context.Background()

	pg, dsn := startPostgres(t, ctx)
	t.Cleanup(func() {
		_ = pg.Terminate(ctx)
	})

	db, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	runner := migrate.NewRunner(os.DirFS(".."))
	if err := runner.Apply(ctx, db, "postgres"); err != nil {
		t.Fatalf("apply migrations: %v", err)
	}

	repo, err := store.NewSQLRepository(db, "postgres")
	if err != nil {
		t.Fatalf("new sql repository: %v", err)
	}

	reg := ingest.NewRegistry()
	reg.Register(github.NewAdapter("gh-secret"))
	reg.Register(gitlab.NewAdapter("gl-token"))
	reg.Register(bitbucket.NewAdapter("bb-secret"))

	srv := api.NewServerWithOptions(repo, export.NewFilesystemExporter(t.TempDir()), api.ServerOptions{
		WebhookRegistry: reg,
	})
	httpSrv := httptest.NewServer(srv.Routes())
	t.Cleanup(httpSrv.Close)

	postGitHubWebhook(t, httpSrv.URL, filepath.Join("..", "testdata", "events", "github_push_webhook_payload.json"), "gh-secret")

	postFixtureEvent(t, httpSrv.URL, filepath.Join("..", "testdata", "events", "argocd_event_valid.json"))

	corrURL := fmt.Sprintf("%s/v1/correlations/commit_sha?value=abc123", httpSrv.URL)
	corrResp, err := http.Get(corrURL)
	if err != nil {
		t.Fatalf("correlation request: %v", err)
	}
	defer corrResp.Body.Close()
	if corrResp.StatusCode != http.StatusOK {
		t.Fatalf("correlation status: %d", corrResp.StatusCode)
	}
	var corrBody struct {
		Items []json.RawMessage `json:"items"`
	}
	if err := json.NewDecoder(corrResp.Body).Decode(&corrBody); err != nil {
		t.Fatalf("decode correlation: %v", err)
	}
	if len(corrBody.Items) < 2 {
		t.Fatalf("expected at least 2 correlated events, got %d", len(corrBody.Items))
	}

	changesURL := fmt.Sprintf(
		"%s/v1/changes?subject=payments-api:prod-eu:eu-1&from=2026-02-16T00:00:00Z&to=2026-02-16T23:59:59Z",
		httpSrv.URL,
	)
	changesResp, err := http.Get(changesURL)
	if err != nil {
		t.Fatalf("changes request: %v", err)
	}
	defer changesResp.Body.Close()
	if changesResp.StatusCode != http.StatusOK {
		t.Fatalf("changes status: %d", changesResp.StatusCode)
	}
	var changesBody struct {
		Items []struct {
			ID string `json:"id"`
		} `json:"items"`
	}
	if err := json.NewDecoder(changesResp.Body).Decode(&changesBody); err != nil {
		t.Fatalf("decode changes: %v", err)
	}
	if len(changesBody.Items) == 0 || changesBody.Items[0].ID == "" {
		t.Fatalf("expected at least one change")
	}

	changeTimelineURL := fmt.Sprintf(
		"%s/v1/changes/%s/timeline?subject=payments-api:prod-eu:eu-1&from=2026-02-16T00:00:00Z&to=2026-02-16T23:59:59Z",
		httpSrv.URL,
		changesBody.Items[0].ID,
	)
	changeTimelineResp, err := http.Get(changeTimelineURL)
	if err != nil {
		t.Fatalf("change timeline request: %v", err)
	}
	defer changeTimelineResp.Body.Close()
	if changeTimelineResp.StatusCode != http.StatusOK {
		t.Fatalf("change timeline status: %d", changeTimelineResp.StatusCode)
	}
	var changeTimelineBody struct {
		Items []json.RawMessage `json:"items"`
	}
	if err := json.NewDecoder(changeTimelineResp.Body).Decode(&changeTimelineBody); err != nil {
		t.Fatalf("decode change timeline: %v", err)
	}
	if len(changeTimelineBody.Items) == 0 {
		t.Fatalf("expected non-empty change timeline")
	}

	exportPayload := map[string]interface{}{
		"format": "json",
		"filter": map[string]interface{}{
			"subject": "payments-api:prod-eu:eu-1",
			"from":    "2026-02-16T00:00:00Z",
			"to":      "2026-02-16T23:59:59Z",
		},
	}
	b, _ := json.Marshal(exportPayload)
	exportReq, _ := http.NewRequest(http.MethodPost, httpSrv.URL+"/v1/exports", bytes.NewReader(b))
	exportReq.Header.Set("Content-Type", "application/json")
	exportResp, err := http.DefaultClient.Do(exportReq)
	if err != nil {
		t.Fatalf("export request: %v", err)
	}
	defer exportResp.Body.Close()
	if exportResp.StatusCode != http.StatusAccepted {
		t.Fatalf("export status: %d", exportResp.StatusCode)
	}
	var job struct {
		ID string `json:"id"`
	}
	if err := json.NewDecoder(exportResp.Body).Decode(&job); err != nil {
		t.Fatalf("decode export job: %v", err)
	}
	if job.ID == "" {
		t.Fatalf("expected export id")
	}
}

func startPostgres(t *testing.T, ctx context.Context) (*postgres.PostgresContainer, string) {
	t.Helper()
	pg, err := postgres.Run(
		ctx,
		"postgres:16-alpine",
		postgres.WithDatabase("evidra"),
		postgres.WithUsername("evidra"),
		postgres.WithPassword("evidra"),
		testcontainers.WithWaitStrategy(wait.ForListeningPort("5432/tcp").WithStartupTimeout(90*time.Second)),
	)
	if err != nil {
		t.Fatalf("start postgres container: %v", err)
	}
	dsn, err := pg.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		t.Fatalf("postgres connection string: %v", err)
	}
	return pg, dsn
}

func postGitHubWebhook(t *testing.T, baseURL, fixturePath, secret string) {
	t.Helper()
	body := mustRead(t, fixturePath)
	req, _ := http.NewRequest(http.MethodPost, baseURL+"/v1/webhooks/github", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-GitHub-Event", "push")
	req.Header.Set("X-GitHub-Delivery", "it-gh-1")
	req.Header.Set("X-Hub-Signature-256", signSHA256(secret, body))
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("github webhook request: %v", err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusAccepted {
		t.Fatalf("github webhook status: %d", res.StatusCode)
	}
}

func postFixtureEvent(t *testing.T, baseURL, fixturePath string) {
	t.Helper()
	body := mustRead(t, fixturePath)
	req, _ := http.NewRequest(http.MethodPost, baseURL+"/v1/events", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("event request: %v", err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusAccepted {
		t.Fatalf("event status: %d", res.StatusCode)
	}
}

func mustRead(t *testing.T, path string) []byte {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return b
}

func signSHA256(secret string, body []byte) string {
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write(body)
	return "sha256=" + hex.EncodeToString(mac.Sum(nil))
}
