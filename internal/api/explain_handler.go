package api

import (
	"errors"
	"net/http"

	"samebits.com/evidra-benchmark/internal/auth"
)

// ExplainComputer generates signal explanations from stored evidence.
type ExplainComputer interface {
	ComputeExplain(tenantID, period string) (interface{}, error)
}

func handleExplain(ec ExplainComputer) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		tenantID := auth.TenantID(r.Context())
		q := r.URL.Query()
		period := q.Get("period")
		if period == "" {
			period = "30d"
		}

		result, err := ec.ComputeExplain(tenantID, period)
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
