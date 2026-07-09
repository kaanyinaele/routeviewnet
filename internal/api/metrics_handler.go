package api

import (
	"net/http"
	"time"

	"routeviewnet/internal/storage"
)

func (s *Server) handleInterfaces(w http.ResponseWriter, r *http.Request) {
	ifaces, err := s.DB.ListInterfaces(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"items": ifaces})
}

// handleAppTraffic serves per-app data usage aggregated over the range.
// TCP connections only; the note travels with the payload so every client
// shows the same caveat.
func (s *Server) handleAppTraffic(w http.ResponseWriter, r *http.Request) {
	dur, ok := rangeDuration(r.URL.Query().Get("range"), time.Hour)
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid_request", "Invalid range parameter (use 15m,1h,6h,24h,7d)")
		return
	}
	items, err := s.DB.AppTrafficTotals(r.Context(), time.Now().Add(-dur))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"items": items,
		"note":  "Direct (TCP) connections only. Traffic using other protocols, like some video calls, is not counted.",
	})
}

// handleBandwidth serves downsampled rate history (§10.1.4). The interface
// defaults to the current primary.
func (s *Server) handleBandwidth(w http.ResponseWriter, r *http.Request) {
	dur, ok := rangeDuration(r.URL.Query().Get("range"), time.Hour)
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid_request", "Invalid range parameter (use 15m,1h,6h,24h,7d)")
		return
	}
	iface := r.URL.Query().Get("interface")
	if iface == "" {
		iface = s.Mgr.Primary()
	}
	cfg := s.Cfg.Get()
	bucket := storage.BucketSeconds(int(dur.Seconds()), cfg.Server.MaxHistoryPoints, cfg.Collection.IntervalSeconds)
	points, err := s.DB.BandwidthHistory(r.Context(), iface, time.Now().Add(-dur), bucket)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"interface":      iface,
		"bucket_seconds": bucket,
		"points":         points,
	})
}

func (s *Server) handleLatencyHistory(w http.ResponseWriter, r *http.Request) {
	dur, ok := rangeDuration(r.URL.Query().Get("range"), time.Hour)
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid_request", "Invalid range parameter")
		return
	}
	checks, err := s.DB.RecentLatencyChecks(r.Context(), time.Now().Add(-dur), 500)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"items": checks})
}

func (s *Server) handleDNSHistory(w http.ResponseWriter, r *http.Request) {
	dur, ok := rangeDuration(r.URL.Query().Get("range"), time.Hour)
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid_request", "Invalid range parameter")
		return
	}
	checks, err := s.DB.RecentDNSChecks(r.Context(), time.Now().Add(-dur), 500)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"items": checks})
}
