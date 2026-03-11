package db

import (
	"embed"
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
	found := make(map[string]bool, len(entries))
	for _, entry := range entries {
		found[entry.Name()] = true
	}
	for _, want := range []string{
		"001_tenants_and_keys.up.sql",
		"002_evidence_entries.up.sql",
		"003_benchmark_runs.up.sql",
		"004_webhook_events.up.sql",
	} {
		if !found[want] {
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
