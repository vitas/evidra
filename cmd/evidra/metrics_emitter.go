package main

import (
	"context"

	"samebits.com/evidra-benchmark/internal/config"
	"samebits.com/evidra-benchmark/internal/telemetry"
)

func emitOperationMetrics(ctx context.Context, payload operationMetricsPayload) error {
	cfg, err := config.ResolveMetricsConfig("", "", "")
	if err != nil {
		return err
	}

	transport, err := telemetry.NewTransport(cfg)
	if err != nil {
		return err
	}
	defer func() {
		_ = transport.Close()
	}()

	resultClass := "failure"
	if payload.ExitCode == 0 {
		resultClass = "success"
	}

	emittedSignal := false
	for signalName, count := range payload.SignalSummary {
		emittedSignal = true
		if err := transport.Emit(ctx, telemetry.OperationMetric{
			Name: "evidra.operation.signal.count",
			Labels: telemetry.MetricLabels{
				Tool:           payload.Tool,
				Environment:    payload.Environment,
				ResultClass:    resultClass,
				SignalName:     signalName,
				ScoreBand:      payload.ScoreBand,
				AssessmentMode: payload.AssessmentMode,
			},
			Value: float64(count),
		}); err != nil {
			return err
		}
	}
	if !emittedSignal {
		if err := transport.Emit(ctx, telemetry.OperationMetric{
			Name: "evidra.operation.signal.count",
			Labels: telemetry.MetricLabels{
				Tool:           payload.Tool,
				Environment:    payload.Environment,
				ResultClass:    resultClass,
				SignalName:     "none",
				ScoreBand:      payload.ScoreBand,
				AssessmentMode: payload.AssessmentMode,
			},
			Value: 0,
		}); err != nil {
			return err
		}
	}

	if err := transport.Emit(ctx, telemetry.OperationMetric{
		Name: "evidra.operation.duration_ms",
		Labels: telemetry.MetricLabels{
			Tool:           payload.Tool,
			Environment:    payload.Environment,
			ResultClass:    resultClass,
			SignalName:     "none",
			ScoreBand:      payload.ScoreBand,
			AssessmentMode: payload.AssessmentMode,
		},
		Value: float64(payload.DurationMs),
	}); err != nil {
		return err
	}

	return transport.Flush(ctx)
}
