package telemetry

import (
	"compress/gzip"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	colmetricpb "go.opentelemetry.io/proto/otlp/collector/metrics/v1"
	"google.golang.org/protobuf/proto"

	"samebits.com/evidra/internal/config"
)

func TestOTLPHTTP_EmitAndFlush_SendsProtobuf(t *testing.T) {
	t.Parallel()

	var mu sync.Mutex
	var received []*colmetricpb.ExportMetricsServiceRequest

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if ct := r.Header.Get("Content-Type"); ct != "application/x-protobuf" {
			t.Errorf("Content-Type=%q want application/x-protobuf", ct)
		}

		var body io.Reader = r.Body
		if r.Header.Get("Content-Encoding") == "gzip" {
			gz, err := gzip.NewReader(r.Body)
			if err != nil {
				t.Errorf("gzip reader: %v", err)
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			defer func() { _ = gz.Close() }()
			body = gz
		}

		data, err := io.ReadAll(body)
		if err != nil {
			t.Errorf("read body: %v", err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		defer func() { _ = r.Body.Close() }()

		var req colmetricpb.ExportMetricsServiceRequest
		if err := proto.Unmarshal(data, &req); err != nil {
			t.Errorf("unmarshal protobuf: %v", err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}

		mu.Lock()
		received = append(received, &req)
		mu.Unlock()

		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	transport, err := NewOTLPHTTP(config.MetricsConfig{
		OTLPEndpoint: server.URL,
		Timeout:      5 * time.Second,
	})
	if err != nil {
		t.Fatalf("NewOTLPHTTP: %v", err)
	}

	ctx := context.Background()
	err = transport.Emit(ctx, OperationMetric{
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

	if err := transport.Flush(ctx); err != nil {
		t.Fatalf("Flush: %v", err)
	}
	if err := transport.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	mu.Lock()
	defer mu.Unlock()

	if len(received) == 0 {
		t.Fatal("no OTLP requests received")
	}

	req := received[0]
	if len(req.ResourceMetrics) == 0 {
		t.Fatal("no ResourceMetrics in request")
	}
	rm := req.ResourceMetrics[0]
	if len(rm.ScopeMetrics) == 0 {
		t.Fatal("no ScopeMetrics in ResourceMetrics")
	}
}

func TestOTLPHTTP_MissingEndpoint_Error(t *testing.T) {
	t.Parallel()

	_, err := NewOTLPHTTP(config.MetricsConfig{
		OTLPEndpoint: "",
		Timeout:      2 * time.Second,
	})
	if err == nil {
		t.Fatal("expected error for empty endpoint")
	}
}

func TestOTLPHTTP_InvalidTimeout_Error(t *testing.T) {
	t.Parallel()

	_, err := NewOTLPHTTP(config.MetricsConfig{
		OTLPEndpoint: "http://localhost:4318",
		Timeout:      0,
	})
	if err == nil {
		t.Fatal("expected error for zero timeout")
	}
}
