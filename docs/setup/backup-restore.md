# Backup and Restore

This runbook covers minimal backup and restore steps for v1.

## Scope

- Primary data: PostgreSQL database
- Export artifacts: files under `EVIDRA_EXPORT_DIR` (for example `/var/evidra/exports`)

## Backup

1. Database dump:

Local/DSN mode:

```bash
pg_dump --format=custom --dbname="$EVIDRA_DB_DSN" --file=evidra_$(date +%Y%m%d_%H%M%S).dump
```

Kubernetes split-variable mode:

```bash
export PGHOST="$EVIDRA_DB_HOST"
export PGPORT="${EVIDRA_DB_PORT:-5432}"
export PGDATABASE="$EVIDRA_DB_NAME"
export PGUSER="$EVIDRA_DB_USER"
export PGPASSWORD="$EVIDRA_DB_PASSWORD"
pg_dump --format=custom --file=evidra_$(date +%Y%m%d_%H%M%S).dump
```

2. Export artifacts:

```bash
tar -czf evidra_exports_$(date +%Y%m%d_%H%M%S).tgz /var/evidra/exports
```

3. Store both files in your backup target with retention policy.

## Restore

1. Stop write traffic to Evidra.
2. Restore database:

Local/DSN mode:

```bash
pg_restore --clean --if-exists --dbname="$EVIDRA_DB_DSN" evidra_YYYYMMDD_HHMMSS.dump
```

Kubernetes split-variable mode:

```bash
export PGHOST="$EVIDRA_DB_HOST"
export PGPORT="${EVIDRA_DB_PORT:-5432}"
export PGDATABASE="$EVIDRA_DB_NAME"
export PGUSER="$EVIDRA_DB_USER"
export PGPASSWORD="$EVIDRA_DB_PASSWORD"
pg_restore --clean --if-exists evidra_YYYYMMDD_HHMMSS.dump
```

3. Restore artifacts:

```bash
tar -xzf evidra_exports_YYYYMMDD_HHMMSS.tgz -C /
```

4. Restart Evidra deployment.
5. Run health and query checks:

```bash
curl -fsS http://127.0.0.1:8080/healthz
curl -fsS http://127.0.0.1:8080/v1/subjects
```

## Frequency

- Database: at least daily, more often for high-change environments.
- Export artifacts: same schedule as database backup.
- Perform a restore drill at least once per quarter.
