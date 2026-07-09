// Package api serves the versioned local HTTP API (§10), the WebSocket
// event stream (§10.3), and the embedded dashboard.
package api

import (
	"context"
	"encoding/json"
	"log/slog"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"routeviewnet/internal/collector"
	"routeviewnet/internal/config"
	"routeviewnet/internal/engine"
	"routeviewnet/internal/storage"
)

// Server carries handler dependencies.
type Server struct {
	DB      *storage.DB
	Bus     *engine.Bus
	Cfg     *config.Store
	Mgr     *collector.Manager
	Health  *engine.HealthEngine
	Log     *slog.Logger
	Version string
	Started time.Time

	// Shutdown is the app-level signal context; WebSocket write loops watch
	// it to send close 1001 on daemon shutdown (§14.9).
	Shutdown context.Context

	// StaticFS serves the embedded dashboard; nil in tests.
	Static http.Handler
}

func (s *Server) Router() http.Handler {
	r := chi.NewRouter()
	r.Use(middleware.Recoverer)
	r.Use(s.hostCheck)
	r.Use(s.cors)

	r.Route("/api/v1", func(r chi.Router) {
		r.Get("/health", s.handleHealth)
		r.Get("/overview", s.handleOverview)
		r.Get("/interfaces", s.handleInterfaces)
		r.Get("/metrics/bandwidth", s.handleBandwidth)
		r.Get("/traffic/apps", s.handleAppTraffic)
		r.Get("/checks/latency", s.handleLatencyHistory)
		r.Get("/checks/dns", s.handleDNSHistory)
		r.Get("/devices", s.handleListDevices)
		r.Patch("/devices/{id}", s.handlePatchDevice)
		r.Get("/alerts", s.handleListAlerts)
		r.Get("/events", s.handleListEvents)
		r.Get("/troubleshoot", s.handleTroubleshoot)
		r.Get("/settings", s.handleGetSettings)
		r.Post("/settings", s.handlePostSettings)
		r.Get("/system/memory", s.handleSystemMemory)
		r.Get("/system/memory/history", s.handleMemoryHistory)
		r.Get("/system/load", s.handleSystemLoad)
		r.Get("/system/processes/memory", s.handleProcessMemory)
		r.Get("/system/daemon/memory", s.handleDaemonMemory)
		r.Get("/health-score/history", s.handleHealthScoreHistory)
		r.Get("/live", s.handleWebSocket)
	})

	if s.Static != nil {
		r.NotFound(s.Static.ServeHTTP)
	}
	return r
}

// hostCheck rejects requests whose Host header is not local or allow-listed
// — DNS-rebinding protection (§10.1.3 [v1.2]).
func (s *Server) hostCheck(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !s.hostAllowed(r.Host) {
			writeError(w, http.StatusForbidden, "forbidden_host", "Host header not allowed")
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (s *Server) hostAllowed(hostport string) bool {
	host := hostport
	if h, _, err := net.SplitHostPort(hostport); err == nil {
		host = h
	}
	host = strings.Trim(strings.ToLower(host), "[]")
	switch host {
	case "localhost", "127.0.0.1", "::1", "":
		return true
	}
	cfg := s.Cfg.Get()
	if host == strings.ToLower(cfg.Server.Host) {
		return true
	}
	for _, allowed := range cfg.Server.AllowedHosts {
		if host == strings.ToLower(allowed) {
			return true
		}
	}
	// If bound to all interfaces the user opted into LAN access (§11.2);
	// accept the machine's own addresses by matching any private literal IP
	// the client used to reach a wildcard bind.
	if cfg.Server.Host == "0.0.0.0" {
		if ip := net.ParseIP(host); ip != nil {
			return true
		}
	}
	return false
}

// cors is restrictive (§10.1.2): no wildcard, only explicit allow-listed
// origins get CORS headers; everything else is same-origin only.
func (s *Server) cors(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")
		if origin != "" {
			for _, allowed := range s.Cfg.Get().Server.CORSAllowedOrigins {
				if strings.EqualFold(origin, allowed) {
					w.Header().Set("Access-Control-Allow-Origin", allowed)
					w.Header().Set("Vary", "Origin")
					break
				}
			}
		}
		if r.Method == http.MethodOptions {
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PATCH, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// --- helpers ---

func writeJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

// writeError uses the documented error envelope (§10.1).
func writeError(w http.ResponseWriter, status int, code, message string) {
	writeJSON(w, status, map[string]interface{}{
		"error": map[string]string{"code": code, "message": message},
	})
}

// rangeDuration parses the documented ?range values (§10.2).
func rangeDuration(s string, def time.Duration) (time.Duration, bool) {
	if s == "" {
		return def, true
	}
	m := map[string]time.Duration{
		"15m": 15 * time.Minute,
		"1h":  time.Hour,
		"6h":  6 * time.Hour,
		"24h": 24 * time.Hour,
		"7d":  7 * 24 * time.Hour,
	}
	d, ok := m[s]
	return d, ok
}
