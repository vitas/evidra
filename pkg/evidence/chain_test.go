package evidence

import (
	"bytes"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

func TestChainIntegrity_AppendAndValidate(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()

	var lastHash string
	for i := 0; i < 3; i++ {
		payload, _ := json.Marshal(map[string]string{"index": fmt.Sprintf("%d", i)})
		entry, err := BuildEntry(EntryBuildParams{
			Type:           EntryTypePrescribe,
			TraceID:        "01TRACE",
			Actor:          Actor{Type: "ci", ID: "test", Provenance: "cli"},
			Payload:        payload,
			PreviousHash:   lastHash,
			SpecVersion:    "0.3.0",
			CanonVersion:   "test/v1",
			AdapterVersion: "0.3.0",
		})
		if err != nil {
			t.Fatalf("BuildEntry %d: %v", i, err)
		}

		if err := AppendEntryAtPath(dir, entry); err != nil {
			t.Fatalf("AppendEntryAtPath %d: %v", i, err)
		}
		lastHash = entry.Hash
	}

	entries, err := ReadAllEntriesAtPath(dir)
	if err != nil {
		t.Fatalf("ReadAllEntriesAtPath: %v", err)
	}
	if len(entries) != 3 {
		t.Fatalf("expected 3 entries, got %d", len(entries))
	}

	if err := ValidateChainAtPath(dir); err != nil {
		t.Fatalf("ValidateChainAtPath: %v", err)
	}
}

// testSigner implements the Signer interface for tests.
type testSigner struct {
	priv ed25519.PrivateKey
	pub  ed25519.PublicKey
}

func newTestSigner(t *testing.T) *testSigner {
	t.Helper()
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	return &testSigner{priv: priv, pub: pub}
}

func (s *testSigner) Sign(payload []byte) []byte      { return ed25519.Sign(s.priv, payload) }
func (s *testSigner) Verify(payload, sig []byte) bool { return ed25519.Verify(s.pub, payload, sig) }
func (s *testSigner) PublicKey() ed25519.PublicKey    { return s.pub }

func TestBuildEntry_WithSigner(t *testing.T) {
	t.Parallel()
	signer := newTestSigner(t)

	entry, err := BuildEntry(EntryBuildParams{
		Type:           EntryTypePrescribe,
		TraceID:        "TRACE-SIGN",
		Actor:          Actor{Type: "ci", ID: "test", Provenance: "cli"},
		Payload:        json.RawMessage(`{"test":"signing"}`),
		SpecVersion:    "0.3.0",
		CanonVersion:   "test/v1",
		AdapterVersion: "0.3.0",
		Signer:         signer,
	})
	if err != nil {
		t.Fatalf("BuildEntry: %v", err)
	}

	if entry.Signature == "" {
		t.Fatal("expected non-empty signature when signer is provided")
	}

	// Verify signature
	sig, err := base64.StdEncoding.DecodeString(entry.Signature)
	if err != nil {
		t.Fatalf("decode signature: %v", err)
	}
	if !ed25519.Verify(signer.pub, []byte(entry.Hash), sig) {
		t.Fatal("signature verification failed")
	}
}

func TestValidateChainWithSignatures_Valid(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	signer := newTestSigner(t)

	var lastHash string
	for i := 0; i < 3; i++ {
		payload, _ := json.Marshal(map[string]string{"i": fmt.Sprintf("%d", i)})
		entry, err := BuildEntry(EntryBuildParams{
			Type:           EntryTypePrescribe,
			TraceID:        "01TRACE",
			Actor:          Actor{Type: "ci", ID: "test", Provenance: "cli"},
			Payload:        payload,
			PreviousHash:   lastHash,
			SpecVersion:    "0.3.0",
			CanonVersion:   "test/v1",
			AdapterVersion: "0.3.0",
			Signer:         signer,
		})
		if err != nil {
			t.Fatalf("BuildEntry %d: %v", i, err)
		}
		if err := AppendEntryAtPath(dir, entry); err != nil {
			t.Fatalf("AppendEntryAtPath %d: %v", i, err)
		}
		lastHash = entry.Hash
	}

	if err := ValidateChainWithSignatures(dir, signer.pub); err != nil {
		t.Fatalf("ValidateChainWithSignatures: %v", err)
	}
}

func TestValidateChainWithSignatures_WrongKey(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	signer := newTestSigner(t)
	otherSigner := newTestSigner(t)

	payload, _ := json.Marshal(map[string]string{"data": "test"})
	entry, err := BuildEntry(EntryBuildParams{
		Type:           EntryTypePrescribe,
		TraceID:        "01T",
		Actor:          Actor{Type: "ci", ID: "t", Provenance: "cli"},
		Payload:        payload,
		SpecVersion:    "0.3.0",
		CanonVersion:   "test/v1",
		AdapterVersion: "0.3.0",
		Signer:         signer,
	})
	if err != nil {
		t.Fatalf("BuildEntry: %v", err)
	}
	if err := AppendEntryAtPath(dir, entry); err != nil {
		t.Fatalf("AppendEntryAtPath: %v", err)
	}

	err = ValidateChainWithSignatures(dir, otherSigner.pub)
	if err == nil {
		t.Fatal("expected error when verifying with wrong public key")
	}
}

func TestChainIntegrity_TamperDetection(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()

	payload, _ := json.Marshal(map[string]string{"data": "first"})
	entry1, _ := BuildEntry(EntryBuildParams{
		Type: EntryTypePrescribe, TraceID: "01T",
		Actor:   Actor{Type: "ci", ID: "t", Provenance: "cli"},
		Payload: payload, SpecVersion: "0.3.0",
		CanonVersion: "test/v1", AdapterVersion: "0.3.0",
	})
	if err := AppendEntryAtPath(dir, entry1); err != nil {
		t.Fatalf("AppendEntryAtPath entry1: %v", err)
	}

	entry2, _ := BuildEntry(EntryBuildParams{
		Type: EntryTypeReport, TraceID: "01T",
		Actor:   Actor{Type: "ci", ID: "t", Provenance: "cli"},
		Payload: payload, PreviousHash: entry1.Hash,
		SpecVersion: "0.3.0", CanonVersion: "test/v1", AdapterVersion: "0.3.0",
	})
	if err := AppendEntryAtPath(dir, entry2); err != nil {
		t.Fatalf("AppendEntryAtPath entry2: %v", err)
	}

	// Tamper: modify first entry's payload in the file.
	files, _ := filepath.Glob(filepath.Join(dir, "segments", "*.jsonl"))
	if len(files) == 0 {
		t.Fatal("no JSONL files found")
	}

	data, _ := os.ReadFile(files[0])
	tampered := bytes.Replace(data, []byte(`"first"`), []byte(`"TAMPERED"`), 1)
	if err := os.WriteFile(files[0], tampered, 0o644); err != nil {
		t.Fatalf("write tampered file: %v", err)
	}

	err := ValidateChainAtPath(dir)
	if err == nil {
		t.Fatal("expected chain validation error after tampering")
	}
}
