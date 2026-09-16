package api

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"routeviewnet/internal/config"
	"routeviewnet/internal/engine"
	"routeviewnet/internal/models"
)

// The test endpoint must reach the live feed without touching alert history:
// a user pressing "Send test notification" should not leave a fake fault in
// the alerts table or move the health score.
func TestTestNotificationPublishesWithoutPersisting(t *testing.T) {
	bus := engine.NewBus()
	events := bus.Subscribe()
	defer bus.Unsubscribe(events)

	s := &Server{
		Bus: bus,
		Cfg: config.NewStore(config.Default()),
		Log: slog.New(slog.NewTextHandler(io.Discard, nil)),
		// DB is deliberately nil: a handler that wrote the alert down would
		// panic here rather than pass quietly.
		DB: nil,
	}

	w := httptest.NewRecorder()
	s.handleTestNotification(w, httptest.NewRequest(http.MethodPost, "/api/v1/notifications/test", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("POST /notifications/test = %d: %s", w.Code, w.Body.String())
	}

	var body struct {
		Published         bool `json:"published"`
		WebhookConfigured bool `json:"webhook_configured"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if !body.Published {
		t.Error("published should be true")
	}
	if body.WebhookConfigured {
		t.Error("webhook_configured should be false with the default empty URL")
	}

	select {
	case ev := <-events:
		if ev.Type != "alert.created" {
			t.Fatalf("published %q, want alert.created", ev.Type)
		}
		alert, ok := ev.Payload.(models.Alert)
		if !ok {
			t.Fatalf("payload is %T, want models.Alert", ev.Payload)
		}
		// ID 0 and a rule key no rule uses are how a receiver tells a test
		// from a real alert.
		if alert.ID != 0 || alert.RuleKey != TestNotificationRule {
			t.Errorf("test alert = id %d rule %q, want id 0 rule %q",
				alert.ID, alert.RuleKey, TestNotificationRule)
		}
	default:
		t.Fatal("no event published to the bus")
	}
}

func TestTestNotificationReportsConfiguredWebhook(t *testing.T) {
	cfg := config.Default()
	cfg.Alerts.WebhookURL = "https://example.com/hook"
	bus := engine.NewBus()
	s := &Server{Bus: bus, Cfg: config.NewStore(cfg), Log: slog.New(slog.NewTextHandler(io.Discard, nil))}

	w := httptest.NewRecorder()
	s.handleTestNotification(w, httptest.NewRequest(http.MethodPost, "/api/v1/notifications/test", nil))

	var body struct {
		WebhookConfigured bool `json:"webhook_configured"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if !body.WebhookConfigured {
		t.Error("webhook_configured should be true when alerts.webhook_url is set")
	}
}
