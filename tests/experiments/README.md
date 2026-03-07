# Experiment Tests

Structure:
- `tests/experiments/runners/` — runner behavior tests (`run-agent-*`).
- `tests/experiments/execution-scenarios/` — JSON scenarios used by execution-mode experiments.
- `internal/experiments/*_test.go` — adapter and runner unit/integration tests in Go.

Quick run:

```bash
go test ./internal/experiments -count=1
bash tests/experiments/runners/run_agent_execution_experiments_test.sh
```
