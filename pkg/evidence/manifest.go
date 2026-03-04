package evidence

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

func ManifestPath(root string) string {
	return filepath.Join(root, manifestFileName)
}

func LoadManifest(path string) (StoreManifest, error) {
	var out StoreManifest
	err := withStoreLock(path, func() error {
		mode, resolved, err := detectStoreMode(path)
		if err != nil {
			return err
		}
		if mode != "segmented" {
			return fmt.Errorf("manifest not available for legacy evidence store")
		}
		out, err = loadOrInitManifest(resolved, segmentMaxBytesFromEnv(), false)
		return err
	})
	if err != nil {
		return StoreManifest{}, err
	}
	return out, nil
}

func loadOrInitManifest(root string, segmentMaxBytes int64, createIfMissing bool) (StoreManifest, error) {
	manifestPath := ManifestPath(root)
	raw, err := os.ReadFile(manifestPath)
	if err == nil {
		var m StoreManifest
		if err := json.Unmarshal(raw, &m); err != nil {
			return StoreManifest{}, fmt.Errorf("parse manifest: %w", err)
		}
		if m.SegmentMaxBytes <= 0 {
			m.SegmentMaxBytes = segmentMaxBytes
		}
		if m.SegmentsDir == "" {
			m.SegmentsDir = segmentsDirName
		}
		if m.CurrentSegment == "" {
			m.CurrentSegment = segmentName(1)
		}
		m.SealedSegments = normalizeSealedSegments(m.SealedSegments)
		return m, nil
	}
	if !errors.Is(err, os.ErrNotExist) {
		return StoreManifest{}, err
	}
	if !createIfMissing {
		return StoreManifest{}, os.ErrNotExist
	}

	now := time.Now().UTC().Format(time.RFC3339)
	m := StoreManifest{
		Format:          "evidra-evidence-manifest-v0.1",
		CreatedAt:       now,
		UpdatedAt:       now,
		SegmentsDir:     segmentsDirName,
		CurrentSegment:  segmentName(1),
		SealedSegments:  []string{},
		SegmentMaxBytes: segmentMaxBytes,
		RecordsTotal:    0,
		LastHash:        "",
		PolicyRef:       "",
		Notes:           "Local segmented evidence store",
	}
	if createIfMissing {
		if err := os.MkdirAll(filepath.Join(root, segmentsDirName), 0o755); err != nil {
			return StoreManifest{}, fmt.Errorf("create segments directory: %w", err)
		}
		if err := os.WriteFile(filepath.Join(root, segmentsDirName, m.CurrentSegment), []byte(""), 0o644); err != nil {
			return StoreManifest{}, fmt.Errorf("create first segment: %w", err)
		}
		if err := writeManifestAtomic(root, m); err != nil {
			return StoreManifest{}, err
		}
	}
	return m, nil
}

func writeManifestAtomic(root string, manifest StoreManifest) error {
	if manifest.Format == "" {
		manifest.Format = "evidra-evidence-manifest-v0.1"
	}
	if manifest.SegmentsDir == "" {
		manifest.SegmentsDir = segmentsDirName
	}
	if manifest.CurrentSegment == "" {
		manifest.CurrentSegment = segmentName(1)
	}
	manifest.SealedSegments = normalizeSealedSegments(manifest.SealedSegments)
	manifest.SealedSegments = removeSegment(manifest.SealedSegments, manifest.CurrentSegment)

	if err := os.MkdirAll(root, 0o755); err != nil {
		return fmt.Errorf("create evidence root: %w", err)
	}
	if err := os.MkdirAll(filepath.Join(root, segmentsDirName), 0o755); err != nil {
		return fmt.Errorf("create segments directory: %w", err)
	}

	b, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal manifest: %w", err)
	}

	manifestPath := ManifestPath(root)
	tmpPath := manifestPath + ".tmp"
	if err := os.WriteFile(tmpPath, append(b, '\n'), 0o644); err != nil {
		return fmt.Errorf("write manifest tmp: %w", err)
	}
	if err := os.Rename(tmpPath, manifestPath); err != nil {
		return fmt.Errorf("rename manifest tmp: %w", err)
	}
	return nil
}
