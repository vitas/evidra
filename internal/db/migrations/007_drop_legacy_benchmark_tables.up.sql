-- 007: Drop legacy benchmark tables replaced by external benchmark storage.
-- Historical migration 003_benchmark_runs.up.sql is preserved in repo history.

DROP TABLE IF EXISTS benchmark_results;
DROP TABLE IF EXISTS benchmark_runs;
