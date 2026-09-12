package api

import (
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/gorilla/websocket"
)

// originAllowed implements §10.3.4 [v1.2]: CORS does not cover WebSockets,
// so the upgrade validates Origin itself. No Origin (curl, native clients)
// is allowed; browser origins must be the daemon's own or allow-listed.
func (s *Server) originAllowed(r *http.Request) bool {
	origin := r.Header.Get("Origin")
	if origin == "" {
		return true
	}
	u, err := url.Parse(origin)
	if err != nil {
		return false
	}
	host := strings.ToLower(u.Hostname())
	cfg := s.Cfg.Get()
	if host == "localhost" || host == "127.0.0.1" || host == "::1" || host == strings.ToLower(cfg.Server.Host) {
		return true
	}
	for _, allowed := range cfg.Server.CORSAllowedOrigins {
		if strings.EqualFold(origin, allowed) {
			return true
		}
	}
	// LAN mode: the browser reached us via a LAN IP, its Origin carries the
	// same host it used — match it against the request Host.
	if cfg.Server.Host == "0.0.0.0" {
		reqHost := r.Host
		if h, _, err2 := splitHostPortLoose(reqHost); err2 == nil {
			reqHost = h
		}
		if strings.EqualFold(host, reqHost) {
			return true
		}
	}
	return false
}

func splitHostPortLoose(hostport string) (string, string, error) {
	if !strings.Contains(hostport, ":") {
		return hostport, "", nil
	}
	i := strings.LastIndex(hostport, ":")
	return hostport[:i], hostport[i+1:], nil
}

func (s *Server) handleWebSocket(w http.ResponseWriter, r *http.Request) {
	if !s.originAllowed(r) {
		writeError(w, http.StatusForbidden, "forbidden_origin", "Origin not allowed for WebSocket")
		return
	}
	// Claim the slot before checking it: load-then-add lets concurrent
	// upgrades both see room and blow past the limit.
	maxClients := int64(s.Cfg.Get().Server.MaxWebsocketClients)
	if s.wsClients.Add(1) > maxClients {
		s.wsClients.Add(-1)
		// Reject with a clear reason rather than dropping old clients (§10.3.3).
		writeError(w, http.StatusServiceUnavailable, "too_many_clients",
			fmt.Sprintf("WebSocket client limit (%d) reached", maxClients))
		return
	}

	upgrader := websocket.Upgrader{
		ReadBufferSize:  1024,
		WriteBufferSize: 4096,
		CheckOrigin:     func(*http.Request) bool { return true }, // validated above
	}
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		s.wsClients.Add(-1)
		return
	}
	s.Log.Debug("websocket client connected", "clients", s.wsClients.Load())

	events := s.Bus.Subscribe()
	done := make(chan struct{})

	// Reader: only consumed for close/ping detection.
	go func() {
		defer close(done)
		conn.SetReadLimit(1024)
		for {
			if _, _, err := conn.ReadMessage(); err != nil {
				return
			}
		}
	}()

	// The write loop runs inline: returning from the handler would cancel
	// r.Context() and tear down the hijacked connection. App shutdown is
	// signaled via s.Shutdown (set from main's signal context).
	defer func() {
		s.Bus.Unsubscribe(events)
		conn.Close()
		s.wsClients.Add(-1)
	}()
	var shutdown <-chan struct{}
	if s.Shutdown != nil {
		shutdown = s.Shutdown.Done()
	}
	ping := time.NewTicker(30 * time.Second)
	defer ping.Stop()
	for {
		select {
		case <-done:
			return
		case <-shutdown:
			// Graceful shutdown: close 1001 going away (§14.9).
			_ = conn.WriteControl(websocket.CloseMessage,
				websocket.FormatCloseMessage(websocket.CloseGoingAway, "daemon shutting down"),
				time.Now().Add(time.Second))
			return
		case <-ping.C:
			if err := conn.WriteControl(websocket.PingMessage, nil, time.Now().Add(5*time.Second)); err != nil {
				return
			}
		case ev := <-events:
			conn.SetWriteDeadline(time.Now().Add(5 * time.Second))
			if err := conn.WriteJSON(ev); err != nil {
				return // slow/stalled client: drop (§10.3.4)
			}
		}
	}
}
