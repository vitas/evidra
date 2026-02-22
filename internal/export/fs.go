package export

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	ce "evidra/internal/cloudevents"
)

type FilesystemExporter struct {
	baseDir string
}

func NewFilesystemExporter(baseDir string) *FilesystemExporter {
	return &FilesystemExporter{baseDir: baseDir}
}

func (f *FilesystemExporter) CreateEvidencePack(_ context.Context, jobID string, events []ce.StoredEvent) (string, error) {
	if err := os.MkdirAll(f.baseDir, 0o755); err != nil {
		return "", err
	}
	path := filepath.Join(f.baseDir, fmt.Sprintf("%s.json", jobID))
	events = sortedEvents(events)
	itemsJSON, err := json.Marshal(events)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(itemsJSON)
	change := derivePrimaryChange(events)
	canonical := map[string]interface{}{
		"change_id":               change.ChangeID,
		"source":                  "argocd",
		"application":             change.Application,
		"cluster":                 change.Cluster,
		"namespace":               change.Namespace,
		"revision":                change.Revision,
		"initiator":               change.Initiator,
		"result":                  change.Result,
		"external_change_id":      change.ExternalChangeID,
		"ticket_id":               change.TicketID,
		"post_deploy_degradation": change.PostDeployDegradation,
		"timeline":                events,
	}
	canonicalJSON, err := json.Marshal(canonical)
	if err != nil {
		return "", err
	}
	canonicalSum := sha256.Sum256(canonicalJSON)
	payload := struct {
		SchemaVersion  string           `json:"schema_version"`
		ChangeID       string           `json:"change_id,omitempty"`
		GeneratedAt    time.Time        `json:"generated_at"`
		Source         string           `json:"source"`
		Application    string           `json:"application,omitempty"`
		Cluster        string           `json:"cluster,omitempty"`
		Namespace      string           `json:"namespace,omitempty"`
		Revision       string           `json:"revision,omitempty"`
		Initiator      string           `json:"initiator,omitempty"`
		Result         string           `json:"result,omitempty"`
		ExternalChange string           `json:"external_change_id,omitempty"`
		TicketID       string           `json:"ticket_id,omitempty"`
		Degradation    map[string]any   `json:"post_deploy_degradation,omitempty"`
		Timeline       []ce.StoredEvent `json:"timeline"`
		CreatedAt      time.Time        `json:"created_at"`
		Count          int              `json:"count"`
		ChecksumSHA256 string           `json:"checksum_sha256"`
		Deterministic  string           `json:"deterministic_hash_sha256,omitempty"`
		Items          []ce.StoredEvent `json:"items"`
	}{
		SchemaVersion:  "evidence-pack/v1",
		ChangeID:       change.ChangeID,
		GeneratedAt:    time.Now().UTC(),
		Source:         "argocd",
		Application:    change.Application,
		Cluster:        change.Cluster,
		Namespace:      change.Namespace,
		Revision:       change.Revision,
		Initiator:      change.Initiator,
		Result:         change.Result,
		ExternalChange: change.ExternalChangeID,
		TicketID:       change.TicketID,
		Degradation:    change.PostDeployDegradation,
		Timeline:       events,
		CreatedAt:      time.Now().UTC(),
		Count:          len(events),
		ChecksumSHA256: hex.EncodeToString(sum[:]),
		Deterministic:  hex.EncodeToString(canonicalSum[:]),
		Items:          events,
	}
	b, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return "", err
	}
	if err := os.WriteFile(path, b, 0o644); err != nil {
		return "", err
	}
	return path, nil
}

type exportChangeView struct {
	ChangeID              string
	Application           string
	Cluster               string
	Namespace             string
	Revision              string
	Initiator             string
	Result                string
	ExternalChangeID      string
	TicketID              string
	PostDeployDegradation map[string]any
}

func derivePrimaryChange(events []ce.StoredEvent) exportChangeView {
	out := exportChangeView{}
	if len(events) == 0 {
		return out
	}
	first := events[0]
	out.Application = first.Subject
	out.Cluster = ce.ExtensionString(first.Extensions, "cluster")
	out.Namespace = ce.ExtensionString(first.Extensions, "namespace")
	result := "unknown"
	postDeploy := map[string]any{"observed": false}
	for _, e := range events {
		if out.ChangeID == "" {
			out.ChangeID = strings.TrimSpace(ce.ExtensionString(e.Extensions, "change_id"))
		}
		if out.Revision == "" {
			for _, k := range []string{"sync_revision", "revision", "commit_sha"} {
				if v := strings.TrimSpace(ce.ExtensionString(e.Extensions, k)); v != "" {
					out.Revision = v
					break
				}
			}
		}
		if out.Initiator == "" {
			out.Initiator = strings.TrimSpace(ce.ExtensionString(e.Extensions, "initiator"))
		}
		if out.ExternalChangeID == "" {
			for _, k := range []string{"external_change_id", "change_id"} {
				if v := strings.TrimSpace(ce.ExtensionString(e.Extensions, k)); v != "" {
					out.ExternalChangeID = v
					break
				}
			}
		}
		if out.TicketID == "" {
			for _, k := range []string{"ticket_id", "ticket_key_primary"} {
				if v := strings.TrimSpace(ce.ExtensionString(e.Extensions, k)); v != "" {
					out.TicketID = v
					break
				}
			}
		}
		statusVal := dataStringField(e.Data, "status", "phase", "result", "outcome")
		bucket := strings.ToLower(e.Type + " " + statusVal)
		if strings.Contains(bucket, "fail") || strings.Contains(bucket, "error") || strings.Contains(bucket, "degrad") || strings.Contains(bucket, "abort") {
			result = "failed"
		} else if result != "failed" && (strings.Contains(bucket, "success") || strings.Contains(bucket, "succeed") || strings.Contains(bucket, "complete")) {
			result = "succeeded"
		}
	}
	out.Result = result
	if out.ChangeID == "" {
		out.ChangeID = "chg_" + stableHash(out.Application+":"+out.Cluster+":"+out.Namespace+":"+out.Revision)
	}
	out.PostDeployDegradation = postDeploy
	return out
}

func dataStringField(data json.RawMessage, keys ...string) string {
	if len(data) == 0 {
		return ""
	}
	var m map[string]interface{}
	if err := json.Unmarshal(data, &m); err != nil {
		return ""
	}
	for _, k := range keys {
		if v, ok := m[k]; ok {
			if s, ok := v.(string); ok && strings.TrimSpace(s) != "" {
				return strings.TrimSpace(s)
			}
		}
	}
	return ""
}

func sortedEvents(events []ce.StoredEvent) []ce.StoredEvent {
	out := make([]ce.StoredEvent, len(events))
	copy(out, events)
	sort.Slice(out, func(i, j int) bool {
		if out[i].Time.Equal(out[j].Time) {
			return out[i].ID < out[j].ID
		}
		return out[i].Time.Before(out[j].Time)
	})
	return out
}

func stableHash(input string) string {
	sum := sha256.Sum256([]byte(input))
	return hex.EncodeToString(sum[:])
}

func (f *FilesystemExporter) ReadArtifact(path string) ([]byte, error) {
	return os.ReadFile(path)
}
