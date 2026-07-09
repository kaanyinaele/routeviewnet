package api

import (
	"io"
	"log/slog"
	"net/http"
	"testing"

	"routeviewnet/internal/config"
)

func testServer(host string, allowedHosts, corsOrigins []string) *Server {
	cfg := config.Default()
	cfg.Server.Host = host
	cfg.Server.AllowedHosts = allowedHosts
	cfg.Server.CORSAllowedOrigins = corsOrigins
	return &Server{
		Cfg: config.NewStore(cfg),
		Log: slog.New(slog.NewTextHandler(io.Discard, nil)),
	}
}

// §10.1.3 [v1.2]: DNS-rebinding protection via Host validation.
func TestHostAllowed(t *testing.T) {
	s := testServer("127.0.0.1", []string{"nas.local"}, nil)
	for host, want := range map[string]bool{
		"localhost:4545":     true,
		"127.0.0.1:4545":     true,
		"[::1]:4545":         true,
		"localhost":          true,
		"nas.local:4545":     true,
		"evil.example.com":   false, // DNS rebinding hostname
		"evil.example:4545":  false,
		"192.168.0.5:4545":   false, // not bound to 0.0.0.0
	} {
		if got := s.hostAllowed(host); got != want {
			t.Errorf("hostAllowed(%q) = %v, want %v", host, got, want)
		}
	}

	// LAN mode: literal IPs accepted when bound to 0.0.0.0.
	lan := testServer("0.0.0.0", nil, nil)
	if !lan.hostAllowed("192.168.0.5:4545") {
		t.Error("LAN mode must accept literal IP hosts")
	}
	if lan.hostAllowed("evil.example.com") {
		t.Error("LAN mode must still reject foreign hostnames")
	}
}

// §10.3.4 [v1.2]: WebSocket Origin validation.
func TestOriginAllowed(t *testing.T) {
	s := testServer("127.0.0.1", nil, []string{"http://localhost:3000"})
	mkReq := func(origin string) *http.Request {
		r, _ := http.NewRequest("GET", "http://localhost:4545/api/v1/live", nil)
		if origin != "" {
			r.Header.Set("Origin", origin)
		}
		return r
	}
	for origin, want := range map[string]bool{
		"":                          true, // non-browser clients
		"http://localhost:4545":     true,
		"http://127.0.0.1:4545":     true,
		"http://localhost:3000":     true,  // allow-listed dev server
		"https://evil.example.com":  false, // cross-site WebSocket hijack
		"http://192.168.0.9:4545":   false,
	} {
		if got := s.originAllowed(mkReq(origin)); got != want {
			t.Errorf("originAllowed(%q) = %v, want %v", origin, got, want)
		}
	}
}

func TestRangeDuration(t *testing.T) {
	for _, valid := range []string{"15m", "1h", "6h", "24h", "7d"} {
		if _, ok := rangeDuration(valid, 0); !ok {
			t.Errorf("range %q should be valid", valid)
		}
	}
	if _, ok := rangeDuration("3w", 0); ok {
		t.Error("range 3w should be invalid")
	}
}
