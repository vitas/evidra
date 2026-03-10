package main

import (
	"context"
	"strings"

	"samebits.com/evidra-benchmark/internal/lifecycle"
	"samebits.com/evidra-benchmark/pkg/evidence"
)

type operationLifecycleService interface {
	Prescribe(ctx context.Context, input lifecycle.PrescribeInput) (lifecycle.PrescribeOutput, error)
	Report(ctx context.Context, input lifecycle.ReportInput) (lifecycle.ReportOutput, error)
}

// OperationRequest captures the inputs required to process one operation through
// the canonical prescribe->report lifecycle path.
type OperationRequest struct {
	PrescribeInput lifecycle.PrescribeInput
	ExitCode       int
	ArtifactDigest string
	ReportActor    evidence.Actor
	ExternalRefs   []evidence.ExternalRef
	SessionID      string
	OperationID    string
	SpanID         string
	ParentSpanID   string
}

// OperationResult contains the paired lifecycle outputs for one operation.
type OperationResult struct {
	PrescribeOutput lifecycle.PrescribeOutput
	ReportOutput    lifecycle.ReportOutput
}

// OperationProcessor ensures all adapters (`run`, `record`) use one lifecycle path.
type OperationProcessor struct {
	service operationLifecycleService
}

func NewOperationProcessor(service operationLifecycleService) *OperationProcessor {
	return &OperationProcessor{service: service}
}

func (p *OperationProcessor) Process(ctx context.Context, req OperationRequest) (OperationResult, error) {
	prescOut, err := p.service.Prescribe(ctx, req.PrescribeInput)
	if err != nil {
		return OperationResult{}, err
	}

	reportActor := req.ReportActor
	if reportActor == (evidence.Actor{}) {
		reportActor = req.PrescribeInput.Actor
	}

	artifactDigest := strings.TrimSpace(req.ArtifactDigest)
	if artifactDigest == "" {
		artifactDigest = prescOut.ArtifactDigest
	}

	reportOut, err := p.service.Report(ctx, lifecycle.ReportInput{
		PrescriptionID: prescOut.PrescriptionID,
		Verdict:        evidence.VerdictFromExitCode(req.ExitCode),
		ExitCode:       intPtr(req.ExitCode),
		ArtifactDigest: artifactDigest,
		Actor:          reportActor,
		ExternalRefs:   req.ExternalRefs,
		SessionID:      req.SessionID,
		OperationID:    req.OperationID,
		SpanID:         req.SpanID,
		ParentSpanID:   req.ParentSpanID,
	})
	if err != nil {
		return OperationResult{}, err
	}

	return OperationResult{
		PrescribeOutput: prescOut,
		ReportOutput:    reportOut,
	}, nil
}

func intPtr(v int) *int {
	return &v
}
