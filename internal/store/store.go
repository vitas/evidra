package store

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	ce "evidra/internal/cloudevents"
	"evidra/internal/model"
)

var (
	ErrNotFound      = errors.New("not found")
	ErrConflict      = errors.New("conflict")
	ErrInvalidInput  = errors.New("invalid input")
	ErrAppendOnly    = errors.New("append-only mutation not allowed")
	ErrInvalidCursor = errors.New("invalid cursor")
)

type IngestStatus string

const (
	IngestAccepted  IngestStatus = "accepted"
	IngestDuplicate IngestStatus = "duplicate"
)

// SubjectInfo represents a distinct (subject, cluster, namespace) triple observed in events.
type SubjectInfo struct {
	Subject   string `json:"subject"`
	Cluster   string `json:"cluster,omitempty"`
	Namespace string `json:"namespace,omitempty"`
}

type TimelineQuery struct {
	Subject           string
	Cluster           string
	Namespace         string
	From              time.Time
	To                time.Time
	Source            string
	Type              string
	CorrelationKey    string
	CorrelationValue  string
	IncludeSupporting bool
	Limit             int
	Cursor            string
}

type TimelineResult struct {
	Items      []ce.StoredEvent
	NextCursor string
}

type Repository interface {
	IngestEvent(ctx context.Context, event ce.StoredEvent) (IngestStatus, time.Time, error)
	GetEvent(ctx context.Context, id string) (ce.StoredEvent, error)
	QueryTimeline(ctx context.Context, q TimelineQuery) (TimelineResult, error)
	ListSubjects(ctx context.Context) ([]SubjectInfo, error)
	EventsByExtension(ctx context.Context, key, value string, limit int) ([]ce.StoredEvent, error)
	CreateExport(ctx context.Context, format string, filter map[string]interface{}) (model.ExportJob, error)
	SetExportCompleted(ctx context.Context, id, artifactURI string) error
	SetExportFailed(ctx context.Context, id, message string) error
	GetExport(ctx context.Context, id string) (model.ExportJob, error)
	DeleteEvent(ctx context.Context, id string) error
}

type MemoryRepository struct {
	mu      sync.RWMutex
	events  map[string]ce.StoredEvent
	exports map[string]model.ExportJob
}

func NewMemoryRepository() *MemoryRepository {
	return &MemoryRepository{
		events:  make(map[string]ce.StoredEvent),
		exports: make(map[string]model.ExportJob),
	}
}

func (m *MemoryRepository) IngestEvent(_ context.Context, event ce.StoredEvent) (IngestStatus, time.Time, error) {
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

	now := time.Now().UTC()
	m.mu.Lock()
	defer m.mu.Unlock()

	if existing, ok := m.events[event.ID]; ok {
		if existing.IntegrityHash == event.IntegrityHash {
			return IngestDuplicate, existing.IngestedAt, nil
		}
		return "", time.Time{}, ErrConflict
	}

	if event.IngestedAt.IsZero() {
		event.IngestedAt = now
	}
	m.events[event.ID] = event
	return IngestAccepted, now, nil
}

func (m *MemoryRepository) GetEvent(_ context.Context, id string) (ce.StoredEvent, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	e, ok := m.events[id]
	if !ok {
		return ce.StoredEvent{}, ErrNotFound
	}
	return e, nil
}

func (m *MemoryRepository) QueryTimeline(_ context.Context, q TimelineQuery) (TimelineResult, error) {
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

	m.mu.RLock()
	defer m.mu.RUnlock()

	items := make([]ce.StoredEvent, 0)
	for _, e := range m.events {
		if !q.From.IsZero() && e.Time.Before(q.From) {
			continue
		}
		if !q.To.IsZero() && e.Time.After(q.To) {
			continue
		}
		if q.Subject != "" && e.Subject != q.Subject {
			continue
		}
		if q.Subject != "" && q.Cluster != "" && ce.ExtensionString(e.Extensions, "cluster") != q.Cluster {
			continue
		}
		if q.Subject != "" && q.Namespace != "" && ce.ExtensionString(e.Extensions, "namespace") != q.Namespace {
			continue
		}
		if q.Source != "" && e.Source != q.Source {
			continue
		}
		if q.Type != "" && e.Type != q.Type {
			continue
		}
		if q.CorrelationKey != "" && q.CorrelationValue != "" {
			if ce.ExtensionString(e.Extensions, q.CorrelationKey) != q.CorrelationValue {
				continue
			}
		}
		if !q.IncludeSupporting && ce.ExtensionBool(e.Extensions, "supporting_observation") {
			continue
		}
		if !cursorTS.IsZero() {
			if e.Time.Before(cursorTS) || (e.Time.Equal(cursorTS) && e.ID <= cursorID) {
				continue
			}
		}
		items = append(items, e)
	}

	sortStoredEvents(items)
	result := TimelineResult{}
	if len(items) > q.Limit {
		result.Items = items[:q.Limit]
		last := result.Items[len(result.Items)-1]
		result.NextCursor = encodeCursor(last.Time, last.ID)
		return result, nil
	}
	result.Items = items
	return result, nil
}

func (m *MemoryRepository) ListSubjects(_ context.Context) ([]SubjectInfo, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	seen := make(map[string]struct{})
	out := make([]SubjectInfo, 0)
	for _, e := range m.events {
		cluster := ce.ExtensionString(e.Extensions, "cluster")
		namespace := ce.ExtensionString(e.Extensions, "namespace")
		key := e.Subject + ":" + cluster + ":" + namespace
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, SubjectInfo{
			Subject:   e.Subject,
			Cluster:   cluster,
			Namespace: namespace,
		})
	}
	sort.Slice(out, func(i, j int) bool {
		li := out[i].Subject + ":" + out[i].Cluster + ":" + out[i].Namespace
		lj := out[j].Subject + ":" + out[j].Cluster + ":" + out[j].Namespace
		return li < lj
	})
	return out, nil
}

func (m *MemoryRepository) EventsByExtension(_ context.Context, key, value string, limit int) ([]ce.StoredEvent, error) {
	if key == "" || value == "" {
		return nil, ErrInvalidInput
	}
	if limit <= 0 {
		limit = 100
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]ce.StoredEvent, 0)
	for _, e := range m.events {
		if ce.ExtensionString(e.Extensions, key) == value {
			out = append(out, e)
		}
	}
	sortStoredEvents(out)
	if len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

func (m *MemoryRepository) CreateExport(_ context.Context, format string, filter map[string]interface{}) (model.ExportJob, error) {
	if format == "" {
		format = "json"
	}
	id := fmt.Sprintf("exp_%d", time.Now().UTC().UnixNano())
	job := model.ExportJob{
		ID:        id,
		Status:    "pending",
		Format:    format,
		Filter:    filter,
		CreatedAt: time.Now().UTC(),
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.exports[id] = job
	return job, nil
}

func (m *MemoryRepository) SetExportCompleted(_ context.Context, id, artifactURI string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	job, ok := m.exports[id]
	if !ok {
		return ErrNotFound
	}
	now := time.Now().UTC()
	job.Status = "completed"
	job.ArtifactURI = artifactURI
	job.CompletedAt = &now
	m.exports[id] = job
	return nil
}

func (m *MemoryRepository) SetExportFailed(_ context.Context, id, message string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	job, ok := m.exports[id]
	if !ok {
		return ErrNotFound
	}
	now := time.Now().UTC()
	job.Status = "failed"
	job.Error = message
	job.CompletedAt = &now
	m.exports[id] = job
	return nil
}

func (m *MemoryRepository) GetExport(_ context.Context, id string) (model.ExportJob, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	job, ok := m.exports[id]
	if !ok {
		return model.ExportJob{}, ErrNotFound
	}
	return job, nil
}

func (m *MemoryRepository) DeleteEvent(_ context.Context, _ string) error {
	return ErrAppendOnly
}

func validateStoredEvent(e ce.StoredEvent) error {
	if e.ID == "" || e.Source == "" || e.Type == "" {
		return ErrInvalidInput
	}
	if len(e.Data) == 0 || string(e.Data) == "null" {
		return ErrInvalidInput
	}
	if e.Time.IsZero() {
		return ErrInvalidInput
	}
	return nil
}

func sortStoredEvents(items []ce.StoredEvent) {
	sort.Slice(items, func(i, j int) bool {
		if items[i].Time.Equal(items[j].Time) {
			return items[i].ID < items[j].ID
		}
		return items[i].Time.Before(items[j].Time)
	})
}

type cursor struct {
	Timestamp string `json:"ts"`
	ID        string `json:"id"`
}

func encodeCursor(ts time.Time, id string) string {
	payload, _ := json.Marshal(cursor{Timestamp: ts.Format(time.RFC3339Nano), ID: id})
	return base64.StdEncoding.EncodeToString(payload)
}

func decodeCursor(in string) (time.Time, string, error) {
	if strings.TrimSpace(in) == "" {
		return time.Time{}, "", nil
	}
	raw, err := base64.StdEncoding.DecodeString(in)
	if err != nil {
		return time.Time{}, "", ErrInvalidCursor
	}
	var c cursor
	if err := json.Unmarshal(raw, &c); err != nil {
		return time.Time{}, "", ErrInvalidCursor
	}
	ts, err := time.Parse(time.RFC3339Nano, c.Timestamp)
	if err != nil {
		return time.Time{}, "", ErrInvalidCursor
	}
	return ts.UTC(), c.ID, nil
}
