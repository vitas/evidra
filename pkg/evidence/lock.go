package evidence

import (
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"samebits.com/evidra-benchmark/pkg/evlock"
)

func lockTimeoutFromEnv() time.Duration {
	raw := strings.TrimSpace(os.Getenv(lockTimeoutEnv))
	if raw == "" {
		return time.Duration(defaultLockTimeoutMS) * time.Millisecond
	}
	ms, err := strconv.Atoi(raw)
	if err != nil || ms <= 0 {
		return time.Duration(defaultLockTimeoutMS) * time.Millisecond
	}
	return time.Duration(ms) * time.Millisecond
}

func lockRootForPath(path string) (string, error) {
	mode, resolved, err := detectStoreMode(path)
	if err != nil {
		return "", err
	}
	if mode == "legacy" {
		return filepath.Dir(resolved), nil
	}
	return resolved, nil
}

func withStoreLock(path string, fn func() error) error {
	root, err := lockRootForPath(path)
	if err != nil {
		return err
	}
	lockPath := filepath.Join(root, lockFileName)
	lock, err := evlock.Acquire(lockPath, lockTimeoutFromEnv())
	if err != nil {
		if errors.Is(err, evlock.ErrBusy) {
			return &StoreError{
				Code:    ErrorCodeStoreBusy,
				Message: "Evidence store is busy (another writer is running)",
				Err:     err,
			}
		}
		if errors.Is(err, evlock.ErrNotSupported) {
			return &StoreError{
				Code:    ErrorCodeLockNotSupportedWindows,
				Message: "Evidence locking is not supported on windows in v0.1",
				Err:     err,
			}
		}
		return err
	}
	defer func() { _ = lock.Release() }()
	return fn()
}

func segmentMaxBytesFromEnv() int64 {
	raw := strings.TrimSpace(os.Getenv(segmentMaxBytesEnv))
	if raw == "" {
		return defaultSegmentMaxBytes
	}
	v, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || v <= 0 {
		return defaultSegmentMaxBytes
	}
	return v
}
