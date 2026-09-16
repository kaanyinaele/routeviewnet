package api

import (
	"encoding/json"
	"net/http"
)

// SettingsOverrideKey is where the applied settings snapshot persists so it
// survives restarts (§9.2.12). main re-applies it at startup.
const SettingsOverrideKey = "config_override"

// handleGetSettings serves the document the user is editing: the live config
// plus any restart-only changes already requested but not yet in effect.
// Serving the live config here would make a pending change look discarded.
func (s *Server) handleGetSettings(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, s.Cfg.Desired())
}

// handlePostSettings takes a full Config document (GET → mutate → POST),
// enforcing the §11.7 hardening: 64KB body cap, unknown fields rejected.
func (s *Server) handlePostSettings(w http.ResponseWriter, r *http.Request) {
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 64*1024))
	dec.DisallowUnknownFields()

	// Start from the current document so a partial body keeps current values.
	next := s.Cfg.Desired()
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

	// Persist what the user asked for — not the live config, which still
	// holds the old values for restart-only fields. Persisting the live copy
	// would discard the change the response just promised to deliver.
	applied := s.Cfg.Desired()
	if blob, err := json.Marshal(applied); err == nil {
		if err := s.DB.SaveSetting(r.Context(), SettingsOverrideKey, string(blob)); err != nil {
			s.Log.Warn("persist settings failed", "error", err)
			writeError(w, http.StatusInternalServerError, "internal",
				"Settings applied to the running daemon but could not be saved; they will be lost on restart")
			return
		}
	}

	if s.SetLogLevel != nil {
		s.SetLogLevel(applied.Logging.Level)
	}

	// LAN-access warning (§11.2).
	if applied.Server.Host == "0.0.0.0" && prev.Server.Host != "0.0.0.0" {
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
