-- 007: Drop legacy run/result tables from the old core experiment.
-- This stays in core so existing deployments can clean up historical tables.

DROP TABLE IF EXISTS benchmark_results;
DROP TABLE IF EXISTS benchmark_runs;
