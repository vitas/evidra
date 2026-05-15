package lifecycle

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"

	"github.com/oklog/ulid/v2"

	"samebits.com/evidra/pkg/evidence"
	"samebits.com/evidra/pkg/version"
)

type prescribeContext struct {
	tool        string
	operation   string
	environment string
	sessionID   string
	traceID     string
	actor       evidence.Actor
}

type reportContext struct {
	sessionID   string
	operationID string
	traceID     string
	actor       evidence.Actor
}

// Prescribe writes pre-execution intent evidence. Canonicalization and
// assessment are optional caller-supplied enrichments.
func (s *Service) Prescribe(_ context.Context, input PrescribeInput) (PrescribeOutput, error) {
	if err := requiredSigner(s.signer); err != nil {
		return PrescribeOutput{}, err
	}

	ctx, err := buildPrescribeContext(input)
	if err != nil {
		return PrescribeOutput{}, err
	}

	if hasDeclaredIntent(input.Intent) || input.CanonicalAction == nil {
		return s.prescribeDeclaredIntent(input, ctx)
	}

	return s.prescribeCanonicalAction(input, ctx)
}

func (s *Service) prescribeDeclaredIntent(input PrescribeInput, ctx prescribeContext) (PrescribeOutput, error) {
	intent, artifactDigest, err := buildDeclaredIntent(input)
	if err != nil {
		return PrescribeOutput{}, err
	}
	assessment, err := normalizeAssessment(input.Assessment)
	if err != nil {
		return PrescribeOutput{}, err
	}
	action, rawAction, canonVersion, err := optionalCanonicalAction(input, ctx)
	if err != nil {
		return PrescribeOutput{}, err
	}

	prescPayload := evidence.PrescriptionPayload{
		PrescriptionID: ulid.Make().String(),
		Intent:         &intent,
		Assessment:     assessment,
		TTLMs:          evidence.DefaultTTLMs,
		Flavor:         input.Flavor,
		Evidence:       payloadEvidenceMetadata(input.EvidenceKind),
		Source:         payloadSourceMetadata(input.SourceSystem),
	}
	if len(rawAction) > 0 {
		prescPayload.CanonicalAction = rawAction
		prescPayload.CanonSource = "external"
	}
	if assessment.Status == evidence.AssessmentProvided {
		prescPayload.RiskInputs = assessment.RiskInputs
		prescPayload.EffectiveRisk = assessment.EffectiveRisk
	}
	payloadJSON, err := json.Marshal(prescPayload)
	if err != nil {
		return PrescribeOutput{}, wrapError(ErrCodeInternal, "failed to marshal prescription payload", err)
	}

	lastHash, err := s.lastHash()
	if err != nil {
		return PrescribeOutput{}, err
	}

	intentDigest := evidence.ComputeDeclaredIntentDigest(intent)
	entry, err := evidence.BuildEntry(evidence.EntryBuildParams{
		EntryID:         prescPayload.PrescriptionID,
		Type:            evidence.EntryTypePrescribe,
		SessionID:       ctx.sessionID,
		OperationID:     strings.TrimSpace(input.OperationID),
		Attempt:         input.Attempt,
		TraceID:         ctx.traceID,
		SpanID:          strings.TrimSpace(input.SpanID),
		ParentSpanID:    strings.TrimSpace(input.ParentSpanID),
		Actor:           ctx.actor,
		IntentDigest:    intentDigest,
		ArtifactDigest:  artifactDigest,
		Payload:         payloadJSON,
		PreviousHash:    lastHash,
		ScopeDimensions: input.ScopeDimensions,
		SpecVersion:     version.SpecVersion,
		CanonVersion:    canonVersion,
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
		s.writeFindingsEvidence(input.ExternalFindings, ctx.sessionID, ctx.traceID, strings.TrimSpace(input.OperationID), input.Attempt, ctx.actor, artifactDigest)
	}

	rawEntry, err := json.Marshal(entry)
	if err != nil {
		return PrescribeOutput{}, wrapError(ErrCodeInternal, "failed to marshal evidence entry", err)
	}

	retryCount := 0
	if s.retryTracker != nil {
		retryShape := artifactDigest
		if action.ResourceShapeHash != "" {
			retryShape = action.ResourceShapeHash
		}
		retryCount = s.retryTracker.Record(intentDigest, retryShape)
	}

	return PrescribeOutput{
		PrescriptionID: entry.EntryID,
		SessionID:      ctx.sessionID,
		TraceID:        ctx.traceID,
		Actor:          ctx.actor,
		Intent:         intent,
		Assessment:     assessment,
		RiskInputs:     assessment.RiskInputs,
		EffectiveRisk:  assessment.EffectiveRisk,
		RiskLevel:      assessment.EffectiveRisk,
		ArtifactDigest: artifactDigest,
		IntentDigest:   intentDigest,
		ShapeHash:      action.ResourceShapeHash,
		ResourceCount:  action.ResourceCount,
		OperationClass: action.OperationClass,
		ScopeClass:     action.ScopeClass,
		CanonVersion:   canonVersion,
		RetryCount:     retryCount,
		Entry:          entry,
		RawEntry:       rawEntry,
		Persisted:      persisted,
	}, nil
}

func (s *Service) prescribeCanonicalAction(input PrescribeInput, ctx prescribeContext) (PrescribeOutput, error) {
	action, rawAction, canonVersion, err := optionalCanonicalAction(input, ctx)
	if err != nil {
		return PrescribeOutput{}, err
	}
	assessment, err := normalizeAssessment(input.Assessment)
	if err != nil {
		return PrescribeOutput{}, err
	}

	artifactDigest := ""
	if len(input.RawArtifact) > 0 {
		artifactDigest = evidence.SHA256Hex(input.RawArtifact)
	}
	intentDigest := evidence.ComputeCanonicalActionDigest(action)
	prescPayload := evidence.PrescriptionPayload{
		PrescriptionID:  ulid.Make().String(),
		CanonicalAction: rawAction,
		Assessment:      assessment,
		TTLMs:           evidence.DefaultTTLMs,
		CanonSource:     "external",
		Flavor:          input.Flavor,
		Evidence:        payloadEvidenceMetadata(input.EvidenceKind),
		Source:          payloadSourceMetadata(input.SourceSystem),
	}
	if assessment.Status == evidence.AssessmentProvided {
		prescPayload.RiskInputs = assessment.RiskInputs
		prescPayload.EffectiveRisk = assessment.EffectiveRisk
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
		SessionID:       ctx.sessionID,
		OperationID:     strings.TrimSpace(input.OperationID),
		Attempt:         input.Attempt,
		TraceID:         ctx.traceID,
		SpanID:          strings.TrimSpace(input.SpanID),
		ParentSpanID:    strings.TrimSpace(input.ParentSpanID),
		Actor:           ctx.actor,
		IntentDigest:    intentDigest,
		ArtifactDigest:  artifactDigest,
		Payload:         payloadJSON,
		PreviousHash:    lastHash,
		ScopeDimensions: input.ScopeDimensions,
		SpecVersion:     version.SpecVersion,
		CanonVersion:    canonVersion,
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
		s.writeFindingsEvidence(input.ExternalFindings, ctx.sessionID, ctx.traceID, strings.TrimSpace(input.OperationID), input.Attempt, ctx.actor, artifactDigest)
	}
	rawEntry, err := json.Marshal(entry)
	if err != nil {
		return PrescribeOutput{}, wrapError(ErrCodeInternal, "failed to marshal evidence entry", err)
	}

	retryCount := 0
	if s.retryTracker != nil {
		retryCount = s.retryTracker.Record(intentDigest, action.ResourceShapeHash)
	}

	return PrescribeOutput{
		PrescriptionID: entry.EntryID,
		SessionID:      ctx.sessionID,
		TraceID:        ctx.traceID,
		Actor:          ctx.actor,
		Assessment:     assessment,
		RiskInputs:     assessment.RiskInputs,
		EffectiveRisk:  assessment.EffectiveRisk,
		RiskLevel:      assessment.EffectiveRisk,
		ArtifactDigest: artifactDigest,
		IntentDigest:   intentDigest,
		ShapeHash:      action.ResourceShapeHash,
		ResourceCount:  action.ResourceCount,
		OperationClass: action.OperationClass,
		ScopeClass:     action.ScopeClass,
		CanonVersion:   canonVersion,
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
	prescriptionEntry, prescriptionFound, err := s.loadReportPrescription(input, prescriptionID)
	if err != nil {
		return ReportOutput{}, err
	}

	reportID := ulid.Make().String()
	reportPayload := evidence.ReportPayload{
		ReportID:        reportID,
		PrescriptionID:  prescriptionID,
		ExitCode:        input.ExitCode,
		Verdict:         input.Verdict,
		DecisionContext: decisionContext,
		ExternalRefs:    input.ExternalRefs,
		Flavor:          input.Flavor,
		Evidence:        payloadEvidenceMetadata(input.EvidenceKind),
		Source:          payloadSourceMetadata(input.SourceSystem),
	}
	payloadJSON, err := json.Marshal(reportPayload)
	if err != nil {
		return ReportOutput{}, wrapError(ErrCodeInternal, "failed to marshal report payload", err)
	}

	ctx := resolveReportContext(input, prescriptionEntry, prescriptionFound)
	if err := ctx.validate(prescriptionFound, prescriptionEntry.SessionID); err != nil {
		return ReportOutput{}, err
	}

	lastHash, err := s.lastHash()
	if err != nil {
		return ReportOutput{}, err
	}

	entry, err := evidence.BuildEntry(evidence.EntryBuildParams{
		Type:           evidence.EntryTypeReport,
		SessionID:      ctx.sessionID,
		OperationID:    ctx.operationID,
		TraceID:        ctx.traceID,
		SpanID:         strings.TrimSpace(input.SpanID),
		ParentSpanID:   strings.TrimSpace(input.ParentSpanID),
		Actor:          ctx.actor,
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
		SessionID:       ctx.sessionID,
		TraceID:         ctx.traceID,
		Actor:           ctx.actor,
		PrescriptionID:  prescriptionID,
		Verdict:         input.Verdict,
		ExitCode:        input.ExitCode,
		DecisionContext: decisionContext,
		Entry:           entry,
		RawEntry:        rawEntry,
		Persisted:       persisted,
	}, nil
}

func buildPrescribeContext(input PrescribeInput) (prescribeContext, error) {
	ctx := prescribeContext{
		tool:        normalizeToken(input.Tool),
		operation:   normalizeToken(input.Operation),
		environment: strings.TrimSpace(input.Environment),
		sessionID:   strings.TrimSpace(input.SessionID),
		traceID:     strings.TrimSpace(input.TraceID),
		actor:       normalizeActor(input.Actor),
	}
	if ctx.sessionID == "" {
		ctx.sessionID = evidence.GenerateSessionID()
	}
	if ctx.traceID == "" {
		ctx.traceID = ctx.sessionID
	}
	if err := validatePrescribeActor(ctx.actor); err != nil {
		return prescribeContext{}, err
	}
	return ctx, nil
}

func hasDeclaredIntent(intent evidence.DeclaredIntent) bool {
	return strings.TrimSpace(intent.Tool) != "" ||
		strings.TrimSpace(intent.Operation) != "" ||
		strings.TrimSpace(intent.Target) != "" ||
		strings.TrimSpace(intent.Command) != "" ||
		strings.TrimSpace(intent.ArtifactDigest) != ""
}

func buildDeclaredIntent(input PrescribeInput) (evidence.DeclaredIntent, string, error) {
	intent := input.Intent
	intent.Tool = normalizeToken(intent.Tool)
	if intent.Tool == "" {
		intent.Tool = normalizeToken(input.Tool)
	}
	intent.Operation = normalizeToken(intent.Operation)
	if intent.Operation == "" {
		intent.Operation = normalizeToken(input.Operation)
	}
	intent.Target = strings.TrimSpace(intent.Target)
	intent.Command = strings.TrimSpace(intent.Command)
	intent.ArtifactDigest = strings.TrimSpace(intent.ArtifactDigest)
	if intent.ArtifactDigest == "" && len(input.RawArtifact) > 0 {
		intent.ArtifactDigest = evidence.SHA256Hex(input.RawArtifact)
	}
	if !hasDeclaredIntent(intent) {
		return evidence.DeclaredIntent{}, "", wrapError(ErrCodeInvalidInput, "prescribe intent is required", nil)
	}
	if err := evidence.ValidateDigest(intent.ArtifactDigest); err != nil {
		return evidence.DeclaredIntent{}, "", wrapError(ErrCodeInvalidInput, err.Error(), err)
	}
	return intent, intent.ArtifactDigest, nil
}

func normalizeAssessment(in *evidence.AssessmentPayload) (*evidence.AssessmentPayload, error) {
	if in == nil {
		return &evidence.AssessmentPayload{Status: evidence.AssessmentNotProvided}, nil
	}
	out := *in
	if out.Status == "" {
		out.Status = evidence.AssessmentProvided
	}
	if err := evidence.ValidateAssessmentStatus(out.Status); err != nil {
		return nil, wrapError(ErrCodeInvalidInput, err.Error(), err)
	}
	if err := evidence.ValidateRiskLevel(out.EffectiveRisk); err != nil {
		return nil, wrapError(ErrCodeInvalidInput, err.Error(), err)
	}
	for _, input := range out.RiskInputs {
		if err := evidence.ValidateRiskLevel(input.RiskLevel); err != nil {
			return nil, wrapError(ErrCodeInvalidInput, err.Error(), err)
		}
	}
	return &out, nil
}

func optionalCanonicalAction(input PrescribeInput, ctx prescribeContext) (evidence.CanonicalAction, json.RawMessage, string, error) {
	if input.CanonicalAction == nil {
		return evidence.CanonicalAction{}, nil, "", nil
	}
	action, err := normalizeCanonicalAction(*input.CanonicalAction, ctx.tool, ctx.operation)
	if err != nil {
		return evidence.CanonicalAction{}, nil, "", err
	}
	raw, err := json.Marshal(action)
	if err != nil {
		return evidence.CanonicalAction{}, nil, "", wrapError(ErrCodeInternal, "failed to marshal canonical action", err)
	}
	return action, raw, "external/v1", nil
}

func payloadEvidenceMetadata(kind evidence.EvidenceKind) *evidence.EvidenceMetadata {
	if strings.TrimSpace(string(kind)) == "" {
		return nil
	}
	return &evidence.EvidenceMetadata{Kind: kind}
}

func payloadSourceMetadata(system string) *evidence.SourceMetadata {
	system = strings.TrimSpace(system)
	if system == "" {
		return nil
	}
	return &evidence.SourceMetadata{System: system}
}

func (s *Service) loadReportPrescription(input ReportInput, prescriptionID string) (evidence.EvidenceEntry, bool, error) {
	if s.evidencePath == "" {
		return evidence.EvidenceEntry{}, false, nil
	}

	entry, found, err := evidence.FindEntryByID(s.evidencePath, prescriptionID)
	if err != nil {
		return evidence.EvidenceEntry{}, false, wrapError(ErrCodeEvidenceRead, fmt.Sprintf("failed to read evidence: %v", err), err)
	}
	if found {
		return entry, true, nil
	}

	signalSessionID := strings.TrimSpace(input.SessionID)
	if signalSessionID == "" {
		signalSessionID = evidence.GenerateSessionID()
	}
	s.writeUnknownPrescriptionSignal(
		normalizeActor(input.Actor),
		prescriptionID,
		signalSessionID,
		strings.TrimSpace(input.OperationID),
	)
	return evidence.EvidenceEntry{}, false, wrapError(ErrCodeNotFound, "prescription_id not found", nil)
}

func resolveReportContext(input ReportInput, prescriptionEntry evidence.EvidenceEntry, prescriptionFound bool) reportContext {
	ctx := reportContext{
		sessionID:   strings.TrimSpace(input.SessionID),
		operationID: strings.TrimSpace(input.OperationID),
		traceID:     evidence.GenerateTraceID(),
		actor:       normalizeActor(input.Actor),
	}
	if prescriptionFound {
		if ctx.actor.ID == "" {
			ctx.actor = prescriptionEntry.Actor
		}
		if prescriptionEntry.TraceID != "" {
			ctx.traceID = prescriptionEntry.TraceID
		}
		if ctx.sessionID == "" {
			ctx.sessionID = prescriptionEntry.SessionID
		}
		if ctx.operationID == "" {
			ctx.operationID = prescriptionEntry.OperationID
		}
	}
	return ctx
}

func (ctx reportContext) validate(prescriptionFound bool, prescriptionSessionID string) error {
	if !prescriptionFound || ctx.sessionID == "" || prescriptionSessionID == "" || ctx.sessionID == prescriptionSessionID {
		return nil
	}
	return wrapError(
		ErrCodeInvalidInput,
		fmt.Sprintf("report session_id %q does not match prescription session_id %q", ctx.sessionID, prescriptionSessionID),
		nil,
	)
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

func normalizeCanonicalAction(action evidence.CanonicalAction, tool, operation string) (evidence.CanonicalAction, error) {
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
		return evidence.CanonicalAction{}, err
	}
	action.ScopeClass = scopeClass
	return action, nil
}

func normalizeIngressScopeClass(raw string) (string, error) {
	v := strings.TrimSpace(raw)
	if v == "" {
		return "unknown", nil
	}
	normalized := evidence.NormalizeScopeClass(v)
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
