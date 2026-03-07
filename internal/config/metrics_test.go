package config

import (
	"testing"
	"time"
)

func TestResolveMetricsConfigDefaultsToNone(t *testing.T) {
	t.Setenv(metricsTransportEnv, "")
	t.Setenv(metricsOTLPEndpointEnv, "")
	t.Setenv(metricsTimeoutEnv, "")

	cfg, err := ResolveMetricsConfig("", "", "")
	if err != nil {
		t.Fatalf("ResolveMetricsConfig: %v", err)
	}
	if cfg.Transport != MetricsTransportNone {
		t.Fatalf("transport=%q want %q", cfg.Transport, MetricsTransportNone)
	}
	if cfg.Timeout != 3*time.Second {
		t.Fatalf("timeout=%s want 3s", cfg.Timeout)
	}
}

func TestResolveMetricsConfigOTLPFromEnv(t *testing.T) {
	t.Setenv(metricsTransportEnv, string(MetricsTransportOTLPHTTP))
	t.Setenv(metricsOTLPEndpointEnv, "http://127.0.0.1:4318/v1/metrics")
	t.Setenv(metricsTimeoutEnv, "5s")

	cfg, err := ResolveMetricsConfig("", "", "")
	if err != nil {
		t.Fatalf("ResolveMetricsConfig: %v", err)
	}
	if cfg.Transport != MetricsTransportOTLPHTTP {
		t.Fatalf("transport=%q want %q", cfg.Transport, MetricsTransportOTLPHTTP)
	}
	if cfg.OTLPEndpoint != "http://127.0.0.1:4318/v1/metrics" {
		t.Fatalf("endpoint=%q", cfg.OTLPEndpoint)
	}
	if cfg.Timeout != 5*time.Second {
		t.Fatalf("timeout=%s want 5s", cfg.Timeout)
	}
}

func TestResolveMetricsConfigOTLPMissingEndpointFails(t *testing.T) {
	t.Setenv(metricsTransportEnv, "")
	t.Setenv(metricsOTLPEndpointEnv, "")

	_, err := ResolveMetricsConfig(string(MetricsTransportOTLPHTTP), "", "")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestResolveMetricsConfigInvalidTransportFails(t *testing.T) {
	_, err := ResolveMetricsConfig("invalid", "", "")
	if err == nil {
		t.Fatal("expected error")
	}
}
