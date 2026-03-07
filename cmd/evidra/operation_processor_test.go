package main

import (
	"context"
	"errors"
	"testing"

	"samebits.com/evidra-benchmark/internal/lifecycle"
	"samebits.com/evidra-benchmark/pkg/evidence"
)

type spyLifecycleService struct {
	prescribeCalls int
	reportCalls    int

	prescribeInput lifecycle.PrescribeInput
	reportInput    lifecycle.ReportInput

	prescribeOut lifecycle.PrescribeOutput
	reportOut    lifecycle.ReportOutput

	prescribeErr error
	reportErr    error
}

func (s *spyLifecycleService) Prescribe(_ context.Context, in lifecycle.PrescribeInput) (lifecycle.PrescribeOutput, error) {
	s.prescribeCalls++
	s.prescribeInput = in
	if s.prescribeErr != nil {
		return lifecycle.PrescribeOutput{}, s.prescribeErr
	}
	return s.prescribeOut, nil
}

func (s *spyLifecycleService) Report(_ context.Context, in lifecycle.ReportInput) (lifecycle.ReportOutput, error) {
	s.reportCalls++
	s.reportInput = in
	if s.reportErr != nil {
		return lifecycle.ReportOutput{}, s.reportErr
	}
	return s.reportOut, nil
}

func TestProcessOperationUsesSingleLifecyclePath(t *testing.T) {
	t.Parallel()

	spy := &spyLifecycleService{
		prescribeOut: lifecycle.PrescribeOutput{
			PrescriptionID: "presc-1",
			ArtifactDigest: "sha256:abc",
		},
		reportOut: lifecycle.ReportOutput{ReportID: "rep-1"},
	}

	processor := NewOperationProcessor(spy)
	result, err := processor.Process(context.Background(), OperationRequest{
		PrescribeInput: lifecycle.PrescribeInput{
			Actor:       evidence.Actor{Type: "ci", ID: "pipeline-1"},
			Tool:        "kubectl",
			Operation:   "apply",
			RawArtifact: []byte("kind: ConfigMap"),
		},
		ExitCode: 1,
	})
	if err != nil {
		t.Fatalf("Process() error = %v", err)
	}
	if spy.prescribeCalls != 1 {
		t.Fatalf("Prescribe calls = %d, want 1", spy.prescribeCalls)
	}
	if spy.reportCalls != 1 {
		t.Fatalf("Report calls = %d, want 1", spy.reportCalls)
	}
	if spy.reportInput.PrescriptionID != "presc-1" {
		t.Fatalf("Report prescription_id = %q, want presc-1", spy.reportInput.PrescriptionID)
	}
	if spy.reportInput.ArtifactDigest != "sha256:abc" {
		t.Fatalf("Report artifact_digest = %q, want prescribe digest", spy.reportInput.ArtifactDigest)
	}
	if result.PrescribeOutput.PrescriptionID != "presc-1" {
		t.Fatalf("result prescription_id = %q, want presc-1", result.PrescribeOutput.PrescriptionID)
	}
	if result.ReportOutput.ReportID != "rep-1" {
		t.Fatalf("result report_id = %q, want rep-1", result.ReportOutput.ReportID)
	}
}

func TestProcessOperationReturnsPrescribeError(t *testing.T) {
	t.Parallel()

	spy := &spyLifecycleService{prescribeErr: errors.New("boom")}
	processor := NewOperationProcessor(spy)
	_, err := processor.Process(context.Background(), OperationRequest{})
	if err == nil {
		t.Fatal("expected error")
	}
	if spy.reportCalls != 0 {
		t.Fatalf("Report calls = %d, want 0", spy.reportCalls)
	}
}
