package store

import (
	"context"
	"database/sql"
	"os"
	"strings"
	"testing"
	"time"

	"evidra/internal/migrate"
)

func TestSQLRepositorySQLiteIntegration(t *testing.T) {
	driver := strings.TrimSpace(os.Getenv("EVIDRA_SQL_TEST_DRIVER"))
	dsn := strings.TrimSpace(os.Getenv("EVIDRA_SQL_TEST_DSN"))
	dialect := strings.TrimSpace(os.Getenv("EVIDRA_SQL_TEST_DIALECT"))
	if driver == "" {
		t.Skip("set EVIDRA_SQL_TEST_DRIVER and EVIDRA_SQL_TEST_DSN to run SQL integration test")
	}
	if dsn == "" {
		t.Skip("set EVIDRA_SQL_TEST_DSN to run SQL integration test")
	}
	if dialect == "" {
		dialect = "sqlite"
	}

	db, err := sql.Open(driver, dsn)
	if err != nil {
		if strings.Contains(err.Error(), "unknown driver") {
			t.Skipf("sql driver not registered: %v", err)
		}
		t.Fatalf("open db: %v", err)
	}
	defer db.Close()

	ctx := context.Background()
	if err := db.PingContext(ctx); err != nil {
		if strings.Contains(err.Error(), "unknown driver") {
			t.Skipf("sql driver not registered: %v", err)
		}
		t.Fatalf("ping db: %v", err)
	}

	runner := migrate.NewRunner(os.DirFS("../.."))
	if err := runner.Apply(ctx, db, dialect); err != nil {
		t.Fatalf("apply migrations: %v", err)
	}

	repo, err := NewSQLRepository(db, dialect)
	if err != nil {
		t.Fatalf("new sql repo: %v", err)
	}

	base := time.Date(2026, 2, 16, 10, 0, 0, 0, time.UTC)
	e1 := sampleEvent("sql_evt_1", base.Add(1*time.Minute), map[string]interface{}{"commit_sha": "sqlabc"})
	e2 := sampleEvent("sql_evt_2", base.Add(2*time.Minute), map[string]interface{}{"commit_sha": "sqlabc"})

	if _, _, err := repo.IngestEvent(ctx, e1); err != nil {
		t.Fatalf("ingest e1: %v", err)
	}
	if _, _, err := repo.IngestEvent(ctx, e2); err != nil {
		t.Fatalf("ingest e2: %v", err)
	}

	res, err := repo.QueryTimeline(ctx, TimelineQuery{
		Subject:   "payments-api",
		Namespace: "prod-eu",
		Cluster:   "eu-1",
		From:      base,
		To:        base.Add(1 * time.Hour),
		Limit:     10,
	})
	if err != nil {
		t.Fatalf("timeline query: %v", err)
	}
	if len(res.Items) != 2 || res.Items[0].ID != "sql_evt_1" || res.Items[1].ID != "sql_evt_2" {
		t.Fatalf("unexpected timeline result: %+v", res.Items)
	}

	items, err := repo.EventsByExtension(ctx, "commit_sha", "sqlabc", 10)
	if err != nil {
		t.Fatalf("extension query: %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("expected 2 events by extension, got %d", len(items))
	}
}
