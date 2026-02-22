BEGIN;

CREATE TABLE IF NOT EXISTS events (
  id             TEXT PRIMARY KEY,
  type           TEXT NOT NULL,
  source         TEXT NOT NULL,
  subject        TEXT,
  event_time     TIMESTAMPTZ NOT NULL,
  extensions     JSONB,
  data           JSONB NOT NULL,
  integrity_hash TEXT NOT NULL,
  ingested_at    TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS exports (
  id            TEXT PRIMARY KEY,
  status        TEXT NOT NULL,
  format        TEXT NOT NULL,
  filter_json   JSONB NOT NULL DEFAULT '{}'::jsonb,
  artifact_uri  TEXT,
  error_message TEXT,
  created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  completed_at  TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS idx_events_subject_time ON events (subject, event_time);
CREATE INDEX IF NOT EXISTS idx_events_type_time    ON events (type, event_time);
CREATE INDEX IF NOT EXISTS idx_events_event_time   ON events (event_time);
CREATE INDEX IF NOT EXISTS idx_events_extensions   ON events USING GIN (extensions);

CREATE OR REPLACE FUNCTION prevent_evidence_mutation()
RETURNS TRIGGER AS $$
BEGIN
  RAISE EXCEPTION 'append-only table: mutations are not allowed';
END;
$$ LANGUAGE plpgsql;

DROP TRIGGER IF EXISTS trg_events_no_update ON events;
DROP TRIGGER IF EXISTS trg_events_no_delete ON events;

CREATE TRIGGER trg_events_no_update
BEFORE UPDATE ON events
FOR EACH ROW EXECUTE FUNCTION prevent_evidence_mutation();

CREATE TRIGGER trg_events_no_delete
BEFORE DELETE ON events
FOR EACH ROW EXECUTE FUNCTION prevent_evidence_mutation();

COMMIT;
