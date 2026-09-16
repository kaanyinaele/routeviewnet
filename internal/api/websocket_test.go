package api

import (
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/gorilla/websocket"

	"routeviewnet/internal/config"
	"routeviewnet/internal/engine"
)

func wsTestServer(t *testing.T, maxClients int) (*Server, *httptest.Server) {
	t.Helper()
	cfg := config.Default()
	cfg.Server.MaxWebsocketClients = maxClients
	s := &Server{
		Cfg: config.NewStore(cfg),
		Bus: engine.NewBus(),
		Log: slog.New(slog.NewTextHandler(io.Discard, nil)),
	}
	ts := httptest.NewServer(s.Router())
	t.Cleanup(ts.Close)
	return s, ts
}

func wsURL(ts *httptest.Server) string {
	return "ws" + strings.TrimPrefix(ts.URL, "http") + "/api/v1/live"
}

// §10.3.3: over the cap, new clients are rejected with a reason rather than
// old ones being dropped.
func TestWebSocketClientLimit(t *testing.T) {
	s, ts := wsTestServer(t, 2)

	var conns []*websocket.Conn
	for i := 0; i < 2; i++ {
		c, _, err := websocket.DefaultDialer.Dial(wsURL(ts), nil)
		if err != nil {
			t.Fatalf("client %d should have been accepted: %v", i, err)
		}
		conns = append(conns, c)
	}
	t.Cleanup(func() {
		for _, c := range conns {
			c.Close()
		}
	})

	_, resp, err := websocket.DefaultDialer.Dial(wsURL(ts), nil)
	if err == nil {
		t.Fatal("the third client should have been rejected")
	}
	if resp == nil || resp.StatusCode != http.StatusServiceUnavailable {
		t.Errorf("want 503 over the cap, got %v", resp)
	}
	if got := s.wsClients.Load(); got != 2 {
		t.Errorf("a rejected upgrade must not leak a slot, got count %d", got)
	}
}

// Regression: the limit was checked with a load and only then incremented, so
// concurrent upgrades could all see room and blow past the cap together.
// Run with -race for the interleavings.
func TestWebSocketClientLimitUnderConcurrency(t *testing.T) {
	const max = 4
	s, ts := wsTestServer(t, max)

	var (
		mu       sync.Mutex
		accepted []*websocket.Conn
		wg       sync.WaitGroup
		start    = make(chan struct{})
	)
	for i := 0; i < max*4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			c, _, err := websocket.DefaultDialer.Dial(wsURL(ts), nil)
			if err != nil {
				return
			}
			mu.Lock()
			accepted = append(accepted, c)
			mu.Unlock()
		}()
	}
	close(start)
	wg.Wait()
	t.Cleanup(func() {
		for _, c := range accepted {
			c.Close()
		}
	})

	if len(accepted) > max {
		t.Errorf("accepted %d clients with a cap of %d", len(accepted), max)
	}
	if len(accepted) == 0 {
		t.Error("no client was accepted at all")
	}
	if got := s.wsClients.Load(); got != int64(len(accepted)) {
		t.Errorf("counter %d does not match %d live clients", got, len(accepted))
	}
}

// The counter lives on the Server, not in a package global, so two servers in
// one process do not share a budget.
func TestWebSocketLimitIsPerServer(t *testing.T) {
	_, tsA := wsTestServer(t, 1)
	_, tsB := wsTestServer(t, 1)

	a, _, err := websocket.DefaultDialer.Dial(wsURL(tsA), nil)
	if err != nil {
		t.Fatalf("server A should accept its first client: %v", err)
	}
	defer a.Close()

	b, _, err := websocket.DefaultDialer.Dial(wsURL(tsB), nil)
	if err != nil {
		t.Fatalf("server B has its own budget and should accept a client: %v", err)
	}
	defer b.Close()
}
