package api

import (
	"errors"
	"net/http"

	"samebits.com/evidra-benchmark/internal/auth"
)

// ScorecardComputer generates scorecards from stored evidence.
type ScorecardComputer interface {
	ComputeScorecard(tenantID, period, tool, scope string, minOps int) (interface{}, error)
}

func handleScorecard(sc ScorecardComputer) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		tenantID := auth.TenantID(r.Context())
		q := r.URL.Query()
		period := q.Get("period")
		if period == "" {
			period = "30d"
		}

		result, err := sc.ComputeScorecard(tenantID, period, q.Get("tool"), q.Get("scope"), 0)
		if err != nil {
			if errors.Is(err, ErrExperimentalAnalytics) {
				writeError(w, http.StatusNotImplemented, experimentalAnalyticsMessage)
				return
			}
			writeError(w, http.StatusInternalServerError, "scorecard computation failed")
			return
		}

		writeJSON(w, http.StatusOK, result)
	}
}
