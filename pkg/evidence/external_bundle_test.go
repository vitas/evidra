package evidence_test

import (
	"bytes"
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"samebits.com/evidra/pkg/evidence"
)

// fixtureDir is the committed external bundle produced by
// scripts/gen_external_bundle. It is the cross-repo format contract: any
// producer (for example, Evidra Bench) must emit stores that validate here.
const fixtureDir = "../../tests/external_bundle_v1"

func copyFixture(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	err := filepath.WalkDir(fixtureDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(fixtureDir, path)
		if err != nil {
			return err
		}
		target := filepath.Join(root, rel)
		if d.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		return os.WriteFile(target, data, 0o644)
	})
	if err != nil {
		t.Fatalf("copy fixture: %v", err)
	}
	return root
}

func TestExternalBundleFixtureValidates(t *testing.T) {
	root := copyFixture(t)

	bundle, err := evidence.ValidateBundle(root)
	if err != nil {
		t.Fatalf("ValidateBundle: %v", err)
	}
	if bundle.Spec != evidence.BundleSpecV1 {
		t.Fatalf("unexpected bundle spec %q", bundle.Spec)
	}
	if bundle.TrustLevel != evidence.TrustEphemeral {
		t.Fatalf("unexpected trust level %q", bundle.TrustLevel)
	}

	entries, err := evidence.ReadAllEntriesAtPath(root)
	if err != nil {
		t.Fatalf("ReadAllEntriesAtPath: %v", err)
	}
	if len(entries) != 4 {
		t.Fatalf("expected 4 entries, got %d", len(entries))
	}
	wantTypes := []evidence.EntryType{
		evidence.EntryTypeSessionStart,
		evidence.EntryTypePrescribe,
		evidence.EntryTypeReport,
		evidence.EntryTypeSessionEnd,
	}
	for i, want := range wantTypes {
		if entries[i].Type != want {
			t.Fatalf("entry %d: type %q, want %q", i, entries[i].Type, want)
		}
	}
	if entries[0].PreviousHash != "" {
		t.Error("first entry must have empty previous_hash")
	}
	for i := range entries {
		if i > 0 && entries[i].PreviousHash != entries[i-1].Hash {
			t.Errorf("entry %d: chain link broken", i)
		}
		if entries[i].Signature == "" {
			t.Errorf("entry %d: missing signature", i)
		}
	}

	// The report must link back to the prescribe's prescription_id.
	var p evidence.PrescriptionPayload
	if err := json.Unmarshal(entries[1].Payload, &p); err != nil {
		t.Fatalf("parse prescription payload: %v", err)
	}
	var rp evidence.ReportPayload
	if err := json.Unmarshal(entries[2].Payload, &rp); err != nil {
		t.Fatalf("parse report payload: %v", err)
	}
	if p.PrescriptionID == "" || p.PrescriptionID != rp.PrescriptionID {
		t.Fatalf("report does not reference its prescription: %q vs %q", p.PrescriptionID, rp.PrescriptionID)
	}
	if rp.Verdict != evidence.VerdictSuccess {
		t.Fatalf("fixture report verdict = %q, want success", rp.Verdict)
	}
}

func TestExternalBundleFixtureTamperDetected(t *testing.T) {
	root := copyFixture(t)

	segPath := filepath.Join(root, "segments", "evidence-000001.jsonl")
	raw, err := os.ReadFile(segPath)
	if err != nil {
		t.Fatalf("read segment: %v", err)
	}
	tampered := bytes.Replace(raw, []byte(`"verdict":"success"`), []byte(`"verdict":"failure"`), 1)
	if bytes.Equal(raw, tampered) {
		t.Fatal("fixture no longer contains the payload field the tamper relies on; update this test with the fixture")
	}
	if err := os.WriteFile(segPath, tampered, 0o644); err != nil {
		t.Fatalf("write tampered segment: %v", err)
	}

	err = evidence.ValidateChainAtPath(root)
	if err == nil || !strings.Contains(err.Error(), "hash mismatch") {
		t.Fatalf("expected hash mismatch on tampered payload, got %v", err)
	}
	if _, err := evidence.ValidateBundle(root); err == nil {
		t.Fatal("ValidateBundle accepted a tampered fixture")
	}
}
