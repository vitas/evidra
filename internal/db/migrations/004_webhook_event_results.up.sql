ALTER TABLE webhook_events
    ADD COLUMN IF NOT EXISTS result_entry_id TEXT,
    ADD COLUMN IF NOT EXISTS result_effective_risk TEXT;
