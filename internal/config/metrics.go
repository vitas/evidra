package config

import (
	"fmt"
	"os"
	"strings"
	"time"
)

const (
	metricsTransportEnv    = "EVIDRA_METRICS_TRANSPORT"
	metricsOTLPEndpointEnv = "EVIDRA_METRICS_OTLP_ENDPOINT"
	metricsTimeoutEnv      = "EVIDRA_METRICS_TIMEOUT"
)

// MetricsTransport selects the CLI metrics export backend.
type MetricsTransport string

const (
	MetricsTransportNone     MetricsTransport = "none"
	MetricsTransportOTLPHTTP MetricsTransport = "otlp_http"
)

// MetricsConfig configures CLI metrics export.
type MetricsConfig struct {
	Transport    MetricsTransport
	OTLPEndpoint string
	Timeout      time.Duration
}

// ResolveMetricsConfig resolves metrics config from explicit values, then env, then defaults.
func ResolveMetricsConfig(explicitTransport, explicitEndpoint, explicitTimeout string) (MetricsConfig, error) {
	transportRaw := strings.TrimSpace(explicitTransport)
	if transportRaw == "" {
		transportRaw = strings.TrimSpace(os.Getenv(metricsTransportEnv))
	}
	if transportRaw == "" {
		transportRaw = string(MetricsTransportNone)
	}

	var transport MetricsTransport
	switch strings.ToLower(transportRaw) {
	case string(MetricsTransportNone):
		transport = MetricsTransportNone
	case string(MetricsTransportOTLPHTTP):
		transport = MetricsTransportOTLPHTTP
	default:
		return MetricsConfig{}, fmt.Errorf("invalid metrics transport %q (expected none|otlp_http)", transportRaw)
	}

	endpoint := strings.TrimSpace(explicitEndpoint)
	if endpoint == "" {
		endpoint = strings.TrimSpace(os.Getenv(metricsOTLPEndpointEnv))
	}
	if transport == MetricsTransportOTLPHTTP && endpoint == "" {
		return MetricsConfig{}, fmt.Errorf("metrics endpoint required for transport otlp_http")
	}

	timeoutRaw := strings.TrimSpace(explicitTimeout)
	if timeoutRaw == "" {
		timeoutRaw = strings.TrimSpace(os.Getenv(metricsTimeoutEnv))
	}
	timeout := 3 * time.Second
	if timeoutRaw != "" {
		parsed, err := time.ParseDuration(timeoutRaw)
		if err != nil {
			return MetricsConfig{}, fmt.Errorf("invalid metrics timeout %q: %w", timeoutRaw, err)
		}
		if parsed <= 0 {
			return MetricsConfig{}, fmt.Errorf("metrics timeout must be > 0")
		}
		timeout = parsed
	}

	return MetricsConfig{
		Transport:    transport,
		OTLPEndpoint: endpoint,
		Timeout:      timeout,
	}, nil
}
