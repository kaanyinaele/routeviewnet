package api

import (
	"net/http"
	"time"

	"routeviewnet/internal/storage"
)

func (s *Server) handleSystemMemory(w http.ResponseWriter, r *http.Request) {
	m, err := s.DB.LatestMemoryMetrics(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, m)
}

func (s *Server) handleMemoryHistory(w http.ResponseWriter, r *http.Request) {
	dur, ok := rangeDuration(r.URL.Query().Get("range"), time.Hour)
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid_request", "Invalid range parameter (use 15m,1h,6h,24h,7d)")
		return
	}
	cfg := s.Cfg.Get()
	bucket := storage.BucketSeconds(int(dur.Seconds()), cfg.Server.MaxHistoryPoints, cfg.Collection.MemoryIntervalSeconds)
	points, err := s.DB.MemoryHistory(r.Context(), time.Now().Add(-dur), bucket)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"bucket_seconds": bucket, "points": points})
}

func (s *Server) handleSystemLoad(w http.ResponseWriter, r *http.Request) {
	m, err := s.DB.LatestLoadMetrics(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, m)
}

func (s *Server) handleProcessMemory(w http.ResponseWriter, r *http.Request) {
	procs, err := s.DB.LatestProcessMemory(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"items": procs})
}

func (s *Server) handleDaemonMemory(w http.ResponseWriter, r *http.Request) {
	m, err := s.DB.LatestDaemonMemory(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, m)
}
