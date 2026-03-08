package client

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"time"
)

// Config holds API connection settings.
type Config struct {
	URL     string
	APIKey  string
	Timeout time.Duration
}

// Client sends requests to the Evidra API server.
type Client struct {
	config Config
	http   *http.Client
}

// New creates a Client. Does NOT check reachability.
func New(cfg Config) *Client {
	timeout := cfg.Timeout
	if timeout == 0 {
		timeout = 30 * time.Second
	}
	return &Client{
		config: cfg,
		http:   &http.Client{Timeout: timeout},
	}
}

// URL returns the configured API base URL.
func (c *Client) URL() string { return c.config.URL }

// ForwardResponse is the response from POST /v1/evidence/forward.
type ForwardResponse struct {
	ReceiptID string `json:"receipt_id"`
	Status    string `json:"status"`
	Signature string `json:"signature,omitempty"`
}

// Forward sends an evidence entry to POST /v1/evidence/forward.
func (c *Client) Forward(ctx context.Context, entry json.RawMessage) (ForwardResponse, error) {
	var resp ForwardResponse
	if err := c.post(ctx, "/v1/evidence/forward", entry, &resp); err != nil {
		return ForwardResponse{}, err
	}
	return resp, nil
}

// BatchResponse is the response from POST /v1/evidence/batch.
type BatchResponse struct {
	Accepted int      `json:"accepted"`
	Errors   []string `json:"errors,omitempty"`
}

// Batch sends multiple entries to POST /v1/evidence/batch.
func (c *Client) Batch(ctx context.Context, entries []json.RawMessage) (BatchResponse, error) {
	var resp BatchResponse
	body := map[string]interface{}{"entries": entries}
	raw, _ := json.Marshal(body)
	if err := c.post(ctx, "/v1/evidence/batch", raw, &resp); err != nil {
		return BatchResponse{}, err
	}
	return resp, nil
}

// Ping checks API reachability via GET /healthz.
func (c *Client) Ping(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.config.URL+"/healthz", nil)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrUnreachable, err)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return classifyTransportError(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("%w: healthz returned HTTP %d", ErrServerError, resp.StatusCode)
	}
	return nil
}

func (c *Client) post(ctx context.Context, path string, body json.RawMessage, out interface{}) error {
	reqID := newRequestID()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.config.URL+path, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("%w: create request: %v", ErrUnreachable, err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.config.APIKey)
	req.Header.Set("X-Request-ID", reqID)

	resp, err := c.http.Do(req)
	if err != nil {
		return classifyTransportError(err)
	}
	defer resp.Body.Close()

	if err := classifyHTTPStatus(resp); err != nil {
		return err
	}

	respBody, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return fmt.Errorf("%w: read response: %v", ErrServerError, err)
	}
	if err := json.Unmarshal(respBody, out); err != nil {
		return fmt.Errorf("%w: decode response: %v", ErrServerError, err)
	}
	return nil
}

func classifyTransportError(err error) error {
	var netErr net.Error
	if errors.Is(err, context.DeadlineExceeded) ||
		errors.Is(err, context.Canceled) ||
		(errors.As(err, &netErr) && netErr.Timeout()) {
		return fmt.Errorf("%w: %v", ErrUnreachable, err)
	}
	return fmt.Errorf("%w: %v", ErrUnreachable, err)
}

func classifyHTTPStatus(resp *http.Response) error {
	switch {
	case resp.StatusCode >= 200 && resp.StatusCode < 300:
		return nil
	case resp.StatusCode == 401:
		return ErrUnauthorized
	case resp.StatusCode == 403:
		return ErrForbidden
	case resp.StatusCode == 422:
		return ErrInvalidInput
	case resp.StatusCode == 429:
		return ErrRateLimited
	case resp.StatusCode >= 500:
		return fmt.Errorf("%w: HTTP %d", ErrServerError, resp.StatusCode)
	default:
		return fmt.Errorf("unexpected HTTP status: %d", resp.StatusCode)
	}
}

func newRequestID() string {
	var uuid [16]byte
	_, _ = rand.Read(uuid[:])
	uuid[6] = (uuid[6] & 0x0f) | 0x40
	uuid[8] = (uuid[8] & 0x3f) | 0x80
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x",
		uuid[0:4], uuid[4:6], uuid[6:8], uuid[8:10], uuid[10:16])
}
