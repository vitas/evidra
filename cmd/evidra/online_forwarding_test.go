package main

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"testing"

	testutil "samebits.com/evidra/internal/testutil"
	"samebits.com/evidra/pkg/evidence"
)

func TestRunPrescribe_ForwardsEvidenceOnline(t *testing.T) {
	t.Parallel()

	signingKey := testutil.TestSigningKeyBase64(t)
	tmp := t.TempDir()
	evidenceDir := filepath.Join(tmp, "evidence")
	artifactPath := filepath.Join(tmp, "artifact.json")
	if err := os.WriteFile(artifactPath, []byte(`{"noop":true}`), 0o644); err != nil {
		t.Fatalf("write artifact: %v", err)
	}

	var (
		mu           sync.Mutex
		requestPaths []string
		rawBodies    [][]byte
	)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer r.Body.Close()

		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("read request body: %v", err)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer test-key" {
			t.Fatalf("authorization header = %q, want Bearer test-key", got)
		}

		mu.Lock()
		requestPaths = append(requestPaths, r.URL.Path)
		rawBodies = append(rawBodies, body)
		mu.Unlock()

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"receipt_id":"r1","status":"accepted"}`))
	}))
	defer server.Close()

	var out, errBuf bytes.Buffer
	code := run([]string{
		"prescribe",
		"--tool", "terraform",
		"--artifact", artifactPath,
		"--canonical-action", testCanonicalAction,
		"--session-id", "session-online-prescribe",
		"--evidence-dir", evidenceDir,
		"--signing-key", signingKey,
		"--url", server.URL,
		"--api-key", "test-key",
	}, &out, &errBuf)
	if code != 0 {
		t.Fatalf("prescribe exit %d: %s", code, errBuf.String())
	}

	mu.Lock()
	defer mu.Unlock()
	if len(requestPaths) != 1 {
		t.Fatalf("forward request count = %d, want 1", len(requestPaths))
	}
	if requestPaths[0] != "/v1/evidence/forward" {
		t.Fatalf("forward path = %q, want /v1/evidence/forward", requestPaths[0])
	}

	var entry evidence.EvidenceEntry
	if err := json.Unmarshal(rawBodies[0], &entry); err != nil {
		t.Fatalf("decode forwarded entry: %v", err)
	}
	if entry.Type != evidence.EntryTypePrescribe {
		t.Fatalf("entry type = %q, want prescribe", entry.Type)
	}
	if entry.SessionID != "session-online-prescribe" {
		t.Fatalf("session_id = %q, want session-online-prescribe", entry.SessionID)
	}
}

func TestRunReport_ForwardsEvidenceOnline(t *testing.T) {
	t.Parallel()

	signingKey := testutil.TestSigningKeyBase64(t)
	tmp := t.TempDir()
	evidenceDir := filepath.Join(tmp, "evidence")
	artifactPath := filepath.Join(tmp, "artifact.json")
	if err := os.WriteFile(artifactPath, []byte(`{"noop":true}`), 0o644); err != nil {
		t.Fatalf("write artifact: %v", err)
	}

	prescriptionID := writeSuccessfulPrescription(t, signingKey, evidenceDir, artifactPath, "session-online-report")

	var (
		mu           sync.Mutex
		requestPaths []string
		rawBodies    [][]byte
	)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer r.Body.Close()

		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("read request body: %v", err)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer test-key" {
			t.Fatalf("authorization header = %q, want Bearer test-key", got)
		}

		mu.Lock()
		requestPaths = append(requestPaths, r.URL.Path)
		rawBodies = append(rawBodies, body)
		mu.Unlock()

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"accepted":2}`))
	}))
	defer server.Close()

	var out, errBuf bytes.Buffer
	code := run([]string{
		"report",
		"--prescription", prescriptionID,
		"--verdict", "success",
		"--exit-code", "0",
		"--session-id", "session-online-report",
		"--evidence-dir", evidenceDir,
		"--signing-key", signingKey,
		"--url", server.URL,
		"--api-key", "test-key",
	}, &out, &errBuf)
	if code != 0 {
		t.Fatalf("report exit %d: %s", code, errBuf.String())
	}

	mu.Lock()
	defer mu.Unlock()
	if len(requestPaths) != 1 {
		t.Fatalf("batch request count = %d, want 1", len(requestPaths))
	}
	if requestPaths[0] != "/v1/evidence/batch" {
		t.Fatalf("batch path = %q, want /v1/evidence/batch", requestPaths[0])
	}

	var batch struct {
		Entries []json.RawMessage `json:"entries"`
	}
	if err := json.Unmarshal(rawBodies[0], &batch); err != nil {
		t.Fatalf("decode batch request: %v", err)
	}
	if len(batch.Entries) != 2 {
		t.Fatalf("forwarded entries = %d, want 2", len(batch.Entries))
	}

	prescribeCount := 0
	reportCount := 0
	for _, raw := range batch.Entries {
		var entry evidence.EvidenceEntry
		if err := json.Unmarshal(raw, &entry); err != nil {
			t.Fatalf("decode forwarded entry: %v", err)
		}
		if entry.SessionID != "session-online-report" {
			t.Fatalf("session_id = %q, want session-online-report", entry.SessionID)
		}
		switch entry.Type {
		case evidence.EntryTypePrescribe:
			prescribeCount++
		case evidence.EntryTypeReport:
			reportCount++
		}
	}
	if prescribeCount != 1 || reportCount != 1 {
		t.Fatalf("forwarded prescribe/report counts = %d/%d, want 1/1", prescribeCount, reportCount)
	}
}
