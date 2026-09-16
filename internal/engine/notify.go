package engine

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"routeviewnet/internal/models"
)

// errBlockedWebhookTarget is returned by the dialer for a destination the
// webhook is not allowed to reach.
var errBlockedWebhookTarget = errors.New("webhook destination is not allowed")

// blockedWebhookIP reports whether an address is off-limits for webhook
// delivery.
//
// The daemon holds CAP_NET_RAW, CAP_SYS_PTRACE and CAP_DAC_READ_SEARCH and
// sits on the user's own machine, so an attacker who can choose the webhook
// URL is choosing which internal service the daemon speaks to on their
// behalf. The two destinations that turn that into a real attack are blocked:
//
//   - loopback, which reaches services deliberately bound away from the
//     network and otherwise unreachable by the attacker;
//   - link-local, which is where cloud and container metadata services live
//     (169.254.169.254 and fe80::/10).
//
// RFC1918 and unique-local addresses are deliberately *allowed*. This is a
// home-network monitor: posting alerts to a NAS, a Home Assistant box or an
// ntfy server on the LAN is the ordinary use of this feature, and blocking it
// would break the product to close a hole that the CSRF guard in
// internal/api already closes at the point where the URL is set.
func blockedWebhookIP(ip net.IP) bool {
	return ip == nil ||
		ip.IsLoopback() ||
		ip.IsLinkLocalUnicast() ||
		ip.IsLinkLocalMulticast() ||
		ip.IsInterfaceLocalMulticast() ||
		ip.IsMulticast() ||
		ip.IsUnspecified()
}

// webhookClient builds the delivery client.
//
// Validating the configured URL is not enough on its own: the name can resolve
// to one address when it is checked and another when it is dialled, and a
// permitted host can redirect to a forbidden one. So the check is enforced
// twice where it cannot be raced —
//
//   - Control runs after DNS resolution with the address actually being
//     connected to, which closes the rebinding window; and
//   - CheckRedirect refuses redirects outright, because a webhook endpoint has
//     no legitimate reason to bounce the daemon somewhere else.
func webhookClient() *http.Client {
	dialer := &net.Dialer{
		Timeout:   10 * time.Second,
		KeepAlive: 30 * time.Second,
		Control: func(network, address string, _ syscall.RawConn) error {
			host, _, err := net.SplitHostPort(address)
			if err != nil {
				return errBlockedWebhookTarget
			}
			if blockedWebhookIP(net.ParseIP(host)) {
				return fmt.Errorf("%w: %s", errBlockedWebhookTarget, host)
			}
			return nil
		},
	}
	return &http.Client{
		Transport: &http.Transport{
			DialContext:         dialer.DialContext,
			TLSHandshakeTimeout: 10 * time.Second,
			DisableKeepAlives:   false,
		},
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return errors.New("webhook endpoint attempted a redirect, which is not followed")
		},
	}
}

// WebhookPayload is the JSON body POSTed to alerts.webhook_url when an alert
// opens or resolves. Host is included because several machines can report to
// one endpoint and the alert itself says nothing about where it came from.
type WebhookPayload struct {
	Event     string       `json:"event"` // alert.created | alert.resolved
	Timestamp time.Time    `json:"timestamp"`
	Host      string       `json:"host"`
	Alert     models.Alert `json:"alert"`
}

// Notifier delivers alert transitions to an outbound webhook — the only
// alert channel that works when no dashboard is open.
//
// It subscribes to the bus like any other client, but never does network I/O
// on the subscription goroutine. The bus drops events for subscribers whose
// buffer is full, and that buffer also carries the high-frequency metric
// events (an interface sample every 5s, plus DNS, latency, memory and load).
// A webhook endpoint that took a few seconds to answer would therefore let
// those metric events crowd out the one alert the user actually needed. The
// reader drains the bus at memory speed into a queue of its own, and a
// second goroutine does the posting.
type Notifier struct {
	// URL and Timeout are read per delivery so a settings change applies to
	// the next alert without a restart.
	URL     func() string
	Timeout func() time.Duration

	Client *http.Client
	Log    *slog.Logger

	// Attempts is the total number of tries per alert, Backoff the pause
	// after the first failure (doubled each retry). A webhook that misses
	// the one alert that mattered is worse than useless, so a failed POST is
	// retried — but only while running; see draining.
	Attempts int
	Backoff  time.Duration

	bus   *Bus
	host  string
	queue chan WebhookPayload
	wg    sync.WaitGroup

	// draining is set once shutdown starts: queued alerts still get one
	// delivery attempt each, but retries are skipped so shutdown stays
	// bounded by the queue length rather than length × attempts × timeout.
	draining atomic.Bool
}

// notifyQueue is generous relative to any plausible alert rate; it exists to
// absorb a burst (every interface down at once) while the endpoint is slow.
const notifyQueue = 64

func NewNotifier(bus *Bus, log *slog.Logger, url func() string, timeout func() time.Duration) *Notifier {
	host, err := os.Hostname()
	if err != nil {
		host = "unknown"
	}
	return &Notifier{
		URL: url, Timeout: timeout,
		Client:   webhookClient(),
		Log:      log,
		Attempts: 3,
		Backoff:  time.Second,
		bus:      bus,
		host:     host,
		queue:    make(chan WebhookPayload, notifyQueue),
	}
}

// Start begins watching the bus. It returns immediately; call Wait after
// cancelling ctx to let queued deliveries finish.
func (n *Notifier) Start(ctx context.Context) {
	events := n.bus.Subscribe()

	n.wg.Add(1)
	go func() {
		defer n.wg.Done()
		defer close(n.queue)
		defer n.bus.Unsubscribe(events)
		for {
			select {
			case <-ctx.Done():
				n.draining.Store(true)
				return
			case ev := <-events:
				payload, ok := n.payloadFor(ev)
				if !ok {
					continue
				}
				// Checked again at delivery; this only avoids queueing work
				// that is already known to have nowhere to go.
				if n.URL() == "" {
					continue
				}
				select {
				case n.queue <- payload:
				default:
					n.Log.Warn("alert webhook queue full, dropping notification",
						"event", payload.Event, "rule", payload.Alert.RuleKey)
				}
			}
		}
	}()

	n.wg.Add(1)
	go func() {
		defer n.wg.Done()
		for payload := range n.queue {
			n.deliver(payload)
		}
	}()
}

// Wait blocks until the bus watcher has stopped and every queued delivery
// has been attempted.
func (n *Notifier) Wait() { n.wg.Wait() }

// payloadFor picks the alert transitions out of the event stream. The two
// publish sites disagree about pointer-ness (an opened alert is a value, a
// resolved one comes from a nil-checked lookup and is a pointer), so both
// shapes are accepted rather than relying on either staying put.
func (n *Notifier) payloadFor(ev Event) (WebhookPayload, bool) {
	if ev.Type != "alert.created" && ev.Type != "alert.resolved" {
		return WebhookPayload{}, false
	}
	var alert models.Alert
	switch v := ev.Payload.(type) {
	case models.Alert:
		alert = v
	case *models.Alert:
		if v == nil {
			return WebhookPayload{}, false
		}
		alert = *v
	default:
		return WebhookPayload{}, false
	}
	return WebhookPayload{
		Event:     ev.Type,
		Timestamp: ev.Timestamp,
		Host:      n.host,
		Alert:     alert,
	}, true
}

func (n *Notifier) deliver(payload WebhookPayload) {
	url := n.URL()
	if url == "" {
		return // disabled between queueing and delivery
	}
	body, err := json.Marshal(payload)
	if err != nil {
		n.Log.Warn("alert webhook payload unmarshalable", "error", err)
		return
	}

	attempts := n.Attempts
	if n.draining.Load() {
		attempts = 1
	}
	for attempt := 1; attempt <= attempts; attempt++ {
		err = n.post(url, body)
		if err == nil {
			n.Log.Debug("alert webhook delivered",
				"event", payload.Event, "rule", payload.Alert.RuleKey, "attempt", attempt)
			return
		}
		if attempt < attempts {
			time.Sleep(n.Backoff * time.Duration(1<<(attempt-1)))
		}
	}
	n.Log.Warn("alert webhook delivery failed",
		"event", payload.Event, "rule", payload.Alert.RuleKey,
		"attempts", attempts, "error", err)
}

// post makes one attempt. It builds its own context rather than inheriting
// the daemon's: a queued alert should still reach the endpoint during a
// graceful shutdown, which is exactly when the daemon context is cancelled.
func (n *Notifier) post(url string, body []byte) error {
	timeout := n.Timeout()
	if timeout <= 0 {
		timeout = 5 * time.Second
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "routeviewnet-webhook")

	resp, err := n.Client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	// The body is not used, but it must be drained for connection reuse.
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return fmt.Errorf("webhook returned %s", resp.Status)
	}
	return nil
}
