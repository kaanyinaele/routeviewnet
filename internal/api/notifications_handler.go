package api

import (
	"net/http"
	"time"

	"routeviewnet/internal/models"
)

// TestNotificationRule is the rule key carried by a test alert. It is not a
// real rule: nothing observes it and the health score has no penalty for it.
const TestNotificationRule = "test_notification"

// handleTestNotification publishes a synthetic alert.created so both delivery
// channels can be exercised on demand. Waiting for a genuine fault is a poor
// way to find out that a webhook URL has a typo in it.
//
// The alert is published to the bus only — never written to the alerts table
// and never fed to the alert engine — so it reaches every connected dashboard
// and the webhook without polluting alert history or moving the health score.
// It carries ID 0 and a rule key no rule uses, which is how a receiver can
// tell a test from the real thing.
func (s *Server) handleTestNotification(w http.ResponseWriter, r *http.Request) {
	now := time.Now().UTC()
	alert := models.Alert{
		ID:       0,
		RuleKey:  TestNotificationRule,
		Severity: models.SeverityInfo,
		Title:    "Test notification",
		Message:  "This is a test from the RouteViewNet settings page. No fault was detected.",
		Source:   "settings",
		Status:   models.StatusOpen,

		OccurrenceCount: 1,
		CreatedAt:       now,
		UpdatedAt:       now,
	}
	s.Bus.Publish("alert.created", alert)

	// Delivery is asynchronous and best-effort, so this reports what was
	// attempted rather than claiming the webhook succeeded.
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"published":          true,
		"webhook_configured": s.Cfg.Get().Alerts.WebhookURL != "",
	})
}
