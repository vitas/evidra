-- 002_evidence_entries.sql
CREATE TABLE IF NOT EXISTS evidence_entries (
    id              TEXT PRIMARY KEY,
    tenant_id       TEXT NOT NULL REFERENCES tenants(id),
    entry_type      TEXT NOT NULL,
    session_id      TEXT NOT NULL,
    operation_id    TEXT NOT NULL DEFAULT '',
    previous_hash   TEXT NOT NULL DEFAULT '',
    hash            TEXT NOT NULL,
    signature       TEXT NOT NULL DEFAULT '',
    intent_digest   TEXT NOT NULL DEFAULT '',
    artifact_digest TEXT NOT NULL DEFAULT '',
    payload         JSONB NOT NULL DEFAULT '{}',
    scope_dimensions JSONB NOT NULL DEFAULT '{}',
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_entries_tenant ON evidence_entries(tenant_id);
CREATE INDEX IF NOT EXISTS idx_entries_type ON evidence_entries(tenant_id, entry_type);
CREATE INDEX IF NOT EXISTS idx_entries_session ON evidence_entries(tenant_id, session_id);
CREATE INDEX IF NOT EXISTS idx_entries_created ON evidence_entries(tenant_id, created_at DESC);
