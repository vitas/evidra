package evidence

import (
	"strings"
	"testing"
	"time"
)

func TestLengthPrefixedJoin(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		values []string
		want   string
	}{
		{
			name:   "empty slice",
			values: []string{},
			want:   "",
		},
		{
			name:   "nil slice",
			values: nil,
			want:   "",
		},
		{
			name:   "single value",
			values: []string{"foo"},
			want:   "3:foo",
		},
		{
			name:   "two values",
			values: []string{"foo", "bar"},
			want:   "3:foo,3:bar",
		},
		{
			name:   "value with comma",
			values: []string{"hello,world"},
			want:   "11:hello,world",
		},
		{
			name:   "multiple values with commas",
			values: []string{"foo", "hello,world", "baz"},
			want:   "3:foo,11:hello,world,3:baz",
		},
		{
			name:   "empty string element",
			values: []string{""},
			want:   "0:",
		},
		{
			name:   "empty string between values",
			values: []string{"a", "", "b"},
			want:   "1:a,0:,1:b",
		},
		{
			name:   "unicode cafe",
			values: []string{"caf\u00e9"},
			want:   "5:caf\u00e9", // 'e' is 2 bytes in UTF-8
		},
		{
			name:   "unicode emoji",
			values: []string{"\U0001f512"},
			want:   "4:\U0001f512", // emoji is 4 bytes
		},
		{
			name:   "unicode mixed",
			values: []string{"hello", "\u4e16\u754c"},
			want:   "5:hello,6:\u4e16\u754c",
		},
		{
			name:   "value with colon",
			values: []string{"key:value"},
			want:   "9:key:value",
		},
		{
			name:   "value with newline",
			values: []string{"line1\nline2"},
			want:   "11:line1\nline2",
		},
		{
			name:   "three from CLAUDE.md example",
			values: []string{"foo", "hello,world", ""},
			want:   "3:foo,11:hello,world,0:",
		},
		{
			name:   "long value",
			values: []string{strings.Repeat("x", 100)},
			want:   "100:" + strings.Repeat("x", 100),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := lengthPrefixedJoin(tt.values)
			if got != tt.want {
				t.Errorf("lengthPrefixedJoin(%v)\n got: %q\nwant: %q", tt.values, got, tt.want)
			}
		})
	}
}

func TestBuildSigningPayload_FullRecord(t *testing.T) {
	t.Parallel()

	ts := time.Date(2026, 2, 26, 12, 30, 45, 123000000, time.UTC)

	rec := &EvidenceRecord{
		EventID:        "evt_01JTEST",
		Timestamp:      ts,
		ServerID:       "api-1",
		TenantID:       "tenant_static",
		Environment:    "staging",
		Actor:          ActorRecord{Type: "agent", ID: "claude", Origin: "mcp"},
		Tool:           "kubectl",
		Operation:      "apply",
		InputHash:      "sha256:abc123",
		PolicyRef:      "sha256:def456",
		BundleRevision: "2026.1",
		ProfileName:    "ops-v0.1",
		Decision: DecisionRecord{
			Allow:     false,
			RiskLevel: "high",
			Reason:    "denied by policy",
			Reasons:   []string{"namespace.forbidden", "image.unsigned"},
			Hints:     []string{"use namespace test-*", "sign your images"},
			Hits:      []string{"namespace.forbidden"},
			RuleIDs:   []string{"R001", "R002"},
		},
	}

	got := BuildSigningPayload(rec)

	want := "evidra.v1\n" +
		"event_id=evt_01JTEST\n" +
		"timestamp=2026-02-26T12:30:45.123Z\n" +
		"server_id=api-1\n" +
		"tenant_id=tenant_static\n" +
		"environment=staging\n" +
		"actor.type=agent\n" +
		"actor.id=claude\n" +
		"actor.origin=mcp\n" +
		"tool=kubectl\n" +
		"operation=apply\n" +
		"input_hash=sha256:abc123\n" +
		"policy_ref=sha256:def456\n" +
		"bundle_revision=2026.1\n" +
		"profile_name=ops-v0.1\n" +
		"allow=false\n" +
		"risk_level=high\n" +
		"reason=denied by policy\n" +
		"reasons=19:namespace.forbidden,14:image.unsigned\n" +
		"hints=20:use namespace test-*,16:sign your images\n" +
		"hits=19:namespace.forbidden\n" +
		"rule_ids=4:R001,4:R002\n"

	if got != want {
		t.Errorf("BuildSigningPayload mismatch\n got:\n%s\nwant:\n%s", got, want)
	}
}

func TestBuildSigningPayload_EmptyOptionalFields(t *testing.T) {
	t.Parallel()

	ts := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

	rec := &EvidenceRecord{
		EventID:   "evt_minimal",
		Timestamp: ts,
		ServerID:  "api-1",
		TenantID:  "t1",
		Actor:     ActorRecord{Type: "agent", ID: "bot", Origin: "cli"},
		Tool:      "terraform",
		Operation: "plan",
		InputHash: "sha256:000",
		PolicyRef: "sha256:111",
		Decision: DecisionRecord{
			Allow:     true,
			RiskLevel: "low",
			Reason:    "all clear",
		},
	}

	got := BuildSigningPayload(rec)

	// Empty optional fields still appear as empty values.
	if !strings.Contains(got, "environment=\n") {
		t.Error("expected empty environment field")
	}
	if !strings.Contains(got, "bundle_revision=\n") {
		t.Error("expected empty bundle_revision field")
	}
	if !strings.Contains(got, "profile_name=\n") {
		t.Error("expected empty profile_name field")
	}
	// Empty list fields produce empty length-prefixed value.
	if !strings.Contains(got, "reasons=\n") {
		t.Error("expected empty reasons list")
	}
	if !strings.Contains(got, "hints=\n") {
		t.Error("expected empty hints list")
	}
	if !strings.Contains(got, "hits=\n") {
		t.Error("expected empty hits list")
	}
	if !strings.Contains(got, "rule_ids=\n") {
		t.Error("expected empty rule_ids list")
	}
}

func TestBuildSigningPayload_Deterministic(t *testing.T) {
	t.Parallel()

	ts := time.Date(2026, 6, 15, 8, 0, 0, 0, time.UTC)

	rec := &EvidenceRecord{
		EventID:   "evt_det",
		Timestamp: ts,
		ServerID:  "s1",
		TenantID:  "t1",
		Actor:     ActorRecord{Type: "agent", ID: "a", Origin: "o"},
		Tool:      "t",
		Operation: "op",
		InputHash: "h",
		PolicyRef: "p",
		Decision: DecisionRecord{
			Allow:     true,
			RiskLevel: "low",
			Reason:    "ok",
			Hints:     []string{"hint1"},
		},
	}

	first := BuildSigningPayload(rec)
	for i := 0; i < 100; i++ {
		got := BuildSigningPayload(rec)
		if got != first {
			t.Fatalf("payload not deterministic on iteration %d", i)
		}
	}
}

func TestBuildSigningPayload_TimestampUTC(t *testing.T) {
	t.Parallel()

	// Use a non-UTC timezone to verify conversion.
	loc := time.FixedZone("UTC+5", 5*3600)
	ts := time.Date(2026, 3, 1, 5, 0, 0, 0, loc) // = 2026-03-01T00:00:00Z in UTC

	rec := &EvidenceRecord{
		EventID:   "evt_tz",
		Timestamp: ts,
		ServerID:  "s",
		TenantID:  "t",
		Actor:     ActorRecord{Type: "a", ID: "b", Origin: "c"},
		Tool:      "t",
		Operation: "o",
		InputHash: "h",
		PolicyRef: "p",
		Decision:  DecisionRecord{Allow: true, RiskLevel: "low", Reason: "ok"},
	}

	got := BuildSigningPayload(rec)
	if !strings.Contains(got, "timestamp=2026-03-01T00:00:00.000Z\n") {
		t.Errorf("timestamp not converted to UTC, got payload:\n%s", got)
	}
}

func TestBuildSigningPayload_VersionPrefix(t *testing.T) {
	t.Parallel()

	rec := &EvidenceRecord{
		EventID:   "evt_ver",
		Timestamp: time.Now(),
		ServerID:  "s",
		TenantID:  "t",
		Actor:     ActorRecord{Type: "a", ID: "b", Origin: "c"},
		Tool:      "t",
		Operation: "o",
		InputHash: "h",
		PolicyRef: "p",
		Decision:  DecisionRecord{Allow: true, RiskLevel: "low", Reason: "ok"},
	}

	got := BuildSigningPayload(rec)
	if !strings.HasPrefix(got, "evidra.v1\n") {
		t.Errorf("payload does not start with version prefix, got: %q", got[:min(len(got), 20)])
	}
}

func TestBuildSigningPayload_AllowTrue(t *testing.T) {
	t.Parallel()

	rec := &EvidenceRecord{
		EventID:   "evt_allow",
		Timestamp: time.Now(),
		ServerID:  "s",
		TenantID:  "t",
		Actor:     ActorRecord{Type: "a", ID: "b", Origin: "c"},
		Tool:      "t",
		Operation: "o",
		InputHash: "h",
		PolicyRef: "p",
		Decision:  DecisionRecord{Allow: true, RiskLevel: "low", Reason: "ok"},
	}

	got := BuildSigningPayload(rec)
	if !strings.Contains(got, "allow=true\n") {
		t.Error("expected allow=true")
	}
}

func TestBuildSigningPayload_AllowFalse(t *testing.T) {
	t.Parallel()

	rec := &EvidenceRecord{
		EventID:   "evt_deny",
		Timestamp: time.Now(),
		ServerID:  "s",
		TenantID:  "t",
		Actor:     ActorRecord{Type: "a", ID: "b", Origin: "c"},
		Tool:      "t",
		Operation: "o",
		InputHash: "h",
		PolicyRef: "p",
		Decision:  DecisionRecord{Allow: false, RiskLevel: "high", Reason: "denied"},
	}

	got := BuildSigningPayload(rec)
	if !strings.Contains(got, "allow=false\n") {
		t.Error("expected allow=false")
	}
}

func TestBuildSigningPayload_UnicodeValues(t *testing.T) {
	t.Parallel()

	rec := &EvidenceRecord{
		EventID:   "evt_uni",
		Timestamp: time.Now(),
		ServerID:  "s",
		TenantID:  "t",
		Actor:     ActorRecord{Type: "agent", ID: "\u5de5\u7a0b\u5e08", Origin: "cli"},
		Tool:      "kubectl",
		Operation: "apply",
		InputHash: "h",
		PolicyRef: "p",
		Decision: DecisionRecord{
			Allow:     true,
			RiskLevel: "low",
			Reason:    "\u901a\u8fc7",
			Hints:     []string{"caf\u00e9", "na\u00efve"},
		},
	}

	got := BuildSigningPayload(rec)
	if !strings.Contains(got, "actor.id=\u5de5\u7a0b\u5e08\n") {
		t.Error("expected unicode actor.id")
	}
	if !strings.Contains(got, "reason=\u901a\u8fc7\n") {
		t.Error("expected unicode reason")
	}
	// caf\u00e9 = 5 bytes, na\u00efve = 6 bytes (\u00ef is 2 bytes)
	if !strings.Contains(got, "hints=5:caf\u00e9,6:na\u00efve\n") {
		t.Errorf("expected unicode hints with byte-length prefix, got payload:\n%s", got)
	}
}

func TestBuildSigningPayload_CommasInListValues(t *testing.T) {
	t.Parallel()

	rec := &EvidenceRecord{
		EventID:   "evt_comma",
		Timestamp: time.Now(),
		ServerID:  "s",
		TenantID:  "t",
		Actor:     ActorRecord{Type: "a", ID: "b", Origin: "c"},
		Tool:      "t",
		Operation: "o",
		InputHash: "h",
		PolicyRef: "p",
		Decision: DecisionRecord{
			Allow:     true,
			RiskLevel: "low",
			Reason:    "ok",
			Reasons:   []string{"a,b,c", "d"},
		},
	}

	got := BuildSigningPayload(rec)
	// "a,b,c" is 5 bytes, "d" is 1 byte
	if !strings.Contains(got, "reasons=5:a,b,c,1:d\n") {
		t.Errorf("commas in values not encoded correctly, got payload:\n%s", got)
	}
}

func TestBuildSigningPayload_FieldOrder(t *testing.T) {
	t.Parallel()

	rec := &EvidenceRecord{
		EventID:   "evt_order",
		Timestamp: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
		ServerID:  "s",
		TenantID:  "t",
		Actor:     ActorRecord{Type: "agent", ID: "id", Origin: "orig"},
		Tool:      "tool",
		Operation: "op",
		InputHash: "ih",
		PolicyRef: "pr",
		Decision: DecisionRecord{
			Allow:     true,
			RiskLevel: "low",
			Reason:    "r",
		},
	}

	got := BuildSigningPayload(rec)
	lines := strings.Split(strings.TrimSuffix(got, "\n"), "\n")

	expectedKeys := []string{
		"evidra.v1", // version line (no key=value)
		"event_id",
		"timestamp",
		"server_id",
		"tenant_id",
		"environment",
		"actor.type",
		"actor.id",
		"actor.origin",
		"tool",
		"operation",
		"input_hash",
		"policy_ref",
		"bundle_revision",
		"profile_name",
		"allow",
		"risk_level",
		"reason",
		"reasons",
		"hints",
		"hits",
		"rule_ids",
	}

	if len(lines) != len(expectedKeys) {
		t.Fatalf("line count = %d, want %d", len(lines), len(expectedKeys))
	}

	for i, line := range lines {
		if i == 0 {
			if line != "evidra.v1" {
				t.Errorf("line 0 = %q, want %q", line, "evidra.v1")
			}
			continue
		}
		key, _, ok := strings.Cut(line, "=")
		if !ok {
			t.Fatalf("line %d missing '=': %q", i, line)
		}
		if key != expectedKeys[i] {
			t.Errorf("line %d key = %q, want %q", i, key, expectedKeys[i])
		}
	}
}

func TestBuildSigningPayload_TrailingNewline(t *testing.T) {
	t.Parallel()

	rec := &EvidenceRecord{
		EventID:   "evt_nl",
		Timestamp: time.Now(),
		ServerID:  "s",
		TenantID:  "t",
		Actor:     ActorRecord{Type: "a", ID: "b", Origin: "c"},
		Tool:      "t",
		Operation: "o",
		InputHash: "h",
		PolicyRef: "p",
		Decision:  DecisionRecord{Allow: true, RiskLevel: "low", Reason: "ok"},
	}

	got := BuildSigningPayload(rec)
	if !strings.HasSuffix(got, "\n") {
		t.Error("payload does not end with newline")
	}
	// Should not have double trailing newline.
	if strings.HasSuffix(got, "\n\n") {
		t.Error("payload has double trailing newline")
	}
}
