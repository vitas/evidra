package export

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	ce "evidra/internal/cloudevents"
)

func TestCreateEvidencePackIncludesContractFields(t *testing.T) {
	e := ce.StoredEvent{
		ID:      "evt_1",
		Source:  "argocd",
		Type:    "argo.sync.finished",
		Time:    time.Date(2026, 2, 17, 11, 0, 0, 0, time.UTC),
		Subject: "payments-api",
		Extensions: map[string]interface{}{
			"cluster":            "eu-1",
			"namespace":          "prod-eu",
			"initiator":          "dev",
			"operation_id":       "op-1",
			"sync_revision":      "abc123",
			"external_change_id": "CHG123",
			"ticket_id":          "JIRA-1",
		},
		Data: json.RawMessage(`{"argocd_app":"payments-api"}`),
	}

	x := NewFilesystemExporter(t.TempDir())
	path, err := x.CreateEvidencePack(context.Background(), "exp_1", []ce.StoredEvent{e})
	if err != nil {
		t.Fatalf("create evidence pack: %v", err)
	}
	b, err := x.ReadArtifact(path)
	if err != nil {
		t.Fatalf("read artifact: %v", err)
	}

	var out struct {
		SchemaVersion           string           `json:"schema_version"`
		ChangeID                string           `json:"change_id"`
		GeneratedAt             time.Time        `json:"generated_at"`
		Source                  string           `json:"source"`
		Application             string           `json:"application"`
		Cluster                 string           `json:"cluster"`
		Namespace               string           `json:"namespace"`
		Revision                string           `json:"revision"`
		Initiator               string           `json:"initiator"`
		Result                  string           `json:"result"`
		ExternalChangeID        string           `json:"external_change_id"`
		TicketID                string           `json:"ticket_id"`
		PostDeployDegradation   map[string]any   `json:"post_deploy_degradation"`
		Timeline                []ce.StoredEvent `json:"timeline"`
		Count                   int              `json:"count"`
		ChecksumSHA256          string           `json:"checksum_sha256"`
		DeterministicHashSHA256 string           `json:"deterministic_hash_sha256"`
	}
	if err := json.Unmarshal(b, &out); err != nil {
		t.Fatalf("decode artifact: %v", err)
	}
	if out.SchemaVersion != "evidence-pack/v1" {
		t.Fatalf("unexpected schema version: %q", out.SchemaVersion)
	}
	if out.Count != 1 {
		t.Fatalf("unexpected count: %d", out.Count)
	}
	if out.ChangeID == "" || out.Source != "argocd" {
		t.Fatalf("expected export metadata, got %+v", out)
	}
	if out.Application != "payments-api" || out.Cluster != "eu-1" || out.Namespace != "prod-eu" {
		t.Fatalf("unexpected target metadata: %+v", out)
	}
	if out.Revision != "abc123" || out.Initiator != "dev" {
		t.Fatalf("unexpected revision/initiator: %+v", out)
	}
	if out.ExternalChangeID != "CHG123" || out.TicketID != "JIRA-1" {
		t.Fatalf("unexpected external metadata: %+v", out)
	}
	if out.GeneratedAt.IsZero() || len(out.Timeline) != 1 {
		t.Fatalf("expected generated_at and timeline")
	}
	if len(out.ChecksumSHA256) != 64 {
		t.Fatalf("unexpected checksum length: %d", len(out.ChecksumSHA256))
	}
	if len(out.DeterministicHashSHA256) != 64 {
		t.Fatalf("unexpected deterministic hash length: %d", len(out.DeterministicHashSHA256))
	}
}

func TestCreateEvidencePackDeterministicHashStable(t *testing.T) {
	e := ce.StoredEvent{
		ID:      "evt_1",
		Source:  "argocd",
		Type:    "argo.sync.finished",
		Time:    time.Date(2026, 2, 17, 11, 0, 0, 0, time.UTC),
		Subject: "payments-api",
		Extensions: map[string]interface{}{
			"cluster":       "eu-1",
			"namespace":     "prod-eu",
			"initiator":     "dev",
			"operation_id":  "op-1",
			"sync_revision": "abc123",
		},
		Data: json.RawMessage(`{"argocd_app":"payments-api"}`),
	}
	x := NewFilesystemExporter(t.TempDir())
	pathA, err := x.CreateEvidencePack(context.Background(), "exp_1", []ce.StoredEvent{e})
	if err != nil {
		t.Fatalf("create pack A: %v", err)
	}
	pathB, err := x.CreateEvidencePack(context.Background(), "exp_2", []ce.StoredEvent{e})
	if err != nil {
		t.Fatalf("create pack B: %v", err)
	}
	readHash := func(path string) string {
		b, err := x.ReadArtifact(path)
		if err != nil {
			t.Fatalf("read artifact: %v", err)
		}
		var out struct {
			DeterministicHashSHA256 string `json:"deterministic_hash_sha256"`
		}
		if err := json.Unmarshal(b, &out); err != nil {
			t.Fatalf("decode artifact: %v", err)
		}
		return out.DeterministicHashSHA256
	}
	a := readHash(pathA)
	b := readHash(pathB)
	if a == "" || b == "" || a != b {
		t.Fatalf("expected stable deterministic hash, got %q vs %q", a, b)
	}
}
