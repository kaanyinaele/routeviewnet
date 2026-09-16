package engine

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"routeviewnet/internal/models"
)

func quietLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, &slog.HandlerOptions{Level: slog.LevelError}))
}

// recorder collects the payloads a test endpoint received.
type recorder struct {
	mu   sync.Mutex
	got  []WebhookPayload
	hits int
}

func (r *recorder) add(p WebhookPayload) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.got = append(r.got, p)
}

func (r *recorder) payloads() []WebhookPayload {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]WebhookPayload(nil), r.got...)
}

// startNotifier wires a bus to a notifier pointed at url and returns a stop
// func that cancels and drains it.
func startNotifier(t *testing.T, bus *Bus, url string) func() {
	t.Helper()
	n := NewNotifier(bus, quietLogger(),
		func() string { return url },
		func() time.Duration { return 2 * time.Second })
	// These tests point the notifier at httptest servers, which bind to
	// loopback — exactly what the production client refuses, to stop a
	// chosen webhook URL being used to reach services that are deliberately
	// not on the network. Swap in a plain client so these tests exercise
	// delivery behaviour; the refusal itself is covered by
	// TestWebhookClientRefusesLoopback and TestWebhookClientRefusesRedirect.
	n.Client = &http.Client{}
	n.Backoff = time.Millisecond
	ctx, cancel := context.WithCancel(context.Background())
	n.Start(ctx)
	return func() {
		cancel()
		n.Wait()
	}
}

func waitFor(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", what)
}

// Both publish sites must reach the webhook: an opened alert is published as
// a value and a resolved one as a pointer.
func TestNotifierDeliversBothAlertShapes(t *testing.T) {
	rec := &recorder{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var p WebhookPayload
		if err := json.NewDecoder(r.Body).Decode(&p); err != nil {
			t.Errorf("decode payload: %v", err)
		}
		if ct := r.Header.Get("Content-Type"); ct != "application/json" {
			t.Errorf("Content-Type = %q, want application/json", ct)
		}
		rec.add(p)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	bus := NewBus()
	stop := startNotifier(t, bus, srv.URL)
	defer stop()

	bus.Publish("alert.created", models.Alert{ID: 1, RuleKey: RuleInterfaceDown, Severity: models.SeverityCritical})
	bus.Publish("alert.resolved", &models.Alert{ID: 1, RuleKey: RuleInterfaceDown, Status: models.StatusResolved})

	waitFor(t, "two deliveries", func() bool { return len(rec.payloads()) == 2 })
	got := rec.payloads()
	if got[0].Event != "alert.created" || got[0].Alert.RuleKey != RuleInterfaceDown {
		t.Errorf("first delivery = %+v", got[0])
	}
	if got[1].Event != "alert.resolved" || got[1].Alert.Status != models.StatusResolved {
		t.Errorf("second delivery = %+v", got[1])
	}
	if got[0].Host == "" {
		t.Error("payload must carry the host so one endpoint can serve several machines")
	}
}

func TestNotifierIgnoresNonAlertEvents(t *testing.T) {
	rec := &recorder{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rec.mu.Lock()
		rec.hits++
		rec.mu.Unlock()
	}))
	defer srv.Close()

	bus := NewBus()
	stop := startNotifier(t, bus, srv.URL)

	bus.Publish("metrics.interface.updated", map[string]string{"iface": "eth0"})
	bus.Publish("health.updated", models.HealthScore{})
	bus.Publish("alert.created", models.Alert{ID: 7, RuleKey: RuleHighLoad})

	waitFor(t, "the alert delivery", func() bool {
		rec.mu.Lock()
		defer rec.mu.Unlock()
		return rec.hits == 1
	})
	stop() // drains, so any stray delivery would have landed by now
	rec.mu.Lock()
	defer rec.mu.Unlock()
	if rec.hits != 1 {
		t.Errorf("posted %d times, want only the alert", rec.hits)
	}
}

// The reason the notifier keeps its own queue: a slow endpoint must not let
// the bus's shared buffer fill with metric events and drop an alert. With
// the POST done inline on the subscription goroutine, the second alert here
// is lost.
func TestSlowEndpointDoesNotDropAlerts(t *testing.T) {
	rec := &recorder{}
	release := make(chan struct{})
	var once sync.Once
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Only the first request blocks; it is held until the flood is over.
		once.Do(func() { <-release })
		var p WebhookPayload
		_ = json.NewDecoder(r.Body).Decode(&p)
		rec.add(p)
	}))
	defer srv.Close()

	bus := NewBus()
	stop := startNotifier(t, bus, srv.URL)
	defer stop()

	bus.Publish("alert.created", models.Alert{ID: 1, RuleKey: RuleGatewayUnreachable})
	// Far more metric events than the bus buffers, while the endpoint hangs.
	for i := 0; i < 500; i++ {
		bus.Publish("metrics.interface.updated", map[string]int{"i": i})
		runtime.Gosched()
	}
	bus.Publish("alert.created", models.Alert{ID: 2, RuleKey: RuleHighMemory})
	close(release)

	waitFor(t, "both alerts", func() bool { return len(rec.payloads()) == 2 })
	got := rec.payloads()
	if got[0].Alert.ID != 1 || got[1].Alert.ID != 2 {
		t.Errorf("alerts arrived as %d then %d, want 1 then 2", got[0].Alert.ID, got[1].Alert.ID)
	}
}

func TestNotifierRetriesFailedDelivery(t *testing.T) {
	rec := &recorder{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rec.mu.Lock()
		rec.hits++
		n := rec.hits
		rec.mu.Unlock()
		if n < 3 {
			w.WriteHeader(http.StatusBadGateway)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	bus := NewBus()
	stop := startNotifier(t, bus, srv.URL)
	defer stop()

	bus.Publish("alert.created", models.Alert{ID: 3, RuleKey: RuleDNSFailure})

	waitFor(t, "the third attempt", func() bool {
		rec.mu.Lock()
		defer rec.mu.Unlock()
		return rec.hits == 3
	})
}

func TestNotifierDisabledWithoutURL(t *testing.T) {
	rec := &recorder{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rec.mu.Lock()
		rec.hits++
		rec.mu.Unlock()
	}))
	defer srv.Close()

	bus := NewBus()
	stop := startNotifier(t, bus, "") // webhook_url unset
	bus.Publish("alert.created", models.Alert{ID: 4, RuleKey: RuleHighSwap})
	stop()

	rec.mu.Lock()
	defer rec.mu.Unlock()
	if rec.hits != 0 {
		t.Errorf("posted %d times with no webhook_url configured", rec.hits)
	}
}

// --- webhook destination policy ---
//
// The webhook URL is user-supplied, so it decides which host the daemon
// speaks to. These cover the destinations that turn that into an attack.

func TestBlockedWebhookIP(t *testing.T) {
	cases := []struct {
		ip      string
		blocked bool
		why     string
	}{
		{"127.0.0.1", true, "loopback reaches services bound away from the network"},
		{"127.0.0.53", true, "the systemd-resolved stub is on loopback too"},
		{"::1", true, "IPv6 loopback"},
		{"169.254.169.254", true, "cloud metadata service"},
		{"fe80::1", true, "IPv6 link-local"},
		{"0.0.0.0", true, "unspecified"},
		{"224.0.0.1", true, "multicast"},
		// Allowed on purpose: posting to a NAS or a hub on the LAN is the
		// ordinary use of this feature on a home-network monitor.
		{"192.168.1.50", false, "RFC1918 is a legitimate webhook target here"},
		{"10.0.0.5", false, "RFC1918"},
		{"172.20.10.1", false, "RFC1918"},
		{"93.184.216.34", false, "ordinary public address"},
	}
	for _, c := range cases {
		if got := blockedWebhookIP(net.ParseIP(c.ip)); got != c.blocked {
			t.Errorf("blockedWebhookIP(%s) = %v, want %v — %s", c.ip, got, c.blocked, c.why)
		}
	}
}

// A hostname that resolves to loopback must fail at dial time, not merely be
// rejected as a string: the name can resolve differently between validation
// and connection.
func TestWebhookClientRefusesLoopback(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("request reached a loopback server; it should have been refused before connecting")
	}))
	defer srv.Close()

	_, err := webhookClient().Post(srv.URL, "application/json", strings.NewReader("{}"))
	if err == nil {
		t.Fatal("posting to a loopback address succeeded; expected it to be refused")
	}
	if !errors.Is(err, errBlockedWebhookTarget) {
		t.Errorf("refused for the wrong reason: %v", err)
	}
}

// A permitted endpoint must not be able to bounce the daemon somewhere else.
func TestWebhookClientRefusesRedirect(t *testing.T) {
	var reached atomic.Bool
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		reached.Store(true)
	}))
	defer target.Close()
	redirector := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, target.URL, http.StatusFound)
	}))
	defer redirector.Close()

	// Use a client with the redirect policy but a permissive dialer, so this
	// asserts the redirect refusal rather than re-testing the loopback block.
	c := webhookClient()
	c.Transport = &http.Transport{}
	if _, err := c.Post(redirector.URL, "application/json", strings.NewReader("{}")); err == nil {
		t.Fatal("redirect was followed; expected the client to refuse it")
	}
	if reached.Load() {
		t.Error("the redirect target was contacted")
	}
}
