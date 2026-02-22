package api

import (
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"evidra/internal/app"
)

func (s *Server) handleChanges(w http.ResponseWriter, r *http.Request) {
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
	q, err := parseChangeQuery(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_QUERY", err.Error(), nil, false)
		return
	}
	items, err := s.service.ListChanges(r.Context(), q)
	if err != nil {
		if errors.Is(err, app.ErrInvalidChangeCursor) {
			writeError(w, http.StatusBadRequest, "INVALID_CURSOR", err.Error(), nil, false)
			return
		}
		handleStoreErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"items": items.Items,
		"page": map[string]interface{}{
			"limit":       q.Limit,
			"next_cursor": items.NextCursor,
		},
	})
}

func (s *Server) handleChangeByID(w http.ResponseWriter, r *http.Request) {
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
	id, view := parseChangePath(r.URL.Path)
	if strings.TrimSpace(id) == "" {
		writeError(w, http.StatusBadRequest, "INVALID_CHANGE_ID", "change id is required", nil, false)
		return
	}
	q, err := parseChangeQuery(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_QUERY", err.Error(), nil, false)
		return
	}
	switch view {
	case "timeline":
		items, err := s.service.GetChangeTimeline(r.Context(), id, q)
		if err != nil {
			handleStoreErr(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]interface{}{"items": items})
	case "evidence":
		item, err := s.service.GetChangeEvidence(r.Context(), id, q)
		if err != nil {
			handleStoreErr(w, err)
			return
		}
		writeJSON(w, http.StatusOK, item)
	case "detail":
		item, err := s.service.GetChange(r.Context(), id, q)
		if err != nil {
			handleStoreErr(w, err)
			return
		}
		writeJSON(w, http.StatusOK, item)
	default:
		writeError(w, http.StatusNotFound, "NOT_FOUND", "route not found", nil, false)
	}
}

func parseChangePath(path string) (id string, view string) {
	tail := strings.TrimPrefix(path, "/v1/changes/")
	tail = strings.TrimSpace(tail)
	if tail == "" {
		return "", "detail"
	}
	if strings.HasSuffix(tail, "/timeline") {
		id = strings.TrimSuffix(tail, "/timeline")
		id = strings.TrimSuffix(id, "/")
		return id, "timeline"
	}
	if strings.HasSuffix(tail, "/evidence") {
		id = strings.TrimSuffix(tail, "/evidence")
		id = strings.TrimSuffix(id, "/")
		return id, "evidence"
	}
	if strings.Contains(tail, "/") {
		return "", "unknown"
	}
	return tail, "detail"
}

func parseChangeQuery(r *http.Request) (app.ChangeQuery, error) {
	from, err := time.Parse(time.RFC3339, r.URL.Query().Get("from"))
	if err != nil {
		return app.ChangeQuery{}, errors.New("invalid from")
	}
	to, err := time.Parse(time.RFC3339, r.URL.Query().Get("to"))
	if err != nil {
		return app.ChangeQuery{}, errors.New("invalid to")
	}
	subjectRaw := strings.TrimSpace(r.URL.Query().Get("subject"))
	if subjectRaw == "" {
		return app.ChangeQuery{}, errors.New("subject is required")
	}
	subject, err := app.ParseSubject(subjectRaw)
	if err != nil {
		return app.ChangeQuery{}, errors.New("subject must be app:environment:cluster")
	}
	q := app.ChangeQuery{
		Subject:               subject,
		From:                  from.UTC(),
		To:                    to.UTC(),
		Q:                     strings.TrimSpace(r.URL.Query().Get("q")),
		ResultStatus:          strings.TrimSpace(r.URL.Query().Get("result_status")),
		HealthStatus:          strings.TrimSpace(r.URL.Query().Get("health_status")),
		ExternalChangeID:      strings.TrimSpace(r.URL.Query().Get("external_change_id")),
		ExternalChangeIDState: strings.ToLower(strings.TrimSpace(r.URL.Query().Get("external_change_id_state"))),
		TicketID:              strings.TrimSpace(r.URL.Query().Get("ticket_id")),
		TicketIDState:         strings.ToLower(strings.TrimSpace(r.URL.Query().Get("ticket_id_state"))),
		ApprovalReference:     strings.TrimSpace(r.URL.Query().Get("approval_reference")),
		HasApprovals:          strings.ToLower(strings.TrimSpace(r.URL.Query().Get("has_approvals"))),
		Limit:                 100,
		Cursor:                strings.TrimSpace(r.URL.Query().Get("cursor")),
	}
	if q.ExternalChangeIDState != "" && q.ExternalChangeIDState != "any" && q.ExternalChangeIDState != "set" && q.ExternalChangeIDState != "unset" {
		return app.ChangeQuery{}, errors.New("external_change_id_state must be one of any|set|unset")
	}
	if q.TicketIDState != "" && q.TicketIDState != "any" && q.TicketIDState != "set" && q.TicketIDState != "unset" {
		return app.ChangeQuery{}, errors.New("ticket_id_state must be one of any|set|unset")
	}
	if q.HasApprovals != "" && q.HasApprovals != "any" && q.HasApprovals != "yes" && q.HasApprovals != "no" {
		return app.ChangeQuery{}, errors.New("has_approvals must be one of any|yes|no")
	}
	if rawLimit := strings.TrimSpace(r.URL.Query().Get("limit")); rawLimit != "" {
		if _, err := fmt.Sscanf(rawLimit, "%d", &q.Limit); err != nil {
			return app.ChangeQuery{}, errors.New("invalid limit")
		}
	}
	return q, nil
}
