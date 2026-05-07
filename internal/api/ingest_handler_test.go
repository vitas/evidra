package api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"samebits.com/evidra/internal/auth"
	"samebits.com/evidra/internal/canon"
	"samebits.com/evidra/internal/ingest"
	"samebits.com/evidra/internal/store"
	testutil "samebits.com/evidra/internal/testutil"
	"samebits.com/evidra/pkg/evidence"
)

type fakeIngestStore struct {
	lastHash      string
	entries       map[string]store.StoredEntry
	claimed       map[string]json.RawMessage
	claimResults  map[string]store.WebhookEventResult
	savedRaw      []json.RawMessage
	lastTenant    string
	getEntryCalls int
	claimCalls    int
	releaseCalls  int
	beginErr      error
}

type fakeIngestService struct {
	prescribeResult ingest.Result
	reportResult    ingest.Result
	prescribeTenant string
	reportTenant    string
	prescribeReq    ingest.PrescribeRequest
	reportReq       ingest.ReportRequest
}

type fakeIngestTx struct {
	parent       *fakeIngestStore
	lastHash     string
	entries      map[string]store.StoredEntry
	claimed      map[string]json.RawMessage
	claimResults map[string]store.WebhookEventResult
	savedRaw     []json.RawMessage
}

func (f *fakeIngestService) Prescribe(_ context.Context, tenantID string, in ingest.PrescribeRequest) (ingest.Result, error) {
	f.prescribeTenant = tenantID
	f.prescribeReq = in
	return f.prescribeResult, nil
}

func (f *fakeIngestService) Report(_ context.Context, tenantID string, in ingest.ReportRequest) (ingest.Result, error) {
	f.reportTenant = tenantID
	f.reportReq = in
	return f.reportResult, nil
}

func (f *fakeIngestStore) LastHash(context.Context, string) (string, error) {
	return f.lastHash, nil
}

func (f *fakeIngestStore) BeginIngestTx(context.Context) (store.IngestTx, error) {
	if f.beginErr != nil {
		return nil, f.beginErr
	}
	return &fakeIngestTx{
		parent:       f,
		lastHash:     f.lastHash,
		entries:      cloneStoredEntries(f.entries),
		claimed:      cloneClaimedEntries(f.claimed),
		claimResults: cloneClaimResults(f.claimResults),
	}, nil
}

func (f *fakeIngestStore) SaveRaw(_ context.Context, tenantID string, raw json.RawMessage) (string, error) {
	f.lastTenant = tenantID
	f.savedRaw = append(f.savedRaw, append(json.RawMessage(nil), raw...))
	var entry evidence.EvidenceEntry
	if err := json.Unmarshal(raw, &entry); err != nil {
		return "", err
	}
	if entry.EntryID == "" {
		return "", errors.New("missing entry_id")
	}
	if f.entries == nil {
		f.entries = make(map[string]store.StoredEntry)
	}
	f.entries[entry.EntryID] = store.StoredEntry{
		ID:          entry.EntryID,
		EntryType:   string(entry.Type),
		SessionID:   entry.SessionID,
		OperationID: entry.OperationID,
		Payload:     append(json.RawMessage(nil), raw...),
	}
	return "receipt-1", nil
}

func (f *fakeIngestStore) GetEntry(_ context.Context, tenantID, entryID string) (store.StoredEntry, error) {
	f.lastTenant = tenantID
	f.getEntryCalls++
	if f.entries != nil {
		if entry, ok := f.entries[entryID]; ok {
			return entry, nil
		}
	}
	return store.StoredEntry{}, store.ErrNotFound
}

func (f *fakeIngestStore) ClaimWebhookEvent(_ context.Context, tenantID, source, key string, _ json.RawMessage) (bool, error) {
	f.lastTenant = tenantID
	f.claimCalls++
	if f.claimed == nil {
		f.claimed = map[string]json.RawMessage{}
	}
	composite := source + ":" + key
	if _, ok := f.claimed[composite]; ok {
		return true, nil
	}
	f.claimed[composite] = json.RawMessage(`true`)
	return false, nil
}

func (f *fakeIngestStore) ReleaseWebhookEvent(_ context.Context, tenantID, source, key string) error {
	f.lastTenant = tenantID
	f.releaseCalls++
	return nil
}

func (t *fakeIngestTx) LastHash(_ context.Context, tenantID string) (string, error) {
	if t.parent != nil {
		t.parent.lastTenant = tenantID
	}
	return t.lastHash, nil
}

func (t *fakeIngestTx) SaveRaw(_ context.Context, tenantID string, raw json.RawMessage) (string, error) {
	if t.parent != nil {
		t.parent.lastTenant = tenantID
	}
	t.savedRaw = append(t.savedRaw, append(json.RawMessage(nil), raw...))
	var entry evidence.EvidenceEntry
	if err := json.Unmarshal(raw, &entry); err != nil {
		return "", err
	}
	if entry.EntryID == "" {
		return "", errors.New("missing entry_id")
	}
	t.entries[entry.EntryID] = store.StoredEntry{
		ID:          entry.EntryID,
		EntryType:   string(entry.Type),
		SessionID:   entry.SessionID,
		OperationID: entry.OperationID,
		Payload:     append(json.RawMessage(nil), raw...),
	}
	t.lastHash = entry.Hash
	return entry.EntryID, nil
}

func (t *fakeIngestTx) GetEntry(_ context.Context, tenantID, entryID string) (store.StoredEntry, error) {
	if t.parent != nil {
		t.parent.lastTenant = tenantID
	}
	entry, ok := t.entries[entryID]
	if !ok {
		return store.StoredEntry{}, store.ErrNotFound
	}
	return entry, nil
}

func (t *fakeIngestTx) ClaimWebhookEvent(_ context.Context, tenantID, source, key string, payload json.RawMessage) (bool, error) {
	if t.parent != nil {
		t.parent.lastTenant = tenantID
	}
	claimKey := tenantID + "|" + source + "|" + key
	if _, ok := t.claimed[claimKey]; ok {
		return true, nil
	}
	t.claimed[claimKey] = append(json.RawMessage(nil), payload...)
	return false, nil
}

func (t *fakeIngestTx) GetWebhookEventResult(_ context.Context, tenantID, source, key string) (store.WebhookEventResult, error) {
	if t.parent != nil {
		t.parent.lastTenant = tenantID
	}
	claimKey := tenantID + "|" + source + "|" + key
	result, ok := t.claimResults[claimKey]
	if !ok {
		return store.WebhookEventResult{}, store.ErrNotFound
	}
	return result, nil
}

func (t *fakeIngestTx) FinalizeWebhookEvent(_ context.Context, tenantID, source, key, entryID, effectiveRisk string) error {
	if t.parent != nil {
		t.parent.lastTenant = tenantID
	}
	claimKey := tenantID + "|" + source + "|" + key
	if _, ok := t.claimed[claimKey]; !ok {
		return store.ErrNotFound
	}
	t.claimResults[claimKey] = store.WebhookEventResult{EntryID: entryID, EffectiveRisk: effectiveRisk}
	return nil
}

func (t *fakeIngestTx) Commit(context.Context) error {
	if t.parent == nil {
		return nil
	}
	if t.parent.entries == nil {
		t.parent.entries = make(map[string]store.StoredEntry)
	}
	if t.parent.claimed == nil {
		t.parent.claimed = make(map[string]json.RawMessage)
	}
	if t.parent.claimResults == nil {
		t.parent.claimResults = make(map[string]store.WebhookEventResult)
	}
	t.parent.lastHash = t.lastHash
	for id, entry := range t.entries {
		t.parent.entries[id] = entry
	}
	for key, payload := range t.claimed {
		t.parent.claimed[key] = append(json.RawMessage(nil), payload...)
	}
	for key, result := range t.claimResults {
		t.parent.claimResults[key] = result
	}
	t.parent.savedRaw = append(t.parent.savedRaw, t.savedRaw...)
	return nil
}

func (t *fakeIngestTx) Rollback(context.Context) error {
	return nil
}

func cloneStoredEntries(src map[string]store.StoredEntry) map[string]store.StoredEntry {
	dst := make(map[string]store.StoredEntry, len(src))
	for k, v := range src {
		dst[k] = v
	}
	return dst
}

func cloneClaimedEntries(src map[string]json.RawMessage) map[string]json.RawMessage {
	dst := make(map[string]json.RawMessage, len(src))
	for k, v := range src {
		dst[k] = append(json.RawMessage(nil), v...)
	}
	return dst
}

func cloneClaimResults(src map[string]store.WebhookEventResult) map[string]store.WebhookEventResult {
	dst := make(map[string]store.WebhookEventResult, len(src))
	for k, v := range src {
		dst[k] = v
	}
	return dst
}

func TestIngestPrescribeHandler_AcceptsValidInput(t *testing.T) {
	t.Parallel()

	store := &fakeIngestStore{lastHash: "sha256:previous"}
	svc := ingest.NewService(store, testutil.TestSigner(t))
	handler := handleIngestPrescribe(svc)

	req := httptest.NewRequest("POST", "/v1/evidence/ingest/prescribe", bytes.NewReader(mustJSON(t, ingest.PrescribeRequest{
		Envelope: ingest.Envelope{
			ContractVersion: ingest.ContractVersionV1,
			Actor: evidence.Actor{
				Type:       " controller ",
				ID:         " ci-bot ",
				Provenance: " github-actions ",
			},
			SessionID:   "session-1",
			OperationID: "operation-1",
			TraceID:     "trace-1",
			Flavor:      evidence.FlavorWorkflow,
			Evidence:    &evidence.EvidenceMetadata{Kind: evidence.EvidenceKindObserved},
			Source:      &evidence.SourceMetadata{System: " argocd "},
		},
		CanonicalAction: &canon.CanonicalAction{
			Tool:           "kubectl",
			Operation:      "apply",
			OperationClass: "mutate",
			ScopeClass:     "production",
			ResourceCount:  1,
		},
	})))
	req.Header.Set("Content-Type", "application/json")
	req = req.WithContext(auth.WithTenantID(req.Context(), "tenant-1"))
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusAccepted {
		t.Fatalf("expected 202, got %d body=%s", rec.Code, rec.Body.String())
	}
	var resp struct {
		Duplicate      bool   `json:"duplicate"`
		EntryID        string `json:"entry_id"`
		EffectiveRisk  string `json:"effective_risk"`
		PrescriptionID string `json:"prescription_id"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.Duplicate {
		t.Fatal("expected non-duplicate response")
	}
	if resp.EntryID == "" {
		t.Fatal("expected entry_id")
	}
	if resp.EffectiveRisk == "" {
		t.Fatal("expected effective_risk")
	}
	if resp.PrescriptionID == "" {
		t.Fatal("expected prescription_id")
	}
	if store.lastTenant != "tenant-1" {
		t.Fatalf("tenant = %q, want tenant-1", store.lastTenant)
	}
	if len(store.savedRaw) != 1 {
		t.Fatalf("saved entries = %d, want 1", len(store.savedRaw))
	}
}

func TestIngestReportHandler_AcceptsValidInput(t *testing.T) {
	t.Parallel()

	store := &fakeIngestStore{
		lastHash: "sha256:previous",
		entries: map[string]store.StoredEntry{
			"presc-1": {
				ID:        "presc-1",
				TenantID:  "tenant-1",
				EntryType: string(evidence.EntryTypePrescribe),
				SessionID: "session-1",
			},
		},
	}
	svc := ingest.NewService(store, testutil.TestSigner(t))
	handler := handleIngestReport(svc)

	req := httptest.NewRequest("POST", "/v1/evidence/ingest/report", bytes.NewReader(mustJSON(t, ingest.ReportRequest{
		Envelope: ingest.Envelope{
			ContractVersion: ingest.ContractVersionV1,
			Actor: evidence.Actor{
				Type:       " controller ",
				ID:         " ci-bot ",
				Provenance: " github-actions ",
			},
			SessionID:   "session-1",
			OperationID: "operation-1",
			TraceID:     "trace-1",
			Flavor:      evidence.FlavorWorkflow,
			Evidence:    &evidence.EvidenceMetadata{Kind: evidence.EvidenceKindObserved},
			Source:      &evidence.SourceMetadata{System: " argocd "},
		},
		PrescriptionID: "presc-1",
		Verdict:        evidence.VerdictSuccess,
		ExitCode:       intPtr(0),
	})))
	req.Header.Set("Content-Type", "application/json")
	req = req.WithContext(auth.WithTenantID(req.Context(), "tenant-1"))
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusAccepted {
		t.Fatalf("expected 202, got %d body=%s", rec.Code, rec.Body.String())
	}
	var resp struct {
		Duplicate     bool   `json:"duplicate"`
		EntryID       string `json:"entry_id"`
		EffectiveRisk string `json:"effective_risk"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.Duplicate {
		t.Fatal("expected non-duplicate response")
	}
	if resp.EntryID == "" {
		t.Fatal("expected entry_id")
	}
	if resp.EffectiveRisk != "" {
		t.Fatalf("effective_risk = %q, want empty string for report", resp.EffectiveRisk)
	}
	if store.lastTenant != "tenant-1" {
		t.Fatalf("tenant = %q, want tenant-1", store.lastTenant)
	}
	if len(store.savedRaw) != 1 {
		t.Fatalf("saved entries = %d, want 1", len(store.savedRaw))
	}
	if store.getEntryCalls != 1 {
		t.Fatalf("get entry calls = %d, want 1", store.getEntryCalls)
	}
}

func TestIngestReportHandler_DuplicateClaimResponseStable(t *testing.T) {
	t.Parallel()

	store := &fakeIngestStore{
		lastHash: "sha256:previous",
		entries: map[string]store.StoredEntry{
			"presc-1": {
				ID:        "presc-1",
				TenantID:  "tenant-1",
				EntryType: string(evidence.EntryTypePrescribe),
				SessionID: "session-dup",
			},
		},
	}
	svc := ingest.NewService(store, testutil.TestSigner(t))
	handler := handleIngestReport(svc)

	body := []byte(`{
		"contract_version":"v1",
		"claim":{"source":"gateway","key":"report-dup"},
		"actor":{"type":"controller","id":"ci-bot","provenance":"github-actions"},
		"session_id":"session-dup",
		"operation_id":"operation-dup",
		"trace_id":"trace-dup",
		"flavor":"workflow",
		"evidence":{"kind":"observed"},
		"source":{"system":"argocd"},
		"prescription_id":"presc-1",
		"verdict":"success",
		"exit_code":0
	}`)

	first := httptest.NewRecorder()
	req1 := httptest.NewRequest("POST", "/v1/evidence/ingest/report", bytes.NewReader(body))
	req1.Header.Set("Content-Type", "application/json")
	req1 = req1.WithContext(auth.WithTenantID(req1.Context(), "tenant-1"))
	handler.ServeHTTP(first, req1)

	second := httptest.NewRecorder()
	req2 := httptest.NewRequest("POST", "/v1/evidence/ingest/report", bytes.NewReader(body))
	req2.Header.Set("Content-Type", "application/json")
	req2 = req2.WithContext(auth.WithTenantID(req2.Context(), "tenant-1"))
	handler.ServeHTTP(second, req2)

	if first.Code != http.StatusAccepted {
		t.Fatalf("first expected 202, got %d body=%s", first.Code, first.Body.String())
	}
	if second.Code != http.StatusOK {
		t.Fatalf("second expected 200, got %d body=%s", second.Code, second.Body.String())
	}

	var firstResp struct {
		Duplicate     bool   `json:"duplicate"`
		EntryID       string `json:"entry_id"`
		EffectiveRisk string `json:"effective_risk"`
	}
	if err := json.Unmarshal(first.Body.Bytes(), &firstResp); err != nil {
		t.Fatalf("decode first response: %v", err)
	}
	var secondResp struct {
		Duplicate     bool   `json:"duplicate"`
		EntryID       string `json:"entry_id"`
		EffectiveRisk string `json:"effective_risk"`
	}
	if err := json.Unmarshal(second.Body.Bytes(), &secondResp); err != nil {
		t.Fatalf("decode second response: %v", err)
	}
	if firstResp.Duplicate {
		t.Fatal("first request should not be duplicate")
	}
	if !secondResp.Duplicate {
		t.Fatal("second request should be duplicate")
	}
	if firstResp.EntryID == "" || firstResp.EntryID != secondResp.EntryID {
		t.Fatalf("entry_id mismatch: first=%q second=%q", firstResp.EntryID, secondResp.EntryID)
	}
	if firstResp.EffectiveRisk != "" || secondResp.EffectiveRisk != "" {
		t.Fatalf("effective_risk mismatch: first=%q second=%q", firstResp.EffectiveRisk, secondResp.EffectiveRisk)
	}
	if len(store.savedRaw) != 1 {
		t.Fatalf("saved entries = %d, want 1", len(store.savedRaw))
	}
	if len(store.claimResults) != 1 {
		t.Fatalf("claim results = %d, want 1", len(store.claimResults))
	}
}

func TestIngestPrescribeHandler_InvalidPayloadReturns400(t *testing.T) {
	t.Parallel()

	svc := ingest.NewService(&fakeIngestStore{}, testutil.TestSigner(t))
	handler := handleIngestPrescribe(svc)

	req := httptest.NewRequest("POST", "/v1/evidence/ingest/prescribe", strings.NewReader(`{
		"contract_version":"v1",
		"actor":{"type":"controller","id":"ci-bot","provenance":"github-actions"},
		"session_id":"session-1",
		"operation_id":"operation-1",
		"trace_id":"trace-1",
		"flavor":"workflow",
		"evidence":{"kind":"observed"},
		"source":{"system":"argocd"}
	}`))
	req.Header.Set("Content-Type", "application/json")
	req = req.WithContext(auth.WithTenantID(req.Context(), "tenant-1"))
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestWriteIngestServiceErrorMappings(t *testing.T) {
	t.Parallel()

	t.Run("not_found", func(t *testing.T) {
		t.Parallel()

		svc := ingest.NewService(&fakeIngestStore{lastHash: "sha256:previous"}, testutil.TestSigner(t))
		_, err := svc.Report(context.Background(), "tenant-1", ingest.ReportRequest{
			Envelope: ingest.Envelope{
				ContractVersion: ingest.ContractVersionV1,
				Actor: evidence.Actor{
					Type:       "controller",
					ID:         "ci-bot",
					Provenance: "github-actions",
				},
				SessionID:   "session-1",
				OperationID: "operation-1",
				TraceID:     "trace-1",
				Flavor:      evidence.FlavorWorkflow,
				Evidence:    &evidence.EvidenceMetadata{Kind: evidence.EvidenceKindObserved},
				Source:      &evidence.SourceMetadata{System: "argocd"},
			},
			PrescriptionID: "missing-prescription",
			Verdict:        evidence.VerdictSuccess,
			ExitCode:       intPtr(0),
		})
		if err == nil {
			t.Fatal("expected not_found error")
		}
		rec := httptest.NewRecorder()
		writeIngestServiceError(rec, err)
		if rec.Code != http.StatusNotFound {
			t.Fatalf("expected 404, got %d body=%s", rec.Code, rec.Body.String())
		}
	})

	t.Run("no_signer_configured", func(t *testing.T) {
		t.Parallel()

		svc := ingest.NewService(&fakeIngestStore{lastHash: "sha256:previous"}, nil)
		_, err := svc.Prescribe(context.Background(), "tenant-1", ingest.PrescribeRequest{
			Envelope: ingest.Envelope{
				ContractVersion: ingest.ContractVersionV1,
				Actor: evidence.Actor{
					Type:       "controller",
					ID:         "ci-bot",
					Provenance: "github-actions",
				},
				SessionID:   "session-1",
				OperationID: "operation-1",
				TraceID:     "trace-1",
				Flavor:      evidence.FlavorWorkflow,
				Evidence:    &evidence.EvidenceMetadata{Kind: evidence.EvidenceKindObserved},
				Source:      &evidence.SourceMetadata{System: "argocd"},
			},
			CanonicalAction: &canon.CanonicalAction{
				Tool:           "kubectl",
				Operation:      "apply",
				OperationClass: "mutate",
				ScopeClass:     "production",
				ResourceCount:  1,
			},
		})
		if err == nil {
			t.Fatal("expected no_signer_configured error")
		}
		rec := httptest.NewRecorder()
		writeIngestServiceError(rec, err)
		if rec.Code != http.StatusServiceUnavailable {
			t.Fatalf("expected 503, got %d body=%s", rec.Code, rec.Body.String())
		}
	})

	t.Run("internal_error", func(t *testing.T) {
		t.Parallel()

		svc := ingest.NewService(&fakeIngestStore{beginErr: errors.New("boom")}, testutil.TestSigner(t))
		_, err := svc.Prescribe(context.Background(), "tenant-1", ingest.PrescribeRequest{
			Envelope: ingest.Envelope{
				ContractVersion: ingest.ContractVersionV1,
				Actor: evidence.Actor{
					Type:       "controller",
					ID:         "ci-bot",
					Provenance: "github-actions",
				},
				SessionID:   "session-1",
				OperationID: "operation-1",
				TraceID:     "trace-1",
				Flavor:      evidence.FlavorWorkflow,
				Evidence:    &evidence.EvidenceMetadata{Kind: evidence.EvidenceKindObserved},
				Source:      &evidence.SourceMetadata{System: "argocd"},
			},
			CanonicalAction: &canon.CanonicalAction{
				Tool:           "kubectl",
				Operation:      "apply",
				OperationClass: "mutate",
				ScopeClass:     "production",
				ResourceCount:  1,
			},
		})
		if err == nil {
			t.Fatal("expected internal error")
		}
		rec := httptest.NewRecorder()
		writeIngestServiceError(rec, err)
		if rec.Code != http.StatusInternalServerError {
			t.Fatalf("expected 500, got %d body=%s", rec.Code, rec.Body.String())
		}
	})
}

func TestIngestPrescribeHandler_DuplicateClaimResponseStable(t *testing.T) {
	t.Parallel()

	store := &fakeIngestStore{
		lastHash: "sha256:previous",
	}
	svc := ingest.NewService(store, testutil.TestSigner(t))
	handler := handleIngestPrescribe(svc)

	body := []byte(`{
		"contract_version":"v1",
		"claim":{"source":"gateway","key":"claim-dup"},
		"actor":{"type":"controller","id":"ci-bot","provenance":"github-actions"},
		"session_id":"session-dup",
		"operation_id":"operation-dup",
		"trace_id":"trace-dup",
		"flavor":"workflow",
		"evidence":{"kind":"observed"},
		"source":{"system":"argocd"},
		"canonical_action":{
			"tool":"kubectl",
			"operation":"apply",
			"operation_class":"mutate",
			"scope_class":"production",
			"resource_count":1
		}
	}`)

	first := httptest.NewRecorder()
	req1 := httptest.NewRequest("POST", "/v1/evidence/ingest/prescribe", bytes.NewReader(body))
	req1.Header.Set("Content-Type", "application/json")
	req1 = req1.WithContext(auth.WithTenantID(req1.Context(), "tenant-1"))
	handler.ServeHTTP(first, req1)

	second := httptest.NewRecorder()
	req2 := httptest.NewRequest("POST", "/v1/evidence/ingest/prescribe", bytes.NewReader(body))
	req2.Header.Set("Content-Type", "application/json")
	req2 = req2.WithContext(auth.WithTenantID(req2.Context(), "tenant-1"))
	handler.ServeHTTP(second, req2)

	if first.Code != http.StatusAccepted {
		t.Fatalf("first expected 202, got %d body=%s", first.Code, first.Body.String())
	}
	if second.Code != http.StatusOK {
		t.Fatalf("second expected 200, got %d body=%s", second.Code, second.Body.String())
	}
	var firstResp struct {
		Duplicate      bool   `json:"duplicate"`
		EntryID        string `json:"entry_id"`
		EffectiveRisk  string `json:"effective_risk"`
		PrescriptionID string `json:"prescription_id"`
	}
	if err := json.Unmarshal(first.Body.Bytes(), &firstResp); err != nil {
		t.Fatalf("decode first response: %v", err)
	}
	var secondResp struct {
		Duplicate      bool   `json:"duplicate"`
		EntryID        string `json:"entry_id"`
		EffectiveRisk  string `json:"effective_risk"`
		PrescriptionID string `json:"prescription_id"`
	}
	if err := json.Unmarshal(second.Body.Bytes(), &secondResp); err != nil {
		t.Fatalf("decode second response: %v", err)
	}
	if firstResp.Duplicate {
		t.Fatal("first request should not be duplicate")
	}
	if !secondResp.Duplicate {
		t.Fatal("second request should be duplicate")
	}
	if firstResp.EntryID == "" || firstResp.EntryID != secondResp.EntryID {
		t.Fatalf("entry_id mismatch: first=%q second=%q", firstResp.EntryID, secondResp.EntryID)
	}
	if firstResp.EffectiveRisk == "" || firstResp.EffectiveRisk != secondResp.EffectiveRisk {
		t.Fatalf("effective_risk mismatch: first=%q second=%q", firstResp.EffectiveRisk, secondResp.EffectiveRisk)
	}
	if firstResp.PrescriptionID == "" || firstResp.PrescriptionID != secondResp.PrescriptionID {
		t.Fatalf("prescription_id mismatch: first=%q second=%q", firstResp.PrescriptionID, secondResp.PrescriptionID)
	}
	if len(store.savedRaw) != 1 {
		t.Fatalf("saved entries = %d, want 1 commit after standard retry", len(store.savedRaw))
	}
	if len(store.claimResults) != 1 {
		t.Fatalf("claim results = %d, want 1", len(store.claimResults))
	}
}

func TestIngestRouter_PrescribeAcceptsAuthenticatedRequest(t *testing.T) {
	t.Parallel()

	fakeSvc := &fakeIngestService{
		prescribeResult: ingest.Result{
			EntryID:        "entry-123",
			EffectiveRisk:  "medium",
			PrescriptionID: "presc-override",
			Entry: evidence.EvidenceEntry{
				EntryID: "entry-123",
				Payload: json.RawMessage(`{"prescription_id":"presc-override"}`),
			},
		},
	}
	router := NewRouter(RouterConfig{
		APIKey:        "test-key",
		DefaultTenant: "tenant-1",
		Ingest:        fakeSvc,
	})

	req := httptest.NewRequest("POST", "/v1/evidence/ingest/prescribe", strings.NewReader(`{
		"contract_version":"v1",
		"claim":{"source":"gateway","key":"claim-1"},
		"actor":{"type":"controller","id":"ci-bot","provenance":"github-actions"},
		"session_id":"session-1",
		"operation_id":"operation-1",
		"trace_id":"trace-1",
		"flavor":"workflow",
		"evidence":{"kind":"observed"},
		"source":{"system":"argocd"},
		"payload_override":{
			"prescription_id":"presc-override",
			"canonical_action":{
				"tool":"kubectl",
				"operation":"apply",
				"operation_class":"mutate",
				"scope_class":"production",
				"resource_count":1
			}
		}
	}`))
	req.Header.Set("Authorization", "Bearer test-key")
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusAccepted {
		t.Fatalf("expected 202, got %d body=%s", rec.Code, rec.Body.String())
	}
	if fakeSvc.prescribeTenant != "tenant-1" {
		t.Fatalf("tenant = %q, want tenant-1", fakeSvc.prescribeTenant)
	}
	if fakeSvc.prescribeReq.ContractVersion != ingest.ContractVersionV1 {
		t.Fatalf("contract_version = %q, want v1", fakeSvc.prescribeReq.ContractVersion)
	}
	var resp map[string]any
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp["prescription_id"] != "presc-override" {
		t.Fatalf("prescription_id = %v, want presc-override", resp["prescription_id"])
	}
}

func TestIngestRouter_ReportAcceptsAuthenticatedRequest(t *testing.T) {
	t.Parallel()

	fakeSvc := &fakeIngestService{
		reportResult: ingest.Result{
			EntryID: "report-1",
			Entry: evidence.EvidenceEntry{
				EntryID: "report-1",
				Payload: json.RawMessage(`{"report_id":"report-1"}`),
			},
		},
	}
	router := NewRouter(RouterConfig{
		APIKey:        "test-key",
		DefaultTenant: "tenant-1",
		Ingest:        fakeSvc,
	})

	req := httptest.NewRequest("POST", "/v1/evidence/ingest/report", strings.NewReader(`{
		"contract_version":"v1",
		"actor":{"type":"controller","id":"ci-bot","provenance":"github-actions"},
		"session_id":"session-1",
		"operation_id":"operation-1",
		"trace_id":"trace-1",
		"flavor":"workflow",
		"evidence":{"kind":"observed"},
		"source":{"system":"argocd"},
		"prescription_id":"presc-1",
		"verdict":"success",
		"exit_code":0
	}`))
	req.Header.Set("Authorization", "Bearer test-key")
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusAccepted {
		t.Fatalf("expected 202, got %d body=%s", rec.Code, rec.Body.String())
	}
	if fakeSvc.reportTenant != "tenant-1" {
		t.Fatalf("tenant = %q, want tenant-1", fakeSvc.reportTenant)
	}
	var resp map[string]any
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp["entry_id"] != "report-1" {
		t.Fatalf("entry_id = %v, want report-1", resp["entry_id"])
	}
}

func TestIngestRouter_RequiresAuth(t *testing.T) {
	t.Parallel()

	router := NewRouter(RouterConfig{
		APIKey:        "test-key",
		DefaultTenant: "tenant-1",
		Ingest: &fakeIngestService{
			prescribeResult: ingest.Result{
				EntryID:        "presc-1",
				PrescriptionID: "presc-1",
				Entry: evidence.EvidenceEntry{
					EntryID: "presc-1",
					Payload: json.RawMessage(`{"prescription_id":"presc-1"}`),
				},
			},
			reportResult: ingest.Result{
				EntryID: "report-1",
				Entry: evidence.EvidenceEntry{
					EntryID: "report-1",
					Payload: json.RawMessage(`{"report_id":"report-1"}`),
				},
			},
		},
	})

	for _, path := range []string{
		"/v1/evidence/ingest/prescribe",
		"/v1/evidence/ingest/report",
	} {
		req := httptest.NewRequest("POST", path, strings.NewReader(`{}`))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()

		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("%s expected 401, got %d body=%s", path, rec.Code, rec.Body.String())
		}
	}
}

func TestHandleIngestPrescribe_ValidPayload(t *testing.T) {
	t.Parallel()

	fakeSvc := &fakeIngestService{
		prescribeResult: ingest.Result{
			EntryID:        "entry-1",
			EffectiveRisk:  "high",
			PrescriptionID: "presc-1",
		},
	}
	handler := handleIngestPrescribe(fakeSvc)

	body := `{
		"contract_version":"v1",
		"actor":{"type":"agent","id":"demo-bot","provenance":"ci-runner"},
		"session_id":"sess-1",
		"operation_id":"op-1",
		"trace_id":"trace-1",
		"flavor":"imperative",
		"evidence":{"kind":"declared"},
		"source":{"system":"ci"},
		"smart_target":{"tool":"kubectl","operation":"apply","resource":"deployment/nginx"}
	}`
	req := httptest.NewRequest("POST", "/v1/evidence/ingest/prescribe", strings.NewReader(body))
	req = req.WithContext(auth.WithTenantID(req.Context(), "t-valid"))
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusAccepted {
		t.Fatalf("expected 202, got %d body=%s", rec.Code, rec.Body.String())
	}
	if fakeSvc.prescribeTenant != "t-valid" {
		t.Fatalf("tenant = %q, want t-valid", fakeSvc.prescribeTenant)
	}
	var resp map[string]any
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp["entry_id"] != "entry-1" {
		t.Fatalf("entry_id = %v, want entry-1", resp["entry_id"])
	}
	if resp["effective_risk"] != "high" {
		t.Fatalf("effective_risk = %v, want high", resp["effective_risk"])
	}
	if resp["duplicate"] != false {
		t.Fatalf("duplicate = %v, want false", resp["duplicate"])
	}
}

func TestHandleIngestPrescribe_ReturnsPrescriptionID(t *testing.T) {
	t.Parallel()

	fakeSvc := &fakeIngestService{
		prescribeResult: ingest.Result{
			EntryID:        "entry-42",
			EffectiveRisk:  "low",
			PrescriptionID: "presc-42",
		},
	}
	handler := handleIngestPrescribe(fakeSvc)

	body := `{
		"contract_version":"v1",
		"actor":{"type":"agent","id":"bot","provenance":"ci"},
		"session_id":"s1",
		"operation_id":"o1",
		"trace_id":"t1",
		"flavor":"imperative",
		"evidence":{"kind":"declared"},
		"source":{"system":"test"},
		"smart_target":{"tool":"helm","operation":"upgrade","resource":"release/app"}
	}`
	req := httptest.NewRequest("POST", "/v1/evidence/ingest/prescribe", strings.NewReader(body))
	req = req.WithContext(auth.WithTenantID(req.Context(), "tenant-1"))
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusAccepted {
		t.Fatalf("expected 202, got %d body=%s", rec.Code, rec.Body.String())
	}
	var resp struct {
		PrescriptionID string `json:"prescription_id"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.PrescriptionID != "presc-42" {
		t.Fatalf("prescription_id = %q, want presc-42", resp.PrescriptionID)
	}
}

func TestHandleIngestPrescribe_RejectsMissingActor(t *testing.T) {
	t.Parallel()

	fakeSvc := &fakeIngestService{}
	handler := handleIngestPrescribe(fakeSvc)

	// Envelope with empty actor fields — validation should reject it.
	body := `{
		"contract_version":"v1",
		"actor":{"type":"","id":"","provenance":""},
		"session_id":"s1",
		"operation_id":"o1",
		"trace_id":"t1",
		"flavor":"imperative",
		"evidence":{"kind":"declared"},
		"source":{"system":"test"},
		"smart_target":{"tool":"kubectl","operation":"apply","resource":"deploy/x"}
	}`
	req := httptest.NewRequest("POST", "/v1/evidence/ingest/prescribe", strings.NewReader(body))
	req = req.WithContext(auth.WithTenantID(req.Context(), "tenant-1"))
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestHandleIngestPrescribe_DuplicateReturns200(t *testing.T) {
	t.Parallel()

	fakeSvc := &fakeIngestService{
		prescribeResult: ingest.Result{
			Duplicate:      true,
			EntryID:        "entry-dup",
			EffectiveRisk:  "medium",
			PrescriptionID: "presc-dup",
		},
	}
	handler := handleIngestPrescribe(fakeSvc)

	body := `{
		"contract_version":"v1",
		"actor":{"type":"agent","id":"bot","provenance":"ci"},
		"session_id":"s1",
		"operation_id":"o1",
		"trace_id":"t1",
		"flavor":"imperative",
		"evidence":{"kind":"declared"},
		"source":{"system":"test"},
		"smart_target":{"tool":"kubectl","operation":"apply","resource":"deploy/x"}
	}`
	req := httptest.NewRequest("POST", "/v1/evidence/ingest/prescribe", strings.NewReader(body))
	req = req.WithContext(auth.WithTenantID(req.Context(), "tenant-1"))
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 for duplicate, got %d body=%s", rec.Code, rec.Body.String())
	}
	var resp struct {
		Duplicate bool `json:"duplicate"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if !resp.Duplicate {
		t.Fatal("expected duplicate=true")
	}
}

func TestHandleIngestReport_ValidPayload(t *testing.T) {
	t.Parallel()

	fakeSvc := &fakeIngestService{
		reportResult: ingest.Result{
			EntryID: "report-1",
		},
	}
	handler := handleIngestReport(fakeSvc)

	body := `{
		"contract_version":"v1",
		"actor":{"type":"agent","id":"bot","provenance":"ci"},
		"session_id":"s1",
		"operation_id":"o1",
		"trace_id":"t1",
		"flavor":"imperative",
		"evidence":{"kind":"declared"},
		"source":{"system":"test"},
		"prescription_id":"presc-1",
		"verdict":"success",
		"exit_code":0
	}`
	req := httptest.NewRequest("POST", "/v1/evidence/ingest/report", strings.NewReader(body))
	req = req.WithContext(auth.WithTenantID(req.Context(), "t-report"))
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusAccepted {
		t.Fatalf("expected 202, got %d body=%s", rec.Code, rec.Body.String())
	}
	if fakeSvc.reportTenant != "t-report" {
		t.Fatalf("tenant = %q, want t-report", fakeSvc.reportTenant)
	}
	var resp map[string]any
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp["entry_id"] != "report-1" {
		t.Fatalf("entry_id = %v, want report-1", resp["entry_id"])
	}
}

func TestHandleIngestReport_RejectsEmptyVerdict(t *testing.T) {
	t.Parallel()

	fakeSvc := &fakeIngestService{}
	handler := handleIngestReport(fakeSvc)

	// Valid envelope but missing verdict — validation rejects.
	body := `{
		"contract_version":"v1",
		"actor":{"type":"agent","id":"bot","provenance":"ci"},
		"session_id":"s1",
		"operation_id":"o1",
		"trace_id":"t1",
		"flavor":"imperative",
		"evidence":{"kind":"declared"},
		"source":{"system":"test"},
		"prescription_id":"presc-1",
		"verdict":"",
		"exit_code":0
	}`
	req := httptest.NewRequest("POST", "/v1/evidence/ingest/report", strings.NewReader(body))
	req = req.WithContext(auth.WithTenantID(req.Context(), "tenant-1"))
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestHandleIngestReport_LinksToExistingPrescription(t *testing.T) {
	t.Parallel()

	// Use the real service with a fakeIngestStore that has a pre-existing prescription.
	fakeStore := &fakeIngestStore{
		lastHash: "sha256:previous",
		entries: map[string]store.StoredEntry{
			"presc-link": {
				ID:        "presc-link",
				TenantID:  "tenant-1",
				EntryType: string(evidence.EntryTypePrescribe),
				SessionID: "session-link",
			},
		},
	}
	svc := ingest.NewService(fakeStore, testutil.TestSigner(t))
	handler := handleIngestReport(svc)

	body := `{
		"contract_version":"v1",
		"actor":{"type":"agent","id":"bot","provenance":"ci"},
		"session_id":"session-link",
		"operation_id":"op-link",
		"trace_id":"trace-link",
		"flavor":"imperative",
		"evidence":{"kind":"declared"},
		"source":{"system":"test"},
		"prescription_id":"presc-link",
		"verdict":"success",
		"exit_code":0
	}`
	req := httptest.NewRequest("POST", "/v1/evidence/ingest/report", strings.NewReader(body))
	req = req.WithContext(auth.WithTenantID(req.Context(), "tenant-1"))
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusAccepted {
		t.Fatalf("expected 202, got %d body=%s", rec.Code, rec.Body.String())
	}
	// The service should have looked up the prescription.
	if fakeStore.getEntryCalls != 1 {
		t.Fatalf("get entry calls = %d, want 1 (prescription lookup)", fakeStore.getEntryCalls)
	}
	// Verify the report was saved and links to the prescription.
	if len(fakeStore.savedRaw) != 1 {
		t.Fatalf("saved entries = %d, want 1", len(fakeStore.savedRaw))
	}
	// The saved raw is a full EvidenceEntry; the prescription_id lives
	// inside the nested payload object.
	var saved struct {
		Type    string          `json:"type"`
		Payload json.RawMessage `json:"payload"`
	}
	if err := json.Unmarshal(fakeStore.savedRaw[0], &saved); err != nil {
		t.Fatalf("unmarshal saved entry: %v", err)
	}
	if saved.Type != string(evidence.EntryTypeReport) {
		t.Fatalf("saved type = %q, want report", saved.Type)
	}
	var payload map[string]any
	if err := json.Unmarshal(saved.Payload, &payload); err != nil {
		t.Fatalf("unmarshal payload: %v", err)
	}
	if payload["prescription_id"] != "presc-link" {
		t.Fatalf("payload prescription_id = %v, want presc-link", payload["prescription_id"])
	}
}

func mustJSON(t *testing.T, v any) []byte {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return b
}

func intPtr(v int) *int {
	return &v
}
