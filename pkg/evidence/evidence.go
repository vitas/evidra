// Package evidence provides the append-only JSONL evidence store with
// hash-linked chain validation.
package evidence

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
)

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
