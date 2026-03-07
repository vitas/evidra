package telemetry

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"samebits.com/evidra-benchmark/internal/config"
)

func TestOTLPExporterFlushesAtEnd(t *testing.T) {
	t.Parallel()

	var requests int32
	var got struct {
		Metrics []OperationMetric `json:"metrics"`
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&requests, 1)
		defer r.Body.Close()
		if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
			t.Fatalf("decode payload: %v", err)
		}
		w.WriteHeader(http.StatusAccepted)
	}))
	defer server.Close()

	transport, err := NewTransport(config.MetricsConfig{
		Transport:    config.MetricsTransportOTLPHTTP,
		OTLPEndpoint: server.URL,
		Timeout:      2 * time.Second,
	})
	if err != nil {
		t.Fatalf("NewTransport: %v", err)
	}

	err = transport.Emit(context.Background(), OperationMetric{
		Name: "evidra.operation.signal.count",
		Labels: MetricLabels{
			Tool:           "kubectl",
			Environment:    "staging",
			ResultClass:    "success",
			SignalName:     "retry_loop",
			ScoreBand:      "good",
			AssessmentMode: "preview",
		},
		Value: 1,
	})
	if err != nil {
		t.Fatalf("Emit: %v", err)
	}
	if err := transport.Flush(context.Background()); err != nil {
		t.Fatalf("Flush: %v", err)
	}

	if gotReq := atomic.LoadInt32(&requests); gotReq != 1 {
		t.Fatalf("requests=%d want 1", gotReq)
	}
	if len(got.Metrics) != 1 {
		t.Fatalf("metrics len=%d want 1", len(got.Metrics))
	}
	if got.Metrics[0].Labels.Tool != "kubectl" {
		t.Fatalf("tool=%q want kubectl", got.Metrics[0].Labels.Tool)
	}
}
