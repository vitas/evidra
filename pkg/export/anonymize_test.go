package export

import (
	"encoding/json"
	"testing"
	"time"

	"samebits.com/evidra/pkg/evidence"
)

func TestAnonymizer_Hash_Deterministic(t *testing.T) {
	a := NewAnonymizer()
	h1 := a.Hash("actor", "claude-code")
	h2 := a.Hash("actor", "claude-code")
	if h1 != h2 {
		t.Fatalf("same input produced different hashes: %s vs %s", h1, h2)
	}
}

func TestAnonymizer_Hash_DifferentInputs(t *testing.T) {
	a := NewAnonymizer()
	h1 := a.Hash("actor", "claude-code")
	h2 := a.Hash("actor", "github-actions")
	if h1 == h2 {
		t.Fatal("different inputs produced same hash")
	}
}

func TestAnonymizer_Hash_DifferentSalts(t *testing.T) {
	a1 := NewAnonymizer()
	a2 := NewAnonymizer()
	h1 := a1.Hash("actor", "claude-code")
	h2 := a2.Hash("actor", "claude-code")
	if h1 == h2 {
		t.Fatal("different salts produced same hash (astronomically unlikely)")
	}
}

func TestAnonymizer_Hash_EmptyString(t *testing.T) {
	a := NewAnonymizer()
	if a.Hash("actor", "") != "" {
		t.Fatal("empty input should produce empty output")
	}
}

func TestAnonymizeEntry_PreservesStructure(t *testing.T) {
	a := NewAnonymizer()
	entry := evidence.EvidenceEntry{
		EntryID:   "eid-123",
		Type:      evidence.EntryTypePrescribe,
		SessionID: "session-abc",
		Actor: evidence.Actor{
			Type:         "agent",
			ID:           "claude-code",
			Provenance:   "infra-bench",
			InstanceID:   "runner-pod-1",
			Version:      "1.0",
			SkillVersion: "v1.0.1",
		},
		Timestamp:   time.Now(),
		SpecVersion: "v1.1.0",
		Payload:     json.RawMessage(`{"prescription_id":"rx-1","canonical_action":{"tool":"kubectl","operation":"apply","operation_class":"mutate","resource_count":1,"resource_identity":[{"kind":"Deployment","namespace":"payments-prod","name":"api-gateway"}]}}`),
	}

	anon := a.AnonymizeEntry(entry)

	// Preserved
	if anon.Type != evidence.EntryTypePrescribe {
		t.Fatalf("type changed: %s", anon.Type)
	}
	if anon.Actor.Type != "agent" {
		t.Fatalf("actor.type changed: %s", anon.Actor.Type)
	}
	if anon.Actor.SkillVersion != "v1.0.1" {
		t.Fatalf("skill_version changed: %s", anon.Actor.SkillVersion)
	}
	if anon.SpecVersion != "v1.1.0" {
		t.Fatalf("spec_version changed: %s", anon.SpecVersion)
	}

	// Anonymized
	if anon.Actor.ID == "claude-code" {
		t.Fatal("actor.id not anonymized")
	}
	if anon.Actor.Provenance == "infra-bench" {
		t.Fatal("actor.provenance not anonymized")
	}
	if anon.SessionID == "session-abc" {
		t.Fatal("session_id not anonymized")
	}

	// Stripped
	if anon.Signature != "" {
		t.Fatal("signature not stripped")
	}
	if anon.Hash != "" {
		t.Fatal("hash not stripped")
	}

	// Payload: check resource identity is anonymized
	var payload struct {
		CanonicalAction struct {
			Tool             string `json:"tool"`
			OperationClass   string `json:"operation_class"`
			ResourceIdentity []struct {
				Kind      string `json:"kind"`
				Namespace string `json:"namespace"`
				Name      string `json:"name"`
			} `json:"resource_identity"`
		} `json:"canonical_action"`
	}
	if err := json.Unmarshal(anon.Payload, &payload); err != nil {
		t.Fatalf("parse anonymized payload: %v", err)
	}
	if payload.CanonicalAction.Tool != "kubectl" {
		t.Fatalf("tool changed: %s", payload.CanonicalAction.Tool)
	}
	if payload.CanonicalAction.OperationClass != "mutate" {
		t.Fatalf("operation_class changed: %s", payload.CanonicalAction.OperationClass)
	}
	if len(payload.CanonicalAction.ResourceIdentity) != 1 {
		t.Fatalf("resource count changed: %d", len(payload.CanonicalAction.ResourceIdentity))
	}
	ri := payload.CanonicalAction.ResourceIdentity[0]
	if ri.Kind != "Deployment" {
		t.Fatalf("kind changed: %s", ri.Kind)
	}
	if ri.Namespace == "payments-prod" {
		t.Fatal("namespace not anonymized")
	}
	if ri.Name == "api-gateway" {
		t.Fatal("name not anonymized")
	}
}

func TestAnonymizeEntry_ScopeDimensions(t *testing.T) {
	a := NewAnonymizer()
	entry := evidence.EvidenceEntry{
		Type:    evidence.EntryTypePrescribe,
		Payload: json.RawMessage(`{}`),
		ScopeDimensions: map[string]string{
			"cluster": "prod-us-east-1",
			"region":  "us-east-1",
		},
	}
	anon := a.AnonymizeEntry(entry)
	if anon.ScopeDimensions["cluster"] == "prod-us-east-1" {
		t.Fatal("scope dimension value not anonymized")
	}
	if _, ok := anon.ScopeDimensions["cluster"]; !ok {
		t.Fatal("scope dimension key was removed")
	}
}
