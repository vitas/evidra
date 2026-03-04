package evidence

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

func appendLegacy(path string, record EvidenceRecord) (EvidenceRecord, error) {
	if record.Timestamp.IsZero() {
		record.Timestamp = time.Now().UTC()
	} else {
		record.Timestamp = record.Timestamp.UTC()
	}

	last, ok, err := readLastRecord(path)
	if err != nil {
		return EvidenceRecord{}, err
	}
	if ok {
		record.PreviousHash = last.Hash
	} else {
		record.PreviousHash = ""
	}

	hash, err := ComputeHash(record)
	if err != nil {
		return EvidenceRecord{}, err
	}
	record.Hash = hash

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return EvidenceRecord{}, fmt.Errorf("create log directory: %w", err)
	}
	if err := appendRecordLine(path, record); err != nil {
		return EvidenceRecord{}, err
	}
	return record, nil
}

func validateLegacyChain(path string) error {
	var prev string
	idx := 0
	policyRef := ""
	err := streamFileRecords(path, func(rec Record, _ int) error {
		if idx == 0 {
			if rec.PreviousHash != "" {
				return &ChainValidationError{Index: idx, EventID: rec.EventID, Message: "non-empty previous_hash in chain head"}
			}
		} else if rec.PreviousHash != prev {
			return &ChainValidationError{Index: idx, EventID: rec.EventID, Message: "previous_hash mismatch"}
		}

		expected, err := ComputeHash(rec)
		if err != nil {
			return fmt.Errorf("compute hash for record %d: %w", idx, err)
		}
		if rec.Hash != expected {
			return &ChainValidationError{Index: idx, EventID: rec.EventID, Message: "hash mismatch"}
		}
		if rec.PolicyRef != "" {
			if policyRef == "" {
				policyRef = rec.PolicyRef
			} else if policyRef != rec.PolicyRef {
				return fmt.Errorf("mixed policy_ref values detected")
			}
		}
		prev = rec.Hash
		idx++
		return nil
	})
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return err
	}
	return nil
}

func metadataFromLegacyFile(path string) (Metadata, error) {
	meta := Metadata{}
	policyRef := ""
	err := streamFileRecords(path, func(rec Record, _ int) error {
		meta.Records++
		meta.LastHash = rec.Hash
		if rec.PolicyRef != "" {
			if policyRef == "" {
				policyRef = rec.PolicyRef
			} else if policyRef != rec.PolicyRef {
				return fmt.Errorf("mixed policy_ref values detected")
			}
		}
		return nil
	})
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return Metadata{}, nil
		}
		return Metadata{}, err
	}
	meta.PolicyRef = policyRef
	return meta, nil
}

func streamLegacyRecords(path string, fn func(Record) error) error {
	return streamFileRecords(path, func(rec Record, _ int) error {
		return fn(rec)
	})
}

func readLastRecord(path string) (EvidenceRecord, bool, error) {
	records, err := readAllRecords(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return EvidenceRecord{}, false, nil
		}
		return EvidenceRecord{}, false, err
	}
	if len(records) == 0 {
		return EvidenceRecord{}, false, nil
	}
	return records[len(records)-1], true, nil
}

func readAllRecords(path string) ([]EvidenceRecord, error) {
	records := make([]EvidenceRecord, 0)
	err := forEachRecordAtPathUnlocked(path, func(rec Record) error {
		records = append(records, rec)
		return nil
	})
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, os.ErrNotExist
		}
		return nil, err
	}
	return records, nil
}
