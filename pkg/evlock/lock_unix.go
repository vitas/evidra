//go:build unix

package evlock

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"syscall"
	"time"
)

var (
	ErrBusy         = errors.New("evidence_store_busy")
	ErrNotSupported = errors.New("evidence_lock_not_supported_on_windows")
)

type Lock struct {
	file *os.File
}

func Acquire(path string, timeout time.Duration) (*Lock, error) {
	if timeout <= 0 {
		timeout = 2 * time.Second
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, fmt.Errorf("create lock directory: %w", err)
	}

	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, fmt.Errorf("open lock file: %w", err)
	}

	deadline := time.Now().Add(timeout)
	for {
		err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
		if err == nil {
			return &Lock{file: f}, nil
		}
		if !errors.Is(err, syscall.EWOULDBLOCK) && !errors.Is(err, syscall.EAGAIN) {
			_ = f.Close()
			return nil, fmt.Errorf("acquire file lock: %w", err)
		}
		if time.Now().After(deadline) {
			_ = f.Close()
			return nil, ErrBusy
		}
		time.Sleep(25 * time.Millisecond)
	}
}

func (l *Lock) Release() error {
	if l == nil || l.file == nil {
		return nil
	}
	unlockErr := syscall.Flock(int(l.file.Fd()), syscall.LOCK_UN)
	closeErr := l.file.Close()
	l.file = nil
	if unlockErr != nil {
		return fmt.Errorf("release file lock: %w", unlockErr)
	}
	if closeErr != nil {
		return fmt.Errorf("close lock file: %w", closeErr)
	}
	return nil
}
