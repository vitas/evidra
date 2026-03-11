package api

import (
	"context"
	"net/http"
	"strconv"

	"samebits.com/evidra/internal/auth"
)

// ExplainComputer generates signal explanations from stored evidence.
type ExplainComputer interface {
	ComputeExplain(ctx context.Context, tenantID string, filters AnalyticsFilters) (interface{}, error)
}

func handleExplain(ec ExplainComputer) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		tenantID := auth.TenantID(r.Context())
		q := r.URL.Query()
		filters := AnalyticsFilters{
			Period:    q.Get("period"),
			Actor:     q.Get("actor"),
			Tool:      q.Get("tool"),
			Scope:     q.Get("scope"),
			SessionID: q.Get("session_id"),
		}
		if filters.Period == "" {
			filters.Period = "30d"
		}
		if raw := q.Get("min_operations"); raw != "" {
			minOps, err := strconv.Atoi(raw)
			if err != nil {
				writeError(w, http.StatusBadRequest, "invalid min_operations")
				return
			}
			filters.MinOperations = minOps
		}

		result, err := ec.ComputeExplain(r.Context(), tenantID, filters)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "explain computation failed")
			return
		}

		writeJSON(w, http.StatusOK, result)
	}
}
