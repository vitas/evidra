package assessment

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"sync"

	"samebits.com/evidra/internal/canon"
	"samebits.com/evidra/internal/score"
	"samebits.com/evidra/internal/signal"
	"samebits.com/evidra/pkg/evidence"
)

type trackerFingerprint struct {
	recordsTotal int
	lastHash     string
}

type sessionState struct {
	prescriptions map[string]canon.CanonicalAction
	signalEntries []signal.Entry
	totalOps      int
	results       []signal.SignalResult
	resultsDirty  bool
	invalid       bool
	snapshots     map[string]Snapshot
}

func newSessionState() *sessionState {
	return &sessionState{
		prescriptions: make(map[string]canon.CanonicalAction),
		snapshots:     make(map[string]Snapshot),
	}
}

func (s *sessionState) observeEntry(entry evidence.EvidenceEntry) error {
	switch entry.Type {
	case evidence.EntryTypePrescribe:
		var p evidence.PrescriptionPayload
		if err := json.Unmarshal(entry.Payload, &p); err != nil {
			s.invalid = true
			return fmt.Errorf("unmarshal prescription %s: %w", entry.EntryID, err)
		}

		se := signal.Entry{
			EventID:        entry.EntryID,
			Timestamp:      entry.Timestamp,
			ActorID:        entry.Actor.ID,
			ArtifactDigest: entry.ArtifactDigest,
			IntentDigest:   entry.IntentDigest,
			IsPrescription: true,
			RiskTags:       p.NativeRiskTags(),
		}
		if ca, err := extractCanonicalAction(p.CanonicalAction); err == nil {
			s.prescriptions[entry.EntryID] = ca
			se.Tool = ca.Tool
			se.Operation = ca.Operation
			se.OperationClass = ca.OperationClass
			se.ScopeClass = ca.ScopeClass
			se.ResourceCount = ca.ResourceCount
			se.ShapeHash = ca.ResourceShapeHash
		}

		s.signalEntries = append(s.signalEntries, se)
		s.totalOps++
		s.resultsDirty = true
		s.snapshots = make(map[string]Snapshot)
		return nil

	case evidence.EntryTypeReport:
		var r evidence.ReportPayload
		if err := json.Unmarshal(entry.Payload, &r); err != nil {
			s.invalid = true
			return fmt.Errorf("unmarshal report %s: %w", entry.EntryID, err)
		}

		se := signal.Entry{
			EventID:        entry.EntryID,
			Timestamp:      entry.Timestamp,
			ActorID:        entry.Actor.ID,
			ArtifactDigest: entry.ArtifactDigest,
			IntentDigest:   entry.IntentDigest,
			IsReport:       true,
			PrescriptionID: r.PrescriptionID,
			ExitCode:       r.ExitCode,
		}
		if ca, ok := s.prescriptions[r.PrescriptionID]; ok {
			se.Tool = ca.Tool
			se.Operation = ca.Operation
			se.OperationClass = ca.OperationClass
			se.ScopeClass = ca.ScopeClass
			se.ResourceCount = ca.ResourceCount
			se.ShapeHash = ca.ResourceShapeHash
		} else {
			s.invalid = true
		}

		s.signalEntries = append(s.signalEntries, se)
		s.resultsDirty = true
		s.snapshots = make(map[string]Snapshot)
		return nil
	}

	return nil
}

func (s *sessionState) snapshot(profile score.Profile) Snapshot {
	if s.resultsDirty {
		s.results = signal.AllSignals(s.signalEntries, signal.DefaultTTL)
		s.resultsDirty = false
		s.snapshots = make(map[string]Snapshot)
	}

	profileID := profile.ID
	if profileID == "" {
		profileID = "default"
	}
	if snapshot, ok := s.snapshots[profileID]; ok {
		return snapshot
	}

	snapshot := BuildFromResultsWithProfile(profile, s.results, s.totalOps)
	s.snapshots[profileID] = snapshot
	return snapshot
}

func extractCanonicalAction(raw json.RawMessage) (canon.CanonicalAction, error) {
	var ca canon.CanonicalAction
	if err := json.Unmarshal(raw, &ca); err != nil {
		return canon.CanonicalAction{}, err
	}
	return ca, nil
}

// Tracker caches assessment state per evidence path and per session.
type Tracker struct {
	evidencePath string

	mu            sync.Mutex
	sessions      map[string]*sessionState
	manifest      trackerFingerprint
	manifestKnown bool
	scanCount     int
}

func NewTracker(evidencePath string) *Tracker {
	return &Tracker{
		evidencePath: evidencePath,
		sessions:     make(map[string]*sessionState),
	}
}

func (t *Tracker) Observe(entry evidence.EvidenceEntry) error {
	if entry.SessionID == "" {
		return nil
	}

	t.mu.Lock()
	defer t.mu.Unlock()

	state := t.sessions[entry.SessionID]
	if state == nil {
		state = newSessionState()
		t.sessions[entry.SessionID] = state
	}

	if err := state.observeEntry(entry); err != nil {
		state.invalid = true
	}

	if t.manifestKnown {
		t.manifest.recordsTotal++
		t.manifest.lastHash = entry.Hash
		return nil
	}

	return t.refreshManifestLocked()
}

func (t *Tracker) Snapshot(sessionID string, profile score.Profile) (Snapshot, error) {
	if sessionID == "" {
		return BuildAtPathWithProfile(t.evidencePath, sessionID, profile)
	}

	t.mu.Lock()
	changed, err := t.refreshAndCheckManifestLocked()
	if err != nil {
		t.mu.Unlock()
		return Snapshot{}, err
	}

	state := t.sessions[sessionID]
	if state != nil && !state.invalid && !changed {
		snapshot := state.snapshot(profile)
		t.mu.Unlock()
		return snapshot, nil
	}
	t.mu.Unlock()

	rebuilt, err := t.rebuildSession(sessionID)
	if err != nil {
		return Snapshot{}, err
	}
	return rebuilt.snapshot(profile), nil
}

var (
	trackerPoolMu sync.Mutex
	trackerPool   = map[string]*Tracker{}
)

func TrackerForPath(evidencePath string) *Tracker {
	trackerPoolMu.Lock()
	defer trackerPoolMu.Unlock()

	tracker, ok := trackerPool[evidencePath]
	if ok {
		return tracker
	}

	tracker = NewTracker(evidencePath)
	trackerPool[evidencePath] = tracker
	return tracker
}

func (t *Tracker) rebuildSession(sessionID string) (*sessionState, error) {
	state := newSessionState()
	if sessionID != "" {
		if err := evidence.ForEachEntryAtPath(t.evidencePath, func(entry evidence.EvidenceEntry) error {
			if entry.SessionID != sessionID {
				return nil
			}
			return state.observeEntry(entry)
		}); err != nil {
			if !errors.Is(err, os.ErrNotExist) {
				return nil, fmt.Errorf("rebuild assessment session %q: %w", sessionID, err)
			}
		}
	}

	t.mu.Lock()
	defer t.mu.Unlock()

	t.scanCount++
	t.sessions[sessionID] = state
	if err := t.refreshManifestLocked(); err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	return state, nil
}

func (t *Tracker) refreshAndCheckManifestLocked() (bool, error) {
	current, known, err := loadTrackerFingerprint(t.evidencePath)
	if err != nil {
		return false, err
	}
	if !known {
		changed := t.manifestKnown && (t.manifest.recordsTotal != 0 || t.manifest.lastHash != "")
		t.manifest = trackerFingerprint{}
		t.manifestKnown = false
		return changed, nil
	}
	if !t.manifestKnown {
		t.manifest = current
		t.manifestKnown = true
		return false, nil
	}

	changed := t.manifest != current
	t.manifest = current
	return changed, nil
}

func (t *Tracker) refreshManifestLocked() error {
	current, known, err := loadTrackerFingerprint(t.evidencePath)
	if err != nil {
		return err
	}
	t.manifest = current
	t.manifestKnown = known
	return nil
}

func loadTrackerFingerprint(evidencePath string) (trackerFingerprint, bool, error) {
	manifest, err := evidence.LoadManifest(evidencePath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return trackerFingerprint{}, false, nil
		}
		return trackerFingerprint{}, false, fmt.Errorf("load evidence manifest: %w", err)
	}
	return trackerFingerprint{
		recordsTotal: manifest.RecordsTotal,
		lastHash:     manifest.LastHash,
	}, true, nil
}
