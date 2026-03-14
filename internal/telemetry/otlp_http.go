package telemetry

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetrichttp"
	otelmetric "go.opentelemetry.io/otel/metric"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"

	"samebits.com/evidra/internal/config"
)

type otlpHTTPTransport struct {
	provider *sdkmetric.MeterProvider
	meter    otelmetric.Meter

	mu     sync.Mutex
	gauges map[string]otelmetric.Float64Gauge
}

// NewOTLPHTTP creates a transport that pushes metrics via OTLP/HTTP protobuf.
func NewOTLPHTTP(cfg config.MetricsConfig) (Transport, error) {
	endpoint := strings.TrimSpace(cfg.OTLPEndpoint)
	if endpoint == "" {
		return nil, fmt.Errorf("otlp_http endpoint is required")
	}
	if cfg.Timeout <= 0 {
		return nil, fmt.Errorf("otlp_http timeout must be > 0")
	}

	ctx, cancel := context.WithTimeout(context.Background(), cfg.Timeout)
	defer cancel()

	exporter, err := otlpmetrichttp.New(ctx,
		otlpmetrichttp.WithEndpointURL(endpoint),
		otlpmetrichttp.WithTimeout(cfg.Timeout),
	)
	if err != nil {
		return nil, fmt.Errorf("create otlp exporter: %w", err)
	}

	res, err := resource.New(ctx,
		resource.WithAttributes(
			attribute.String("service.name", "evidra-cli"),
		),
	)
	if err != nil {
		return nil, fmt.Errorf("create otel resource: %w", err)
	}

	provider := sdkmetric.NewMeterProvider(
		sdkmetric.WithResource(res),
		sdkmetric.WithReader(
			sdkmetric.NewPeriodicReader(exporter,
				sdkmetric.WithInterval(1*time.Hour), // manual flush only
			),
		),
	)

	meter := provider.Meter("samebits.com/evidra")

	return &otlpHTTPTransport{
		provider: provider,
		meter:    meter,
		gauges:   make(map[string]otelmetric.Float64Gauge),
	}, nil
}

func (t *otlpHTTPTransport) Emit(ctx context.Context, metric OperationMetric) error {
	metric.Labels = BoundedLabels(metric.Labels)
	if strings.TrimSpace(metric.Name) == "" {
		metric.Name = "evidra.operation.metric"
	}

	gauge, err := t.getOrCreateGauge(metric.Name)
	if err != nil {
		return fmt.Errorf("create gauge %q: %w", metric.Name, err)
	}

	attrs := otelmetric.WithAttributes(
		attribute.String("tool", metric.Labels.Tool),
		attribute.String("environment", metric.Labels.Environment),
		attribute.String("result_class", metric.Labels.ResultClass),
		attribute.String("signal_name", metric.Labels.SignalName),
		attribute.String("score_band", metric.Labels.ScoreBand),
		attribute.String("assessment_mode", metric.Labels.AssessmentMode),
	)

	gauge.Record(ctx, metric.Value, attrs)
	return nil
}

func (t *otlpHTTPTransport) getOrCreateGauge(name string) (otelmetric.Float64Gauge, error) {
	t.mu.Lock()
	defer t.mu.Unlock()

	if g, ok := t.gauges[name]; ok {
		return g, nil
	}

	g, err := t.meter.Float64Gauge(name)
	if err != nil {
		return nil, err
	}
	t.gauges[name] = g
	return g, nil
}

func (t *otlpHTTPTransport) Flush(ctx context.Context) error {
	if err := t.provider.ForceFlush(ctx); err != nil {
		return fmt.Errorf("flush metrics: %w", err)
	}
	return nil
}

func (t *otlpHTTPTransport) Close() error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := t.provider.Shutdown(ctx); err != nil {
		return fmt.Errorf("shutdown meter provider: %w", err)
	}
	return nil
}
