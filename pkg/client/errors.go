// Package client provides an HTTP client for the Evidra API.
package client

import "errors"

// Sentinel errors returned by Client methods.
var (
	ErrUnreachable  = errors.New("api_unreachable")
	ErrUnauthorized = errors.New("unauthorized")
	ErrForbidden    = errors.New("forbidden")
	ErrRateLimited  = errors.New("rate_limited")
	ErrServerError  = errors.New("server_error")
	ErrInvalidInput = errors.New("invalid_input")
)

// IsReachabilityError returns true for errors that can trigger fallback-offline.
// Auth errors (401/403), validation (422), and rate limit (429) always fail immediately.
func IsReachabilityError(err error) bool {
	return errors.Is(err, ErrUnreachable) || errors.Is(err, ErrServerError)
}
