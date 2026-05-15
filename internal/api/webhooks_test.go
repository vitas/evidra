package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"samebits.com/evidra/internal/ingest"
	testutil "samebits.com/evidra/internal/testutil"
	"samebits.com/evidra/pkg/evidence"
)

func TestBuildGenericWebhookRequestsSharePrescriptionID(t *testing.T) {
	t.Parallel()

	startBody := json.RawMessage(`{
		"event_type":"operation_started",
		"tool":"kubectl",
		"operation":"apply",
		"operation_id":"op-123",
		"environment":"production",
		"actor":"ci",
		"session_id":"sess-1"
	}`)
	startReq, err := buildGenericWebhookPrescribeRequest(genericWebhookPayload{
		EventType:   "operation_started",
		Tool:        "kubectl",
		Operation:   "apply",
		OperationID: "op-123",
		Environment: "production",
		Actor:       "ci",
		SessionID:   "sess-1",
	}, startBody)
	if err != nil {
		t.Fatalf("buildGenericWebhookPrescribeRequest: %v", err)
	}

	completeBody := json.RawMessage(`{
		"event_type":"operation_completed",
		"tool":"kubectl",
		"operation":"apply",
		"operation_id":"op-123",
		"environment":"production",
		"actor":"ci",
		"session_id":"sess-1",
		"idempotency_key":"evt-123",
		"verdict":"success",
		"exit_code":0
	}`)
	completeReq, err := buildGenericWebhookReportRequest(genericWebhookPayload{
		EventType:      "operation_completed",
		Tool:           "kubectl",
		Operation:      "apply",
		OperationID:    "op-123",
		Environment:    "production",
		Actor:          "ci",
		SessionID:      "sess-1",
		IdempotencyKey: "evt-123",
		Verdict:        evidence.VerdictSuccess,
		ExitCode:       intPtr(0),
	}, completeBody)
	if err != nil {
		t.Fatalf("buildGenericWebhookReportRequest: %v", err)
	}

	wantID := mappedPrescriptionID("generic", "kubectl", "apply", "", "op-123", "production", "")
	if startReq.PrescriptionID != wantID {
		t.Fatalf("start prescription_id = %q, want %q", startReq.PrescriptionID, wantID)
	}
	if completeReq.PrescriptionID != wantID {
		t.Fatalf("report prescription_id = %q, want %q", completeReq.PrescriptionID, wantID)
	}
	if completeReq.SessionID != "sess-1" {
		t.Fatalf("report session_id = %q, want sess-1", completeReq.SessionID)
	}
	if startReq.TraceID != "sess-1" {
		t.Fatalf("start trace_id = %q, want sess-1", startReq.TraceID)
	}
	if startReq.SessionID != "sess-1" {
		t.Fatalf("start session_id = %q, want sess-1", startReq.SessionID)
	}
	if startReq.ContractVersion != ingest.ContractVersionV1 {
		t.Fatalf("start contract_version = %q, want v1", startReq.ContractVersion)
	}
	if startReq.ArtifactDigest != evidence.SHA256Hex(startBody) {
		t.Fatalf("start artifact_digest = %q, want body digest", startReq.ArtifactDigest)
	}
	if startReq.Flavor != evidence.FlavorImperative || completeReq.Flavor != evidence.FlavorImperative {
		t.Fatalf("generic flavor = %q/%q, want imperative", startReq.Flavor, completeReq.Flavor)
	}
	if startReq.Source == nil || startReq.Source.System != "generic" {
		t.Fatalf("start source = %+v, want generic", startReq.Source)
	}
	if completeReq.Source == nil || completeReq.Source.System != "generic" {
		t.Fatalf("report source = %+v, want generic", completeReq.Source)
	}
	if startReq.Evidence == nil || startReq.Evidence.Kind != evidence.EvidenceKindTranslated {
		t.Fatalf("start evidence = %+v, want translated", startReq.Evidence)
	}
	if completeReq.Evidence == nil || completeReq.Evidence.Kind != evidence.EvidenceKindTranslated {
		t.Fatalf("report evidence = %+v, want translated", completeReq.Evidence)
	}
}

func TestBuildArgoCDWebhookRequestsSharePrescriptionID(t *testing.T) {
	t.Parallel()

	startBody := json.RawMessage(`{
		"event":"sync_started",
		"app_name":"demo-app",
		"app_namespace":"production",
		"initiated_by":"argocd-bot",
		"operation_id":"argo-op-123",
		"revision":"abc123"
	}`)
	startReq, err := buildArgoCDWebhookPrescribeRequest(argoCDWebhookPayload{
		Event:        "sync_started",
		AppName:      "demo-app",
		AppNamespace: "production",
		InitiatedBy:  "argocd-bot",
		OperationID:  "argo-op-123",
		Revision:     "abc123",
	}, startBody)
	if err != nil {
		t.Fatalf("buildArgoCDWebhookPrescribeRequest: %v", err)
	}

	completeBody := json.RawMessage(`{
		"event":"sync_completed",
		"app_name":"demo-app",
		"app_namespace":"production",
		"initiated_by":"argocd-bot",
		"operation_id":"argo-op-123",
		"phase":"Succeeded",
		"revision":"abc123"
	}`)
	completeReq, err := buildArgoCDWebhookReportRequest(argoCDWebhookPayload{
		Event:        "sync_completed",
		AppName:      "demo-app",
		AppNamespace: "production",
		InitiatedBy:  "argocd-bot",
		OperationID:  "argo-op-123",
		Phase:        "Succeeded",
		Revision:     "abc123",
	}, completeBody)
	if err != nil {
		t.Fatalf("buildArgoCDWebhookReportRequest: %v", err)
	}

	wantID := mappedPrescriptionID("argocd", "demo-app", "sync", "argocd-bot", "argo-op-123", "production", "")
	if startReq.PrescriptionID != wantID {
		t.Fatalf("start prescription_id = %q, want %q", startReq.PrescriptionID, wantID)
	}
	if completeReq.PrescriptionID != wantID {
		t.Fatalf("report prescription_id = %q, want %q", completeReq.PrescriptionID, wantID)
	}
	if startReq.TraceID != "argo-op-123" {
		t.Fatalf("start trace_id = %q, want argo-op-123", startReq.TraceID)
	}
	if startReq.ContractVersion != ingest.ContractVersionV1 {
		t.Fatalf("start contract_version = %q, want v1", startReq.ContractVersion)
	}
	if startReq.ArtifactDigest != evidence.SHA256Hex(startBody) {
		t.Fatalf("start artifact_digest = %q, want body digest", startReq.ArtifactDigest)
	}
	if startReq.Flavor != evidence.FlavorReconcile || completeReq.Flavor != evidence.FlavorReconcile {
		t.Fatalf("argocd flavor = %q/%q, want reconcile", startReq.Flavor, completeReq.Flavor)
	}
	if startReq.Source == nil || startReq.Source.System != "argocd" {
		t.Fatalf("start source = %+v, want argocd", startReq.Source)
	}
	if completeReq.Source == nil || completeReq.Source.System != "argocd" {
		t.Fatalf("report source = %+v, want argocd", completeReq.Source)
	}
	if startReq.Evidence == nil || startReq.Evidence.Kind != evidence.EvidenceKindTranslated {
		t.Fatalf("start evidence = %+v, want translated", startReq.Evidence)
	}
	if completeReq.Evidence == nil || completeReq.Evidence.Kind != evidence.EvidenceKindTranslated {
		t.Fatalf("report evidence = %+v, want translated", completeReq.Evidence)
	}
	wantRefs := []evidence.ExternalRef{
		{Type: "argocd_application", ID: "production/demo-app"},
		{Type: "argocd_revision", ID: "abc123"},
		{Type: "argocd_operation", ID: "argo-op-123"},
	}
	if !reflect.DeepEqual(completeReq.ExternalRefs, wantRefs) {
		t.Fatalf("report external_refs = %#v, want %#v", completeReq.ExternalRefs, wantRefs)
	}
}

func TestHandleGenericWebhook_RequiresTenantAPIKey(t *testing.T) {
	t.Parallel()

	store := &fakeIngestStore{}
	handler := handleGenericWebhookWithTenantResolver(store, testutil.TestSigner(t), "route-secret", func(context.Context, string) (string, error) {
		return "", nil
	})

	req := httptest.NewRequest("POST", "/v1/hooks/generic", strings.NewReader(`{
		"event_type":"operation_started",
		"tool":"kubectl",
		"operation":"apply",
		"environment":"production"
	}`))
	req.Header.Set("Authorization", "Bearer route-secret")
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rec.Code)
	}
}

func TestHandleGenericWebhook_UsesResolvedTenant(t *testing.T) {
	t.Parallel()

	store := &fakeIngestStore{}
	handler := handleGenericWebhookWithTenantResolver(store, testutil.TestSigner(t), "route-secret", func(_ context.Context, token string) (string, error) {
		if token != "tenant-api-key" {
			return "", errors.New("unknown key")
		}
		return "tenant-123", nil
	})

	req := httptest.NewRequest("POST", "/v1/hooks/generic", strings.NewReader(`{
		"event_type":"operation_started",
		"tool":"kubectl",
		"operation":"apply",
		"operation_id":"op-1",
		"environment":"production",
		"actor":"ci"
	}`))
	req.Header.Set("Authorization", "Bearer route-secret")
	req.Header.Set("X-Evidra-API-Key", "tenant-api-key")
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusAccepted {
		t.Fatalf("expected 202, got %d", rec.Code)
	}
	if store.lastTenant != "tenant-123" {
		t.Fatalf("saved tenant = %q, want tenant-123", store.lastTenant)
	}
	if len(store.savedRaw) != 1 {
		t.Fatalf("saved entries = %d, want 1", len(store.savedRaw))
	}
}

func TestHandleGenericWebhook_RequiresOperationID(t *testing.T) {
	t.Parallel()

	store := &fakeIngestStore{}
	handler := handleGenericWebhookWithTenantResolver(store, testutil.TestSigner(t), "route-secret", func(context.Context, string) (string, error) {
		return "tenant-123", nil
	})

	req := httptest.NewRequest("POST", "/v1/hooks/generic", strings.NewReader(`{
		"event_type":"operation_started",
		"tool":"kubectl",
		"operation":"apply",
		"environment":"production",
		"actor":"ci"
	}`))
	req.Header.Set("Authorization", "Bearer route-secret")
	req.Header.Set("X-Evidra-API-Key", "tenant-api-key")
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
}

func TestHandleGenericWebhook_UsesOperationIDForLifecycleCorrelation(t *testing.T) {
	t.Parallel()

	store := &fakeIngestStore{}
	handler := handleGenericWebhookWithTenantResolver(store, testutil.TestSigner(t), "route-secret", func(context.Context, string) (string, error) {
		return "tenant-123", nil
	})

	startReq := httptest.NewRequest("POST", "/v1/hooks/generic", strings.NewReader(`{
		"event_type":"operation_started",
		"tool":"kubectl",
		"operation":"apply",
		"operation_id":"op-123",
		"environment":"production",
		"actor":"ci"
	}`))
	startReq.Header.Set("Authorization", "Bearer route-secret")
	startReq.Header.Set("X-Evidra-API-Key", "tenant-api-key")
	startReq.Header.Set("Content-Type", "application/json")
	startRec := httptest.NewRecorder()

	handler.ServeHTTP(startRec, startReq)

	if startRec.Code != http.StatusAccepted {
		t.Fatalf("start expected 202, got %d", startRec.Code)
	}

	completeReq := httptest.NewRequest("POST", "/v1/hooks/generic", strings.NewReader(`{
		"event_type":"operation_completed",
		"tool":"kubectl",
		"operation":"apply",
		"operation_id":"op-123",
		"environment":"production",
		"actor":"ci",
		"idempotency_key":"evt-123",
		"verdict":"success",
		"exit_code":0
	}`))
	completeReq.Header.Set("Authorization", "Bearer route-secret")
	completeReq.Header.Set("X-Evidra-API-Key", "tenant-api-key")
	completeReq.Header.Set("Content-Type", "application/json")
	completeRec := httptest.NewRecorder()

	handler.ServeHTTP(completeRec, completeReq)

	if completeRec.Code != http.StatusAccepted {
		t.Fatalf("complete expected 202, got %d", completeRec.Code)
	}
	if len(store.savedRaw) != 2 {
		t.Fatalf("saved entries = %d, want 2", len(store.savedRaw))
	}

	var prescribe evidence.EvidenceEntry
	if err := json.Unmarshal(store.savedRaw[0], &prescribe); err != nil {
		t.Fatalf("decode prescribe entry: %v", err)
	}
	var report evidence.EvidenceEntry
	if err := json.Unmarshal(store.savedRaw[1], &report); err != nil {
		t.Fatalf("decode report entry: %v", err)
	}
	var payload evidence.ReportPayload
	if err := json.Unmarshal(report.Payload, &payload); err != nil {
		t.Fatalf("decode report payload: %v", err)
	}

	if payload.PrescriptionID != prescribe.EntryID {
		t.Fatalf("report prescription_id = %q, want %q", payload.PrescriptionID, prescribe.EntryID)
	}

	var prescribePayload evidence.PrescriptionPayload
	if err := json.Unmarshal(prescribe.Payload, &prescribePayload); err != nil {
		t.Fatalf("decode prescribe payload: %v", err)
	}
	if prescribePayload.EffectiveRisk != "" {
		t.Fatalf("mapped prescribe payload effective_risk = %q, want empty without assessment", prescribePayload.EffectiveRisk)
	}
	if len(prescribePayload.RiskInputs) != 0 {
		t.Fatalf("mapped prescribe risk_inputs len = %d, want 0 without assessment", len(prescribePayload.RiskInputs))
	}
	if prescribePayload.Assessment == nil || prescribePayload.Assessment.Status != evidence.AssessmentNotProvided {
		t.Fatalf("mapped prescribe assessment = %+v, want not_provided", prescribePayload.Assessment)
	}
}

func TestHandleGenericWebhook_PreservesExplicitSessionID(t *testing.T) {
	t.Parallel()

	store := &fakeIngestStore{}
	handler := handleGenericWebhookWithTenantResolver(store, testutil.TestSigner(t), "route-secret", func(context.Context, string) (string, error) {
		return "tenant-123", nil
	})

	startReq := httptest.NewRequest("POST", "/v1/hooks/generic", strings.NewReader(`{
		"event_type":"operation_started",
		"tool":"kubectl",
		"operation":"apply",
		"operation_id":"op-456",
		"environment":"production",
		"actor":"ci",
		"session_id":"sess-456"
	}`))
	startReq.Header.Set("Authorization", "Bearer route-secret")
	startReq.Header.Set("X-Evidra-API-Key", "tenant-api-key")
	startReq.Header.Set("Content-Type", "application/json")
	startRec := httptest.NewRecorder()

	handler.ServeHTTP(startRec, startReq)

	if startRec.Code != http.StatusAccepted {
		t.Fatalf("start expected 202, got %d", startRec.Code)
	}

	completeReq := httptest.NewRequest("POST", "/v1/hooks/generic", strings.NewReader(`{
		"event_type":"operation_completed",
		"tool":"kubectl",
		"operation":"apply",
		"operation_id":"op-456",
		"environment":"production",
		"actor":"ci",
		"session_id":"sess-456",
		"idempotency_key":"evt-456",
		"verdict":"success",
		"exit_code":0
	}`))
	completeReq.Header.Set("Authorization", "Bearer route-secret")
	completeReq.Header.Set("X-Evidra-API-Key", "tenant-api-key")
	completeReq.Header.Set("Content-Type", "application/json")
	completeRec := httptest.NewRecorder()

	handler.ServeHTTP(completeRec, completeReq)

	if completeRec.Code != http.StatusAccepted {
		t.Fatalf("complete expected 202, got %d", completeRec.Code)
	}
	if len(store.savedRaw) != 2 {
		t.Fatalf("saved entries = %d, want 2", len(store.savedRaw))
	}

	var prescribe evidence.EvidenceEntry
	if err := json.Unmarshal(store.savedRaw[0], &prescribe); err != nil {
		t.Fatalf("decode prescribe entry: %v", err)
	}
	var report evidence.EvidenceEntry
	if err := json.Unmarshal(store.savedRaw[1], &report); err != nil {
		t.Fatalf("decode report entry: %v", err)
	}
	if prescribe.SessionID != "sess-456" {
		t.Fatalf("prescribe session_id = %q, want sess-456", prescribe.SessionID)
	}
	if report.SessionID != "sess-456" {
		t.Fatalf("report session_id = %q, want sess-456", report.SessionID)
	}
}

func TestHandleGenericWebhook_AcceptsTranslatedCompletionWithoutStartEntry(t *testing.T) {
	t.Parallel()

	store := &fakeIngestStore{}
	handler := handleGenericWebhookWithTenantResolver(store, testutil.TestSigner(t), "route-secret", func(context.Context, string) (string, error) {
		return "tenant-123", nil
	})

	completeReq := httptest.NewRequest("POST", "/v1/hooks/generic", strings.NewReader(`{
		"event_type":"operation_completed",
		"tool":"kubectl",
		"operation":"apply",
		"operation_id":"op-soft",
		"environment":"production",
		"actor":"ci",
		"session_id":"sess-soft",
		"idempotency_key":"evt-soft",
		"verdict":"success",
		"exit_code":0
	}`))
	completeReq.Header.Set("Authorization", "Bearer route-secret")
	completeReq.Header.Set("X-Evidra-API-Key", "tenant-api-key")
	completeReq.Header.Set("Content-Type", "application/json")
	completeRec := httptest.NewRecorder()

	handler.ServeHTTP(completeRec, completeReq)

	if completeRec.Code != http.StatusAccepted {
		t.Fatalf("complete expected 202, got %d", completeRec.Code)
	}
	if len(store.savedRaw) != 1 {
		t.Fatalf("saved entries = %d, want 1", len(store.savedRaw))
	}

	var report evidence.EvidenceEntry
	if err := json.Unmarshal(store.savedRaw[0], &report); err != nil {
		t.Fatalf("decode report entry: %v", err)
	}
	if report.SessionID != "sess-soft" {
		t.Fatalf("report session_id = %q, want sess-soft", report.SessionID)
	}
	if report.OperationID != "op-soft" {
		t.Fatalf("report operation_id = %q, want op-soft", report.OperationID)
	}
	var payload evidence.ReportPayload
	if err := json.Unmarshal(report.Payload, &payload); err != nil {
		t.Fatalf("decode report payload: %v", err)
	}
	wantID := mappedPrescriptionID("generic", "kubectl", "apply", "", "op-soft", "production", "")
	if payload.PrescriptionID != wantID {
		t.Fatalf("report prescription_id = %q, want %q", payload.PrescriptionID, wantID)
	}
}

func TestHandleArgoCDWebhook_UsesOperationIDForLifecycleCorrelation(t *testing.T) {
	t.Parallel()

	store := &fakeIngestStore{}
	handler := handleArgoCDWebhookWithTenantResolver(store, testutil.TestSigner(t), "route-secret", func(context.Context, string) (string, error) {
		return "tenant-123", nil
	})

	startReq := httptest.NewRequest("POST", "/v1/hooks/argocd", strings.NewReader(`{
		"event":"sync_started",
		"app_name":"demo-app",
		"app_namespace":"production",
		"initiated_by":"argocd-bot",
		"operation_id":"argo-op-123",
		"revision":"abc123"
	}`))
	startReq.Header.Set("Authorization", "Bearer route-secret")
	startReq.Header.Set("X-Evidra-API-Key", "tenant-api-key")
	startReq.Header.Set("Content-Type", "application/json")
	startRec := httptest.NewRecorder()

	handler.ServeHTTP(startRec, startReq)

	if startRec.Code != http.StatusAccepted {
		t.Fatalf("start expected 202, got %d", startRec.Code)
	}

	completeReq := httptest.NewRequest("POST", "/v1/hooks/argocd", strings.NewReader(`{
		"event":"sync_completed",
		"app_name":"demo-app",
		"app_namespace":"production",
		"initiated_by":"argocd-bot",
		"operation_id":"argo-op-123",
		"phase":"Succeeded",
		"revision":"abc123"
	}`))
	completeReq.Header.Set("Authorization", "Bearer route-secret")
	completeReq.Header.Set("X-Evidra-API-Key", "tenant-api-key")
	completeReq.Header.Set("Content-Type", "application/json")
	completeRec := httptest.NewRecorder()

	handler.ServeHTTP(completeRec, completeReq)

	if completeRec.Code != http.StatusAccepted {
		t.Fatalf("complete expected 202, got %d", completeRec.Code)
	}
	if len(store.savedRaw) != 2 {
		t.Fatalf("saved entries = %d, want 2", len(store.savedRaw))
	}

	var prescribe evidence.EvidenceEntry
	if err := json.Unmarshal(store.savedRaw[0], &prescribe); err != nil {
		t.Fatalf("decode prescribe entry: %v", err)
	}
	var report evidence.EvidenceEntry
	if err := json.Unmarshal(store.savedRaw[1], &report); err != nil {
		t.Fatalf("decode report entry: %v", err)
	}
	var payload evidence.ReportPayload
	if err := json.Unmarshal(report.Payload, &payload); err != nil {
		t.Fatalf("decode report payload: %v", err)
	}

	if payload.PrescriptionID != prescribe.EntryID {
		t.Fatalf("report prescription_id = %q, want %q", payload.PrescriptionID, prescribe.EntryID)
	}

	var typedPrescribe evidence.PrescriptionPayload
	if err := json.Unmarshal(prescribe.Payload, &typedPrescribe); err != nil {
		t.Fatalf("decode prescribe payload: %v", err)
	}
	if typedPrescribe.Flavor != evidence.FlavorReconcile {
		t.Fatalf("prescribe payload flavor = %q, want reconcile", typedPrescribe.Flavor)
	}
	if typedPrescribe.Evidence == nil || typedPrescribe.Evidence.Kind != evidence.EvidenceKindTranslated {
		t.Fatalf("prescribe payload evidence = %+v, want translated", typedPrescribe.Evidence)
	}
	if typedPrescribe.Source == nil || typedPrescribe.Source.System != "argocd" {
		t.Fatalf("prescribe payload source = %+v, want argocd", typedPrescribe.Source)
	}

	if payload.Flavor != evidence.FlavorReconcile {
		t.Fatalf("report payload flavor = %q, want reconcile", payload.Flavor)
	}
	if payload.Evidence == nil || payload.Evidence.Kind != evidence.EvidenceKindTranslated {
		t.Fatalf("report payload evidence = %+v, want translated", payload.Evidence)
	}
	if payload.Source == nil || payload.Source.System != "argocd" {
		t.Fatalf("report payload source = %+v, want argocd", payload.Source)
	}

	wantScope := map[string]string{
		"source_kind":   "mapped",
		"source_system": "argocd",
		"environment":   "production",
		"application":   "demo-app",
		"revision":      "abc123",
	}
	if !reflect.DeepEqual(prescribe.ScopeDimensions, wantScope) {
		t.Fatalf("prescribe scope_dimensions = %#v, want %#v", prescribe.ScopeDimensions, wantScope)
	}
	if !reflect.DeepEqual(report.ScopeDimensions, wantScope) {
		t.Fatalf("report scope_dimensions = %#v, want %#v", report.ScopeDimensions, wantScope)
	}

	wantRefs := []evidence.ExternalRef{
		{Type: "argocd_application", ID: "production/demo-app"},
		{Type: "argocd_revision", ID: "abc123"},
		{Type: "argocd_operation", ID: "argo-op-123"},
	}
	if !reflect.DeepEqual(payload.ExternalRefs, wantRefs) {
		t.Fatalf("report external_refs = %#v, want %#v", payload.ExternalRefs, wantRefs)
	}
}
