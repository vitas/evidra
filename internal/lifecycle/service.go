package lifecycle

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/oklog/ulid/v2"

	"samebits.com/evidra-benchmark/internal/canon"
	"samebits.com/evidra-benchmark/internal/risk"
	"samebits.com/evidra-benchmark/pkg/evidence"
	"samebits.com/evidra-benchmark/pkg/version"
)

// Prescribe canonicalizes an operation intent and writes a prescription entry.
func (s *Service) Prescribe(_ context.Context, input PrescribeInput) (PrescribeOutput, error) {
	if err := requiredSigner(s.signer); err != nil {
		return PrescribeOutput{}, err
	}

	tool := normalizeToken(input.Tool)
	operation := normalizeToken(input.Operation)
	environment := strings.TrimSpace(input.Environment)
	sessionID := strings.TrimSpace(input.SessionID)
	if sessionID == "" {
		sessionID = evidence.GenerateSessionID()
	}
	traceID := strings.TrimSpace(input.TraceID)
	if traceID == "" {
		traceID = evidence.GenerateTraceID()
	}
	actor := normalizeActor(input.Actor)

	var cr canon.CanonResult
	if input.CanonicalAction != nil {
		preCanon := normalizeCanonicalAction(*input.CanonicalAction, tool, operation)
		actionJSON, err := json.Marshal(preCanon)
		if err != nil {
			return PrescribeOutput{}, wrapError(ErrCodeInternal, "failed to marshal canonical action", err)
		}
		cr = canon.CanonResult{
			ArtifactDigest:  canon.SHA256Hex(input.RawArtifact),
			IntentDigest:    canon.ComputeIntentDigest(preCanon),
			CanonicalAction: preCanon,
			CanonVersion:    "external/v1",
			RawAction:       actionJSON,
		}
	} else {
		cr = canon.Canonicalize(tool, operation, environment, input.RawArtifact)
		if cr.ParseError != nil {
			s.writeCanonicalizationFailure(actor, cr, sessionID, traceID, strings.TrimSpace(input.OperationID), input.Attempt)
			return PrescribeOutput{}, wrapError(ErrCodeParseError, cr.ParseError.Error(), cr.ParseError)
		}
	}

	riskTags := risk.RunAll(cr.CanonicalAction, input.RawArtifact)
	riskLevel := risk.ElevateRiskLevel(
		risk.RiskLevel(cr.CanonicalAction.OperationClass, cr.CanonicalAction.ScopeClass),
		riskTags,
	)

	retryCount := 0
	if s.retryTracker != nil {
		retryCount = s.retryTracker.Record(cr.IntentDigest, cr.CanonicalAction.ResourceShapeHash)
	}

	canonSource := "adapter"
	if input.CanonicalAction != nil {
		canonSource = "external"
	}

	prescPayload := evidence.PrescriptionPayload{
		CanonicalAction: cr.RawAction,
		RiskLevel:       riskLevel,
		RiskTags:        riskTags,
		TTLMs:           evidence.DefaultTTLMs,
		CanonSource:     canonSource,
	}
	payloadJSON, err := json.Marshal(prescPayload)
	if err != nil {
		return PrescribeOutput{}, wrapError(ErrCodeInternal, "failed to marshal prescription payload", err)
	}

	lastHash, err := s.lastHash()
	if err != nil {
		return PrescribeOutput{}, err
	}

	entry, err := evidence.BuildEntry(evidence.EntryBuildParams{
		Type:            evidence.EntryTypePrescribe,
		SessionID:       sessionID,
		OperationID:     strings.TrimSpace(input.OperationID),
		Attempt:         input.Attempt,
		TraceID:         traceID,
		SpanID:          strings.TrimSpace(input.SpanID),
		ParentSpanID:    strings.TrimSpace(input.ParentSpanID),
		Actor:           actor,
		IntentDigest:    cr.IntentDigest,
		ArtifactDigest:  cr.ArtifactDigest,
		Payload:         payloadJSON,
		PreviousHash:    lastHash,
		ScopeDimensions: input.ScopeDimensions,
		SpecVersion:     version.SpecVersion,
		CanonVersion:    cr.CanonVersion,
		AdapterVersion:  version.Version,
		Signer:          s.signer,
	})
	if err != nil {
		return PrescribeOutput{}, wrapError(ErrCodeInternal, err.Error(), err)
	}

	// Set prescription_id = entry_id to keep stable identity between payload and entry.
	prescPayload.PrescriptionID = entry.EntryID
	payloadJSON, err = json.Marshal(prescPayload)
	if err != nil {
		return PrescribeOutput{}, wrapError(ErrCodeInternal, "failed to marshal prescription payload", err)
	}
	entry.Payload = payloadJSON
	if err := evidence.RehashEntry(&entry, s.signer); err != nil {
		return PrescribeOutput{}, wrapError(ErrCodeInternal, err.Error(), err)
	}

	if err := s.appendEntry(entry); err != nil {
		return PrescribeOutput{}, err
	}

	return PrescribeOutput{
		PrescriptionID: entry.EntryID,
		SessionID:      sessionID,
		TraceID:        traceID,
		Actor:          actor,
		RiskLevel:      riskLevel,
		RiskTags:       riskTags,
		ArtifactDigest: cr.ArtifactDigest,
		IntentDigest:   cr.IntentDigest,
		ShapeHash:      cr.CanonicalAction.ResourceShapeHash,
		ResourceCount:  cr.CanonicalAction.ResourceCount,
		OperationClass: cr.CanonicalAction.OperationClass,
		ScopeClass:     cr.CanonicalAction.ScopeClass,
		CanonVersion:   cr.CanonVersion,
		RetryCount:     retryCount,
	}, nil
}

// Report records operation outcome and links it to a previous prescription.
func (s *Service) Report(_ context.Context, input ReportInput) (ReportOutput, error) {
	if err := requiredSigner(s.signer); err != nil {
		return ReportOutput{}, err
	}

	prescriptionID := strings.TrimSpace(input.PrescriptionID)
	if prescriptionID == "" {
		return ReportOutput{}, wrapError(ErrCodeInvalidInput, "prescription_id is required", nil)
	}
	inputSessionID := strings.TrimSpace(input.SessionID)
	inputOperationID := strings.TrimSpace(input.OperationID)

	var prescriptionEntry evidence.EvidenceEntry
	prescriptionFound := false
	if s.evidencePath != "" {
		entry, found, err := evidence.FindEntryByID(s.evidencePath, prescriptionID)
		if err != nil {
			return ReportOutput{}, wrapError(ErrCodeEvidenceRead, fmt.Sprintf("failed to read evidence: %v", err), err)
		}
		if !found {
			signalSessionID := inputSessionID
			if signalSessionID == "" {
				signalSessionID = evidence.GenerateSessionID()
			}
			s.writeUnknownPrescriptionSignal(
				normalizeActor(input.Actor),
				prescriptionID,
				signalSessionID,
				inputOperationID,
			)
			return ReportOutput{}, wrapError(ErrCodeNotFound, "prescription_id not found", nil)
		}
		prescriptionEntry = entry
		prescriptionFound = true
	}

	if prescriptionFound && inputSessionID != "" && prescriptionEntry.SessionID != "" && inputSessionID != prescriptionEntry.SessionID {
		return ReportOutput{}, wrapError(
			ErrCodeInvalidInput,
			fmt.Sprintf("report session_id %q does not match prescription session_id %q", inputSessionID, prescriptionEntry.SessionID),
			nil,
		)
	}

	reportID := ulid.Make().String()
	reportPayload := evidence.ReportPayload{
		ReportID:       reportID,
		PrescriptionID: prescriptionID,
		ExitCode:       input.ExitCode,
		Verdict:        evidence.VerdictFromExitCode(input.ExitCode),
		ExternalRefs:   input.ExternalRefs,
	}
	payloadJSON, err := json.Marshal(reportPayload)
	if err != nil {
		return ReportOutput{}, wrapError(ErrCodeInternal, "failed to marshal report payload", err)
	}

	actor := normalizeActor(input.Actor)
	if actor.ID == "" && prescriptionFound {
		actor = prescriptionEntry.Actor
	}

	traceID := ""
	if prescriptionFound {
		traceID = prescriptionEntry.TraceID
	}
	if traceID == "" {
		traceID = evidence.GenerateTraceID()
	}

	sessionID := inputSessionID
	if sessionID == "" && prescriptionFound {
		sessionID = prescriptionEntry.SessionID
	}

	operationID := inputOperationID
	if operationID == "" && prescriptionFound {
		operationID = prescriptionEntry.OperationID
	}

	lastHash, err := s.lastHash()
	if err != nil {
		return ReportOutput{}, err
	}

	entry, err := evidence.BuildEntry(evidence.EntryBuildParams{
		Type:           evidence.EntryTypeReport,
		SessionID:      sessionID,
		OperationID:    operationID,
		TraceID:        traceID,
		SpanID:         strings.TrimSpace(input.SpanID),
		ParentSpanID:   strings.TrimSpace(input.ParentSpanID),
		Actor:          actor,
		ArtifactDigest: input.ArtifactDigest,
		Payload:        payloadJSON,
		PreviousHash:   lastHash,
		SpecVersion:    version.SpecVersion,
		AdapterVersion: version.Version,
		Signer:         s.signer,
	})
	if err != nil {
		return ReportOutput{}, wrapError(ErrCodeInternal, err.Error(), err)
	}

	if err := s.appendEntry(entry); err != nil {
		return ReportOutput{}, err
	}

	return ReportOutput{
		ReportID:       entry.EntryID,
		SessionID:      sessionID,
		TraceID:        traceID,
		Actor:          actor,
		PrescriptionID: prescriptionID,
	}, nil
}

func (s *Service) writeCanonicalizationFailure(actor evidence.Actor, cr canon.CanonResult, sessionID, traceID, operationID string, attempt int) {
	if s.evidencePath == "" {
		return
	}

	if traceID == "" {
		traceID = evidence.GenerateTraceID()
	}

	failPayload, _ := json.Marshal(evidence.CanonFailurePayload{
		ErrorCode:    "parse_error",
		ErrorMessage: cr.ParseError.Error(),
		Adapter:      cr.CanonVersion,
		RawDigest:    cr.ArtifactDigest,
	})

	lastHash, _ := evidence.LastHashAtPath(s.evidencePath)
	entry, err := evidence.BuildEntry(evidence.EntryBuildParams{
		Type:           evidence.EntryTypeCanonFailure,
		SessionID:      sessionID,
		OperationID:    operationID,
		Attempt:        attempt,
		TraceID:        traceID,
		Actor:          actor,
		ArtifactDigest: cr.ArtifactDigest,
		Payload:        failPayload,
		PreviousHash:   lastHash,
		SpecVersion:    version.SpecVersion,
		AdapterVersion: version.Version,
		Signer:         s.signer,
	})
	if err == nil {
		_ = evidence.AppendEntryAtPath(s.evidencePath, entry)
	}
}

func (s *Service) writeUnknownPrescriptionSignal(actor evidence.Actor, prescriptionID, sessionID, operationID string) {
	if s.evidencePath == "" {
		return
	}
	sigPayload, _ := json.Marshal(evidence.SignalPayload{
		SignalName: "protocol_violation",
		SubSignal:  "unprescribed_action",
		EntryRefs:  []string{prescriptionID},
		Details:    "report references unknown prescription " + prescriptionID,
	})

	lastHash, _ := evidence.LastHashAtPath(s.evidencePath)
	traceID := evidence.GenerateTraceID()
	entry, err := evidence.BuildEntry(evidence.EntryBuildParams{
		Type:           evidence.EntryTypeSignal,
		SessionID:      sessionID,
		OperationID:    operationID,
		TraceID:        traceID,
		Actor:          actor,
		Payload:        sigPayload,
		PreviousHash:   lastHash,
		SpecVersion:    version.SpecVersion,
		AdapterVersion: version.Version,
		Signer:         s.signer,
	})
	if err == nil {
		_ = evidence.AppendEntryAtPath(s.evidencePath, entry)
	}
}

func (s *Service) appendEntry(entry evidence.EvidenceEntry) error {
	if s.evidencePath == "" {
		return nil
	}
	if err := evidence.AppendEntryAtPath(s.evidencePath, entry); err != nil {
		return wrapError(ErrCodeEvidenceWrite, fmt.Sprintf("failed to write evidence: %v", err), err)
	}
	return nil
}

func (s *Service) lastHash() (string, error) {
	if s.evidencePath == "" {
		return "", nil
	}
	lastHash, err := evidence.LastHashAtPath(s.evidencePath)
	if err != nil {
		return "", wrapError(ErrCodeEvidenceRead, fmt.Sprintf("failed to read evidence: %v", err), err)
	}
	return lastHash, nil
}

func normalizeCanonicalAction(action canon.CanonicalAction, tool, operation string) canon.CanonicalAction {
	action.Tool = normalizeToken(action.Tool)
	action.Operation = normalizeToken(action.Operation)
	if action.Tool == "" {
		action.Tool = tool
	}
	if action.Operation == "" {
		action.Operation = operation
	}
	return action
}

func normalizeActor(actor evidence.Actor) evidence.Actor {
	actor.Type = strings.TrimSpace(actor.Type)
	actor.ID = strings.TrimSpace(actor.ID)
	actor.Provenance = strings.TrimSpace(actor.Provenance)
	actor.InstanceID = strings.TrimSpace(actor.InstanceID)
	actor.Version = strings.TrimSpace(actor.Version)
	return actor
}

func normalizeToken(v string) string {
	return strings.ToLower(strings.TrimSpace(v))
}
