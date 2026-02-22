package cloudevents

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"sort"
	"time"
)

// StoredEvent is the canonical CloudEvents-native event type used throughout Evidra.
type StoredEvent struct {
	ID            string                 `json:"id"`
	Type          string                 `json:"type"`
	Source        string                 `json:"source"`
	Subject       string                 `json:"subject,omitempty"`
	Time          time.Time              `json:"time"`
	Extensions    map[string]interface{} `json:"extensions,omitempty"`
	Data          json.RawMessage        `json:"data"`
	IntegrityHash string                 `json:"integrity_hash"`
	IngestedAt    time.Time              `json:"ingested_at,omitempty"`
}

// ComputeIntegrityHash returns SHA256 of canonical JSON encoding.
func ComputeIntegrityHash(e StoredEvent) (string, error) {
	canon := struct {
		SpecVersion string                 `json:"specversion"`
		ID          string                 `json:"id"`
		Source      string                 `json:"source"`
		Type        string                 `json:"type"`
		Subject     string                 `json:"subject,omitempty"`
		Time        string                 `json:"time"`
		Extensions  map[string]interface{} `json:"extensions,omitempty"`
		Data        json.RawMessage        `json:"data"`
	}{
		SpecVersion: "1.0",
		ID:          e.ID,
		Source:      e.Source,
		Type:        e.Type,
		Subject:     e.Subject,
		Time:        e.Time.UTC().Format(time.RFC3339Nano),
		Extensions:  sortedExtensions(e.Extensions),
		Data:        e.Data,
	}
	b, err := json.Marshal(canon)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:]), nil
}

func sortedExtensions(ext map[string]interface{}) map[string]interface{} {
	if ext == nil {
		return nil
	}
	keys := make([]string, 0, len(ext))
	for k := range ext {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	out := make(map[string]interface{}, len(ext))
	for _, k := range keys {
		out[k] = ext[k]
	}
	return out
}
