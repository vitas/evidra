package telemetry

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"samebits.com/evidra-benchmark/internal/config"
)

type otlpHTTPTransport struct {
	endpoint string
	timeout  time.Duration
	client   *http.Client

	mu      sync.Mutex
	metrics []OperationMetric
}

// NewOTLPHTTP creates a transport that pushes metrics via OTLP/HTTP endpoint.
func NewOTLPHTTP(cfg config.MetricsConfig) (Transport, error) {
	endpoint := strings.TrimSpace(cfg.OTLPEndpoint)
	if endpoint == "" {
		return nil, fmt.Errorf("otlp_http endpoint is required")
	}
	if cfg.Timeout <= 0 {
		return nil, fmt.Errorf("otlp_http timeout must be > 0")
	}

	return &otlpHTTPTransport{
		endpoint: endpoint,
		timeout:  cfg.Timeout,
		client:   &http.Client{Timeout: cfg.Timeout},
	}, nil
}

func (t *otlpHTTPTransport) Emit(_ context.Context, metric OperationMetric) error {
	metric.Labels = BoundedLabels(metric.Labels)
	if strings.TrimSpace(metric.Name) == "" {
		metric.Name = "evidra.operation.metric"
	}

	t.mu.Lock()
	defer t.mu.Unlock()
	t.metrics = append(t.metrics, metric)
	return nil
}

func (t *otlpHTTPTransport) Flush(ctx context.Context) error {
	t.mu.Lock()
	batch := append([]OperationMetric(nil), t.metrics...)
	t.metrics = nil
	t.mu.Unlock()

	if len(batch) == 0 {
		return nil
	}

	payload := struct {
		Exporter string            `json:"exporter"`
		SentAt   string            `json:"sent_at"`
		Metrics  []OperationMetric `json:"metrics"`
	}{
		Exporter: "evidra-cli",
		SentAt:   time.Now().UTC().Format(time.RFC3339Nano),
		Metrics:  batch,
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal metrics payload: %w", err)
	}

	flushCtx, cancel := context.WithTimeout(ctx, t.timeout)
	defer cancel()

	req, err := http.NewRequestWithContext(flushCtx, http.MethodPost, t.endpoint, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("build metrics request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := t.client.Do(req)
	if err != nil {
		return fmt.Errorf("send metrics payload: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		snippet, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return fmt.Errorf("metrics endpoint status %d: %s", resp.StatusCode, strings.TrimSpace(string(snippet)))
	}
	return nil
}

func (t *otlpHTTPTransport) Close() error { return nil }
