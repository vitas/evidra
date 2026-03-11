CREATE TABLE IF NOT EXISTS webhook_events (
    tenant_id        TEXT NOT NULL REFERENCES tenants(id),
    source           TEXT NOT NULL,
    idempotency_key  TEXT NOT NULL,
    payload          JSONB NOT NULL DEFAULT '{}',
    created_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (tenant_id, source, idempotency_key)
);

CREATE INDEX IF NOT EXISTS idx_webhook_events_created
    ON webhook_events(tenant_id, source, created_at DESC);
