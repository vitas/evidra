package api

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/oklog/ulid/v2"

	iauth "samebits.com/evidra/internal/auth"
	"samebits.com/evidra/internal/canon"
	"samebits.com/evidra/internal/risk"
	"samebits.com/evidra/internal/store"
	pkevidence "samebits.com/evidra/pkg/evidence"
	"samebits.com/evidra/pkg/version"
)

type WebhookStore interface {
	LastHash(ctx context.Context, tenantID string) (string, error)
	SaveRaw(ctx context.Context, tenantID string, raw json.RawMessage) (string, error)
	ClaimWebhookEvent(ctx context.Context, tenantID, source, key string, payload json.RawMessage) (bool, error)
	ReleaseWebhookEvent(ctx context.Context, tenantID, source, key string) error
}

type WebhookTenantResolver func(ctx context.Context, apiKey string) (string, error)

const webhookTenantAPIKeyHeader = "X-Evidra-API-Key"

type genericWebhookPayload struct {
	EventType      string             `json:"event_type"`
	Tool           string             `json:"tool"`
	Operation      string             `json:"operation"`
	OperationID    string             `json:"operation_id"`
	Environment    string             `json:"environment"`
	Actor          string             `json:"actor"`
	SessionID      string             `json:"session_id"`
	ExitCode       *int               `json:"exit_code,omitempty"`
	Verdict        pkevidence.Verdict `json:"verdict,omitempty"`
	IdempotencyKey string             `json:"idempotency_key,omitempty"`
}

type argoCDWebhookPayload struct {
	Event        string `json:"event"`
	AppName      string `json:"app_name"`
	AppNamespace string `json:"app_namespace"`
	Revision     string `json:"revision"`
	InitiatedBy  string `json:"initiated_by"`
	OperationID  string `json:"operation_id"`
	Phase        string `json:"phase"`
	Message      string `json:"message"`
}

type mappedWebhookBuilder func(lastHash string) (pkevidence.EvidenceEntry, int, error)

func handleGenericWebhookWithTenantResolver(store WebhookStore, signer pkevidence.Signer, secret string, resolveTenant WebhookTenantResolver) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		body, ok := webhookRequestBody(w, r, secret, signer)
		if !ok {
			return
		}
		tenantID, ok := resolveWebhookTenant(w, r, resolveTenant)
		if !ok {
			return
		}

		var payload genericWebhookPayload
		if err := json.Unmarshal(body, &payload); err != nil {
			writeError(w, http.StatusBadRequest, "invalid JSON")
			return
		}
		if strings.TrimSpace(payload.Tool) == "" || strings.TrimSpace(payload.Operation) == "" {
			writeError(w, http.StatusBadRequest, "tool and operation are required")
			return
		}
		operationID := strings.TrimSpace(payload.OperationID)
		if operationID == "" {
			writeError(w, http.StatusBadRequest, "operation_id is required")
			return
		}
		if payload.EventType != "operation_started" && payload.EventType != "operation_completed" {
			writeError(w, http.StatusBadRequest, "unsupported event_type")
			return
		}
		if payload.EventType == "operation_completed" && strings.TrimSpace(payload.IdempotencyKey) == "" {
			writeError(w, http.StatusBadRequest, "idempotency_key is required for operation_completed")
			return
		}

		idempotencyKey := strings.TrimSpace(payload.IdempotencyKey)
		if idempotencyKey == "" {
			idempotencyKey = "generic:" + operationID + ":start"
		}

		action := mappedCanonicalAction(payload.Tool, payload.Operation, payload.Environment)
		actor := mappedActor(payload.Actor, "generic")
		sessionID := strings.TrimSpace(payload.SessionID)
		if sessionID == "" {
			sessionID = operationID
		}
		prescriptionID := mappedPrescriptionID("generic", payload.Tool, payload.Operation, "", operationID, payload.Environment, "")
		scope := mappedScopeDimensions("generic", payload.Environment, map[string]string{})
		artifactDigest := canon.SHA256Hex(body)

		processMappedWebhook(w, r, store, tenantID, "generic", idempotencyKey, body, func(lastHash string) (pkevidence.EvidenceEntry, int, error) {
			if payload.EventType == "operation_started" {
				entry, err := buildMappedPrescribeEntry(lastHash, signer, actor, sessionID, operationID, prescriptionID, action, artifactDigest, scope)
				return entry, http.StatusInternalServerError, err
			}
			exitCode := payload.ExitCode
			if exitCode == nil {
				defaultCode := exitCodeForVerdict(payload.Verdict)
				exitCode = &defaultCode
			}
			entry, err := buildMappedReportEntry(lastHash, signer, actor, sessionID, operationID, prescriptionID, artifactDigest, scope, payload.Verdict, exitCode)
			return entry, http.StatusInternalServerError, err
		})
	}
}

func handleArgoCDWebhookWithTenantResolver(store WebhookStore, signer pkevidence.Signer, secret string, resolveTenant WebhookTenantResolver) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		body, ok := webhookRequestBody(w, r, secret, signer)
		if !ok {
			return
		}
		tenantID, ok := resolveWebhookTenant(w, r, resolveTenant)
		if !ok {
			return
		}

		var payload argoCDWebhookPayload
		if err := json.Unmarshal(body, &payload); err != nil {
			writeError(w, http.StatusBadRequest, "invalid JSON")
			return
		}
		if strings.TrimSpace(payload.AppName) == "" || strings.TrimSpace(payload.OperationID) == "" {
			writeError(w, http.StatusBadRequest, "app_name and operation_id are required")
			return
		}

		sourceKey := mappedPrescriptionID("argocd", payload.AppName, "sync", payload.InitiatedBy, payload.OperationID, payload.AppNamespace, "")
		idempotencyKey := payload.AppName + ":" + payload.OperationID
		source := "argocd_start"
		if payload.Event != "sync_started" {
			source = "argocd_complete"
			idempotencyKey += ":complete"
		}

		action := mappedCanonicalAction("argocd", "sync", payload.AppNamespace)
		actor := mappedActor(payload.InitiatedBy, "argocd")
		scope := mappedScopeDimensions("argocd", payload.AppNamespace, map[string]string{
			"application": payload.AppName,
			"revision":    payload.Revision,
		})
		artifactDigest := canon.SHA256Hex(body)

		processMappedWebhook(w, r, store, tenantID, source, idempotencyKey, body, func(lastHash string) (pkevidence.EvidenceEntry, int, error) {
			switch payload.Event {
			case "sync_started":
				entry, err := buildMappedPrescribeEntry(lastHash, signer, actor, payload.OperationID, payload.OperationID, sourceKey, action, artifactDigest, scope)
				return entry, http.StatusInternalServerError, err
			case "sync_completed":
				verdict, exitCode, ok := argoCDVerdict(payload.Phase)
				if !ok {
					return pkevidence.EvidenceEntry{}, http.StatusBadRequest, fmt.Errorf("unsupported argocd phase")
				}
				entry, err := buildMappedReportEntry(lastHash, signer, actor, payload.OperationID, payload.OperationID, sourceKey, artifactDigest, scope, verdict, &exitCode)
				return entry, http.StatusInternalServerError, err
			default:
				return pkevidence.EvidenceEntry{}, http.StatusBadRequest, fmt.Errorf("unsupported argocd event")
			}
		})
	}
}

func resolveWebhookTenant(w http.ResponseWriter, r *http.Request, resolveTenant WebhookTenantResolver) (string, bool) {
	apiKey := strings.TrimSpace(r.Header.Get(webhookTenantAPIKeyHeader))
	if apiKey == "" {
		writeError(w, http.StatusUnauthorized, "missing tenant api key")
		return "", false
	}
	tenantID, err := resolveTenant(r.Context(), apiKey)
	if err != nil || strings.TrimSpace(tenantID) == "" {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return "", false
	}
	return tenantID, true
}

func tenantResolverFromKeyStore(ks interface {
	LookupKey(ctx context.Context, plaintext string) (store.KeyRecord, error)
}) WebhookTenantResolver {
	return func(ctx context.Context, apiKey string) (string, error) {
		rec, err := ks.LookupKey(ctx, apiKey)
		if err != nil {
			return "", err
		}
		return rec.TenantID, nil
	}
}

func claimWebhook(ctx context.Context, store WebhookStore, tenantID, source, key string, body json.RawMessage) (bool, func(), error) {
	duplicate, err := store.ClaimWebhookEvent(ctx, tenantID, source, key, body)
	if err != nil {
		return false, nil, err
	}
	released := false
	release := func() {
		if released {
			return
		}
		released = true
		_ = store.ReleaseWebhookEvent(ctx, tenantID, source, key)
	}
	if duplicate {
		released = true
		return true, release, nil
	}
	return false, release, nil
}

func webhookRequestBody(w http.ResponseWriter, r *http.Request, secret string, signer pkevidence.Signer) (json.RawMessage, bool) {
	if subtle.ConstantTimeCompare([]byte(strings.TrimSpace(iauth.ParseBearerToken(r.Header.Get("Authorization")))), []byte(secret)) != 1 {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return nil, false
	}
	if signer == nil {
		writeError(w, http.StatusServiceUnavailable, "webhook ingestion requires server signing")
		return nil, false
	}
	if ct := r.Header.Get("Content-Type"); ct != "" && !strings.HasPrefix(strings.ToLower(ct), "application/json") {
		writeError(w, http.StatusBadRequest, "content-type must be application/json")
		return nil, false
	}

	body, err := io.ReadAll(r.Body)
	if err != nil || len(body) == 0 {
		writeError(w, http.StatusBadRequest, "empty or unreadable body")
		return nil, false
	}
	return body, true
}

func processMappedWebhook(
	w http.ResponseWriter,
	r *http.Request,
	store WebhookStore,
	tenantID, source, idempotencyKey string,
	body json.RawMessage,
	build mappedWebhookBuilder,
) {
	duplicate, release, err := claimWebhook(r.Context(), store, tenantID, source, idempotencyKey, body)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "webhook idempotency failed")
		return
	}
	if duplicate {
		writeJSON(w, http.StatusOK, map[string]string{"status": "duplicate"})
		return
	}
	success := false
	defer func() {
		if !success {
			release()
		}
	}()

	lastHash, err := store.LastHash(r.Context(), tenantID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "load evidence chain failed")
		return
	}

	entry, status, err := build(lastHash)
	if err != nil {
		if status == 0 {
			status = http.StatusInternalServerError
		}
		if status == http.StatusInternalServerError {
			writeError(w, status, "build mapped evidence failed")
			return
		}
		writeError(w, status, err.Error())
		return
	}

	raw, err := json.Marshal(entry)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "encode mapped evidence failed")
		return
	}
	if _, err := store.SaveRaw(r.Context(), tenantID, raw); err != nil {
		writeError(w, http.StatusInternalServerError, "store mapped evidence failed")
		return
	}

	success = true
	writeJSON(w, http.StatusAccepted, map[string]string{"status": "accepted"})
}

func mappedActor(actorID, source string) pkevidence.Actor {
	actorID = strings.TrimSpace(actorID)
	if actorID == "" {
		actorID = source + "-controller"
	}
	return pkevidence.Actor{
		Type:       "controller",
		ID:         actorID,
		Provenance: "mapped:" + source,
	}
}

func mappedCanonicalAction(tool, operation, environment string) canon.CanonicalAction {
	scope := canon.NormalizeScopeClass(environment)
	return canon.CanonicalAction{
		Tool:              strings.TrimSpace(tool),
		Operation:         strings.TrimSpace(operation),
		OperationClass:    mappedOperationClass(operation),
		ScopeClass:        scope,
		ResourceCount:     1,
		ResourceShapeHash: canon.SHA256Hex([]byte(tool + "|" + operation + "|" + scope)),
	}
}

func mappedOperationClass(operation string) string {
	switch strings.ToLower(strings.TrimSpace(operation)) {
	case "delete", "destroy", "remove", "uninstall":
		return "destroy"
	case "get", "read", "describe", "list":
		return "read"
	case "plan":
		return "plan"
	default:
		return "mutate"
	}
}

func mappedScopeDimensions(source, environment string, extra map[string]string) map[string]string {
	scope := map[string]string{
		"source_kind":   "mapped",
		"source_system": source,
	}
	if environment != "" {
		scope["environment"] = environment
	}
	for k, v := range extra {
		if strings.TrimSpace(v) == "" {
			continue
		}
		scope[k] = v
	}
	return scope
}

func mappedPrescriptionID(source, tool, operation, actor, sessionID, environment, suffix string) string {
	parts := []string{source, tool, operation, actor, sessionID, environment, suffix}
	return "map-" + canon.SHA256Hex([]byte(strings.Join(parts, "|")))
}

func buildMappedPrescribeEntry(lastHash string, signer pkevidence.Signer, actor pkevidence.Actor, sessionID, operationID, prescriptionID string, action canon.CanonicalAction, artifactDigest string, scope map[string]string) (pkevidence.EvidenceEntry, error) {
	rawAction, err := json.Marshal(action)
	if err != nil {
		return pkevidence.EvidenceEntry{}, err
	}
	riskLevel := risk.ElevateRiskLevel(risk.RiskLevel(action.OperationClass, action.ScopeClass), nil)
	payload, err := json.Marshal(pkevidence.PrescriptionPayload{
		PrescriptionID:  prescriptionID,
		CanonicalAction: rawAction,
		RiskInputs: []pkevidence.RiskInput{
			{
				Source:    "evidra/matrix",
				RiskLevel: riskLevel,
			},
		},
		EffectiveRisk: riskLevel,
		RiskLevel:     riskLevel,
		TTLMs:         pkevidence.DefaultTTLMs,
		CanonSource:   "mapped",
	})
	if err != nil {
		return pkevidence.EvidenceEntry{}, err
	}
	traceID := sessionID
	if traceID == "" {
		traceID = prescriptionID
	}
	return pkevidence.BuildEntry(pkevidence.EntryBuildParams{
		EntryID:         prescriptionID,
		Type:            pkevidence.EntryTypePrescribe,
		SessionID:       sessionID,
		OperationID:     operationID,
		TraceID:         traceID,
		Actor:           actor,
		IntentDigest:    canon.ComputeIntentDigest(action),
		ArtifactDigest:  artifactDigest,
		Payload:         payload,
		PreviousHash:    lastHash,
		ScopeDimensions: scope,
		SpecVersion:     version.SpecVersion,
		CanonVersion:    "mapped/v1",
		AdapterVersion:  version.Version,
		ScoringVersion:  version.ScoringVersion,
		Signer:          signer,
	})
}

func buildMappedReportEntry(lastHash string, signer pkevidence.Signer, actor pkevidence.Actor, sessionID, operationID, prescriptionID, artifactDigest string, scope map[string]string, verdict pkevidence.Verdict, exitCode *int) (pkevidence.EvidenceEntry, error) {
	if !verdict.Valid() {
		return pkevidence.EvidenceEntry{}, fmt.Errorf("invalid verdict %q", verdict)
	}
	payload, err := json.Marshal(pkevidence.ReportPayload{
		ReportID:       ulid.Make().String(),
		PrescriptionID: prescriptionID,
		ExitCode:       exitCode,
		Verdict:        verdict,
	})
	if err != nil {
		return pkevidence.EvidenceEntry{}, err
	}
	traceID := sessionID
	if traceID == "" {
		traceID = prescriptionID
	}
	return pkevidence.BuildEntry(pkevidence.EntryBuildParams{
		Type:            pkevidence.EntryTypeReport,
		SessionID:       sessionID,
		OperationID:     operationID,
		TraceID:         traceID,
		Actor:           actor,
		ArtifactDigest:  artifactDigest,
		Payload:         payload,
		PreviousHash:    lastHash,
		ScopeDimensions: scope,
		SpecVersion:     version.SpecVersion,
		CanonVersion:    "mapped/v1",
		AdapterVersion:  version.Version,
		ScoringVersion:  version.ScoringVersion,
		Signer:          signer,
	})
}

func argoCDVerdict(phase string) (pkevidence.Verdict, int, bool) {
	switch strings.ToLower(strings.TrimSpace(phase)) {
	case "succeeded":
		return pkevidence.VerdictSuccess, 0, true
	case "failed":
		return pkevidence.VerdictFailure, 1, true
	case "error", "degraded":
		return pkevidence.VerdictError, -1, true
	default:
		return "", 0, false
	}
}

func exitCodeForVerdict(verdict pkevidence.Verdict) int {
	switch verdict {
	case pkevidence.VerdictSuccess:
		return 0
	case pkevidence.VerdictError:
		return -1
	default:
		return 1
	}
}
