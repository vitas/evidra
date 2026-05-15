package api

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	iauth "samebits.com/evidra/internal/auth"
	"samebits.com/evidra/internal/ingest"
	"samebits.com/evidra/internal/store"
	pkevidence "samebits.com/evidra/pkg/evidence"
)

type WebhookStore interface {
	ingest.Store
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

func handleGenericWebhookWithTenantResolver(store WebhookStore, signer pkevidence.Signer, secret string, resolveTenant WebhookTenantResolver) http.HandlerFunc {
	svc := ingest.NewWebhookService(store, signer)

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

		switch payload.EventType {
		case "operation_started":
			req, err := buildGenericWebhookPrescribeRequest(payload, body)
			if err != nil {
				writeError(w, http.StatusBadRequest, err.Error())
				return
			}
			result, err := svc.Prescribe(r.Context(), tenantID, req)
			writeWebhookIngestResult(w, result, err)
		case "operation_completed":
			req, err := buildGenericWebhookReportRequest(payload, body)
			if err != nil {
				writeError(w, http.StatusBadRequest, err.Error())
				return
			}
			result, err := svc.Report(r.Context(), tenantID, req)
			writeWebhookIngestResult(w, result, err)
		default:
			writeError(w, http.StatusBadRequest, "unsupported event_type")
		}
	}
}

func handleArgoCDWebhookWithTenantResolver(store WebhookStore, signer pkevidence.Signer, secret string, resolveTenant WebhookTenantResolver) http.HandlerFunc {
	svc := ingest.NewWebhookService(store, signer)

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

		switch payload.Event {
		case "sync_started":
			req, err := buildArgoCDWebhookPrescribeRequest(payload, body)
			if err != nil {
				writeError(w, http.StatusBadRequest, err.Error())
				return
			}
			result, err := svc.Prescribe(r.Context(), tenantID, req)
			writeWebhookIngestResult(w, result, err)
		case "sync_completed":
			req, err := buildArgoCDWebhookReportRequest(payload, body)
			if err != nil {
				writeError(w, http.StatusBadRequest, err.Error())
				return
			}
			result, err := svc.Report(r.Context(), tenantID, req)
			writeWebhookIngestResult(w, result, err)
		default:
			writeError(w, http.StatusBadRequest, "unsupported argocd event")
		}
	}
}

func buildGenericWebhookPrescribeRequest(payload genericWebhookPayload, body json.RawMessage) (ingest.PrescribeRequest, error) {
	if strings.TrimSpace(payload.Tool) == "" || strings.TrimSpace(payload.Operation) == "" {
		return ingest.PrescribeRequest{}, fmt.Errorf("tool and operation are required")
	}
	operationID := strings.TrimSpace(payload.OperationID)
	if operationID == "" {
		return ingest.PrescribeRequest{}, fmt.Errorf("operation_id is required")
	}
	sessionID := strings.TrimSpace(payload.SessionID)
	if sessionID == "" {
		sessionID = operationID
	}
	idempotencyKey := strings.TrimSpace(payload.IdempotencyKey)
	if idempotencyKey == "" {
		idempotencyKey = "generic:" + operationID + ":start"
	}
	action := mappedCanonicalAction(payload.Tool, payload.Operation, payload.Environment)
	return ingest.PrescribeRequest{
		Envelope: ingest.Envelope{
			ContractVersion: ingest.ContractVersionV1,
			Claim: &ingest.Claim{
				Source:  "generic",
				Key:     idempotencyKey,
				Payload: body,
			},
			Actor:           mappedActor(payload.Actor, "generic"),
			SessionID:       sessionID,
			OperationID:     operationID,
			TraceID:         sessionID,
			ScopeDimensions: mappedScopeDimensions("generic", payload.Environment, map[string]string{}),
			Flavor:          pkevidence.FlavorImperative,
			Evidence:        &pkevidence.EvidenceMetadata{Kind: pkevidence.EvidenceKindTranslated},
			Source:          &pkevidence.SourceMetadata{System: "generic"},
		},
		PrescriptionID:  mappedPrescriptionID("generic", payload.Tool, payload.Operation, "", operationID, payload.Environment, ""),
		ArtifactDigest:  pkevidence.SHA256Hex(body),
		CanonicalAction: &action,
	}, nil
}

func buildGenericWebhookReportRequest(payload genericWebhookPayload, body json.RawMessage) (ingest.ReportRequest, error) {
	if strings.TrimSpace(payload.Tool) == "" || strings.TrimSpace(payload.Operation) == "" {
		return ingest.ReportRequest{}, fmt.Errorf("tool and operation are required")
	}
	operationID := strings.TrimSpace(payload.OperationID)
	if operationID == "" {
		return ingest.ReportRequest{}, fmt.Errorf("operation_id is required")
	}
	if strings.TrimSpace(payload.IdempotencyKey) == "" {
		return ingest.ReportRequest{}, fmt.Errorf("idempotency_key is required for operation_completed")
	}
	exitCode := payload.ExitCode
	if exitCode == nil {
		defaultCode := exitCodeForVerdict(payload.Verdict)
		exitCode = &defaultCode
	}
	sessionID := strings.TrimSpace(payload.SessionID)
	if sessionID == "" {
		sessionID = operationID
	}
	return ingest.ReportRequest{
		Envelope: ingest.Envelope{
			ContractVersion: ingest.ContractVersionV1,
			Claim: &ingest.Claim{
				Source:  "generic",
				Key:     strings.TrimSpace(payload.IdempotencyKey),
				Payload: body,
			},
			Actor:           mappedActor(payload.Actor, "generic"),
			SessionID:       sessionID,
			OperationID:     operationID,
			TraceID:         sessionID,
			ScopeDimensions: mappedScopeDimensions("generic", payload.Environment, map[string]string{}),
			Flavor:          pkevidence.FlavorImperative,
			Evidence:        &pkevidence.EvidenceMetadata{Kind: pkevidence.EvidenceKindTranslated},
			Source:          &pkevidence.SourceMetadata{System: "generic"},
		},
		PrescriptionID: mappedPrescriptionID("generic", payload.Tool, payload.Operation, "", operationID, payload.Environment, ""),
		ArtifactDigest: pkevidence.SHA256Hex(body),
		Verdict:        payload.Verdict,
		ExitCode:       exitCode,
	}, nil
}

func buildArgoCDWebhookPrescribeRequest(payload argoCDWebhookPayload, body json.RawMessage) (ingest.PrescribeRequest, error) {
	if strings.TrimSpace(payload.AppName) == "" || strings.TrimSpace(payload.OperationID) == "" {
		return ingest.PrescribeRequest{}, fmt.Errorf("app_name and operation_id are required")
	}
	operationID := strings.TrimSpace(payload.OperationID)
	action := mappedCanonicalAction("argocd", "sync", payload.AppNamespace)
	return ingest.PrescribeRequest{
		Envelope: ingest.Envelope{
			ContractVersion: ingest.ContractVersionV1,
			Claim: &ingest.Claim{
				Source:  "argocd_start",
				Key:     payload.AppName + ":" + operationID,
				Payload: body,
			},
			Actor:           mappedActor(payload.InitiatedBy, "argocd"),
			SessionID:       operationID,
			OperationID:     operationID,
			TraceID:         operationID,
			ScopeDimensions: mappedScopeDimensions("argocd", payload.AppNamespace, map[string]string{"application": payload.AppName, "revision": payload.Revision}),
			Flavor:          pkevidence.FlavorReconcile,
			Evidence:        &pkevidence.EvidenceMetadata{Kind: pkevidence.EvidenceKindTranslated},
			Source:          &pkevidence.SourceMetadata{System: "argocd"},
		},
		PrescriptionID:  mappedPrescriptionID("argocd", payload.AppName, "sync", payload.InitiatedBy, operationID, payload.AppNamespace, ""),
		ArtifactDigest:  pkevidence.SHA256Hex(body),
		CanonicalAction: &action,
	}, nil
}

func buildArgoCDWebhookReportRequest(payload argoCDWebhookPayload, body json.RawMessage) (ingest.ReportRequest, error) {
	if strings.TrimSpace(payload.AppName) == "" || strings.TrimSpace(payload.OperationID) == "" {
		return ingest.ReportRequest{}, fmt.Errorf("app_name and operation_id are required")
	}
	verdict, exitCode, ok := argoCDVerdict(payload.Phase)
	if !ok {
		return ingest.ReportRequest{}, fmt.Errorf("unsupported argocd phase")
	}
	operationID := strings.TrimSpace(payload.OperationID)
	return ingest.ReportRequest{
		Envelope: ingest.Envelope{
			ContractVersion: ingest.ContractVersionV1,
			Claim: &ingest.Claim{
				Source:  "argocd_complete",
				Key:     payload.AppName + ":" + operationID + ":complete",
				Payload: body,
			},
			Actor:           mappedActor(payload.InitiatedBy, "argocd"),
			SessionID:       operationID,
			OperationID:     operationID,
			TraceID:         operationID,
			ScopeDimensions: mappedScopeDimensions("argocd", payload.AppNamespace, map[string]string{"application": payload.AppName, "revision": payload.Revision}),
			Flavor:          pkevidence.FlavorReconcile,
			Evidence:        &pkevidence.EvidenceMetadata{Kind: pkevidence.EvidenceKindTranslated},
			Source:          &pkevidence.SourceMetadata{System: "argocd"},
		},
		PrescriptionID: mappedPrescriptionID("argocd", payload.AppName, "sync", payload.InitiatedBy, operationID, payload.AppNamespace, ""),
		ArtifactDigest: pkevidence.SHA256Hex(body),
		Verdict:        verdict,
		ExitCode:       &exitCode,
		ExternalRefs: []pkevidence.ExternalRef{
			{Type: "argocd_application", ID: payload.AppNamespace + "/" + payload.AppName},
			{Type: "argocd_revision", ID: payload.Revision},
			{Type: "argocd_operation", ID: operationID},
		},
	}, nil
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

func writeWebhookIngestResult(w http.ResponseWriter, result ingest.Result, err error) {
	if err != nil {
		writeIngestServiceError(w, err)
		return
	}
	if result.Duplicate {
		writeJSON(w, http.StatusOK, map[string]string{"status": "duplicate"})
		return
	}
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

func mappedCanonicalAction(tool, operation, environment string) pkevidence.CanonicalAction {
	scope := pkevidence.NormalizeScopeClass(environment)
	return pkevidence.CanonicalAction{
		Tool:              strings.TrimSpace(tool),
		Operation:         strings.TrimSpace(operation),
		OperationClass:    mappedOperationClass(operation),
		ScopeClass:        scope,
		ResourceCount:     1,
		ResourceShapeHash: pkevidence.SHA256Hex([]byte(tool + "|" + operation + "|" + scope)),
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
	return "map-" + pkevidence.SHA256Hex([]byte(strings.Join(parts, "|")))
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
