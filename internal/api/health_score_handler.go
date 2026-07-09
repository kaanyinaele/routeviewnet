package api

import (
	"net/http"
	"time"

	"routeviewnet/internal/storage"
)

func (s *Server) handleHealthScoreHistory(w http.ResponseWriter, r *http.Request) {
	dur, ok := rangeDuration(r.URL.Query().Get("range"), 24*time.Hour)
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid_request", "Invalid range parameter (use 15m,1h,6h,24h,7d)")
		return
	}
	cfg := s.Cfg.Get()
	// Health scores are event-driven, so bucket by the main interval.
	bucket := storage.BucketSeconds(int(dur.Seconds()), cfg.Server.MaxHistoryPoints, cfg.Collection.IntervalSeconds)
	points, err := s.DB.HealthScoreHistory(r.Context(), time.Now().Add(-dur), bucket)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"bucket_seconds": bucket, "points": points})
}
