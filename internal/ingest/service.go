package ingest

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/oklog/ulid/v2"

	"samebits.com/evidra/internal/store"
	"samebits.com/evidra/pkg/evidence"
	"samebits.com/evidra/pkg/version"
)

// Store is the minimal persistence dependency required by the ingest service.
type Store interface {
	LastHash(ctx context.Context, tenantID string) (string, error)
	SaveRaw(ctx context.Context, tenantID string, raw json.RawMessage) (string, error)
	GetEntry(ctx context.Context, tenantID, entryID string) (store.StoredEntry, error)
	ClaimWebhookEvent(ctx context.Context, tenantID, source, key string, payload json.RawMessage) (bool, error)
	ReleaseWebhookEvent(ctx context.Context, tenantID, source, key string) error
	BeginIngestTx(ctx context.Context) (store.IngestTx, error)
}

// Result captures the stable ingest response shape.
type Result struct {
	Duplicate      bool
	EntryID        string
	EffectiveRisk  string
	PrescriptionID string
	Entry          evidence.EvidenceEntry
}

// Service owns server-side external lifecycle ingest.
type Service struct {
	store                        Store
	signer                       evidence.Signer
	claimNamespacePrefix         string
	allowLegacyDuplicateFallback bool
}

// NewService creates an ingest service.
func NewService(store Store, signer evidence.Signer) *Service {
	return &Service{
		store:                store,
		signer:               signer,
		claimNamespacePrefix: "api:",
	}
}

// NewWebhookService creates an ingest service configured for legacy webhook compatibility.
func NewWebhookService(store Store, signer evidence.Signer) *Service {
	return &Service{
		store:                        store,
		signer:                       signer,
		allowLegacyDuplicateFallback: true,
	}
}

// Prescribe builds, signs, and persists a prescribe entry.
func (s *Service) Prescribe(ctx context.Context, tenantID string, in PrescribeRequest) (Result, error) {
	if err := requiredSigner(s.signer); err != nil {
		return Result{}, err
	}
	if err := ValidatePrescribeRequest(in); err != nil {
		return Result{}, wrapError(ErrCodeInvalidInput, err.Error(), err)
	}

	in.Actor = normalizeActor(in.Actor)
	in.SessionID = strings.TrimSpace(in.SessionID)
	in.OperationID = strings.TrimSpace(in.OperationID)
	in.TraceID = strings.TrimSpace(in.TraceID)
	in.SpanID = strings.TrimSpace(in.SpanID)
	in.ParentSpanID = strings.TrimSpace(in.ParentSpanID)

	tx, err := s.store.BeginIngestTx(ctx)
	if err != nil {
		return Result{}, wrapError(ErrCodeInternal, "failed to begin ingest transaction", err)
	}
	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback(ctx)
		}
	}()

	claimSource := s.claimSource(in.Claim)
	duplicate, err := s.claimIfNeeded(ctx, tx, tenantID, claimSource, in.Claim)
	if err != nil {
		return Result{}, err
	}
	if duplicate {
		result, err := s.loadDuplicatePrescribeResult(ctx, tx, tenantID, claimSource, in.Claim)
		if err != nil {
			return Result{}, err
		}
		return result, nil
	}

	var (
		effectiveRisk string
		entry         evidence.EvidenceEntry
	)
	entry, err = s.saveEntry(ctx, tx, tenantID, func(lastHash string) (evidence.EvidenceEntry, error) {
		var buildErr error
		entry, effectiveRisk, buildErr = buildPrescribeEntry(lastHash, s.signer, in)
		return entry, buildErr
	})
	if err != nil {
		return Result{}, err
	}

	if err := s.finalizeClaim(ctx, tx, tenantID, claimSource, in.Claim, entry.EntryID, effectiveRisk); err != nil {
		return Result{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Result{}, wrapError(ErrCodeInternal, "failed to commit ingest transaction", err)
	}
	committed = true
	return Result{
		EntryID:        entry.EntryID,
		EffectiveRisk:  effectiveRisk,
		PrescriptionID: entry.EntryID,
		Entry:          entry,
	}, nil
}

// Report builds, signs, and persists a report entry after resolving the prescription.
func (s *Service) Report(ctx context.Context, tenantID string, in ReportRequest) (Result, error) {
	if err := requiredSigner(s.signer); err != nil {
		return Result{}, err
	}
	if err := ValidateReportRequest(in); err != nil {
		return Result{}, wrapError(ErrCodeInvalidInput, err.Error(), err)
	}

	in.Actor = normalizeActor(in.Actor)
	in.SessionID = strings.TrimSpace(in.SessionID)
	in.OperationID = strings.TrimSpace(in.OperationID)
	in.TraceID = strings.TrimSpace(in.TraceID)
	in.SpanID = strings.TrimSpace(in.SpanID)
	in.ParentSpanID = strings.TrimSpace(in.ParentSpanID)

	tx, err := s.store.BeginIngestTx(ctx)
	if err != nil {
		return Result{}, wrapError(ErrCodeInternal, "failed to begin ingest transaction", err)
	}
	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback(ctx)
		}
	}()

	claimSource := s.claimSource(in.Claim)
	duplicate, err := s.claimIfNeeded(ctx, tx, tenantID, claimSource, in.Claim)
	if err != nil {
		return Result{}, err
	}
	if duplicate {
		result, err := s.loadDuplicateReportResult(ctx, tx, tenantID, claimSource, in.Claim)
		if err != nil {
			return Result{}, err
		}
		return result, nil
	}

	reportPayload, prescriptionID, err := buildReportPayloadState(in)
	if err != nil {
		return Result{}, err
	}

	softResolve := s.allowLegacyDuplicateFallback && reportUsesSoftResolution(in)
	prescription, err := s.resolvePrescriptionForReport(ctx, tenantID, prescriptionID, softResolve)
	if err != nil {
		return Result{}, err
	}

	entry, err := s.saveEntry(ctx, tx, tenantID, func(lastHash string) (evidence.EvidenceEntry, error) {
		return buildReportEntry(lastHash, s.signer, in, reportPayload, prescription, softResolve)
	})
	if err != nil {
		return Result{}, err
	}

	if err := s.finalizeClaim(ctx, tx, tenantID, claimSource, in.Claim, entry.EntryID, ""); err != nil {
		return Result{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Result{}, wrapError(ErrCodeInternal, "failed to commit ingest transaction", err)
	}
	committed = true
	return Result{
		EntryID: entry.EntryID,
		Entry:   entry,
	}, nil
}

func (s *Service) loadPrescription(ctx context.Context, tenantID, prescriptionID string) (store.StoredEntry, error) {
	entry, err := s.store.GetEntry(ctx, tenantID, prescriptionID)
	if err != nil {
		if errorsIsNotFound(err) {
			return store.StoredEntry{}, wrapError(ErrCodeNotFound, "prescription_id not found", err)
		}
		return store.StoredEntry{}, wrapError(ErrCodeInternal, "failed to read prescription entry", err)
	}
	if entry.EntryType != string(evidence.EntryTypePrescribe) {
		return store.StoredEntry{}, wrapError(ErrCodeInvalidInput, fmt.Sprintf("entry %q is not a prescribe entry", prescriptionID), nil)
	}
	return entry, nil
}

func (s *Service) resolvePrescriptionForReport(ctx context.Context, tenantID, prescriptionID string, softResolve bool) (store.StoredEntry, error) {
	if !softResolve {
		return s.loadPrescription(ctx, tenantID, prescriptionID)
	}

	entry, err := s.store.GetEntry(ctx, tenantID, prescriptionID)
	if err != nil {
		if errorsIsNotFound(err) {
			return store.StoredEntry{ID: prescriptionID}, nil
		}
		return store.StoredEntry{}, wrapError(ErrCodeInternal, "failed to read prescription entry", err)
	}
	if entry.EntryType != string(evidence.EntryTypePrescribe) {
		return store.StoredEntry{}, wrapError(ErrCodeInvalidInput, fmt.Sprintf("entry %q is not a prescribe entry", prescriptionID), nil)
	}
	return entry, nil
}

func buildPrescribeEntry(lastHash string, signer evidence.Signer, in PrescribeRequest) (evidence.EvidenceEntry, string, error) {
	state, err := buildPrescribePayload(in)
	if err != nil {
		return evidence.EvidenceEntry{}, "", err
	}

	entry, err := evidence.BuildEntry(evidence.EntryBuildParams{
		EntryID:         state.entryID,
		Type:            evidence.EntryTypePrescribe,
		SessionID:       strings.TrimSpace(in.SessionID),
		OperationID:     strings.TrimSpace(in.OperationID),
		TraceID:         strings.TrimSpace(in.TraceID),
		SpanID:          strings.TrimSpace(in.SpanID),
		ParentSpanID:    strings.TrimSpace(in.ParentSpanID),
		Actor:           in.Actor,
		IntentDigest:    state.intentDigest,
		ArtifactDigest:  state.artifactDigest,
		Payload:         state.payload,
		PreviousHash:    lastHash,
		ScopeDimensions: in.ScopeDimensions,
		SpecVersion:     version.SpecVersion,
		CanonVersion:    state.canonVersion,
		AdapterVersion:  version.Version,
		ScoringVersion:  version.ScoringVersion,
		Signer:          signer,
	})
	if err != nil {
		return evidence.EvidenceEntry{}, "", wrapError(ErrCodeInternal, err.Error(), err)
	}
	return entry, state.effectiveRisk, nil
}

type ingestPersistence interface {
	LastHash(ctx context.Context, tenantID string) (string, error)
	SaveRaw(ctx context.Context, tenantID string, raw json.RawMessage) (string, error)
}

func (s *Service) saveEntry(ctx context.Context, persist ingestPersistence, tenantID string, build func(lastHash string) (evidence.EvidenceEntry, error)) (evidence.EvidenceEntry, error) {
	lastHash, err := persist.LastHash(ctx, tenantID)
	if err != nil {
		return evidence.EvidenceEntry{}, wrapError(ErrCodeInternal, "failed to read last hash", err)
	}

	entry, err := build(lastHash)
	if err != nil {
		return evidence.EvidenceEntry{}, err
	}

	raw, err := json.Marshal(entry)
	if err != nil {
		return evidence.EvidenceEntry{}, wrapError(ErrCodeInternal, "failed to marshal evidence entry", err)
	}
	if _, err := persist.SaveRaw(ctx, tenantID, raw); err != nil {
		return evidence.EvidenceEntry{}, wrapError(ErrCodeInternal, "failed to store evidence entry", err)
	}
	return entry, nil
}

func buildReportEntry(lastHash string, signer evidence.Signer, in ReportRequest, payload evidence.ReportPayload, prescription store.StoredEntry, softResolve bool) (evidence.EvidenceEntry, error) {
	entryID := ulid.Make().String()
	payload.ReportID = entryID

	sessionID := strings.TrimSpace(in.SessionID)
	if sessionID == "" {
		sessionID = strings.TrimSpace(prescription.SessionID)
	}
	if !softResolve {
		if prescriptionSession := strings.TrimSpace(prescription.SessionID); sessionID != "" && prescriptionSession != "" && sessionID != prescriptionSession {
			return evidence.EvidenceEntry{}, wrapError(ErrCodeInvalidInput, fmt.Sprintf("report session_id %q does not match prescription session_id %q", sessionID, prescriptionSession), nil)
		}
	}
	operationID := strings.TrimSpace(in.OperationID)
	if operationID == "" {
		operationID = strings.TrimSpace(prescription.OperationID)
	}
	traceID := strings.TrimSpace(in.TraceID)
	if traceID == "" {
		traceID = sessionID
	}
	if traceID == "" {
		traceID = prescription.ID
	}

	rawPayload, err := json.Marshal(payload)
	if err != nil {
		return evidence.EvidenceEntry{}, wrapError(ErrCodeInternal, "failed to marshal report payload", err)
	}

	entry, err := evidence.BuildEntry(evidence.EntryBuildParams{
		EntryID:         entryID,
		Type:            evidence.EntryTypeReport,
		SessionID:       sessionID,
		OperationID:     operationID,
		TraceID:         traceID,
		SpanID:          strings.TrimSpace(in.SpanID),
		ParentSpanID:    strings.TrimSpace(in.ParentSpanID),
		Actor:           in.Actor,
		ArtifactDigest:  strings.TrimSpace(in.ArtifactDigest),
		Payload:         rawPayload,
		PreviousHash:    lastHash,
		ScopeDimensions: in.ScopeDimensions,
		SpecVersion:     version.SpecVersion,
		CanonVersion:    "external/v1",
		AdapterVersion:  version.Version,
		ScoringVersion:  version.ScoringVersion,
		Signer:          signer,
	})
	if err != nil {
		return evidence.EvidenceEntry{}, wrapError(ErrCodeInternal, err.Error(), err)
	}
	return entry, nil
}

type prescribePayloadState struct {
	payload        json.RawMessage
	entryID        string
	intentDigest   string
	artifactDigest string
	canonVersion   string
	effectiveRisk  string
}

func buildPrescribePayload(in PrescribeRequest) (prescribePayloadState, error) {
	var (
		payload      evidence.PrescriptionPayload
		action       evidence.CanonicalAction
		canonVersion string
		entryID      string
		intent       evidence.DeclaredIntent
		useIntent    bool
	)

	switch {
	case in.PayloadOverride != nil:
		if err := json.Unmarshal(*in.PayloadOverride, &payload); err != nil {
			return prescribePayloadState{}, wrapError(ErrCodeInvalidInput, "payload_override must be valid prescribe payload JSON", err)
		}
		switch {
		case len(payload.CanonicalAction) > 0:
			if err := json.Unmarshal(payload.CanonicalAction, &action); err != nil {
				return prescribePayloadState{}, wrapError(ErrCodeInvalidInput, "payload_override canonical_action is invalid", err)
			}
			normalized, err := normalizeCanonicalAction(action)
			if err != nil {
				return prescribePayloadState{}, err
			}
			action = normalized
			canonVersion = "external/v1"
		case payload.Intent != nil:
			var err error
			intent, err = normalizeDeclaredIntent(*payload.Intent, strings.TrimSpace(in.ArtifactDigest))
			if err != nil {
				return prescribePayloadState{}, err
			}
			useIntent = true
		default:
			return prescribePayloadState{}, wrapError(ErrCodeInvalidInput, "payload_override must include intent or canonical_action", nil)
		}
		entryID = strings.TrimSpace(in.PrescriptionID)
		if entryID == "" {
			entryID = strings.TrimSpace(payload.PrescriptionID)
		}
	case in.Intent != nil:
		var err error
		intent, err = normalizeDeclaredIntent(*in.Intent, strings.TrimSpace(in.ArtifactDigest))
		if err != nil {
			return prescribePayloadState{}, err
		}
		useIntent = true
		entryID = strings.TrimSpace(in.PrescriptionID)
	case in.CanonicalAction != nil:
		normalized, err := normalizeCanonicalAction(*in.CanonicalAction)
		if err != nil {
			return prescribePayloadState{}, err
		}
		action = normalized
		canonVersion = "external/v1"
		entryID = strings.TrimSpace(in.PrescriptionID)
	case in.SmartTarget != nil:
		normalized, err := normalizeDeclaredIntent(smartTargetDeclaredIntent(*in.SmartTarget), strings.TrimSpace(in.ArtifactDigest))
		if err != nil {
			return prescribePayloadState{}, err
		}
		intent = normalized
		useIntent = true
		entryID = strings.TrimSpace(in.PrescriptionID)
	default:
		return prescribePayloadState{}, wrapError(ErrCodeInvalidInput, "intent, canonical_action, or smart_target is required", nil)
	}

	if strings.TrimSpace(entryID) == "" {
		entryID = ulid.Make().String()
	}
	payload.PrescriptionID = entryID
	payload.Flavor = in.Flavor
	payload.Evidence = payloadEvidenceMetadata(in.Evidence)
	payload.Source = payloadSourceMetadata(in.Source)

	if useIntent {
		assessment, err := normalizeIngestAssessment(firstAssessment(in.Assessment, payload.Assessment))
		if err != nil {
			return prescribePayloadState{}, err
		}
		payload.Intent = &intent
		payload.Assessment = assessment
		if assessment.Status == evidence.AssessmentProvided {
			payload.RiskInputs = assessment.RiskInputs
			payload.EffectiveRisk = assessment.EffectiveRisk
		}
		if payload.TTLMs == 0 {
			payload.TTLMs = evidence.DefaultTTLMs
		}
		rawPayload, err := json.Marshal(payload)
		if err != nil {
			return prescribePayloadState{}, wrapError(ErrCodeInternal, "failed to marshal prescribe payload", err)
		}
		return prescribePayloadState{
			payload:        rawPayload,
			entryID:        entryID,
			intentDigest:   evidence.ComputeDeclaredIntentDigest(intent),
			artifactDigest: intent.ArtifactDigest,
			effectiveRisk:  assessment.EffectiveRisk,
		}, nil
	}

	assessment, err := normalizeIngestAssessment(firstAssessment(in.Assessment, payload.Assessment))
	if err != nil {
		return prescribePayloadState{}, err
	}
	rawAction, err := json.Marshal(action)
	if err != nil {
		return prescribePayloadState{}, wrapError(ErrCodeInternal, "failed to marshal canonical action", err)
	}
	payload.CanonicalAction = rawAction
	payload.Assessment = assessment
	if assessment.Status == evidence.AssessmentProvided {
		payload.RiskInputs = assessment.RiskInputs
		payload.EffectiveRisk = assessment.EffectiveRisk
	} else {
		payload.RiskInputs = nil
		payload.EffectiveRisk = ""
	}
	if payload.TTLMs == 0 {
		payload.TTLMs = evidence.DefaultTTLMs
	}
	if payload.CanonSource == "" {
		payload.CanonSource = "external"
	}

	rawPayload, err := json.Marshal(payload)
	if err != nil {
		return prescribePayloadState{}, wrapError(ErrCodeInternal, "failed to marshal prescribe payload", err)
	}
	return prescribePayloadState{
		payload:        rawPayload,
		entryID:        entryID,
		intentDigest:   evidence.ComputeCanonicalActionDigest(action),
		artifactDigest: strings.TrimSpace(in.ArtifactDigest),
		canonVersion:   canonVersion,
		effectiveRisk:  payload.EffectiveRisk,
	}, nil
}

func buildReportPayloadState(in ReportRequest) (evidence.ReportPayload, string, error) {
	if in.PayloadOverride != nil {
		return parseReportPayloadOverride(*in.PayloadOverride, in.Envelope)
	}

	payload := evidence.ReportPayload{
		PrescriptionID:  strings.TrimSpace(in.PrescriptionID),
		ExitCode:        in.ExitCode,
		Verdict:         in.Verdict,
		DecisionContext: in.DecisionContext,
		ExternalRefs:    in.ExternalRefs,
		Flavor:          in.Flavor,
		Evidence:        payloadEvidenceMetadata(in.Evidence),
		Source:          payloadSourceMetadata(in.Source),
	}
	return payload, payload.PrescriptionID, nil
}

func normalizeDeclaredIntent(in evidence.DeclaredIntent, fallbackArtifactDigest string) (evidence.DeclaredIntent, error) {
	out := evidence.DeclaredIntent{
		Tool:           normalizeToken(in.Tool),
		Operation:      normalizeToken(in.Operation),
		Target:         strings.TrimSpace(in.Target),
		Command:        strings.TrimSpace(in.Command),
		ArtifactDigest: strings.TrimSpace(in.ArtifactDigest),
	}
	if out.ArtifactDigest == "" {
		out.ArtifactDigest = strings.TrimSpace(fallbackArtifactDigest)
	}
	if strings.TrimSpace(out.Tool) == "" &&
		strings.TrimSpace(out.Operation) == "" &&
		strings.TrimSpace(out.Target) == "" &&
		strings.TrimSpace(out.Command) == "" &&
		strings.TrimSpace(out.ArtifactDigest) == "" {
		return evidence.DeclaredIntent{}, wrapError(ErrCodeInvalidInput, "intent must not be empty", nil)
	}
	if err := evidence.ValidateDigest(out.ArtifactDigest); err != nil {
		return evidence.DeclaredIntent{}, wrapError(ErrCodeInvalidInput, err.Error(), err)
	}
	return out, nil
}

func firstAssessment(primary, fallback *evidence.AssessmentPayload) *evidence.AssessmentPayload {
	if primary != nil {
		return primary
	}
	return fallback
}

func normalizeIngestAssessment(in *evidence.AssessmentPayload) (*evidence.AssessmentPayload, error) {
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

func parseReportPayloadOverride(raw json.RawMessage, env Envelope) (evidence.ReportPayload, string, error) {
	var payload evidence.ReportPayload
	if err := json.Unmarshal(raw, &payload); err != nil {
		return evidence.ReportPayload{}, "", wrapError(ErrCodeInvalidInput, "payload_override must be valid report payload JSON", err)
	}
	if err := validateReportPayloadBody(payload); err != nil {
		return evidence.ReportPayload{}, "", err
	}

	payload.Flavor = env.Flavor
	payload.Evidence = payloadEvidenceMetadata(env.Evidence)
	payload.Source = payloadSourceMetadata(env.Source)

	return payload, strings.TrimSpace(payload.PrescriptionID), nil
}

func validateReportPayloadBody(payload evidence.ReportPayload) error {
	if strings.TrimSpace(payload.PrescriptionID) == "" {
		return wrapError(ErrCodeInvalidInput, "payload_override prescription_id is required", nil)
	}
	if !payload.Verdict.Valid() {
		return wrapError(ErrCodeInvalidInput, "payload_override verdict is required and must be one of success, failure, error, declined", nil)
	}
	if payload.Verdict == evidence.VerdictDeclined {
		if payload.ExitCode != nil {
			return wrapError(ErrCodeInvalidInput, "declined reports must not include exit_code", nil)
		}
		if payload.DecisionContext == nil {
			return wrapError(ErrCodeInvalidInput, "decision_context is required for declined reports", nil)
		}
		if strings.TrimSpace(payload.DecisionContext.Trigger) == "" {
			return wrapError(ErrCodeInvalidInput, "decision_context.trigger is required", nil)
		}
		if strings.TrimSpace(payload.DecisionContext.Reason) == "" {
			return wrapError(ErrCodeInvalidInput, "decision_context.reason is required", nil)
		}
		if len(strings.TrimSpace(payload.DecisionContext.Reason)) > 512 {
			return wrapError(ErrCodeInvalidInput, "decision_context.reason exceeds 512 characters", nil)
		}
		return nil
	}
	if payload.DecisionContext != nil {
		return wrapError(ErrCodeInvalidInput, "decision_context is only valid for declined reports", nil)
	}
	if payload.ExitCode == nil {
		return wrapError(ErrCodeInvalidInput, fmt.Sprintf("report verdict %s requires exit_code", payload.Verdict), nil)
	}
	if inferred := evidence.VerdictFromExitCode(*payload.ExitCode); inferred != payload.Verdict {
		return wrapError(ErrCodeInvalidInput, fmt.Sprintf("report verdict %s does not match exit_code %d", payload.Verdict, *payload.ExitCode), nil)
	}
	return nil
}

func smartTargetDeclaredIntent(target SmartTarget) evidence.DeclaredIntent {
	resource := strings.TrimSpace(target.Resource)
	if namespace := strings.TrimSpace(target.Namespace); namespace != "" && resource != "" {
		resource = namespace + "/" + resource
	}
	return evidence.DeclaredIntent{
		Tool:      strings.TrimSpace(target.Tool),
		Operation: strings.TrimSpace(target.Operation),
		Target:    resource,
	}
}

func normalizeCanonicalAction(action evidence.CanonicalAction) (evidence.CanonicalAction, error) {
	action.Tool = normalizeToken(action.Tool)
	action.Operation = normalizeToken(action.Operation)
	action.OperationClass = strings.TrimSpace(action.OperationClass)
	action.ResourceShapeHash = strings.TrimSpace(action.ResourceShapeHash)

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
		fmt.Sprintf("invalid canonical_action.scope_class %q; expected one of production, staging, development, unknown (aliases: prod, stage, dev, test, sandbox)", v),
		nil,
	)
}

func payloadEvidenceMetadata(meta *EvidenceMetadata) *evidence.EvidenceMetadata {
	if meta == nil || strings.TrimSpace(string(meta.Kind)) == "" {
		return nil
	}
	return &evidence.EvidenceMetadata{Kind: meta.Kind}
}

func payloadSourceMetadata(meta *SourceMetadata) *evidence.SourceMetadata {
	if meta == nil {
		return nil
	}
	system := strings.TrimSpace(meta.System)
	if system == "" {
		return nil
	}
	return &evidence.SourceMetadata{System: system}
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

func normalizeToken(v string) string {
	return strings.ToLower(strings.TrimSpace(v))
}

func (s *Service) claimIfNeeded(ctx context.Context, tx store.IngestTx, tenantID, claimSource string, claim *Claim) (bool, error) {
	if !hasMeaningfulClaim(claim) {
		return false, nil
	}

	duplicate, err := tx.ClaimWebhookEvent(ctx, tenantID, claimSource, claim.Key, claim.Payload)
	if err != nil {
		return false, wrapError(ErrCodeInternal, "failed to claim ingest event", err)
	}
	if duplicate {
		return true, nil
	}
	return false, nil
}

func hasMeaningfulClaim(claim *Claim) bool {
	return claim != nil && (strings.TrimSpace(claim.Source) != "" || strings.TrimSpace(claim.Key) != "" || len(claim.Payload) > 0)
}

func (s *Service) loadDuplicatePrescribeResult(ctx context.Context, tx store.IngestTx, tenantID, claimSource string, claim *Claim) (Result, error) {
	return s.loadDuplicateResult(ctx, tx, tenantID, claimSource, claim, s.allowLegacyDuplicateFallback, func(entry store.StoredEntry) (Result, error) {
		decoded, err := decodeStoredEvidenceEntry(entry)
		if err != nil {
			return Result{}, err
		}
		effectiveRisk := ""
		return Result{
			Duplicate:      true,
			EntryID:        decoded.EntryID,
			EffectiveRisk:  effectiveRisk,
			PrescriptionID: decoded.EntryID,
			Entry:          decoded,
		}, nil
	})
}

func (s *Service) loadDuplicateReportResult(ctx context.Context, tx store.IngestTx, tenantID, claimSource string, claim *Claim) (Result, error) {
	return s.loadDuplicateResult(ctx, tx, tenantID, claimSource, claim, s.allowLegacyDuplicateFallback, func(entry store.StoredEntry) (Result, error) {
		decoded, err := decodeStoredEvidenceEntry(entry)
		if err != nil {
			return Result{}, err
		}
		return Result{
			Duplicate: true,
			EntryID:   decoded.EntryID,
			Entry:     decoded,
		}, nil
	})
}

func (s *Service) loadDuplicateResult(ctx context.Context, tx store.IngestTx, tenantID, claimSource string, claim *Claim, allowLegacyFallback bool, build func(store.StoredEntry) (Result, error)) (Result, error) {
	if !hasMeaningfulClaim(claim) {
		return Result{Duplicate: true}, nil
	}

	meta, err := tx.GetWebhookEventResult(ctx, tenantID, claimSource, claim.Key)
	if err != nil {
		return Result{}, err
	}
	if strings.TrimSpace(meta.EntryID) == "" {
		if allowLegacyFallback {
			return Result{Duplicate: true}, nil
		}
		return Result{}, wrapError(ErrCodeInternal, "duplicate ingest claim missing result entry id", nil)
	}

	entry, err := tx.GetEntry(ctx, tenantID, meta.EntryID)
	if err != nil {
		if errorsIsNotFound(err) {
			return Result{}, wrapError(ErrCodeInternal, "duplicate ingest result entry not found", err)
		}
		return Result{}, err
	}

	result, err := build(entry)
	if err != nil {
		return Result{}, err
	}
	result.EntryID = meta.EntryID
	result.EffectiveRisk = meta.EffectiveRisk
	return result, nil
}

func (s *Service) finalizeClaim(ctx context.Context, tx store.IngestTx, tenantID, claimSource string, claim *Claim, entryID, effectiveRisk string) error {
	if !hasMeaningfulClaim(claim) {
		return nil
	}
	if err := tx.FinalizeWebhookEvent(ctx, tenantID, claimSource, claim.Key, entryID, effectiveRisk); err != nil {
		if errorsIsNotFound(err) {
			return wrapError(ErrCodeInternal, "failed to finalize ingest claim", err)
		}
		return wrapError(ErrCodeInternal, "failed to finalize ingest claim", err)
	}
	return nil
}

func (s *Service) claimSource(claim *Claim) string {
	if claim == nil {
		return ""
	}
	source := strings.TrimSpace(claim.Source)
	if source == "" {
		return ""
	}
	return s.claimNamespacePrefix + source
}

func reportUsesSoftResolution(in ReportRequest) bool {
	return in.Evidence != nil && in.Evidence.Kind == evidence.EvidenceKindTranslated
}

func decodeStoredEvidenceEntry(entry store.StoredEntry) (evidence.EvidenceEntry, error) {
	var decoded evidence.EvidenceEntry
	if err := json.Unmarshal(entry.Payload, &decoded); err != nil {
		return evidence.EvidenceEntry{}, wrapError(ErrCodeInternal, "failed to decode stored evidence entry", err)
	}
	return decoded, nil
}

func errorsIsNotFound(err error) bool {
	return errors.Is(err, store.ErrNotFound)
}
