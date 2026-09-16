// Package api serves the versioned local HTTP API (§10), the WebSocket
// event stream (§10.3), and the embedded dashboard.
package api

import (
	"context"
	"encoding/json"
	"log/slog"
	"mime"
	"net"
	"net/http"
	"strings"
	"sync/atomic"
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

	// SetLogLevel re-points the daemon's log level after a settings change;
	// nil in tests.
	SetLogLevel func(level string)

	// wsClients counts live WebSocket connections against
	// server.max_websocket_clients. Per-Server, not package-global, so two
	// servers in one process (tests) do not share a budget.
	wsClients atomic.Int64
}

func (s *Server) Router() http.Handler {
	r := chi.NewRouter()
	r.Use(middleware.Recoverer)
	r.Use(s.hostCheck)
	r.Use(s.cors)
	r.Use(s.csrfGuard)

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
		r.Post("/notifications/test", s.handleTestNotification)
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
	//
	// Restricting this to addresses actually assigned to this host was
	// considered and rejected: a rebinding attack arrives with the attacker's
	// *hostname* in Host (rejected above), never a bare IP, so the narrower
	// check buys nothing — while breaking legitimate access through NAT, a
	// port-forward, or a container where InterfaceAddrs cannot see the address
	// the client actually used.
	if cfg.Server.Host == "0.0.0.0" {
		if ip := net.ParseIP(host); ip != nil {
			return true
		}
	}
	return false
}

// csrfGuard blocks cross-site state-changing requests.
//
// The API has no authentication, so the browser's ambient authority *is* the
// authority: any page the user visits can POST here. CORS does not help —
// it governs whether a response may be read, not whether a request is sent,
// and the damage is done by the time the response is discarded.
//
// Two checks, because either alone has a hole:
//
//   - Content-Type must be JSON. A form can only send text/plain,
//     multipart/form-data or application/x-www-form-urlencoded without a CORS
//     preflight, so requiring JSON forces any cross-origin attempt through a
//     preflight, which the allowlist in cors() then refuses. Without this, a
//     form with enctype="text/plain" posts a body that json.Decode happily
//     parses — the trailing "=" it appends is ignored, since Decode reads one
//     value and stops.
//   - Origin must be allowed when present. Browsers have sent Origin on
//     cross-origin POSTs for years; non-browser clients (curl, scripts) send
//     none, which is why this cannot be the only check.
func (s *Server) csrfGuard(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet, http.MethodHead, http.MethodOptions:
			next.ServeHTTP(w, r)
			return
		}
		mediaType, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
		if err != nil || mediaType != "application/json" {
			writeError(w, http.StatusUnsupportedMediaType, "unsupported_media_type",
				"State-changing requests must send Content-Type: application/json")
			return
		}
		if !s.originAllowed(r) {
			writeError(w, http.StatusForbidden, "forbidden_origin", "Origin not allowed")
			return
		}
		next.ServeHTTP(w, r)
	})
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
