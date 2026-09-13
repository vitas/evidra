package evidence

// Reading and verifying a v2 store (§21, §20 aggregation rules).
//
// Verification answers two separate questions and never merges them: is the
// chain intact (nothing edited or removed inside what is present), and is
// coverage complete (nothing was dropped while the store was failing). A
// recorder_degraded event can sit in a perfectly valid chain, and that
// combination is the honest statement about a degraded window.

import (
	"bufio"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
)

func sha256Canonical(b []byte) []byte {
	sum := sha256.Sum256(b)
	return sum[:]
}

// ReadStore returns the events in one recorder directory, oldest first, optionally
// filtered by a lower time bound. Filtering happens at read time because the
// store is append-only evidence: a summary window is a view, not a deletion.
func ReadStore(dir string, since time.Time) ([]Event, Meta, error) {
	var m Meta
	raw, err := os.ReadFile(filepath.Join(dir, metaFile))
	if err != nil {
		return nil, m, fmt.Errorf("read %s: %w", metaFile, err)
	}
	if err := json.Unmarshal(raw, &m); err != nil {
		return nil, m, fmt.Errorf("parse %s: %w", metaFile, err)
	}
	if m.SchemaVersion != SchemaVersion {
		return nil, m, fmt.Errorf("%s: schema_version %q, this build writes %q", dir, m.SchemaVersion, SchemaVersion)
	}
	f, err := os.Open(filepath.Join(dir, eventsFile))
	if err != nil {
		return nil, m, err
	}
	defer func() { _ = f.Close() }()
	var out []Event
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 32*1024*1024)
	line := 0
	for sc.Scan() {
		line++
		trimmed := strings.TrimSpace(string(sc.Bytes()))
		if trimmed == "" {
			continue
		}
		var ev Event
		if err := json.Unmarshal([]byte(trimmed), &ev); err != nil {
			return out, m, fmt.Errorf("%s/%s:%d: %w", dir, eventsFile, line, err)
		}
		if !since.IsZero() && ev.RecordedAt.Before(since) {
			continue
		}
		out = append(out, ev)
	}
	return out, m, sc.Err()
}

// StoreReport is the verification statement for one recorder directory.
type StoreReport struct {
	Dir                string         `json:"dir"`
	RecorderInstanceID string         `json:"recorder_instance_id"`
	UpstreamID         string         `json:"upstream_id"`
	ServerName         string         `json:"server_name,omitempty"`
	EnforceMode        string         `json:"enforce_mode"`
	DigestKeyID        string         `json:"digest_key_id"`
	Records            int            `json:"records"`
	FirstSeq           uint64         `json:"first_seq"`
	LastSeq            uint64         `json:"last_seq"`
	ChainValid         bool           `json:"chain_valid"`
	SignatureValid     bool           `json:"signature_valid"`
	Coverage           string         `json:"evidence_coverage"`
	Counts             map[string]int `json:"event_counts"`
	Findings           []string       `json:"findings,omitempty"`
}

// Complete returns true only when both integrity statements hold.
func (r StoreReport) Complete() bool {
	return r.ChainValid && r.SignatureValid && r.Coverage == "complete"
}

// VerifyStore recomputes every hash and signature in a recorder directory.
func VerifyStore(dir string) (StoreReport, error) {
	events, meta, err := ReadStore(dir, time.Time{})
	rep := StoreReport{
		Dir: dir, RecorderInstanceID: meta.RecorderInstanceID, UpstreamID: meta.UpstreamID,
		ServerName: meta.ServerName, EnforceMode: meta.EnforceMode, DigestKeyID: meta.DigestKeyID,
		Coverage: "complete", Counts: map[string]int{},
	}
	if err != nil {
		return rep, err
	}
	pub, err := base64.StdEncoding.DecodeString(meta.PublicKey)
	if err != nil {
		rep.Findings = append(rep.Findings, "meta.public_key is not base64")
		return rep, nil
	}
	rep.Records = len(events)
	prev := ""
	chain, sigs := true, true
	for i, ev := range events {
		rep.Counts[string(ev.EventType)]++
		if i == 0 {
			rep.FirstSeq = ev.Seq
		}
		rep.LastSeq = ev.Seq
		if ev.Seq != uint64(i)+rep.FirstSeq {
			chain = false
			rep.Findings = append(rep.Findings, fmt.Sprintf("seq gap at record %d: seq %d after %d", i+1, ev.Seq, rep.LastSeq))
		}
		if ev.PreviousHash != prev {
			chain = false
			rep.Findings = append(rep.Findings, fmt.Sprintf("record %d: previous_hash does not match its predecessor", ev.Seq))
		}
		sum, err := EventHash(ev)
		if err != nil || hex.EncodeToString(sum) != ev.Hash {
			chain = false
			rep.Findings = append(rep.Findings, fmt.Sprintf("record %d: hash mismatch", ev.Seq))
		}
		sig, err := base64.StdEncoding.DecodeString(ev.Signature)
		if err != nil || !ed25519.Verify(ed25519.PublicKey(pub), sum, sig) {
			sigs = false
			rep.Findings = append(rep.Findings, fmt.Sprintf("record %d: signature invalid", ev.Seq))
		}
		if ev.EventType == EventRecorderDegraded {
			rep.Coverage = "degraded"
		}
		prev = ev.Hash
	}
	rep.ChainValid, rep.SignatureValid = chain, sigs
	if rep.LastSeq == 0 {
		rep.Findings = append(rep.Findings, "store contains no records")
	}
	if _, err := os.Stat(filepath.Join(dir, eventsFile)); errors.Is(err, os.ErrNotExist) {
		rep.Findings = append(rep.Findings, "events file missing")
	}
	return rep, nil
}

// VerifyRoot verifies every recorder directory under a root, keeping each store's
// answer separate. Summaries may add raw counts, but identity and chain state
// never merge across recorders (§20).
func VerifyRoot(root string) ([]StoreReport, error) {
	entries, err := os.ReadDir(root)
	if err != nil {
		return nil, err
	}
	var out []StoreReport
	for _, e := range entries {
		if !e.IsDir() || !strings.HasPrefix(e.Name(), "recorder-") {
			continue
		}
		rep, err := VerifyStore(filepath.Join(root, e.Name()))
		if err != nil {
			return out, fmt.Errorf("verify %s: %w", e.Name(), err)
		}
		out = append(out, rep)
	}
	return out, nil
}

var reSince = regexp.MustCompile(`^(\d+)([smhdw])$`)

// ParseSince accepts the two spellings the CLI documents: a duration (7d, 36h) or
// an RFC 3339 instant.
func ParseSince(value string, now time.Time) (time.Time, error) {
	if strings.TrimSpace(value) == "" {
		return time.Time{}, nil
	}
	if m := reSince.FindStringSubmatch(value); m != nil {
		n, err := strconv.Atoi(m[1])
		if err != nil {
			return time.Time{}, fmt.Errorf("--since %q: %w", value, err)
		}
		var unit time.Duration
		switch m[2] {
		case "s":
			unit = time.Second
		case "m":
			unit = time.Minute
		case "h":
			unit = time.Hour
		case "d":
			unit = 24 * time.Hour
		case "w":
			unit = 7 * 24 * time.Hour
		}
		return now.Add(-time.Duration(n) * unit), nil
	}
	t, err := time.Parse(time.RFC3339, value)
	if err != nil {
		return time.Time{}, fmt.Errorf("--since %q: want a duration like 7d or an RFC 3339 timestamp", value)
	}
	return t.UTC(), nil
}

// GroupKey is what aggregation must group by before combining anything: two
// enforcement modes or two recorder processes are different observations, and
// averaging them produces a number that describes nothing (§20).
func GroupKey(meta Meta) string {
	upstream := meta.UpstreamID
	if upstream == "" {
		upstream = "unlabeled"
	}
	mode := meta.EnforceMode
	if mode == "" {
		mode = "unknown"
	}
	return strings.Join([]string{mode, upstream, meta.DigestKeyID}, "|")
}
