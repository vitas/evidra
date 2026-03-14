package lifecycle

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"

	"github.com/oklog/ulid/v2"

	"samebits.com/evidra/internal/canon"
	"samebits.com/evidra/internal/detectors"
	_ "samebits.com/evidra/internal/detectors/all"
	"samebits.com/evidra/internal/risk"
	"samebits.com/evidra/pkg/evidence"
	"samebits.com/evidra/pkg/version"
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
		traceID = sessionID
	}
	actor := normalizeActor(input.Actor)
	if err := validatePrescribeActor(actor); err != nil {
		return PrescribeOutput{}, err
	}

	var cr canon.CanonResult
	if input.CanonicalAction != nil {
		preCanon, err := normalizeCanonicalAction(*input.CanonicalAction, tool, operation)
		if err != nil {
			return PrescribeOutput{}, err
		}
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

	matrixLevel := risk.RiskLevel(cr.CanonicalAction.OperationClass, cr.CanonicalAction.ScopeClass)
	var riskInputs []evidence.RiskInput
	if len(input.RawArtifact) > 0 {
		nativeTags := detectors.ProduceAll(cr.CanonicalAction, input.RawArtifact)
		nativeLevel := risk.ElevateRiskLevel(matrixLevel, nativeTags)
		riskInputs = append(riskInputs, evidence.RiskInput{
			Source:    "evidra/native",
			RiskLevel: nativeLevel,
			RiskTags:  nativeTags,
		})
	} else {
		riskInputs = append(riskInputs, evidence.RiskInput{
			Source:    "evidra/matrix",
			RiskLevel: matrixLevel,
		})
	}
	for _, src := range input.ExternalFindings {
		riskInputs = append(riskInputs, buildSARIFRiskInput(src))
	}
	effectiveRisk := computeEffectiveRisk(riskInputs)
	nativeTags := []string(nil)
	if len(riskInputs) > 0 && riskInputs[0].Source == "evidra/native" {
		nativeTags = riskInputs[0].RiskTags
	}

	retryCount := 0
	if s.retryTracker != nil {
		retryCount = s.retryTracker.Record(cr.IntentDigest, cr.CanonicalAction.ResourceShapeHash)
	}

	canonSource := "adapter"
	if input.CanonicalAction != nil {
		canonSource = "external"
	}

	prescPayload := evidence.PrescriptionPayload{
		PrescriptionID:  ulid.Make().String(),
		CanonicalAction: cr.RawAction,
		RiskInputs:      riskInputs,
		EffectiveRisk:   effectiveRisk,
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
		EntryID:         prescPayload.PrescriptionID,
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
		ScoringVersion:  version.ScoringVersion,
		Signer:          s.signer,
	})
	if err != nil {
		return PrescribeOutput{}, wrapError(ErrCodeInternal, err.Error(), err)
	}

	persisted, err := s.appendEntry(entry)
	if err != nil {
		return PrescribeOutput{}, err
	}
	if persisted {
		s.writeFindingsEvidence(input.ExternalFindings, sessionID, traceID, strings.TrimSpace(input.OperationID), input.Attempt, actor, cr.ArtifactDigest)
	}

	rawEntry, err := json.Marshal(entry)
	if err != nil {
		return PrescribeOutput{}, wrapError(ErrCodeInternal, "failed to marshal evidence entry", err)
	}

	return PrescribeOutput{
		PrescriptionID: entry.EntryID,
		SessionID:      sessionID,
		TraceID:        traceID,
		Actor:          actor,
		RiskInputs:     riskInputs,
		EffectiveRisk:  effectiveRisk,
		RiskLevel:      effectiveRisk,
		RiskTags:       nativeTags,
		ArtifactDigest: cr.ArtifactDigest,
		IntentDigest:   cr.IntentDigest,
		ShapeHash:      cr.CanonicalAction.ResourceShapeHash,
		ResourceCount:  cr.CanonicalAction.ResourceCount,
		OperationClass: cr.CanonicalAction.OperationClass,
		ScopeClass:     cr.CanonicalAction.ScopeClass,
		CanonVersion:   cr.CanonVersion,
		RetryCount:     retryCount,
		Entry:          entry,
		RawEntry:       rawEntry,
		Persisted:      persisted,
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
	decisionContext, err := validateDecisionReportInput(input)
	if err != nil {
		return ReportOutput{}, err
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
		ReportID:        reportID,
		PrescriptionID:  prescriptionID,
		ExitCode:        input.ExitCode,
		Verdict:         input.Verdict,
		DecisionContext: decisionContext,
		ExternalRefs:    input.ExternalRefs,
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
		ScoringVersion: version.ScoringVersion,
		Signer:         s.signer,
	})
	if err != nil {
		return ReportOutput{}, wrapError(ErrCodeInternal, err.Error(), err)
	}

	persisted, err := s.appendEntry(entry)
	if err != nil {
		return ReportOutput{}, err
	}

	rawEntry, err := json.Marshal(entry)
	if err != nil {
		return ReportOutput{}, wrapError(ErrCodeInternal, "failed to marshal evidence entry", err)
	}

	return ReportOutput{
		ReportID:        entry.EntryID,
		SessionID:       sessionID,
		TraceID:         traceID,
		Actor:           actor,
		PrescriptionID:  prescriptionID,
		Verdict:         input.Verdict,
		ExitCode:        input.ExitCode,
		DecisionContext: decisionContext,
		Entry:           entry,
		RawEntry:        rawEntry,
		Persisted:       persisted,
	}, nil
}

func validateDecisionReportInput(input ReportInput) (*evidence.DecisionContext, error) {
	verdict := input.Verdict
	if !verdict.Valid() {
		return nil, wrapError(ErrCodeInvalidInput, "verdict is required and must be one of success, failure, error, declined", nil)
	}

	if verdict == evidence.VerdictDeclined {
		if input.ExitCode != nil {
			return nil, wrapError(ErrCodeInvalidInput, "declined reports must not include exit_code", nil)
		}
		if input.DecisionContext == nil {
			return nil, wrapError(ErrCodeInvalidInput, "decision_context is required for declined reports", nil)
		}
		trigger := strings.TrimSpace(input.DecisionContext.Trigger)
		if trigger == "" {
			return nil, wrapError(ErrCodeInvalidInput, "decision_context.trigger is required", nil)
		}
		reason := strings.TrimSpace(input.DecisionContext.Reason)
		if reason == "" {
			return nil, wrapError(ErrCodeInvalidInput, "decision_context.reason is required", nil)
		}
		if len(reason) > 512 {
			return nil, wrapError(ErrCodeInvalidInput, "decision_context.reason exceeds 512 characters", nil)
		}
		return &evidence.DecisionContext{
			Trigger: trigger,
			Reason:  reason,
		}, nil
	}

	if input.DecisionContext != nil {
		return nil, wrapError(ErrCodeInvalidInput, "decision_context is only valid for declined reports", nil)
	}
	if input.ExitCode == nil {
		return nil, wrapError(ErrCodeInvalidInput, fmt.Sprintf("report verdict %s requires exit_code", verdict), nil)
	}
	if inferred := evidence.VerdictFromExitCode(*input.ExitCode); inferred != verdict {
		return nil, wrapError(ErrCodeInvalidInput, fmt.Sprintf("report verdict %s does not match exit_code %d", verdict, *input.ExitCode), nil)
	}
	return nil, nil
}

func (s *Service) writeCanonicalizationFailure(actor evidence.Actor, cr canon.CanonResult, sessionID, traceID, operationID string, attempt int) {
	if s.evidencePath == "" {
		return
	}

	if traceID == "" {
		traceID = evidence.GenerateTraceID()
	}

	failPayload, _ := json.Marshal(evidence.CanonFailurePayload{ // best-effort: struct is always marshalable
		ErrorCode:    "parse_error",
		ErrorMessage: cr.ParseError.Error(),
		Adapter:      cr.CanonVersion,
		RawDigest:    cr.ArtifactDigest,
	})

	lastHash, _ := evidence.LastHashAtPath(s.evidencePath) // best-effort: failure recording is advisory
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
		ScoringVersion: version.ScoringVersion,
		Signer:         s.signer,
	})
	if err == nil {
		_ = evidence.AppendEntryAtPath(s.evidencePath, entry) // best-effort: failure signal is advisory
	}
}

func (s *Service) writeUnknownPrescriptionSignal(actor evidence.Actor, prescriptionID, sessionID, operationID string) {
	if s.evidencePath == "" {
		return
	}
	sigPayload, _ := json.Marshal(evidence.SignalPayload{ // best-effort: struct is always marshalable
		SignalName: "protocol_violation",
		SubSignal:  "unprescribed_action",
		EntryRefs:  []string{prescriptionID},
		Details:    "report references unknown prescription " + prescriptionID,
	})

	lastHash, _ := evidence.LastHashAtPath(s.evidencePath) // best-effort: signal recording is advisory
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
		ScoringVersion: version.ScoringVersion,
		Signer:         s.signer,
	})
	if err == nil {
		_ = evidence.AppendEntryAtPath(s.evidencePath, entry) // best-effort: signal entry is advisory
	}
}

func (s *Service) writeFindingsEvidence(sources []ExternalFindingsSource, sessionID, traceID, operationID string, attempt int, actor evidence.Actor, artifactDigest string) {
	if s.evidencePath == "" {
		return
	}
	if traceID == "" {
		traceID = sessionID
	}

	for _, src := range sources {
		for _, finding := range src.Findings {
			payload, err := json.Marshal(finding)
			if err != nil {
				slog.Warn("failed to marshal finding payload", "rule_id", finding.RuleID, "error", err)
				continue
			}
			lastHash, err := s.lastHash()
			if err != nil {
				slog.Warn("failed to read last hash for finding entry", "rule_id", finding.RuleID, "error", err)
				continue
			}
			entry, err := evidence.BuildEntry(evidence.EntryBuildParams{
				Type:           evidence.EntryTypeFinding,
				SessionID:      sessionID,
				OperationID:    operationID,
				Attempt:        attempt,
				TraceID:        traceID,
				Actor:          actor,
				ArtifactDigest: artifactDigest,
				Payload:        payload,
				PreviousHash:   lastHash,
				SpecVersion:    version.SpecVersion,
				AdapterVersion: version.Version,
				ScoringVersion: version.ScoringVersion,
				Signer:         s.signer,
			})
			if err != nil {
				slog.Warn("failed to build finding entry", "rule_id", finding.RuleID, "error", err)
				continue
			}
			if _, err := s.appendEntry(entry); err != nil {
				slog.Warn("failed to append finding entry", "rule_id", finding.RuleID, "error", err)
			}
		}
	}
}

func (s *Service) appendEntry(entry evidence.EvidenceEntry) (bool, error) {
	if s.evidencePath == "" {
		return false, nil
	}
	if err := evidence.AppendEntryAtPath(s.evidencePath, entry); err != nil {
		if s.bestEffortWrites {
			slog.Warn(
				"best-effort evidence write failed",
				"entry_id", entry.EntryID,
				"entry_type", string(entry.Type),
				"error", err,
			)
			return false, nil
		}
		return false, wrapError(ErrCodeEvidenceWrite, fmt.Sprintf("failed to write evidence: %v", err), err)
	}
	return true, nil
}

func (s *Service) lastHash() (string, error) {
	if s.evidencePath == "" {
		return "", nil
	}
	lastHash, err := evidence.LastHashAtPath(s.evidencePath)
	if err != nil {
		if s.bestEffortWrites {
			slog.Warn(
				"best-effort evidence read failed",
				"operation", "last_hash",
				"error", err,
			)
			return "", nil
		}
		return "", wrapError(ErrCodeEvidenceRead, fmt.Sprintf("failed to read evidence: %v", err), err)
	}
	return lastHash, nil
}

func normalizeCanonicalAction(action canon.CanonicalAction, tool, operation string) (canon.CanonicalAction, error) {
	action.Tool = normalizeToken(action.Tool)
	action.Operation = normalizeToken(action.Operation)
	if action.Tool == "" {
		action.Tool = tool
	}
	if action.Operation == "" {
		action.Operation = operation
	}
	scopeClass, err := normalizeIngressScopeClass(action.ScopeClass)
	if err != nil {
		return canon.CanonicalAction{}, err
	}
	action.ScopeClass = scopeClass
	return action, nil
}

func normalizeIngressScopeClass(raw string) (string, error) {
	v := strings.TrimSpace(raw)
	if v == "" {
		return "unknown", nil
	}
	normalized := canon.NormalizeScopeClass(v)
	if normalized != "unknown" || strings.EqualFold(v, "unknown") {
		return normalized, nil
	}
	return "", wrapError(
		ErrCodeInvalidInput,
		fmt.Sprintf(
			"invalid canonical_action.scope_class %q; expected one of production, staging, development, unknown (aliases: prod, stage, dev, test, sandbox)",
			v,
		),
		nil,
	)
}

func normalizeActor(actor evidence.Actor) evidence.Actor {
	actor.Type = strings.TrimSpace(actor.Type)
	actor.ID = strings.TrimSpace(actor.ID)
	actor.Provenance = strings.TrimSpace(actor.Provenance)
	actor.InstanceID = strings.TrimSpace(actor.InstanceID)
	actor.Version = strings.TrimSpace(actor.Version)
	actor.SkillVersion = strings.TrimSpace(actor.SkillVersion)
	return actor
}

func validatePrescribeActor(actor evidence.Actor) error {
	switch {
	case actor.Type == "":
		return wrapError(ErrCodeInvalidInput, "actor.type is required", nil)
	case actor.ID == "":
		return wrapError(ErrCodeInvalidInput, "actor.id is required", nil)
	case actor.Provenance == "":
		return wrapError(ErrCodeInvalidInput, "actor.provenance is required", nil)
	default:
		return nil
	}
}

func normalizeToken(v string) string {
	return strings.ToLower(strings.TrimSpace(v))
}
