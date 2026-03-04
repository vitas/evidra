// Package evidence provides the append-only JSONL evidence store with
// hash-linked chain validation.
package evidence

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func ValidateChainAtPath(path string) error {
	return validateChainAtPath(path)
}

func MetadataAtPath(path string) (Metadata, error) {
	var out Metadata
	err := withStoreLock(path, func() error {
		var err error
		out, err = metadataAtPathUnlocked(path)
		return err
	})
	if err != nil {
		return Metadata{}, err
	}
	return out, nil
}

func metadataAtPathUnlocked(path string) (Metadata, error) {
	mode, resolved, err := detectStoreMode(path)
	if err != nil {
		return Metadata{}, err
	}

	switch mode {
	case "segmented":
		m, err := loadOrInitManifest(resolved, segmentMaxBytesFromEnv(), false)
		if err != nil {
			return Metadata{}, err
		}
		return Metadata{Records: m.RecordsTotal, LastHash: m.LastHash, PolicyRef: m.PolicyRef}, nil
	case "legacy":
		return metadataFromLegacyFile(resolved)
	default:
		return Metadata{}, fmt.Errorf("unsupported evidence store mode")
	}
}

func ReadAllAtPath(path string) ([]EvidenceRecord, error) {
	records := make([]EvidenceRecord, 0)
	err := ForEachRecordAtPath(path, func(rec Record) error {
		records = append(records, rec)
		return nil
	})
	if err != nil {
		return nil, err
	}
	return records, nil
}

func ForEachRecordAtPath(path string, fn func(Record) error) error {
	return withStoreLock(path, func() error {
		return forEachRecordAtPathUnlocked(path, fn)
	})
}

func forEachRecordAtPathUnlocked(path string, fn func(Record) error) error {
	mode, resolved, err := detectStoreMode(path)
	if err != nil {
		return err
	}

	switch mode {
	case "segmented":
		return streamSegmentedRecords(resolved, fn)
	case "legacy":
		return streamLegacyRecords(resolved, fn)
	default:
		return fmt.Errorf("unsupported evidence store mode")
	}
}

func FindByEventID(path string, eventID string) (Record, bool, error) {
	var out Record
	found := false
	err := withStoreLock(path, func() error {
		if err := validateChainAtPathUnlocked(path); err != nil {
			return fmt.Errorf("%w: %v", ErrChainInvalid, err)
		}

		errFound := errors.New("record_found")
		err := forEachRecordAtPathUnlocked(path, func(rec Record) error {
			if rec.EventID == eventID {
				out = rec
				found = true
				return errFound
			}
			return nil
		})
		if err != nil && !errors.Is(err, errFound) {
			return err
		}
		return nil
	})
	if err != nil {
		return Record{}, false, err
	}
	return out, found, nil
}

func StoreFormatAtPath(path string) (string, error) {
	mode, _, err := detectStoreMode(path)
	if err != nil {
		return "", err
	}
	return mode, nil
}

func AppendAtPath(path string, record EvidenceRecord) (EvidenceRecord, error) {
	var out EvidenceRecord
	err := withStoreLock(path, func() error {
		var err error
		out, err = appendAtPathUnlocked(path, record)
		return err
	})
	if err != nil {
		return EvidenceRecord{}, err
	}
	return out, nil
}

func appendAtPathUnlocked(path string, record EvidenceRecord) (EvidenceRecord, error) {
	mode, resolved, err := detectStoreMode(path)
	if err != nil {
		return EvidenceRecord{}, err
	}

	switch mode {
	case "segmented":
		return appendSegmented(resolved, record)
	case "legacy":
		return appendLegacy(resolved, record)
	default:
		return EvidenceRecord{}, fmt.Errorf("unsupported evidence store mode")
	}
}

func validateChainAtPath(path string) error {
	return withStoreLock(path, func() error {
		return validateChainAtPathUnlocked(path)
	})
}

func validateChainAtPathUnlocked(path string) error {
	mode, resolved, err := detectStoreMode(path)
	if err != nil {
		return err
	}

	switch mode {
	case "segmented":
		return validateSegmentedChain(resolved)
	case "legacy":
		return validateLegacyChain(resolved)
	default:
		return fmt.Errorf("unsupported evidence store mode")
	}
}

func detectStoreMode(path string) (string, string, error) {
	clean := filepath.Clean(path)
	info, err := os.Stat(clean)
	if err == nil {
		if info.IsDir() {
			return "segmented", clean, nil
		}
		return "legacy", clean, nil
	}
	if !errors.Is(err, os.ErrNotExist) {
		return "", "", err
	}

	ext := strings.ToLower(filepath.Ext(clean))
	if ext == ".log" || ext == ".jsonl" {
		return "legacy", clean, nil
	}
	return "segmented", clean, nil
}
