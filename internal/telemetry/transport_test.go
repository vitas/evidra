package telemetry

import (
	"context"
	"testing"

	"samebits.com/evidra-benchmark/internal/config"
)

func TestNewTransportNoneReturnsNoop(t *testing.T) {
	t.Parallel()

	transport, err := NewTransport(config.MetricsConfig{Transport: config.MetricsTransportNone})
	if err != nil {
		t.Fatalf("NewTransport: %v", err)
	}
	if err := transport.Emit(context.Background(), OperationMetric{}); err != nil {
		t.Fatalf("Emit: %v", err)
	}
	if err := transport.Flush(context.Background()); err != nil {
		t.Fatalf("Flush: %v", err)
	}
	if err := transport.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
}

func TestMetricsLabelsBoundedCardinality(t *testing.T) {
	t.Parallel()

	labels := BoundedLabels(MetricLabels{
		Tool:           "my-custom-tool-instance-12345",
		Environment:    "dev",
		ResultClass:    "EXIT_7",
		SignalName:     "some_unbound_signal",
		ScoreBand:      "99th",
		AssessmentMode: "draft",
	})

	if labels.Tool != "other" {
		t.Fatalf("tool=%q want other", labels.Tool)
	}
	if labels.Environment != "development" {
		t.Fatalf("environment=%q want development", labels.Environment)
	}
	if labels.ResultClass != "unknown" {
		t.Fatalf("result_class=%q want unknown", labels.ResultClass)
	}
	if labels.SignalName != "other" {
		t.Fatalf("signal_name=%q want other", labels.SignalName)
	}
	if labels.ScoreBand != "unknown" {
		t.Fatalf("score_band=%q want unknown", labels.ScoreBand)
	}
	if labels.AssessmentMode != "unknown" {
		t.Fatalf("assessment_mode=%q want unknown", labels.AssessmentMode)
	}
}
