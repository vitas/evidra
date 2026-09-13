package evidence

// v2 evidence store (§18-§21).
//
// One directory per recorder process, one append-only file inside it, and one
// serialized critical section that reads the tail, builds the event, hashes,
// signs, appends, and updates the tail. The single-writer property is structural
// rather than negotiated: because no other process opens this directory, a
// cross-process chain race is not something the code has to survive, and v1 does
// not claim multi-writer support (§19).

import (
	"bufio"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/oklog/ulid/v2"
)

// Store files, named by §20.
const (
	eventsFile   = "events.jsonl"
	metaFile     = "meta.json"
	signingKeyFn = "signing.key"
	digestKeyFn  = "digest.key"
)

// Meta is what a reader needs before it parses a single event.
type Meta struct {
	SchemaVersion      string    `json:"schema_version"`
	CreatedAt          time.Time `json:"created_at"`
	RecorderInstanceID string    `json:"recorder_instance_id"`
	UpstreamID         string    `json:"upstream_id"`
	ServerName         string    `json:"server_name,omitempty"`
	UpstreamCommand    string    `json:"upstream_command,omitempty"`
	PublicKey          string    `json:"public_key"`
	DigestKeyID        string    `json:"digest_key_id"`
	EnforceMode        string    `json:"enforce_mode"`
}

// Options configures one recorder store.
type Options struct {
	// Root is the evidence root; the store creates its own directory inside it.
	Root string
	// Dir overrides the generated recorder directory name (used by tests and by
	// callers that already allocated a per-run path).
	Dir           string
	UpstreamID    string
	ServerName    string
	UpstreamCmd   string
	EnforceMode   string
	EvidraVersion string
	// MaxResultBytes bounds result fingerprints; zero uses DefaultMaxResultBytes.
	MaxResultBytes int
	// Stderr receives the fatal diagnostic when evidence is lost after an
	// execution already happened. It must not be silenced: the process keeps
	// serving the agent while its own record is incomplete (§18).
	Stderr io.Writer
}

// Store is an append-only, hash-chained evidence file owned by one process.
type Store struct {
	mu   sync.Mutex
	dir  string
	meta Meta
	sign ed25519.PrivateKey
	dig  []byte

	f    *os.File
	w    *bufio.Writer
	seq  uint64
	tail string

	unhealthy  bool
	failedAt   []time.Time
	windowOpen *time.Time
	maxResult  int
	stderr     io.Writer
	started    time.Time
}

// OpenStore creates or resumes a recorder directory and writes recorder_started.
func OpenStore(o Options) (*Store, error) {
	if o.Root == "" && o.Dir == "" {
		return nil, errors.New("store: Root or Dir is required")
	}
	dir := o.Dir
	if dir == "" {
		dir = filepath.Join(o.Root, NewRecorderDirName(time.Now()))
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, fmt.Errorf("store %s: %w", dir, err)
	}
	sign, err := loadOrCreateSigningKey(filepath.Join(dir, signingKeyFn))
	if err != nil {
		return nil, err
	}
	digest, err := LoadOrCreateDigestKey(filepath.Join(dir, digestKeyFn))
	if err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	meta := Meta{
		SchemaVersion:      SchemaVersion,
		CreatedAt:          now,
		RecorderInstanceID: "REC-" + ulid.Make().String(),
		UpstreamID:         o.UpstreamID,
		ServerName:         o.ServerName,
		UpstreamCommand:    o.UpstreamCmd,
		PublicKey:          base64.StdEncoding.EncodeToString(sign.Public().(ed25519.PublicKey)),
		DigestKeyID:        DigestKeyID(digest),
		EnforceMode:        o.EnforceMode,
	}
	if err := writeMeta(filepath.Join(dir, metaFile), meta); err != nil {
		return nil, err
	}
	f, err := os.OpenFile(filepath.Join(dir, eventsFile), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return nil, fmt.Errorf("store %s: open events: %w", dir, err)
	}
	s := &Store{
		dir: dir, meta: meta, sign: sign, dig: digest,
		f: f, w: bufio.NewWriterSize(f, 64*1024),
		maxResult: o.MaxResultBytes, stderr: o.Stderr, started: now,
	}
	if err := s.resumeTail(); err != nil {
		_ = f.Close()
		return nil, err
	}
	if _, err := s.Append(func(prev string, seq uint64) (Event, error) {
		ev := s.newEvent(EventRecorderStarted, ProvenanceRecorderGenerated)
		return ev, ev.SetPayload(LifecyclePayload{
			Reason: "started", EvidraVersion: o.EvidraVersion,
			EnforceMode: o.EnforceMode, UpstreamCommand: o.UpstreamCmd,
		})
	}); err != nil {
		_ = f.Close()
		return nil, fmt.Errorf("store %s: first event: %w", dir, err)
	}
	return s, nil
}

// NewRecorderDirName builds the stable directory name from §20.
func NewRecorderDirName(t time.Time) string {
	return fmt.Sprintf("recorder-%s-%s", t.UTC().Format("20060102-150405"),
		strings.ToLower(ulid.Make().String()[:8]))
}

// resumeTail reads the last existing record so a resumed store continues the
// chain instead of restarting it at seq 1.
func (s *Store) resumeTail() error {
	raw, err := os.ReadFile(filepath.Join(s.dir, eventsFile))
	if err != nil {
		return err
	}
	lines := strings.Split(strings.TrimRight(string(raw), "\n"), "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		line := strings.TrimSpace(lines[i])
		if line == "" {
			continue
		}
		var ev Event
		if err := json.Unmarshal([]byte(line), &ev); err != nil {
			// A torn final line from a killed process is recoverable by dropping
			// it, but anything else means the file was rewritten by someone.
			if i != len(lines)-1 {
				return fmt.Errorf("store %s: malformed record at line %d: %w", s.dir, i+1, err)
			}
			continue
		}
		s.seq, s.tail = ev.Seq, ev.Hash
		return nil
	}
	return nil
}

// Dir reports where the store writes.
func (s *Store) Dir() string { return s.dir }

// Meta returns the store descriptor.
func (s *Store) Meta() Meta { return s.meta }

// DigestKey is the HMAC key, for the proxy that fingerprints live traffic.
func (s *Store) DigestKey() []byte { return s.dig }

// MaxResultBytes is the configured fingerprint bound.
func (s *Store) MaxResultBytes() int {
	if s.maxResult <= 0 {
		return DefaultMaxResultBytes
	}
	return s.maxResult
}

// Unhealthy reports whether evidence has been lost in this process's lifetime.
// Enforcement continues either way; what changes is that subsequent operational
// calls are refused, because a recorder that cannot record has no business
// pretending otherwise (§18).
func (s *Store) Unhealthy() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.unhealthy
}

// NewEvent starts an envelope with the store's identity fields filled in.
func (s *Store) NewEvent(t EventType, p Provenance, sessionID, operationID string) Event {
	ev := s.newEvent(t, p)
	ev.SessionID = sessionID
	ev.OperationID = operationID
	return ev
}

func (s *Store) newEvent(t EventType, p Provenance) Event {
	return Event{
		SchemaVersion:      SchemaVersion,
		EventID:            "EVT-" + ulid.Make().String(),
		EventType:          t,
		RecordedAt:         time.Now().UTC().Truncate(time.Microsecond),
		RecorderInstanceID: s.meta.RecorderInstanceID,
		UpstreamID:         s.meta.UpstreamID,
		ServerName:         s.meta.ServerName,
		Provenance:         p,
	}
}

// Append runs the whole read-tail / build / hash / sign / write / update-tail
// sequence under one serialization boundary (§19). Handing the builder prevHash
// and seq rather than letting it read them is what makes a torn write impossible:
// there is no window between deciding and recording where another writer could
// interleave.
func (s *Store) Append(build func(prevHash string, seq uint64) (Event, error)) (Event, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.appendLocked(build)
}

// appendLocked is the critical section. It is separate from Append so lifecycle
// code that already holds the boundary (Close) does not deadlock on it, which the
// first version of this file did.
func (s *Store) appendLocked(build func(prevHash string, seq uint64) (Event, error)) (Event, error) {
	if s.f == nil {
		return Event{}, errors.New("store: closed")
	}
	if s.windowOpen != nil {
		if err := s.flushDegradedLocked(); err != nil {
			return Event{}, err
		}
	}
	ev, err := build(s.tail, s.seq+1)
	if err != nil {
		return Event{}, err
	}
	if err := s.sealLocked(&ev); err != nil {
		return Event{}, err
	}
	if err := s.writeLocked(ev); err != nil {
		return Event{}, err
	}
	s.seq, s.tail = ev.Seq, ev.Hash
	return ev, nil
}

// AppendEvent is the common case: the caller already built the event.
func (s *Store) AppendEvent(ev Event) (Event, error) {
	return s.Append(func(prevHash string, seq uint64) (Event, error) {
		ev.PreviousHash, ev.Seq = prevHash, seq
		return ev, nil
	})
}

func (s *Store) sealLocked(ev *Event) error {
	ev.PreviousHash = s.tail
	ev.Seq = s.seq + 1
	sum, err := EventHash(*ev)
	if err != nil {
		return fmt.Errorf("store: hash event: %w", err)
	}
	ev.Hash = hex.EncodeToString(sum)
	ev.Signature = base64.StdEncoding.EncodeToString(ed25519.Sign(s.sign, sum))
	return nil
}

func (s *Store) writeLocked(ev Event) error {
	raw, err := json.Marshal(ev)
	if err != nil {
		s.noteFailureLocked(ev.RecordedAt, "encode: "+err.Error())
		return fmt.Errorf("store: encode event %d: %w", ev.Seq, err)
	}
	if _, err := s.w.Write(append(raw, '\n')); err != nil {
		s.noteFailureLocked(ev.RecordedAt, "write: "+err.Error())
		return err
	}
	// Every event is flushed. A buffered recorder is a recorder that loses the
	// tail on crash, and the tail is exactly what an incomplete execution needs.
	if err := s.w.Flush(); err != nil {
		s.noteFailureLocked(ev.RecordedAt, "flush: "+err.Error())
		return err
	}
	return nil
}

// noteFailureLocked marks the store unhealthy and opens a degraded window. The
// window is what later proves that enforcement kept deciding without evidence.
func (s *Store) noteFailureLocked(at time.Time, reason string) {
	s.unhealthy = true
	s.failedAt = append(s.failedAt, at)
	if s.windowOpen == nil {
		start := at
		s.windowOpen = &start
	}
	if s.stderr != nil {
		fmt.Fprintf(s.stderr, "evidra: evidence store failure (%s): %s\n", s.dir, reason)
	}
}

// MarkUnhealthy is how the caller reports a failure it detected outside Append,
// such as being unable to write execution_started before a call was forwarded.
func (s *Store) MarkUnhealthy(reason string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.noteFailureLocked(time.Now().UTC(), reason)
}

// flushDegradedLocked writes the pending recorder_degraded record first, so the
// event that proves recovery follows the statement about the gap (§18).
func (s *Store) flushDegradedLocked() error {
	start := *s.windowOpen
	end := time.Now().UTC()
	s.windowOpen = nil
	ev := s.newEvent(EventRecorderDegraded, ProvenanceRecorderGenerated)
	if err := ev.SetPayload(DegradedPayload{
		WindowStart: start, WindowEnd: end,
		Reason: fmt.Sprintf("%d append failures", len(s.failedAt)),
	}); err != nil {
		return err
	}
	if err := s.sealLocked(&ev); err != nil {
		return err
	}
	if err := s.writeLocked(ev); err != nil {
		return err
	}
	s.seq, s.tail = ev.Seq, ev.Hash
	return nil
}

// Substantive reports whether anything worth keeping was recorded. A recorder
// that only started and stopped deletes its directory rather than leaving noise
// in the evidence root (§20).
func (s *Store) Substantive() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.substantiveLocked()
}

func (s *Store) substantiveLocked() bool {
	return s.countSubstantiveLocked()
}

func (s *Store) countSubstantiveLocked() bool {
	f, err := os.Open(filepath.Join(s.dir, eventsFile))
	if err != nil {
		return false
	}
	defer func() { _ = f.Close() }()
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 16*1024*1024)
	for sc.Scan() {
		var ev Event
		if json.Unmarshal(sc.Bytes(), &ev) == nil && ev.EventType.Substantive() {
			return true
		}
	}
	return false
}

// Close writes recorder_stopped and releases the file. It reports an error when
// the final event could not be persisted, because that is a coverage fact.
func (s *Store) Close(reason string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.f == nil {
		return nil
	}
	_, err := s.appendLocked(func(prevHash string, seq uint64) (Event, error) {
		ev := s.newEvent(EventRecorderStopped, ProvenanceRecorderGenerated)
		return ev, ev.SetPayload(LifecyclePayload{Reason: reason, EnforceMode: s.meta.EnforceMode})
	})
	closeErr := s.f.Close()
	s.f = nil
	if err != nil {
		return err
	}
	return closeErr
}

// Abandon closes the file and removes the directory when nothing substantive was
// recorded. It never removes a store that has evidence in it.
func (s *Store) Abandon() error {
	if err := s.Close("stopped"); err != nil {
		return err
	}
	if s.Substantive() {
		return nil
	}
	return os.RemoveAll(s.dir)
}

// EventHash is the digest covered by the signature: the whole envelope minus the
// hash and signature themselves, canonicalized so the same event hashes the same
// way on any machine.
func EventHash(ev Event) ([]byte, error) {
	ev.Hash = ""
	ev.Signature = ""
	canonical, err := CanonicalizeJCS(ev)
	if err != nil {
		return nil, err
	}
	sum := sha256Canonical(canonical)
	return sum, nil
}

func loadOrCreateSigningKey(path string) (ed25519.PrivateKey, error) {
	raw, err := readIfExists(path)
	if err != nil {
		return nil, err
	}
	if len(raw) > 0 {
		key, err := hex.DecodeString(trimOneSpace(raw))
		if err != nil || len(key) != ed25519.PrivateKeySize {
			return nil, fmt.Errorf("%s: not an ed25519 private key", path)
		}
		return ed25519.PrivateKey(key), nil
	}
	_, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return nil, fmt.Errorf("generate signing key: %w", err)
	}
	return priv, writeExclusive(path, []byte(hex.EncodeToString(priv)+"\n"), 0o600)
}

func writeMeta(path string, m Meta) error {
	raw, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	return writeExclusive(path, append(raw, '\n'), 0o600)
}

func readIfExists(path string) ([]byte, error) {
	raw, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	return raw, err
}

func trimOneSpace(raw []byte) string { return strings.TrimSpace(string(raw)) }

// writeExclusive writes through a temporary file so a reader never sees a
// half-written key or meta file, and applies mode before the rename.
func writeExclusive(path string, data []byte, mode os.FileMode) error {
	tmp := path + ".tmp"
	f, err := os.OpenFile(tmp, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, mode)
	if err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	if _, err := f.Write(data); err != nil {
		_ = f.Close()
		_ = os.Remove(tmp)
		return fmt.Errorf("write %s: %w", path, err)
	}
	if err := f.Close(); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return os.Rename(tmp, path)
}
