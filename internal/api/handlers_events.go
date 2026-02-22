package api

import (
	"errors"
	"net/http"
	"time"

	ce "evidra/internal/cloudevents"
	"evidra/internal/store"
)

func (s *Server) handleHealth(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"status":  "ok",
		"service": "evidra",
		"time":    time.Now().UTC().Format(time.RFC3339),
	})
}

func (s *Server) handleEvents(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "method not allowed", nil, false)
		return
	}
	body, err := readBodyLimited(w, r, maxIngestBodyBytes)
	if err != nil {
		var maxErr *http.MaxBytesError
		if errors.As(err, &maxErr) {
			writeError(w, http.StatusRequestEntityTooLarge, "PAYLOAD_TOO_LARGE", "request body is too large", nil, false)
			return
		}
		writeError(w, http.StatusBadRequest, "INVALID_BODY", "unable to read body", nil, false)
		return
	}
	if err := s.authorizeIngest(r, body); err != nil {
		if errors.Is(err, errRateLimited) {
			writeError(w, http.StatusTooManyRequests, "RATE_LIMITED", err.Error(), nil, true)
			return
		}
		writeError(w, http.StatusUnauthorized, "UNAUTHORIZED", err.Error(), nil, false)
		return
	}

	events, err := ce.ParseRequest(r, body)
	if err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_CLOUDEVENT", err.Error(), nil, false)
		return
	}

	results := make([]map[string]interface{}, 0, len(events))
	lastStatus := http.StatusAccepted
	for _, event := range events {
		status, ingestedAt, err := s.service.IngestEvent(r.Context(), event)
		if err != nil {
			if errors.Is(err, store.ErrConflict) {
				writeError(w, http.StatusConflict, "EVENT_ID_CONFLICT", "event id already exists with different payload", nil, false)
				return
			}
			handleStoreErr(w, err)
			return
		}
		httpStatus := http.StatusAccepted
		if status == store.IngestDuplicate {
			httpStatus = http.StatusOK
			lastStatus = http.StatusOK
		}
		_ = httpStatus
		results = append(results, map[string]interface{}{
			"id":          event.ID,
			"status":      status,
			"ingested_at": ingestedAt.Format(time.RFC3339),
		})
	}

	if len(results) == 1 {
		writeJSON(w, lastStatus, results[0])
	} else {
		writeJSON(w, http.StatusAccepted, results)
	}
}
