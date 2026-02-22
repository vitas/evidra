package api

import (
	"errors"
	"net/http"
	"strings"
)

func (s *Server) handleExports(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "method not allowed", nil, false)
		return
	}
	if err := s.authorizeExport(r); err != nil {
		if errors.Is(err, errRateLimited) {
			writeError(w, http.StatusTooManyRequests, "RATE_LIMITED", err.Error(), nil, true)
			return
		}
		writeError(w, http.StatusUnauthorized, "UNAUTHORIZED", err.Error(), nil, false)
		return
	}
	var in struct {
		Format string                 `json:"format"`
		Filter map[string]interface{} `json:"filter"`
	}
	if err := decodeJSON(r.Body, &in); err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_JSON", err.Error(), nil, false)
		return
	}
	job, err := s.service.CreateExport(r.Context(), in.Format, in.Filter)
	if err != nil {
		handleStoreErr(w, err)
		return
	}
	writeJSON(w, http.StatusAccepted, job)
}

func (s *Server) handleExportByID(w http.ResponseWriter, r *http.Request) {
	if err := s.authorizeExport(r); err != nil {
		if errors.Is(err, errRateLimited) {
			writeError(w, http.StatusTooManyRequests, "RATE_LIMITED", err.Error(), nil, true)
			return
		}
		writeError(w, http.StatusUnauthorized, "UNAUTHORIZED", err.Error(), nil, false)
		return
	}
	path := strings.TrimPrefix(r.URL.Path, "/v1/exports/")
	if strings.HasSuffix(path, "/download") {
		id := strings.TrimSuffix(path, "/download")
		id = strings.TrimSuffix(id, "/")
		s.handleExportDownload(w, r, id)
		return
	}
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "method not allowed", nil, false)
		return
	}
	job, err := s.service.GetExport(r.Context(), path)
	if err != nil {
		handleStoreErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, job)
}

func (s *Server) handleExportDownload(w http.ResponseWriter, r *http.Request, id string) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "method not allowed", nil, false)
		return
	}
	job, err := s.service.GetExport(r.Context(), id)
	if err != nil {
		handleStoreErr(w, err)
		return
	}
	if job.ArtifactURI == "" {
		writeError(w, http.StatusConflict, "EXPORT_NOT_READY", "export artifact is not ready", nil, true)
		return
	}
	b, err := s.service.ReadArtifact(job.ArtifactURI)
	if err != nil {
		handleStoreErr(w, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(b)
}
