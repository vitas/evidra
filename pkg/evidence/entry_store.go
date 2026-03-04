package evidence

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// AppendEntryAtPath writes a pre-built EvidenceEntry to the segmented store.
// The entry must already have Hash computed (via BuildEntry).
// Updates manifest RecordsTotal, LastHash, and UpdatedAt.
func AppendEntryAtPath(path string, entry EvidenceEntry) error {
	return withStoreLock(path, func() error {
		return appendEntryUnlocked(path, entry)
	})
}

func appendEntryUnlocked(path string, entry EvidenceEntry) error {
	maxBytes := segmentMaxBytesFromEnv()
	manifest, err := loadOrInitManifest(path, maxBytes, true)
	if err != nil {
		return err
	}
	if manifest.SegmentMaxBytes <= 0 {
		manifest.SegmentMaxBytes = maxBytes
	}
	if manifest.CurrentSegment == "" {
		manifest.CurrentSegment = segmentName(1)
	}
	manifest.SealedSegments = normalizeSealedSegments(manifest.SealedSegments)

	segPath := filepath.Join(path, segmentsDirName, manifest.CurrentSegment)
	if err := os.MkdirAll(filepath.Dir(segPath), 0o755); err != nil {
		return fmt.Errorf("create segments directory: %w", err)
	}
	if err := appendEntryLine(segPath, entry); err != nil {
		return err
	}

	manifest.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
	manifest.RecordsTotal++
	manifest.LastHash = entry.Hash

	// Segment rotation: if the current segment exceeds size, seal it and
	// create the next one.
	info, err := os.Stat(segPath)
	if err == nil && info.Size() > manifest.SegmentMaxBytes {
		manifest.SealedSegments = append(manifest.SealedSegments, manifest.CurrentSegment)
		manifest.SealedSegments = normalizeSealedSegments(manifest.SealedSegments)
		_, names, listErr := orderedSegmentNames(path)
		if listErr != nil {
			return listErr
		}
		next := 1
		if len(names) > 0 {
			lastIndex, parseErr := parseSegmentIndex(names[len(names)-1])
			if parseErr != nil {
				return parseErr
			}
			next = lastIndex + 1
		}
		manifest.CurrentSegment = segmentName(next)
		manifest.SealedSegments = removeSegment(manifest.SealedSegments, manifest.CurrentSegment)
		nextPath := filepath.Join(path, segmentsDirName, manifest.CurrentSegment)
		if _, statErr := os.Stat(nextPath); errors.Is(statErr, os.ErrNotExist) {
			if writeErr := os.WriteFile(nextPath, []byte(""), 0o644); writeErr != nil {
				return fmt.Errorf("create next segment: %w", writeErr)
			}
		}
	}

	return writeManifestAtomic(path, manifest)
}

// ReadAllEntriesAtPath reads all EvidenceEntry records from the segmented store.
func ReadAllEntriesAtPath(path string) ([]EvidenceEntry, error) {
	entries := make([]EvidenceEntry, 0)
	err := ForEachEntryAtPath(path, func(e EvidenceEntry) error {
		entries = append(entries, e)
		return nil
	})
	if err != nil {
		return nil, err
	}
	return entries, nil
}

// ForEachEntryAtPath iterates over all entries in the segmented store,
// calling fn for each EvidenceEntry in order.
func ForEachEntryAtPath(path string, fn func(EvidenceEntry) error) error {
	return withStoreLock(path, func() error {
		return forEachEntryAtPathUnlocked(path, fn)
	})
}

func forEachEntryAtPathUnlocked(path string, fn func(EvidenceEntry) error) error {
	_, names, err := orderedSegmentNames(path)
	if err != nil {
		return err
	}
	for _, name := range names {
		segPath := filepath.Join(path, segmentsDirName, name)
		if err := streamFileEntries(segPath, func(e EvidenceEntry, _ int) error {
			return fn(e)
		}); err != nil {
			return err
		}
	}
	return nil
}

// FindEntryByID finds an entry by its entry_id in the segmented store.
func FindEntryByID(path string, entryID string) (EvidenceEntry, bool, error) {
	var out EvidenceEntry
	found := false
	errFound := errors.New("entry_found")
	err := ForEachEntryAtPath(path, func(e EvidenceEntry) error {
		if e.EntryID == entryID {
			out = e
			found = true
			return errFound
		}
		return nil
	})
	if err != nil && !errors.Is(err, errFound) {
		return EvidenceEntry{}, false, err
	}
	return out, found, nil
}

// LastHashAtPath returns the last hash from the manifest for chain linking.
// Returns empty string if the store is empty or does not exist yet.
func LastHashAtPath(path string) (string, error) {
	var lastHash string
	err := withStoreLock(path, func() error {
		manifest, err := loadOrInitManifest(path, segmentMaxBytesFromEnv(), false)
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				return nil
			}
			return err
		}
		lastHash = manifest.LastHash
		return nil
	})
	if err != nil {
		return "", err
	}
	return lastHash, nil
}

// ValidateChainAtPath reads all entries and verifies hash chain integrity.
// For each entry it checks that previous_hash links correctly and that the
// stored hash matches a recomputed hash over the entry fields.
func ValidateChainAtPath(root string) error {
	entries, err := ReadAllEntriesAtPath(root)
	if err != nil {
		return fmt.Errorf("validate chain: %w", err)
	}

	for i, entry := range entries {
		// Verify previous_hash link.
		if i == 0 {
			if entry.PreviousHash != "" {
				return &ChainValidationError{
					Index:   i,
					EventID: entry.EntryID,
					Message: "first entry should have empty previous_hash",
				}
			}
		} else {
			if entry.PreviousHash != entries[i-1].Hash {
				return &ChainValidationError{
					Index:   i,
					EventID: entry.EntryID,
					Message: fmt.Sprintf("previous_hash mismatch: got %s, want %s", entry.PreviousHash, entries[i-1].Hash),
				}
			}
		}

		// Recompute hash and verify.
		recomputed, hashErr := computeEntryHash(entry)
		if hashErr != nil {
			return &ChainValidationError{
				Index:   i,
				EventID: entry.EntryID,
				Message: fmt.Sprintf("hash computation failed: %v", hashErr),
			}
		}
		if entry.Hash != recomputed {
			return &ChainValidationError{
				Index:   i,
				EventID: entry.EntryID,
				Message: fmt.Sprintf("hash mismatch: stored %s, computed %s", entry.Hash, recomputed),
			}
		}
	}

	return nil
}
