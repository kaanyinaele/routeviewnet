package api

import "net/http"

func (s *Server) handleListAlerts(w http.ResponseWriter, r *http.Request) {
	limit, cursor, err := parsePagination(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "Invalid cursor")
		return
	}
	status := r.URL.Query().Get("status")
	severity := r.URL.Query().Get("severity")
	switch status {
	case "", "open", "resolved":
	default:
		writeError(w, http.StatusBadRequest, "invalid_request", "status must be open or resolved")
		return
	}
	switch severity {
	case "", "info", "warning", "critical":
	default:
		writeError(w, http.StatusBadRequest, "invalid_request", "severity must be info, warning, or critical")
		return
	}
	alerts, next, err := s.DB.ListAlerts(r.Context(), status, severity, limit, cursor)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"items": alerts, "next_cursor": nullable(next)})
}

func (s *Server) handleListEvents(w http.ResponseWriter, r *http.Request) {
	limit, cursor, err := parsePagination(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "Invalid cursor")
		return
	}
	events, next, err := s.DB.ListEvents(r.Context(), limit, cursor)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"items": events, "next_cursor": nullable(next)})
}
