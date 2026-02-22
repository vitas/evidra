package api

import (
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"evidra/internal/app"
	"evidra/internal/store"
)

func (s *Server) handleTimeline(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "method not allowed", nil, false)
		return
	}
	if err := s.authorizeRead(r); err != nil {
		if errors.Is(err, errRateLimited) {
			writeError(w, http.StatusTooManyRequests, "RATE_LIMITED", err.Error(), nil, true)
			return
		}
		writeError(w, http.StatusUnauthorized, "UNAUTHORIZED", err.Error(), nil, false)
		return
	}
	from, err := time.Parse(time.RFC3339, r.URL.Query().Get("from"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_FROM", "invalid from", nil, false)
		return
	}
	to, err := time.Parse(time.RFC3339, r.URL.Query().Get("to"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_TO", "invalid to", nil, false)
		return
	}
	q := store.TimelineQuery{
		From:   from.UTC(),
		To:     to.UTC(),
		Source: r.URL.Query().Get("source"),
		Type:   r.URL.Query().Get("type"),
		IncludeSupporting: strings.EqualFold(
			strings.TrimSpace(r.URL.Query().Get("include_supporting")),
			"true",
		),
		Cursor: r.URL.Query().Get("cursor"),
	}
	if limit := r.URL.Query().Get("limit"); limit != "" {
		fmt.Sscanf(limit, "%d", &q.Limit)
	}
	subject := r.URL.Query().Get("subject")
	if subject != "" {
		parsed, err := app.ParseSubject(subject)
		if err != nil {
			writeError(w, http.StatusBadRequest, "INVALID_SUBJECT", "subject must be app:environment:cluster", nil, false)
			return
		}
		q.Subject = parsed.App
		q.Namespace = parsed.Environment
		q.Cluster = parsed.Cluster
	}
	// support direct extension-based correlation via query params
	q.CorrelationKey = r.URL.Query().Get("correlation_key")
	q.CorrelationValue = r.URL.Query().Get("correlation_value")

	if q.Subject == "" && (q.CorrelationKey == "" || q.CorrelationValue == "") {
		writeError(w, http.StatusBadRequest, "SCOPE_REQUIRED", "subject or correlation filters required", nil, false)
		return
	}
	res, err := s.service.QueryTimeline(r.Context(), q)
	if err != nil {
		handleStoreErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"items": res.Items,
		"page": map[string]interface{}{
			"limit":       q.Limit,
			"next_cursor": res.NextCursor,
		},
	})
}

func (s *Server) handleEventByID(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "method not allowed", nil, false)
		return
	}
	if err := s.authorizeRead(r); err != nil {
		if errors.Is(err, errRateLimited) {
			writeError(w, http.StatusTooManyRequests, "RATE_LIMITED", err.Error(), nil, true)
			return
		}
		writeError(w, http.StatusUnauthorized, "UNAUTHORIZED", err.Error(), nil, false)
		return
	}
	id := strings.TrimPrefix(r.URL.Path, "/v1/events/")
	e, err := s.service.GetEvent(r.Context(), id)
	if err != nil {
		handleStoreErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, e)
}

func (s *Server) handleSubjects(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "method not allowed", nil, false)
		return
	}
	if err := s.authorizeRead(r); err != nil {
		if errors.Is(err, errRateLimited) {
			writeError(w, http.StatusTooManyRequests, "RATE_LIMITED", err.Error(), nil, true)
			return
		}
		writeError(w, http.StatusUnauthorized, "UNAUTHORIZED", err.Error(), nil, false)
		return
	}
	items, err := s.service.ListSubjects(r.Context())
	if err != nil {
		handleStoreErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"items": items})
}

func (s *Server) handleCorrelations(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "method not allowed", nil, false)
		return
	}
	if err := s.authorizeRead(r); err != nil {
		if errors.Is(err, errRateLimited) {
			writeError(w, http.StatusTooManyRequests, "RATE_LIMITED", err.Error(), nil, true)
			return
		}
		writeError(w, http.StatusUnauthorized, "UNAUTHORIZED", err.Error(), nil, false)
		return
	}
	key := strings.TrimPrefix(r.URL.Path, "/v1/correlations/")
	value := r.URL.Query().Get("value")
	items, err := s.service.EventsByExtension(r.Context(), key, value, 100)
	if err != nil {
		handleStoreErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"items": items})
}
