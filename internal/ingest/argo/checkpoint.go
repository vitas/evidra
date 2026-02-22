package argo

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"time"
)

type Checkpoint struct {
	Apps map[string]AppCheckpoint `json:"apps"`
}

type AppCheckpoint struct {
	LastHistoryID   int64     `json:"last_history_id"`
	LastHistoryAt   time.Time `json:"last_history_at"`
	LastStartKey    string    `json:"last_start_key"`
	LastTerminalKey string    `json:"last_terminal_key"`
	LastHealth      string    `json:"last_health"`
}

type CheckpointStore interface {
	Load() (Checkpoint, error)
	Save(Checkpoint) error
}

type FileCheckpointStore struct {
	Path string
}

func (f FileCheckpointStore) Load() (Checkpoint, error) {
	b, err := os.ReadFile(f.Path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return Checkpoint{}, nil
		}
		return Checkpoint{}, err
	}
	var cp Checkpoint
	if err := json.Unmarshal(b, &cp); err != nil {
		return Checkpoint{}, err
	}
	for app, c := range cp.Apps {
		c.LastHistoryAt = c.LastHistoryAt.UTC()
		cp.Apps[app] = c
	}
	return cp, nil
}

func (f FileCheckpointStore) Save(cp Checkpoint) error {
	if err := os.MkdirAll(filepath.Dir(f.Path), 0o755); err != nil {
		return err
	}
	if cp.Apps == nil {
		cp.Apps = map[string]AppCheckpoint{}
	}
	for app, c := range cp.Apps {
		c.LastHistoryAt = c.LastHistoryAt.UTC()
		cp.Apps[app] = c
	}
	b, err := json.MarshalIndent(cp, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(f.Path, b, 0o644)
}
