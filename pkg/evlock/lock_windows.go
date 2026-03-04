//go:build windows

package evlock

import (
	"errors"
	"time"
)

var (
	ErrBusy         = errors.New("evidence_store_busy")
	ErrNotSupported = errors.New("evidence_lock_not_supported_on_windows")
)

type Lock struct{}

func Acquire(_ string, _ time.Duration) (*Lock, error) {
	return nil, ErrNotSupported
}

func (l *Lock) Release() error {
	return nil
}
