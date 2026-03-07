package telemetry

import "context"

type noopTransport struct{}

func NewNoop() Transport {
	return noopTransport{}
}

func (noopTransport) Emit(context.Context, OperationMetric) error { return nil }

func (noopTransport) Flush(context.Context) error { return nil }

func (noopTransport) Close() error { return nil }
