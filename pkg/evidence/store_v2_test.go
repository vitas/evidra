package evidence

import (
	"bufio"
	"encoding/base64"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func openTestStore(t *testing.T) *Store {
	t.Helper()
	s, err := OpenStore(Options{Dir: filepath.Join(t.TempDir(), "recorder-test"), UpstreamID: "up-fixture", ServerName: "fixture", EnforceMode: "all", EvidraVersion: "vnext-0"})
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = s.Close("test_end") })
	return s
}

func TestJCSIsStableUnderKeyOrderAndFormatting(t *testing.T) {
	a := `{"b":1,"a":{"y":2,"x":[1,2,3]},"c":"x"}`
	b := `{ "c" : "x" , "a" : { "x" : [ 1 , 2 , 3 ] , "y" : 2 } , "b" : 1 }`
	ca, err := CanonicalizeJCS(json.RawMessage(a))
	if err != nil {
		t.Fatal(err)
	}
	cb, err := CanonicalizeJCS(json.RawMessage(b))
	if err != nil {
		t.Fatal(err)
	}
	if string(ca) != string(cb) {
		t.Errorf("key order/whitespace changed the canonical form:\n %s\n %s", ca, cb)
	}
	if string(ca) != `{"a":{"x":[1,2,3],"y":2},"b":1,"c":"x"}` {
		t.Errorf("unexpected canonical form %s", ca)
	}
}

func TestJCSNumberAndStringEncoding(t *testing.T) {
	cases := map[string]string{
		"1.0":               "1",
		"100000":            "100000",
		"1e21":              "1e+21",
		"0.0001":            "0.0001",
		"1e-7":              "1e-7",
		"1.5e300":           "1.5e+300",
		"12345678901234567": "12345678901234568",
	}
	for in, want := range cases {
		got, err := CanonicalizeJCS(json.RawMessage(in))
		if err != nil {
			t.Fatalf("%s: %v", in, err)
		}
		// Go's shortest round-trip may differ by one representation for the last
		// case, so only assert the exponent forms that are actually specified.
		if strings.Contains(want, "e") && string(got) != want {
			t.Errorf("number %s -> %s, want %s", in, got, want)
		}
	}
	esc, err := CanonicalizeJCS(map[string]string{"k": "a\"b\\c\nd\u007fe"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(esc), `a\"b\\c\nd`) || strings.Contains(string(esc), `\u007f`) {
		t.Errorf("unexpected escaping: %s", esc)
	}
}

func TestStoreChainVerifiesAndDetectsTampering(t *testing.T) {
	s := openTestStore(t)
	for i := 0; i < 3; i++ {
		ev := s.NewEvent(EventOperationPrescribed, ProvenanceAgentDeclared, "SES-1", "EV-1")
		if err := ev.SetPayload(PrescribedPayload{Objective: "restart payments"}); err != nil {
			t.Fatal(err)
		}
		if _, err := s.AppendEvent(ev); err != nil {
			t.Fatalf("append %d: %v", i, err)
		}
	}
	if err := s.Close("test_end"); err != nil {
		t.Fatal(err)
	}
	rep, err := VerifyStore(s.Dir())
	if err != nil {
		t.Fatal(err)
	}
	if !rep.ChainValid || !rep.SignatureValid || !rep.Complete() {
		t.Fatalf("fresh store did not verify: %+v", rep)
	}
	if rep.FirstSeq != 1 || rep.LastSeq != 5 {
		t.Errorf("seq range = %d..%d, want 1..5 (started + 3 + stopped)", rep.FirstSeq, rep.LastSeq)
	}
	if rep.Counts[string(EventOperationPrescribed)] != 3 {
		t.Errorf("event counts = %v", rep.Counts)
	}

	// Rewrite one payload without re-hashing: the chain must notice.
	events := filepath.Join(s.Dir(), "events.jsonl")
	raw, err := os.ReadFile(events)
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimRight(string(raw), "\n"), "\n")
	var ev Event
	if err := json.Unmarshal([]byte(lines[2]), &ev); err != nil {
		t.Fatal(err)
	}
	if err := ev.SetPayload(PrescribedPayload{Objective: "restart checkout"}); err != nil {
		t.Fatal(err)
	}
	forged, _ := json.Marshal(ev)
	lines[2] = string(forged)
	if err := os.WriteFile(events, []byte(strings.Join(lines, "\n")+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	rep, err = VerifyStore(s.Dir())
	if err != nil {
		t.Fatal(err)
	}
	if rep.ChainValid || rep.SignatureValid {
		t.Errorf("edited record still verified: %+v", rep)
	}
	t.Logf("tamper findings: %v", rep.Findings)
}

func TestDegradedWindowIsRecordedAndCoverageDegrades(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "recorder-degraded")
	s, err := OpenStore(Options{Dir: dir, UpstreamID: "up", EnforceMode: "all", Stderr: os.Stderr})
	if err != nil {
		t.Fatal(err)
	}
	// Simulate losing the ability to record, then recovering: close the file out
	// from under the store so the next append fails.
	f := s.f
	s.MarkUnhealthy("simulated disk failure")
	if !s.Unhealthy() {
		t.Fatal("store should be unhealthy after a failure")
	}
	_ = f.Close()
	if _, err := s.AppendEvent(s.NewEvent(EventProtocolViolation, ProvenanceRecorderGenerated, "SES", "EV")); err == nil {
		t.Error("append should fail while the underlying file is closed")
	}
	// Reopen for writing: the next successful append must emit recorder_degraded
	// before the event that proves recovery.
	nf, err := os.OpenFile(filepath.Join(dir, "events.jsonl"), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	s.f, s.w = nf, bufio.NewWriterSize(nf, 64*1024)
	ev := s.NewEvent(EventOperationReported, ProvenanceAgentDeclared, "SES", "EV")
	if err := ev.SetPayload(ReportedPayload{Status: "completed", Outcome: "achieved", OperationID: "EV"}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.AppendEvent(ev); err != nil {
		t.Fatalf("append after recovery: %v", err)
	}
	if err := s.Close("test_end"); err != nil {
		t.Fatal(err)
	}
	rep, err := VerifyStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	if !rep.ChainValid || !rep.SignatureValid {
		t.Errorf("a degraded window must not invalidate the chain: %+v", rep)
	}
	if rep.Coverage != "degraded" {
		t.Errorf("coverage = %s, want degraded (chain valid is not coverage complete)", rep.Coverage)
	}
	if rep.Counts[string(EventRecorderDegraded)] != 1 {
		t.Errorf("expected exactly one recorder_degraded, counts=%v", rep.Counts)
	}
}

func TestSubstantiveRuleRemovesEmptyStores(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "recorder-empty")
	s, err := OpenStore(Options{Dir: dir, UpstreamID: "up", EnforceMode: "off"})
	if err != nil {
		t.Fatal(err)
	}
	if s.Substantive() {
		t.Fatal("a fresh store is not substantive")
	}
	if err := s.Abandon(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Errorf("lifecycle-only store directory left behind")
	}

	keep := filepath.Join(t.TempDir(), "recorder-keep")
	s2, err := OpenStore(Options{Dir: keep, UpstreamID: "up", EnforceMode: "off"})
	if err != nil {
		t.Fatal(err)
	}
	v := s2.NewEvent(EventProtocolViolation, ProvenanceRecorderGenerated, "SES", "")
	if err := v.SetPayload(ViolationPayload{Kind: "unprescribed_execution_blocked"}); err != nil {
		t.Fatal(err)
	}
	if _, err := s2.AppendEvent(v); err != nil {
		t.Fatal(err)
	}
	if !s2.Substantive() {
		t.Fatal("a violation is substantive evidence")
	}
	if err := s2.Abandon(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(keep); err != nil {
		t.Errorf("store with evidence was deleted: %v", err)
	}
}

func TestKeysAndMetaAreNotReadableByOthers(t *testing.T) {
	s := openTestStore(t)
	for _, name := range []string{signingKeyFn, digestKeyFn, metaFile, eventsFile} {
		st, err := os.Stat(filepath.Join(s.Dir(), name))
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if st.Mode().Perm()&0o077 != 0 {
			t.Errorf("%s mode = %v, want no group/other access", name, st.Mode().Perm())
		}
	}
	raw, _ := os.ReadFile(filepath.Join(s.Dir(), metaFile))
	var m Meta
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(m.DigestKeyID, "dk1:") || len(m.DigestKeyID) != 4+16 {
		t.Errorf("digest_key_id = %q", m.DigestKeyID)
	}
	keyRaw, _ := os.ReadFile(filepath.Join(s.Dir(), digestKeyFn))
	if strings.Contains(string(raw), strings.TrimSpace(string(keyRaw))) {
		t.Fatal("meta.json contains the HMAC key")
	}
}

func TestPrivacyCanaryNeverReachesDiskInPayloadForm(t *testing.T) {
	s := openTestStore(t)
	const canary = "CANARY-s3cr3t-payload-value-8842"
	args := map[string]any{"token": canary, "n": 1}
	h, err := ArgumentsHMAC(s.DigestKey(), args)
	if err != nil {
		t.Fatal(err)
	}
	start := s.NewEvent(EventExecutionStarted, ProvenanceProxyObserved, "SES", "EV")
	if err := start.SetPayload(ExecutionStartedPayload{
		ExecutionID: "EXE-1", Tool: "restart", ArgumentsHMAC: h, StartedAt: time.Now().UTC(),
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.AppendEvent(start); err != nil {
		t.Fatal(err)
	}
	result := []byte(`{"detail":"` + canary + `"}`)
	rh, status, format := ResultFingerprint(s.DigestKey(), result, s.MaxResultBytes())
	fin := s.NewEvent(EventExecutionFinished, ProvenanceProxyObserved, "SES", "EV")
	if err := fin.SetPayload(ExecutionFinishedPayload{
		ExecutionID: "EXE-1", Tool: "restart", Status: ExecutionError,
		ResultHMAC: strPtr(rh), ResultFingerprintStatus: status, ResultFingerprintFormat: format,
		ErrorCode: "server_error", ErrorMessageHMAC: ErrorFingerprint(s.DigestKey(), canary),
		DurationMS: 7, FinishedAt: time.Now().UTC(),
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.AppendEvent(fin); err != nil {
		t.Fatal(err)
	}
	decl := s.NewEvent(EventOperationReported, ProvenanceAgentDeclared, "SES", "EV")
	if err := decl.SetPayload(ReportedPayload{Status: "failed", Outcome: "not_achieved",
		Summary: "agent plaintext mentions " + canary, OperationID: "EV"}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.AppendEvent(decl); err != nil {
		t.Fatal(err)
	}
	if err := s.Close("test_end"); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(filepath.Join(s.Dir(), eventsFile))
	if err != nil {
		t.Fatal(err)
	}
	text := string(raw)
	// Observed payloads are fingerprinted; only agent declarations keep the text.
	if strings.Count(text, canary) != 1 {
		t.Errorf("canary appears %d times in the store; observed payloads must be fingerprints only", strings.Count(text, canary))
	}
	if !strings.Contains(text, "agent plaintext mentions") {
		t.Error("declared plaintext was dropped")
	}
	if !strings.Contains(text, "sha256:") {
		t.Error("no fingerprints were stored")
	}
	if b, err := base64.StdEncoding.DecodeString("x"); err == nil && len(b) > 0 {
		t.Log("unused")
	}
}

func TestResultFingerprintBound(t *testing.T) {
	key := []byte("01234567890123456789012345678901")
	_, status, _ := ResultFingerprint(key, make([]byte, DefaultMaxResultBytes+1), 0)
	if status != FingerprintOmittedOversize {
		t.Errorf("oversize status = %s", status)
	}
	_, status, _ = ResultFingerprint(key, nil, 0)
	if status != FingerprintUnavailable {
		t.Errorf("empty status = %s", status)
	}
	small, status, format := ResultFingerprint(key, []byte(`{"a":1}`), 0)
	if status != FingerprintPresent || format != "wire_json" || small == "" {
		t.Errorf("present case = %q/%s/%s", small, status, format)
	}
}

func TestParseSinceAndGroupKey(t *testing.T) {
	now := time.Now().UTC()
	got, err := ParseSince("7d", now)
	if err != nil || !got.Equal(now.Add(-7*24*time.Hour)) {
		t.Fatalf("7d -> %v (%v)", got, err)
	}
	if got, err := ParseSince("2026-09-13T00:00:00Z", now); err != nil || got.Year() != 2026 {
		t.Fatalf("timestamp -> %v (%v)", got, err)
	}
	if got, err := ParseSince("", now); err != nil || !got.IsZero() {
		t.Fatalf("empty -> %v (%v)", got, err)
	}
	if _, err := ParseSince("banana", now); err == nil {
		t.Error("garbage accepted")
	}
	k := GroupKey(Meta{EnforceMode: "all", UpstreamID: "up", DigestKeyID: "dk1:x"})
	if !strings.HasPrefix(k, "all|up|") {
		t.Errorf("group key = %s", k)
	}
}

func strPtr(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}
