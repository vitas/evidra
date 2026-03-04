package evidence

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"
)

func SegmentFiles(root string) ([]string, error) {
	var files []string
	err := withStoreLock(root, func() error {
		mode, resolved, err := detectStoreMode(root)
		if err != nil {
			return err
		}
		if mode != "segmented" {
			return fmt.Errorf("segments not available for legacy evidence store")
		}
		_, names, err := orderedSegmentNames(resolved)
		if err != nil {
			return err
		}
		files = make([]string, 0, len(names))
		for _, n := range names {
			files = append(files, filepath.Join(resolved, segmentsDirName, n))
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return files, nil
}

func appendSegmented(root string, record EvidenceRecord) (EvidenceRecord, error) {
	maxBytes := segmentMaxBytesFromEnv()
	manifest, err := loadOrInitManifest(root, maxBytes, true)
	if err != nil {
		return EvidenceRecord{}, err
	}
	if manifest.SegmentMaxBytes <= 0 {
		manifest.SegmentMaxBytes = maxBytes
	}
	if manifest.CurrentSegment == "" {
		manifest.CurrentSegment = segmentName(1)
	}
	manifest.SealedSegments = normalizeSealedSegments(manifest.SealedSegments)

	if record.Timestamp.IsZero() {
		record.Timestamp = time.Now().UTC()
	} else {
		record.Timestamp = record.Timestamp.UTC()
	}
	record.PreviousHash = manifest.LastHash

	hash, err := ComputeHash(record)
	if err != nil {
		return EvidenceRecord{}, err
	}
	record.Hash = hash

	segPath := filepath.Join(root, segmentsDirName, manifest.CurrentSegment)
	if err := os.MkdirAll(filepath.Dir(segPath), 0o755); err != nil {
		return EvidenceRecord{}, fmt.Errorf("create segments directory: %w", err)
	}
	if err := appendRecordLine(segPath, record); err != nil {
		return EvidenceRecord{}, err
	}

	manifest.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
	manifest.RecordsTotal++
	manifest.LastHash = record.Hash
	if manifest.PolicyRef == "" && record.PolicyRef != "" {
		manifest.PolicyRef = record.PolicyRef
	} else if manifest.PolicyRef != "" && record.PolicyRef != "" && manifest.PolicyRef != record.PolicyRef {
		manifest.PolicyRef = ""
	}

	info, err := os.Stat(segPath)
	if err == nil && info.Size() > manifest.SegmentMaxBytes {
		manifest.SealedSegments = append(manifest.SealedSegments, manifest.CurrentSegment)
		manifest.SealedSegments = normalizeSealedSegments(manifest.SealedSegments)
		_, names, listErr := orderedSegmentNames(root)
		if listErr != nil {
			return EvidenceRecord{}, listErr
		}
		next := 1
		if len(names) > 0 {
			lastIndex, parseErr := parseSegmentIndex(names[len(names)-1])
			if parseErr != nil {
				return EvidenceRecord{}, parseErr
			}
			next = lastIndex + 1
		}
		manifest.CurrentSegment = segmentName(next)
		manifest.SealedSegments = removeSegment(manifest.SealedSegments, manifest.CurrentSegment)
		nextPath := filepath.Join(root, segmentsDirName, manifest.CurrentSegment)
		if _, err := os.Stat(nextPath); errors.Is(err, os.ErrNotExist) {
			if err := os.WriteFile(nextPath, []byte(""), 0o644); err != nil {
				return EvidenceRecord{}, fmt.Errorf("create next segment: %w", err)
			}
		}
	}

	if err := writeManifestAtomic(root, manifest); err != nil {
		return EvidenceRecord{}, err
	}
	return record, nil
}

func validateSegmentedChain(root string) error {
	manifest, err := loadOrInitManifest(root, segmentMaxBytesFromEnv(), false)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			if _, statErr := os.Stat(root); statErr == nil {
				return fmt.Errorf("manifest not found")
			}
			return nil
		}
		return err
	}

	_, names, err := orderedSegmentNames(root)
	if err != nil {
		return err
	}
	if err := validateManifestSealedInvariants(root, manifest); err != nil {
		return err
	}
	if len(names) == 0 {
		if manifest.RecordsTotal != 0 || manifest.LastHash != "" {
			return fmt.Errorf("manifest indicates records but no segments exist")
		}
		return nil
	}

	var prev string
	total := 0
	lastHash := ""
	policyRef := ""

	for _, name := range names {
		segPath := filepath.Join(root, segmentsDirName, name)
		err := streamFileRecords(segPath, func(rec Record, _ int) error {
			if total == 0 {
				if rec.PreviousHash != "" {
					return &ChainValidationError{Index: total, EventID: rec.EventID, Message: "non-empty previous_hash in chain head"}
				}
			} else if rec.PreviousHash != prev {
				return &ChainValidationError{Index: total, EventID: rec.EventID, Message: "previous_hash mismatch"}
			}

			expected, err := ComputeHash(rec)
			if err != nil {
				return fmt.Errorf("compute hash for record %d: %w", total, err)
			}
			if rec.Hash != expected {
				return &ChainValidationError{Index: total, EventID: rec.EventID, Message: "hash mismatch"}
			}

			if rec.PolicyRef != "" {
				if policyRef == "" {
					policyRef = rec.PolicyRef
				} else if policyRef != rec.PolicyRef {
					return fmt.Errorf("mixed policy_ref values detected")
				}
			}

			prev = rec.Hash
			lastHash = rec.Hash
			total++
			return nil
		})
		if err != nil {
			return err
		}
	}

	if total != manifest.RecordsTotal {
		return fmt.Errorf("manifest records_total mismatch")
	}
	if lastHash != manifest.LastHash {
		return fmt.Errorf("manifest last_hash mismatch")
	}
	if manifest.CurrentSegment != names[len(names)-1] {
		return fmt.Errorf("manifest current_segment mismatch")
	}
	if manifest.PolicyRef != "" && policyRef != "" && manifest.PolicyRef != policyRef {
		return fmt.Errorf("manifest policy_ref mismatch")
	}

	return nil
}

func streamSegmentedRecords(root string, fn func(Record) error) error {
	_, names, err := orderedSegmentNames(root)
	if err != nil {
		return err
	}
	for _, name := range names {
		segPath := filepath.Join(root, segmentsDirName, name)
		err := streamFileRecords(segPath, func(rec Record, _ int) error {
			return fn(rec)
		})
		if err != nil {
			return err
		}
	}
	return nil
}

func orderedSegmentNames(root string) ([]int, []string, error) {
	segDir := filepath.Join(root, segmentsDirName)
	matches, err := filepath.Glob(filepath.Join(segDir, "evidence-*.jsonl"))
	if err != nil {
		return nil, nil, err
	}
	if len(matches) == 0 {
		return nil, nil, nil
	}

	names := make([]string, 0, len(matches))
	indices := make([]int, 0, len(matches))
	for _, m := range matches {
		name := filepath.Base(m)
		idx, err := parseSegmentIndex(name)
		if err != nil {
			return nil, nil, err
		}
		names = append(names, name)
		indices = append(indices, idx)
	}

	sort.SliceStable(names, func(i, j int) bool { return names[i] < names[j] })
	sort.Ints(indices)

	for i, idx := range indices {
		expected := i + 1
		if idx != expected {
			return nil, nil, fmt.Errorf("missing segment in sequence: expected %s", segmentName(expected))
		}
	}

	for i, name := range names {
		expected := segmentName(i + 1)
		if name != expected {
			return nil, nil, fmt.Errorf("unexpected segment name: %s", name)
		}
	}

	return indices, names, nil
}

func parseSegmentIndex(name string) (int, error) {
	var idx int
	n, err := fmt.Sscanf(name, "evidence-%06d.jsonl", &idx)
	if err != nil || n != 1 || idx <= 0 {
		return 0, fmt.Errorf("invalid segment filename: %s", name)
	}
	if name != segmentName(idx) {
		return 0, fmt.Errorf("invalid segment filename: %s", name)
	}
	return idx, nil
}

func segmentName(idx int) string {
	return fmt.Sprintf("evidence-%06d.jsonl", idx)
}

func validateManifestSealedInvariants(root string, manifest StoreManifest) error {
	if manifest.CurrentSegment == "" {
		return fmt.Errorf("manifest current_segment is empty")
	}
	if containsSegment(manifest.SealedSegments, manifest.CurrentSegment) {
		return fmt.Errorf("manifest corruption: current_segment is listed in sealed_segments")
	}

	expected := normalizeSealedSegments(manifest.SealedSegments)
	if len(expected) != len(manifest.SealedSegments) {
		return fmt.Errorf("manifest sealed_segments must be unique and ordered")
	}
	for i := range expected {
		if expected[i] != manifest.SealedSegments[i] {
			return fmt.Errorf("manifest sealed_segments must be unique and ordered")
		}
	}

	for _, sealed := range manifest.SealedSegments {
		segPath := filepath.Join(root, segmentsDirName, sealed)
		if _, err := os.Stat(segPath); err != nil {
			if errors.Is(err, os.ErrNotExist) {
				return fmt.Errorf("sealed segment missing: %s", sealed)
			}
			return err
		}
	}
	return nil
}

func normalizeSealedSegments(in []string) []string {
	if len(in) == 0 {
		return []string{}
	}
	out := make([]string, 0, len(in))
	seen := make(map[string]struct{}, len(in))
	for _, s := range in {
		if s == "" {
			continue
		}
		if _, ok := seen[s]; ok {
			continue
		}
		seen[s] = struct{}{}
		out = append(out, s)
	}
	sort.Strings(out)
	return out
}

func removeSegment(in []string, segment string) []string {
	if len(in) == 0 {
		return in
	}
	out := make([]string, 0, len(in))
	for _, s := range in {
		if s == segment {
			continue
		}
		out = append(out, s)
	}
	return out
}

func containsSegment(in []string, segment string) bool {
	for _, s := range in {
		if s == segment {
			return true
		}
	}
	return false
}
