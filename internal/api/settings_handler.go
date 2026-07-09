package api

import (
	"encoding/json"
	"net/http"
)

// SettingsOverrideKey is where the applied settings snapshot persists so it
// survives restarts (§9.2.12). main re-applies it at startup.
const SettingsOverrideKey = "config_override"

func (s *Server) handleGetSettings(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, s.Cfg.Get())
}

// handlePostSettings takes a full Config document (GET → mutate → POST),
// enforcing the §11.7 hardening: 64KB body cap, unknown fields rejected.
func (s *Server) handlePostSettings(w http.ResponseWriter, r *http.Request) {
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 64*1024))
	dec.DisallowUnknownFields()

	// Start from current config so a partial document keeps current values.
	next := s.Cfg.Get()
	if err := dec.Decode(&next); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "Invalid settings body: "+err.Error())
		return
	}

	prev := s.Cfg.Get()
	restart, err := s.Cfg.Apply(next)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}

	// Persist the applied snapshot so it survives restarts.
	applied := s.Cfg.Get()
	if blob, err := json.Marshal(applied); err == nil {
		if err := s.DB.SaveSetting(r.Context(), SettingsOverrideKey, string(blob)); err != nil {
			s.Log.Warn("persist settings failed", "error", err)
		}
	}

	// LAN-access warning (§11.2).
	if next.Server.Host == "0.0.0.0" && prev.Server.Host != "0.0.0.0" {
		s.Log.Warn("settings request enables LAN access: the dashboard and API will be reachable from other machines after restart; there is no authentication in v1")
	}
	if restart {
		s.Log.Warn("settings changed that require a daemon restart to take effect")
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"settings":         applied,
		"restart_required": restart,
	})
}
