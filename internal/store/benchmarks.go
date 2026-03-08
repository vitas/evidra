package store

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/oklog/ulid/v2"
)

// BenchmarkRun represents a submitted benchmark run.
type BenchmarkRun struct {
	ID          string          `json:"id"`
	TenantID    string          `json:"tenant_id"`
	Suite       string          `json:"suite"`
	Score       *float64        `json:"score,omitempty"`
	Band        string          `json:"band"`
	Metadata    json.RawMessage `json:"metadata,omitempty"`
	StartedAt   time.Time       `json:"started_at"`
	CompletedAt *time.Time      `json:"completed_at,omitempty"`
}

// BenchmarkResult represents one case result within a run.
type BenchmarkResult struct {
	ID             string          `json:"id"`
	RunID          string          `json:"run_id"`
	CaseID         string          `json:"case_id"`
	ExpectedSignal string          `json:"expected_signal"`
	ActualSignal   string          `json:"actual_signal"`
	Passed         bool            `json:"passed"`
	Details        json.RawMessage `json:"details,omitempty"`
}

// BenchmarkStore manages benchmark runs backed by PostgreSQL.
type BenchmarkStore struct {
	pool *pgxpool.Pool
}

// NewBenchmarkStore creates a BenchmarkStore.
func NewBenchmarkStore(pool *pgxpool.Pool) *BenchmarkStore {
	return &BenchmarkStore{pool: pool}
}

// SaveRun persists a benchmark run and its results.
func (bs *BenchmarkStore) SaveRun(ctx context.Context, run BenchmarkRun, results []BenchmarkResult) (string, error) {
	if run.ID == "" {
		run.ID = ulid.Make().String()
	}

	tx, err := bs.pool.Begin(ctx)
	if err != nil {
		return "", fmt.Errorf("store.SaveRun: begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	_, err = tx.Exec(ctx,
		`INSERT INTO benchmark_runs (id, tenant_id, suite, score, band, metadata, started_at, completed_at)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8)`,
		run.ID, run.TenantID, run.Suite, run.Score, run.Band, run.Metadata, run.StartedAt, run.CompletedAt,
	)
	if err != nil {
		return "", fmt.Errorf("store.SaveRun: insert run: %w", err)
	}

	for _, r := range results {
		if r.ID == "" {
			r.ID = ulid.Make().String()
		}
		_, err = tx.Exec(ctx,
			`INSERT INTO benchmark_results (id, run_id, case_id, expected_signal, actual_signal, passed, details)
			 VALUES ($1,$2,$3,$4,$5,$6,$7)`,
			r.ID, run.ID, r.CaseID, r.ExpectedSignal, r.ActualSignal, r.Passed, r.Details,
		)
		if err != nil {
			return "", fmt.Errorf("store.SaveRun: insert result: %w", err)
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return "", fmt.Errorf("store.SaveRun: commit: %w", err)
	}

	return run.ID, nil
}

// ListRuns returns benchmark runs for a tenant, newest first.
func (bs *BenchmarkStore) ListRuns(ctx context.Context, tenantID string, limit, offset int) ([]BenchmarkRun, error) {
	if limit <= 0 {
		limit = 50
	}
	if limit > 200 {
		limit = 200
	}

	rows, err := bs.pool.Query(ctx,
		`SELECT id, tenant_id, suite, score, band, metadata, started_at, completed_at
		 FROM benchmark_runs
		 WHERE tenant_id = $1
		 ORDER BY started_at DESC
		 LIMIT $2 OFFSET $3`,
		tenantID, limit, offset,
	)
	if err != nil {
		return nil, fmt.Errorf("store.ListRuns: %w", err)
	}
	defer rows.Close()

	var runs []BenchmarkRun
	for rows.Next() {
		var r BenchmarkRun
		if err := rows.Scan(&r.ID, &r.TenantID, &r.Suite, &r.Score, &r.Band,
			&r.Metadata, &r.StartedAt, &r.CompletedAt); err != nil {
			return nil, fmt.Errorf("store.ListRuns: scan: %w", err)
		}
		runs = append(runs, r)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store.ListRuns: rows: %w", err)
	}
	return runs, nil
}

// GetRunWithResults retrieves a run and all its case results.
func (bs *BenchmarkStore) GetRunWithResults(ctx context.Context, tenantID, runID string) (BenchmarkRun, []BenchmarkResult, error) {
	var run BenchmarkRun
	err := bs.pool.QueryRow(ctx,
		`SELECT id, tenant_id, suite, score, band, metadata, started_at, completed_at
		 FROM benchmark_runs WHERE id = $1 AND tenant_id = $2`,
		runID, tenantID,
	).Scan(&run.ID, &run.TenantID, &run.Suite, &run.Score, &run.Band,
		&run.Metadata, &run.StartedAt, &run.CompletedAt)
	if err != nil {
		return BenchmarkRun{}, nil, fmt.Errorf("store.GetRunWithResults: run: %w", err)
	}

	rows, err := bs.pool.Query(ctx,
		`SELECT id, run_id, case_id, expected_signal, actual_signal, passed, details
		 FROM benchmark_results WHERE run_id = $1 ORDER BY case_id`,
		runID,
	)
	if err != nil {
		return run, nil, fmt.Errorf("store.GetRunWithResults: results: %w", err)
	}
	defer rows.Close()

	var results []BenchmarkResult
	for rows.Next() {
		var r BenchmarkResult
		if err := rows.Scan(&r.ID, &r.RunID, &r.CaseID, &r.ExpectedSignal,
			&r.ActualSignal, &r.Passed, &r.Details); err != nil {
			return run, nil, fmt.Errorf("store.GetRunWithResults: scan: %w", err)
		}
		results = append(results, r)
	}
	if err := rows.Err(); err != nil {
		return run, nil, fmt.Errorf("store.GetRunWithResults: rows: %w", err)
	}
	return run, results, nil
}
