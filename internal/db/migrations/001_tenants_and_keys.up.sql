-- 001_tenants_and_keys.sql
CREATE TABLE IF NOT EXISTS tenants (
    id          TEXT PRIMARY KEY,
    label       TEXT NOT NULL DEFAULT '',
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Seed the default tenant (used by static-key auth in Phase 0).
INSERT INTO tenants (id, label) VALUES ('default', 'Default Tenant')
ON CONFLICT (id) DO NOTHING;

CREATE TABLE IF NOT EXISTS api_keys (
    id          TEXT PRIMARY KEY,
    tenant_id   TEXT NOT NULL REFERENCES tenants(id),
    key_hash    BYTEA NOT NULL,
    prefix      TEXT NOT NULL,
    label       TEXT NOT NULL DEFAULT '',
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    revoked_at  TIMESTAMPTZ,
    last_used_at TIMESTAMPTZ
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_api_keys_hash ON api_keys(key_hash) WHERE revoked_at IS NULL;
CREATE INDEX IF NOT EXISTS idx_api_keys_tenant ON api_keys(tenant_id);
