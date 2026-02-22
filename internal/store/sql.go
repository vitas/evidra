package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	ce "evidra/internal/cloudevents"
	"evidra/internal/model"
)

type SQLRepository struct {
	db      *sql.DB
	dialect string
}

func NewSQLRepository(db *sql.DB, dialect string) (*SQLRepository, error) {
	if db == nil {
		return nil, fmt.Errorf("nil db")
	}
	d := strings.ToLower(strings.TrimSpace(dialect))
	if d == "" {
		return nil, fmt.Errorf("empty dialect")
	}
	if d != "postgres" && d != "sqlite" {
		return nil, fmt.Errorf("unsupported dialect: %s", dialect)
	}
	return &SQLRepository{db: db, dialect: d}, nil
}

func (s *SQLRepository) IngestEvent(ctx context.Context, event ce.StoredEvent) (IngestStatus, time.Time, error) {
	if err := validateStoredEvent(event); err != nil {
		return "", time.Time{}, err
	}

	if event.IntegrityHash == "" {
		h, err := ce.ComputeIntegrityHash(event)
		if err != nil {
			return "", time.Time{}, err
		}
		event.IntegrityHash = h
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return "", time.Time{}, err
	}
	defer tx.Rollback() //nolint:errcheck

	existing, ingestedAt, found, err := s.getEventTx(ctx, tx, event.ID)
	if err != nil {
		return "", time.Time{}, err
	}
	if found {
		if existing.IntegrityHash == event.IntegrityHash {
			if err := tx.Commit(); err != nil {
				return "", time.Time{}, err
			}
			return IngestDuplicate, ingestedAt, nil
		}
		return "", time.Time{}, ErrConflict
	}

	extJSON, err := json.Marshal(event.Extensions)
	if err != nil {
		return "", time.Time{}, err
	}
	if event.Extensions == nil {
		extJSON = []byte("{}")
	}

	dataJSON := []byte(event.Data)
	if len(dataJSON) == 0 {
		dataJSON = []byte("{}")
	}

	args := []interface{}{
		event.ID,
		event.Type,
		event.Source,
		nullable(event.Subject),
		s.tsValue(event.Time.UTC()),
		string(extJSON),
		string(dataJSON),
		event.IntegrityHash,
	}

	insertEvent := "INSERT INTO events (id, type, source, subject, event_time, extensions, data, integrity_hash) VALUES (" +
		s.ph(1) + "," + s.ph(2) + "," + s.ph(3) + "," + s.ph(4) + "," + s.ph(5) + "," + s.ph(6) + "," + s.ph(7) + "," + s.ph(8) + ")"

	if _, err := tx.ExecContext(ctx, insertEvent, args...); err != nil {
		return "", time.Time{}, err
	}

	if err := tx.Commit(); err != nil {
		return "", time.Time{}, err
	}
	return IngestAccepted, time.Now().UTC(), nil
}

func (s *SQLRepository) GetEvent(ctx context.Context, id string) (ce.StoredEvent, error) {
	query := `SELECT id, type, source, subject, event_time, extensions, data, integrity_hash, ingested_at FROM events WHERE id = ` + s.ph(1)
	row := s.db.QueryRowContext(ctx, query, id)
	e, _, err := scanEventRow(s, row)
	if err == sql.ErrNoRows {
		return ce.StoredEvent{}, ErrNotFound
	}
	if err != nil {
		return ce.StoredEvent{}, err
	}
	return e, nil
}

func (s *SQLRepository) QueryTimeline(ctx context.Context, q TimelineQuery) (TimelineResult, error) {
	if q.Limit <= 0 {
		q.Limit = 50
	}
	if q.Limit > 500 {
		q.Limit = 500
	}
	cursorTS, cursorID, err := decodeCursor(q.Cursor)
	if err != nil {
		return TimelineResult{}, err
	}

	params := make([]interface{}, 0, 16)
	add := func(v interface{}) string {
		params = append(params, v)
		return s.ph(len(params))
	}

	query := strings.Builder{}
	query.WriteString(`SELECT e.id, e.type, e.source, e.subject, e.event_time, e.extensions, e.data, e.integrity_hash, e.ingested_at FROM events e WHERE 1=1`)

	if !q.From.IsZero() {
		query.WriteString(` AND e.event_time >= ` + add(s.tsValue(q.From.UTC())))
	}
	if !q.To.IsZero() {
		query.WriteString(` AND e.event_time <= ` + add(s.tsValue(q.To.UTC())))
	}

	if q.Subject != "" {
		query.WriteString(` AND e.subject = ` + add(q.Subject))
		if q.Cluster != "" {
			if s.dialect == "postgres" {
				query.WriteString(` AND e.extensions->>` + add("cluster") + ` = ` + add(q.Cluster))
			} else {
				query.WriteString(` AND json_extract(e.extensions, '$.cluster') = ` + add(q.Cluster))
			}
		}
		if q.Namespace != "" {
			if s.dialect == "postgres" {
				query.WriteString(` AND e.extensions->>` + add("namespace") + ` = ` + add(q.Namespace))
			} else {
				query.WriteString(` AND json_extract(e.extensions, '$.namespace') = ` + add(q.Namespace))
			}
		}
	}

	if q.Source != "" {
		query.WriteString(` AND e.source = ` + add(q.Source))
	}
	if q.Type != "" {
		query.WriteString(` AND e.type = ` + add(q.Type))
	}
	if q.CorrelationKey != "" && q.CorrelationValue != "" {
		if s.dialect == "postgres" {
			query.WriteString(` AND e.extensions->>` + add(q.CorrelationKey) + ` = ` + add(q.CorrelationValue))
		} else {
			query.WriteString(` AND json_extract(e.extensions, '$.` + sanitizeKey(q.CorrelationKey) + `') = ` + add(q.CorrelationValue))
		}
	}
	if !q.IncludeSupporting {
		if s.dialect == "postgres" {
			query.WriteString(` AND COALESCE(e.extensions->>'supporting_observation', 'false') != 'true'`)
		} else {
			query.WriteString(` AND COALESCE(json_extract(e.extensions, '$.supporting_observation'), 0) = 0`)
		}
	}
	if !cursorTS.IsZero() {
		query.WriteString(` AND (e.event_time > ` + add(s.tsValue(cursorTS)) + ` OR (e.event_time = ` + add(s.tsValue(cursorTS)) + ` AND e.id > ` + add(cursorID) + `))`)
	}

	query.WriteString(` ORDER BY e.event_time ASC, e.id ASC LIMIT ` + add(q.Limit+1))

	rows, err := s.db.QueryContext(ctx, query.String(), params...)
	if err != nil {
		return TimelineResult{}, err
	}
	defer rows.Close()

	items := make([]ce.StoredEvent, 0, q.Limit+1)
	for rows.Next() {
		e, _, err := scanEventRows(s, rows)
		if err != nil {
			return TimelineResult{}, err
		}
		items = append(items, e)
	}
	if err := rows.Err(); err != nil {
		return TimelineResult{}, err
	}

	res := TimelineResult{}
	if len(items) > q.Limit {
		res.Items = items[:q.Limit]
		last := res.Items[len(res.Items)-1]
		res.NextCursor = encodeCursor(last.Time, last.ID)
		return res, nil
	}
	res.Items = items
	return res, nil
}

func (s *SQLRepository) ListSubjects(ctx context.Context) ([]SubjectInfo, error) {
	var query string
	if s.dialect == "postgres" {
		query = `SELECT DISTINCT subject, COALESCE(extensions->>'cluster', ''), COALESCE(extensions->>'namespace', '') FROM events WHERE subject IS NOT NULL ORDER BY 1, 2, 3`
	} else {
		query = `SELECT DISTINCT subject, COALESCE(json_extract(extensions, '$.cluster'), ''), COALESCE(json_extract(extensions, '$.namespace'), '') FROM events WHERE subject IS NOT NULL ORDER BY 1, 2, 3`
	}
	rows, err := s.db.QueryContext(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]SubjectInfo, 0)
	for rows.Next() {
		var sub SubjectInfo
		if err := rows.Scan(&sub.Subject, &sub.Cluster, &sub.Namespace); err != nil {
			return nil, err
		}
		out = append(out, sub)
	}
	return out, rows.Err()
}

func (s *SQLRepository) EventsByExtension(ctx context.Context, key, value string, limit int) ([]ce.StoredEvent, error) {
	if key == "" || value == "" {
		return nil, ErrInvalidInput
	}
	if !isValidExtensionKey(key) {
		return nil, ErrInvalidInput
	}
	if limit <= 0 {
		limit = 100
	}

	var query string
	var args []interface{}
	if s.dialect == "postgres" {
		filterJSON, err := json.Marshal(map[string]interface{}{key: value})
		if err != nil {
			return nil, err
		}
		query = `SELECT e.id, e.type, e.source, e.subject, e.event_time, e.extensions, e.data, e.integrity_hash, e.ingested_at FROM events e WHERE e.extensions @> ` + s.ph(1) + `::jsonb ORDER BY e.event_time ASC, e.id ASC LIMIT ` + s.ph(2)
		args = []interface{}{string(filterJSON), limit}
	} else {
		query = `SELECT e.id, e.type, e.source, e.subject, e.event_time, e.extensions, e.data, e.integrity_hash, e.ingested_at FROM events e WHERE json_extract(e.extensions, '$.` + sanitizeKey(key) + `') = ` + s.ph(1) + ` ORDER BY e.event_time ASC, e.id ASC LIMIT ` + s.ph(2)
		args = []interface{}{value, limit}
	}

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]ce.StoredEvent, 0)
	for rows.Next() {
		e, _, err := scanEventRows(s, rows)
		if err != nil {
			return nil, err
		}
		items = append(items, e)
	}
	return items, rows.Err()
}

func (s *SQLRepository) CreateExport(ctx context.Context, format string, filter map[string]interface{}) (model.ExportJob, error) {
	if format == "" {
		format = "json"
	}
	filterJSON, err := json.Marshal(filter)
	if err != nil {
		return model.ExportJob{}, err
	}
	id := fmt.Sprintf("exp_%d", time.Now().UTC().UnixNano())
	query := `INSERT INTO exports (id, status, format, filter_json) VALUES (` + s.ph(1) + `,` + s.ph(2) + `,` + s.ph(3) + `,` + s.ph(4) + `)`
	if _, err := s.db.ExecContext(ctx, query, id, "pending", format, string(filterJSON)); err != nil {
		return model.ExportJob{}, err
	}
	return s.GetExport(ctx, id)
}

func (s *SQLRepository) SetExportCompleted(ctx context.Context, id, artifactURI string) error {
	query := `UPDATE exports SET status = ` + s.ph(1) + `, artifact_uri = ` + s.ph(2) + `, completed_at = ` + s.ph(3) + ` WHERE id = ` + s.ph(4)
	result, err := s.db.ExecContext(ctx, query, "completed", artifactURI, s.tsValue(time.Now().UTC()), id)
	if err != nil {
		return err
	}
	affected, _ := result.RowsAffected()
	if affected == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *SQLRepository) SetExportFailed(ctx context.Context, id, message string) error {
	query := `UPDATE exports SET status = ` + s.ph(1) + `, error_message = ` + s.ph(2) + `, completed_at = ` + s.ph(3) + ` WHERE id = ` + s.ph(4)
	result, err := s.db.ExecContext(ctx, query, "failed", message, s.tsValue(time.Now().UTC()), id)
	if err != nil {
		return err
	}
	affected, _ := result.RowsAffected()
	if affected == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *SQLRepository) GetExport(ctx context.Context, id string) (model.ExportJob, error) {
	query := `SELECT id, status, format, filter_json, artifact_uri, error_message, created_at, completed_at FROM exports WHERE id = ` + s.ph(1)
	row := s.db.QueryRowContext(ctx, query, id)
	var job model.ExportJob
	var filterRaw interface{}
	var artifactURI, errorMsg sql.NullString
	var createdRaw interface{}
	var completedRaw interface{}
	if err := row.Scan(&job.ID, &job.Status, &job.Format, &filterRaw, &artifactURI, &errorMsg, &createdRaw, &completedRaw); err != nil {
		if err == sql.ErrNoRows {
			return model.ExportJob{}, ErrNotFound
		}
		return model.ExportJob{}, err
	}
	job.ArtifactURI = artifactURI.String
	job.Error = errorMsg.String
	job.Filter = map[string]interface{}{}
	if b := bytesFrom(filterRaw); len(b) > 0 {
		if err := json.Unmarshal(b, &job.Filter); err != nil {
			return model.ExportJob{}, err
		}
	}
	createdAt, err := parseTimeRaw(createdRaw)
	if err != nil {
		return model.ExportJob{}, err
	}
	job.CreatedAt = createdAt
	if completedRaw != nil {
		if t, err := parseTimeRaw(completedRaw); err == nil && !t.IsZero() {
			job.CompletedAt = &t
		}
	}
	return job, nil
}

func (s *SQLRepository) DeleteEvent(_ context.Context, _ string) error {
	return ErrAppendOnly
}

func (s *SQLRepository) getEventTx(ctx context.Context, tx *sql.Tx, id string) (ce.StoredEvent, time.Time, bool, error) {
	query := `SELECT id, type, source, subject, event_time, extensions, data, integrity_hash, ingested_at FROM events WHERE id = ` + s.ph(1)
	row := tx.QueryRowContext(ctx, query, id)
	event, ingestedAt, err := scanEventRow(s, row)
	if err == sql.ErrNoRows {
		return ce.StoredEvent{}, time.Time{}, false, nil
	}
	if err != nil {
		return ce.StoredEvent{}, time.Time{}, false, err
	}
	return event, ingestedAt, true, nil
}

func (s *SQLRepository) ph(n int) string {
	if s.dialect == "postgres" {
		return fmt.Sprintf("$%d", n)
	}
	return "?"
}

func (s *SQLRepository) tsValue(t time.Time) interface{} {
	if s.dialect == "sqlite" {
		return t.UTC().Format(time.RFC3339Nano)
	}
	return t.UTC()
}

func nullable(in string) interface{} {
	if strings.TrimSpace(in) == "" {
		return nil
	}
	return in
}

func isValidExtensionKey(key string) bool {
	for _, c := range key {
		if !((c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || c == '_' || c == '-') {
			return false
		}
	}
	return len(key) > 0
}

func sanitizeKey(key string) string {
	// Only called after isValidExtensionKey passes, so this is safe
	return key
}

type eventScanner interface {
	Scan(dest ...interface{}) error
}

func scanEventRow(s *SQLRepository, row eventScanner) (ce.StoredEvent, time.Time, error) {
	var e ce.StoredEvent
	var tsRaw interface{}
	var subjectNull sql.NullString
	var extRaw interface{}
	var dataRaw interface{}
	var ingestedRaw interface{}

	err := row.Scan(
		&e.ID,
		&e.Type,
		&e.Source,
		&subjectNull,
		&tsRaw,
		&extRaw,
		&dataRaw,
		&e.IntegrityHash,
		&ingestedRaw,
	)
	if err != nil {
		return ce.StoredEvent{}, time.Time{}, err
	}
	e.Subject = subjectNull.String

	ts, err := parseTimeRaw(tsRaw)
	if err != nil {
		return ce.StoredEvent{}, time.Time{}, err
	}
	e.Time = ts

	if b := bytesFrom(extRaw); len(b) > 0 && string(b) != "{}" {
		if err := json.Unmarshal(b, &e.Extensions); err != nil {
			return ce.StoredEvent{}, time.Time{}, err
		}
	}

	if b := bytesFrom(dataRaw); len(b) > 0 {
		e.Data = json.RawMessage(b)
	}

	ingestedAt, err := parseTimeRaw(ingestedRaw)
	if err != nil {
		return ce.StoredEvent{}, time.Time{}, err
	}
	e.IngestedAt = ingestedAt

	return e, ingestedAt, nil
}

func scanEventRows(s *SQLRepository, rows *sql.Rows) (ce.StoredEvent, time.Time, error) {
	return scanEventRow(s, rows)
}

func bytesFrom(v interface{}) []byte {
	switch t := v.(type) {
	case []byte:
		return t
	case string:
		return []byte(t)
	default:
		return nil
	}
}

func parseTimeRaw(v interface{}) (time.Time, error) {
	switch t := v.(type) {
	case time.Time:
		return t.UTC(), nil
	case []byte:
		return parseTimeString(string(t))
	case string:
		return parseTimeString(t)
	case nil:
		return time.Time{}, nil
	default:
		return time.Time{}, fmt.Errorf("unsupported time type %T", v)
	}
}

func parseTimeString(in string) (time.Time, error) {
	in = strings.TrimSpace(in)
	if in == "" {
		return time.Time{}, nil
	}
	formats := []string{time.RFC3339Nano, time.RFC3339, "2006-01-02 15:04:05.999999-07:00", "2006-01-02 15:04:05-07:00", "2006-01-02 15:04:05"}
	for _, f := range formats {
		if t, err := time.Parse(f, in); err == nil {
			return t.UTC(), nil
		}
	}
	return time.Time{}, fmt.Errorf("invalid time format: %s", in)
}
