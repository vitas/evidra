// Package store provides database-backed storage for API keys and evidence entries.
package store

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/oklog/ulid/v2"
)

// KeyRecord holds API key metadata (never the plaintext).
type KeyRecord struct {
	ID         string
	TenantID   string
	Prefix     string
	Label      string
	CreatedAt  time.Time
	LastUsedAt *time.Time
}

// KeyStore manages API key lifecycle backed by PostgreSQL.
type KeyStore struct {
	pool *pgxpool.Pool
	begin func(context.Context) (keyTx, error)
}

// NewKeyStore creates a KeyStore with the given connection pool.
func NewKeyStore(pool *pgxpool.Pool) *KeyStore {
	return &KeyStore{pool: pool}
}

type keyTx interface {
	Exec(ctx context.Context, sql string, args ...interface{}) (pgconn.CommandTag, error)
	Commit(ctx context.Context) error
	Rollback(ctx context.Context) error
}

type pgxTx struct {
	tx interface {
		Exec(ctx context.Context, sql string, args ...interface{}) (pgconn.CommandTag, error)
		Commit(ctx context.Context) error
		Rollback(ctx context.Context) error
	}
}

func (p pgxTx) Exec(ctx context.Context, sql string, args ...interface{}) (pgconn.CommandTag, error) {
	return p.tx.Exec(ctx, sql, args...)
}

func (p pgxTx) Commit(ctx context.Context) error {
	return p.tx.Commit(ctx)
}

func (p pgxTx) Rollback(ctx context.Context) error {
	return p.tx.Rollback(ctx)
}

// CreateKey generates a new API key for the given tenant.
// Returns the plaintext key (shown once) and the record metadata.
func (ks *KeyStore) CreateKey(ctx context.Context, tenantID, label string) (string, KeyRecord, error) {
	plaintext, err := generateKeyPlaintext()
	if err != nil {
		return "", KeyRecord{}, fmt.Errorf("store.CreateKey: generate: %w", err)
	}

	id := ulid.Make().String()
	hash := hashKey(plaintext)
	prefix := plaintext[:8]
	now := time.Now().UTC()

	tx, err := ks.beginTx(ctx)
	if err != nil {
		return "", KeyRecord{}, fmt.Errorf("store.CreateKey: begin: %w", err)
	}
	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback(ctx)
		}
	}()

	if _, err = tx.Exec(ctx,
		`INSERT INTO tenants (id) VALUES ($1)
		 ON CONFLICT (id) DO NOTHING`,
		tenantID,
	); err != nil {
		return "", KeyRecord{}, fmt.Errorf("store.CreateKey: ensure tenant: %w", err)
	}

	_, err = tx.Exec(ctx,
		`INSERT INTO api_keys (id, tenant_id, key_hash, prefix, label, created_at)
		 VALUES ($1, $2, $3, $4, $5, $6)`,
		id, tenantID, hash, prefix, label, now,
	)
	if err != nil {
		return "", KeyRecord{}, fmt.Errorf("store.CreateKey: insert: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return "", KeyRecord{}, fmt.Errorf("store.CreateKey: commit: %w", err)
	}
	committed = true

	return plaintext, KeyRecord{
		ID:        id,
		TenantID:  tenantID,
		Prefix:    prefix,
		Label:     label,
		CreatedAt: now,
	}, nil
}

func (ks *KeyStore) beginTx(ctx context.Context) (keyTx, error) {
	if ks.begin != nil {
		return ks.begin(ctx)
	}
	tx, err := ks.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	return pgxTx{tx: tx}, nil
}

// LookupKey finds an active (non-revoked) key by plaintext.
// Returns the key record or a wrapped query error.
func (ks *KeyStore) LookupKey(ctx context.Context, plaintext string) (KeyRecord, error) {
	hash := hashKey(plaintext)
	var rec KeyRecord
	err := ks.pool.QueryRow(ctx,
		`SELECT id, tenant_id, prefix, label, created_at, last_used_at
		 FROM api_keys
		 WHERE key_hash = $1 AND revoked_at IS NULL`,
		hash,
	).Scan(&rec.ID, &rec.TenantID, &rec.Prefix, &rec.Label, &rec.CreatedAt, &rec.LastUsedAt)
	if err != nil {
		return KeyRecord{}, fmt.Errorf("store.LookupKey: %w", err)
	}

	// Async touch — fire and forget.
	go ks.touchKey(rec.ID)

	return rec, nil
}

func (ks *KeyStore) touchKey(keyID string) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	_, _ = ks.pool.Exec(ctx,
		`UPDATE api_keys SET last_used_at = now() WHERE id = $1`, keyID)
}

// RevokeKey marks a key as revoked.
func (ks *KeyStore) RevokeKey(ctx context.Context, keyID string) error {
	_, err := ks.pool.Exec(ctx,
		`UPDATE api_keys SET revoked_at = now() WHERE id = $1 AND revoked_at IS NULL`, keyID)
	if err != nil {
		return fmt.Errorf("store.RevokeKey: %w", err)
	}
	return nil
}

func hashKey(plaintext string) []byte {
	h := sha256.Sum256([]byte(plaintext))
	return h[:]
}

func generateKeyPlaintext() (string, error) {
	var buf [24]byte
	if _, err := rand.Read(buf[:]); err != nil {
		return "", err
	}
	return "ev1_" + base64.RawURLEncoding.EncodeToString(buf[:]), nil
}
