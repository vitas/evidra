package api

import (
	"errors"
	"net/http"
	"strconv"

	"samebits.com/evidra-benchmark/internal/auth"
)

// ExplainComputer generates signal explanations from stored evidence.
type ExplainComputer interface {
	ComputeExplain(tenantID string, filters AnalyticsFilters) (interface{}, error)
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

		result, err := ec.ComputeExplain(tenantID, filters)
		if err != nil {
			if errors.Is(err, ErrExperimentalAnalytics) {
				writeError(w, http.StatusNotImplemented, experimentalAnalyticsMessage)
				return
			}
			writeError(w, http.StatusInternalServerError, "explain computation failed")
			return
		}

		writeJSON(w, http.StatusOK, result)
	}
}
