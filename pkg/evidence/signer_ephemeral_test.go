package evidence

import (
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"testing"
)

func TestEphemeralSignerRoundTrip(t *testing.T) {
	s, err := NewEphemeralSigner()
	if err != nil {
		t.Fatalf("NewEphemeralSigner: %v", err)
	}
	payload := []byte("sha256:deadbeef")
	sig := s.Sign(payload)
	if !s.Verify(payload, sig) {
		t.Fatal("Verify rejected own signature")
	}
	if s.Verify([]byte("other"), sig) {
		t.Fatal("Verify accepted tampered payload")
	}
	if len(s.PublicKey()) != ed25519.PublicKeySize {
		t.Fatalf("unexpected public key size %d", len(s.PublicKey()))
	}
	raw, err := base64.StdEncoding.DecodeString(s.PublicKeyBase64())
	if err != nil || len(raw) != ed25519.PublicKeySize {
		t.Fatalf("PublicKeyBase64 not parseable: %v (len %d)", err, len(raw))
	}
}

func TestBuildEntryWithEphemeralSignerValidates(t *testing.T) {
	s, err := NewEphemeralSigner()
	if err != nil {
		t.Fatalf("NewEphemeralSigner: %v", err)
	}
	root := t.TempDir()

	payload := json.RawMessage(`{"report_id":"r1","prescription_id":"p1","exit_code":0,"verdict":"success"}`)
	var prev string
	for i := 0; i < 2; i++ {
		entry, err := BuildEntry(EntryBuildParams{
			Type:         EntryTypeReport,
			SessionID:    "sess-1",
			OperationID:  "op-1",
			TraceID:      "trace-1",
			Actor:        Actor{Type: "agent", ID: "tester", Provenance: "test"},
			Payload:      payload,
			PreviousHash: prev,
			SpecVersion:  "v1.1.0",
			Signer:       s,
		})
		if err != nil {
			t.Fatalf("BuildEntry: %v", err)
		}
		if entry.Signature == "" {
			t.Fatal("entry signature not populated")
		}
		if err := AppendEntryAtPath(root, entry); err != nil {
			t.Fatalf("AppendEntryAtPath: %v", err)
		}
		prev = entry.Hash
	}

	if err := ValidateChainAtPath(root); err != nil {
		t.Fatalf("ValidateChainAtPath: %v", err)
	}
	if err := ValidateChainWithSignatures(root, s.PublicKey()); err != nil {
		t.Fatalf("ValidateChainWithSignatures: %v", err)
	}
}
