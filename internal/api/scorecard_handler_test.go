package api

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"samebits.com/evidra-benchmark/internal/auth"
)

type captureScorecardComputer struct {
	tenantID string
	filters  AnalyticsFilters
}

func (c *captureScorecardComputer) ComputeScorecard(tenantID string, filters AnalyticsFilters) (interface{}, error) {
	c.tenantID = tenantID
	c.filters = filters
	return map[string]string{"status": "ok"}, nil
}

func TestHandleScorecard_ForwardsTenantWideFilters(t *testing.T) {
	t.Parallel()

	capture := &captureScorecardComputer{}
	req := httptest.NewRequest("GET", "/v1/evidence/scorecard?period=7d&actor=agent-1&tool=kubectl&scope=production&session_id=sess-123&min_operations=25", nil)
	req = req.WithContext(auth.WithTenantID(req.Context(), "tenant-1"))
	rec := httptest.NewRecorder()

	handleScorecard(capture).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	if capture.tenantID != "tenant-1" {
		t.Fatalf("tenant_id = %q, want tenant-1", capture.tenantID)
	}
	if capture.filters.Period != "7d" {
		t.Fatalf("period = %q, want 7d", capture.filters.Period)
	}
	if capture.filters.Actor != "agent-1" {
		t.Fatalf("actor = %q, want agent-1", capture.filters.Actor)
	}
	if capture.filters.Tool != "kubectl" {
		t.Fatalf("tool = %q, want kubectl", capture.filters.Tool)
	}
	if capture.filters.Scope != "production" {
		t.Fatalf("scope = %q, want production", capture.filters.Scope)
	}
	if capture.filters.SessionID != "sess-123" {
		t.Fatalf("session_id = %q, want sess-123", capture.filters.SessionID)
	}
	if capture.filters.MinOperations != 25 {
		t.Fatalf("min_operations = %d, want 25", capture.filters.MinOperations)
	}
}

func TestHandleScorecard_DefaultsToTenantWidePeriod(t *testing.T) {
	t.Parallel()

	capture := &captureScorecardComputer{}
	req := httptest.NewRequest("GET", "/v1/evidence/scorecard", nil)
	req = req.WithContext(auth.WithTenantID(req.Context(), "tenant-1"))
	rec := httptest.NewRecorder()

	handleScorecard(capture).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	if capture.filters.Period != "30d" {
		t.Fatalf("period = %q, want 30d", capture.filters.Period)
	}
	if capture.filters.Actor != "" || capture.filters.Tool != "" || capture.filters.Scope != "" || capture.filters.SessionID != "" {
		t.Fatalf("expected tenant-wide defaults, got %+v", capture.filters)
	}
	if capture.filters.MinOperations != 0 {
		t.Fatalf("min_operations = %d, want 0", capture.filters.MinOperations)
	}
}
