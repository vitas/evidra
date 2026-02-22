package api

import "net/http"

func (s *Server) Routes() http.Handler {
	mux := http.NewServeMux()
	ui := uiHandler()
	mux.HandleFunc("/ui", func(w http.ResponseWriter, r *http.Request) { serveUI(w, r, ui) })
	mux.HandleFunc("/ui/", func(w http.ResponseWriter, r *http.Request) { serveUI(w, r, ui) })
	mux.HandleFunc("/healthz", s.handleHealth)
	mux.HandleFunc("/v1/events", s.handleEvents)
	mux.HandleFunc("/v1/timeline", s.handleTimeline)
	mux.HandleFunc("/v1/changes", s.handleChanges)
	mux.HandleFunc("/v1/changes/", s.handleChangeByID)
	mux.HandleFunc("/v1/events/", s.handleEventByID)
	mux.HandleFunc("/v1/subjects", s.handleSubjects)
	mux.HandleFunc("/v1/correlations/", s.handleCorrelations)
	mux.HandleFunc("/v1/exports", s.handleExports)
	mux.HandleFunc("/v1/exports/", s.handleExportByID)
	return mux
}
