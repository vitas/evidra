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
		"005_webhook_event_results.up.sql",
		"006_bench_tables.up.sql",
		"006_bench_tables.down.sql",
		"007_drop_legacy_benchmark_tables.up.sql",
		"007_drop_legacy_benchmark_tables.down.sql",
		"008_bench_runs_archive.up.sql",
		"008_bench_runs_archive.down.sql",
		"009_bench_scenarios_track_level.up.sql",
		"009_bench_scenarios_track_level.down.sql",
		"010_bench_runs_tool_server.up.sql",
		"010_bench_runs_tool_server.down.sql",
		"011_bench_runs_versions.up.sql",
		"011_bench_runs_versions.down.sql",
		"012_global_scenarios_and_models.up.sql",
		"012_global_scenarios_and_models.down.sql",
		"013_bench_jobs_progress_tracking.up.sql",
		"013_bench_jobs_progress_tracking.down.sql",
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
