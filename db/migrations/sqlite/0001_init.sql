PRAGMA foreign_keys = ON;

CREATE TABLE IF NOT EXISTS events (
  id             TEXT PRIMARY KEY,
  type           TEXT NOT NULL,
  source         TEXT NOT NULL,
  subject        TEXT,
  event_time     TEXT NOT NULL,
  extensions     TEXT,
  data           TEXT NOT NULL,
  integrity_hash TEXT NOT NULL,
  ingested_at    TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now'))
);

CREATE TABLE IF NOT EXISTS exports (
  id            TEXT PRIMARY KEY,
  status        TEXT NOT NULL,
  format        TEXT NOT NULL,
  filter_json   TEXT NOT NULL DEFAULT '{}',
  artifact_uri  TEXT,
  error_message TEXT,
  created_at    TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
  completed_at  TEXT
);

CREATE INDEX IF NOT EXISTS idx_events_subject_time ON events (subject, event_time);
CREATE INDEX IF NOT EXISTS idx_events_type_time    ON events (type, event_time);
CREATE INDEX IF NOT EXISTS idx_events_event_time   ON events (event_time);

CREATE TRIGGER IF NOT EXISTS trg_events_no_update
BEFORE UPDATE ON events
BEGIN
  SELECT RAISE(ABORT, 'append-only table: mutations are not allowed');
END;

CREATE TRIGGER IF NOT EXISTS trg_events_no_delete
BEFORE DELETE ON events
BEGIN
  SELECT RAISE(ABORT, 'append-only table: mutations are not allowed');
END;
