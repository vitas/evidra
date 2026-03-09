package telemetry

import (
	"context"
	"fmt"
	"strings"

	"samebits.com/evidra-benchmark/internal/config"
)

// MetricLabels is the bounded-cardinality label set for emitted metrics.
type MetricLabels struct {
	Tool           string `json:"tool"`
	Environment    string `json:"environment"`
	ResultClass    string `json:"result_class"`
	SignalName     string `json:"signal_name"`
	ScoreBand      string `json:"score_band"`
	AssessmentMode string `json:"assessment_mode"`
}

// OperationMetric is a single point emitted by CLI metrics exporters.
type OperationMetric struct {
	Name   string       `json:"name"`
	Labels MetricLabels `json:"labels"`
	Value  float64      `json:"value"`
}

// Transport is a bounded metrics export sink.
type Transport interface {
	Emit(ctx context.Context, metric OperationMetric) error
	Flush(ctx context.Context) error
	Close() error
}

func NewTransport(cfg config.MetricsConfig) (Transport, error) {
	switch cfg.Transport {
	case config.MetricsTransportNone:
		return NewNoop(), nil
	case config.MetricsTransportOTLPHTTP:
		return NewOTLPHTTP(cfg)
	default:
		return nil, fmt.Errorf("unsupported metrics transport %q", cfg.Transport)
	}
}

var allowedTools = map[string]struct{}{
	"terraform":      {},
	"kubectl":        {},
	"helm":           {},
	"ansible":        {},
	"docker":         {},
	"bash":           {},
	"argocd":         {},
	"github_actions": {},
	"ci":             {},
	"other":          {},
}

var allowedEnvironments = map[string]struct{}{
	"production":  {},
	"staging":     {},
	"development": {},
	"unknown":     {},
}

var allowedResultClass = map[string]struct{}{
	"success": {},
	"failure": {},
	"unknown": {},
}

var allowedSignalNames = map[string]struct{}{
	"protocol_violation": {},
	"artifact_drift":     {},
	"retry_loop":         {},
	"blast_radius":       {},
	"new_scope":          {},
	"repair_loop":        {},
	"thrashing":          {},
	"risk_escalation":    {},
	"none":               {},
	"other":              {},
}

var allowedScoreBands = map[string]struct{}{
	"excellent":         {},
	"good":              {},
	"fair":              {},
	"poor":              {},
	"insufficient_data": {},
	"unknown":           {},
}

var allowedAssessmentModes = map[string]struct{}{
	"preview":    {},
	"sufficient": {},
	"unknown":    {},
}

// BoundedLabels normalizes dynamic input to a fixed-cardinality label domain.
func BoundedLabels(in MetricLabels) MetricLabels {
	tool := strings.ToLower(strings.TrimSpace(in.Tool))
	if _, ok := allowedTools[tool]; !ok {
		tool = "other"
	}

	environment := normalizeEnvironment(in.Environment)
	if _, ok := allowedEnvironments[environment]; !ok {
		environment = "unknown"
	}

	resultClass := strings.ToLower(strings.TrimSpace(in.ResultClass))
	if _, ok := allowedResultClass[resultClass]; !ok {
		resultClass = "unknown"
	}

	signalName := strings.ToLower(strings.TrimSpace(in.SignalName))
	if _, ok := allowedSignalNames[signalName]; !ok {
		signalName = "other"
	}

	scoreBand := strings.ToLower(strings.TrimSpace(in.ScoreBand))
	if _, ok := allowedScoreBands[scoreBand]; !ok {
		scoreBand = "unknown"
	}

	assessmentMode := strings.ToLower(strings.TrimSpace(in.AssessmentMode))
	if _, ok := allowedAssessmentModes[assessmentMode]; !ok {
		assessmentMode = "unknown"
	}

	return MetricLabels{
		Tool:           tool,
		Environment:    environment,
		ResultClass:    resultClass,
		SignalName:     signalName,
		ScoreBand:      scoreBand,
		AssessmentMode: assessmentMode,
	}
}

func normalizeEnvironment(raw string) string {
	environment := strings.ToLower(strings.TrimSpace(raw))
	switch environment {
	case "prod":
		return "production"
	case "stage":
		return "staging"
	case "dev", "test", "sandbox":
		return "development"
	case "":
		return "unknown"
	default:
		return environment
	}
}
