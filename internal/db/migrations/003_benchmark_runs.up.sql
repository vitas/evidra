-- 003_benchmark_runs.sql
CREATE TABLE IF NOT EXISTS benchmark_runs (
    id           TEXT PRIMARY KEY,
    tenant_id    TEXT NOT NULL REFERENCES tenants(id),
    suite        TEXT NOT NULL DEFAULT '',
    score        DOUBLE PRECISION,
    band         TEXT NOT NULL DEFAULT '',
    metadata     JSONB NOT NULL DEFAULT '{}',
    started_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    completed_at TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS idx_benchmark_runs_tenant ON benchmark_runs(tenant_id, started_at DESC);

CREATE TABLE IF NOT EXISTS benchmark_results (
    id              TEXT PRIMARY KEY,
    run_id          TEXT NOT NULL REFERENCES benchmark_runs(id),
    case_id         TEXT NOT NULL,
    expected_signal TEXT NOT NULL DEFAULT '',
    actual_signal   TEXT NOT NULL DEFAULT '',
    passed          BOOLEAN NOT NULL DEFAULT false,
    details         JSONB NOT NULL DEFAULT '{}'
);

CREATE INDEX IF NOT EXISTS idx_benchmark_results_run ON benchmark_results(run_id);
