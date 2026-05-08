package db

import (
	"embed"
	"strings"
	"testing"
)

//go:embed migrations/*.sql
var testMigrations embed.FS

func TestMigrationsEmbedded(t *testing.T) {
	t.Parallel()
	entries, err := testMigrations.ReadDir("migrations")
	if err != nil {
		t.Fatalf("read embedded migrations: %v", err)
	}

	wantMigrations := map[string]bool{
		"001_tenants_and_keys.up.sql":      false,
		"002_evidence_entries.up.sql":      false,
		"003_webhook_events.up.sql":        false,
		"004_webhook_event_results.up.sql": false,
	}
	for _, entry := range entries {
		name := entry.Name()
		for _, forbidden := range []string{"bench", "benchmark", "core_compatibility", "drop_legacy", "legacy"} {
			if strings.Contains(name, forbidden) {
				t.Fatalf("core migration %s contains forbidden marker %q", name, forbidden)
			}
		}
		if _, ok := wantMigrations[name]; !ok {
			t.Fatalf("unexpected embedded migration %s", name)
		}
		wantMigrations[name] = true
	}
	for want, found := range wantMigrations {
		if !found {
			t.Fatalf("missing embedded migration %s", want)
		}
	}
}

func TestConnect_InvalidURL(t *testing.T) {
	t.Parallel()
	_, err := Connect("postgres://invalid:5432/nonexistent?connect_timeout=1")
	if err == nil {
		t.Fatal("expected error for invalid database URL")
	}
}

func TestMigrationDatabaseURLUsesCoreMigrationTable(t *testing.T) {
	t.Parallel()

	got, err := migrationDatabaseURL("postgres://evidra:secret@postgres:5432/evidra?sslmode=disable")
	if err != nil {
		t.Fatalf("migrationDatabaseURL: %v", err)
	}

	if !strings.HasPrefix(got, "pgx5://") {
		t.Fatalf("migration URL = %q, want pgx5 scheme", got)
	}
	for _, want := range []string{"sslmode=disable", "x-migrations-table=evidra_schema_migrations"} {
		if !strings.Contains(got, want) {
			t.Fatalf("migration URL = %q, missing %q", got, want)
		}
	}
}
