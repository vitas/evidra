package evidence

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

func ForwarderStatePath(root string) string {
	return filepath.Join(root, forwarderStateFileName)
}

func LoadForwarderState(root string) (ForwarderState, bool, error) {
	var state ForwarderState
	found := false
	err := withStoreLock(root, func() error {
		mode, resolved, err := detectStoreMode(root)
		if err != nil {
			return err
		}
		if mode != "segmented" {
			return fmt.Errorf("cursor not supported for legacy evidence")
		}

		raw, err := os.ReadFile(ForwarderStatePath(resolved))
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				return nil
			}
			return err
		}

		if err := json.Unmarshal(raw, &state); err != nil {
			return fmt.Errorf("parse forwarder state: %w", err)
		}
		found = true
		return nil
	})
	if err != nil {
		return ForwarderState{}, false, err
	}
	return state, found, nil
}

func SaveForwarderState(root string, state ForwarderState) error {
	return withStoreLock(root, func() error {
		mode, resolved, err := detectStoreMode(root)
		if err != nil {
			return err
		}
		if mode != "segmented" {
			return fmt.Errorf("cursor not supported for legacy evidence")
		}

		if state.Format == "" {
			state.Format = "evidra-forwarder-state-v0.1"
		}
		if state.Destination.Type == "" {
			state.Destination.Type = "none"
		}

		if err := os.MkdirAll(resolved, 0o755); err != nil {
			return fmt.Errorf("create evidence root: %w", err)
		}

		b, err := json.MarshalIndent(state, "", "  ")
		if err != nil {
			return fmt.Errorf("marshal forwarder state: %w", err)
		}

		path := ForwarderStatePath(resolved)
		tmp := path + ".tmp"
		if err := os.WriteFile(tmp, append(b, '\n'), 0o644); err != nil {
			return fmt.Errorf("write forwarder state tmp: %w", err)
		}
		if err := os.Rename(tmp, path); err != nil {
			return fmt.Errorf("rename forwarder state tmp: %w", err)
		}
		return nil
	})
}

func ResolveCursorRecord(root, segment string, line int) (Record, error) {
	var out Record
	err := withStoreLock(root, func() error {
		var err error
		out, err = resolveCursorRecordUnlocked(root, segment, line)
		return err
	})
	if err != nil {
		return Record{}, err
	}
	return out, nil
}

func resolveCursorRecordUnlocked(root, segment string, line int) (Record, error) {
	if line < 0 {
		return Record{}, ErrCursorLineOutOfRange
	}

	mode, resolved, err := detectStoreMode(root)
	if err != nil {
		return Record{}, err
	}
	if mode != "segmented" {
		return Record{}, fmt.Errorf("cursor not supported for legacy evidence")
	}

	segPath := filepath.Join(resolved, segmentsDirName, segment)
	if _, err := os.Stat(segPath); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return Record{}, ErrCursorSegmentNotFound
		}
		return Record{}, err
	}

	current := 0
	var out Record
	found := false
	err = streamFileRecords(segPath, func(rec Record, _ int) error {
		if current == line {
			out = rec
			found = true
			return errCursorResolved
		}
		current++
		return nil
	})
	if err != nil {
		if errors.Is(err, errCursorResolved) {
			return out, nil
		}
		return Record{}, err
	}
	if !found {
		return Record{}, ErrCursorLineOutOfRange
	}
	return out, nil
}
