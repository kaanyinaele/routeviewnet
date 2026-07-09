package engine

import (
	"context"
	"io"
	"log/slog"
	"path/filepath"
	"testing"

	"routeviewnet/internal/models"
	"routeviewnet/internal/storage"
)

func TestComputeScoreHealthy(t *testing.T) {
	s := ComputeScore(nil)
	if s.Score != 100 || s.Status != models.HealthHealthy {
		t.Errorf("empty alerts: want 100/healthy, got %d/%s", s.Score, s.Status)
	}
}

func TestComputeScorePenaltiesAndCaps(t *testing.T) {
	open := []models.Alert{
		{RuleKey: RuleGatewayUnreachable}, // -40 connectivity
		{RuleKey: RuleDNSFailure},         // -20 dns
	}
	s := ComputeScore(open)
	if s.Score != 40 || s.Status != models.HealthCritical {
		t.Errorf("want 40/critical, got %d/%s", s.Score, s.Status)
	}

	// Category cap: all three connectivity rules sum to 85 raw but cap at 40 (§6.2).
	open = []models.Alert{
		{RuleKey: RuleGatewayUnreachable},
		{RuleKey: RuleInternetUnreachable},
		{RuleKey: RuleHighPacketLoss},
	}
	s = ComputeScore(open)
	if s.Score != 60 {
		t.Errorf("connectivity cap: want 60, got %d", s.Score)
	}

	// Duplicate rule across sources counts once.
	open = []models.Alert{
		{RuleKey: RuleHighDNSLatency, Source: "google.com"},
		{RuleKey: RuleHighDNSLatency, Source: "cloudflare.com"},
	}
	s = ComputeScore(open)
	if s.Score != 90 || s.Status != models.HealthHealthy {
		t.Errorf("dup rule: want 90/healthy, got %d/%s", s.Score, s.Status)
	}
}

func TestComputeScoreFloor(t *testing.T) {
	var open []models.Alert
	for k := range penalties {
		open = append(open, models.Alert{RuleKey: k})
	}
	s := ComputeScore(open)
	// caps: 40+25+20+20+5 = 110 → floor at 0
	if s.Score != 0 || s.Status != models.HealthCritical {
		t.Errorf("want 0/critical, got %d/%s", s.Score, s.Status)
	}
}

func testDB(t *testing.T) *storage.DB {
	t.Helper()
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	db, err := storage.Open(filepath.Join(t.TempDir(), "test.db"), log)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

func newTestEngine(t *testing.T) (*AlertEngine, *storage.DB) {
	db := testDB(t)
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	return NewAlertEngine(db, NewBus(), log), db
}

func failing(rule string) Observation {
	return Observation{RuleKey: rule, Source: "src", Severity: models.SeverityWarning,
		Title: "t", Message: "m", Failing: true}
}

func healthy(rule string) Observation {
	o := failing(rule)
	o.Failing = false
	return o
}

// §8.4.1: N consecutive failures before an alert opens.
func TestAlertTriggerDebounce(t *testing.T) {
	eng, db := newTestEngine(t)
	ctx := context.Background()

	eng.Observe(ctx, failing(RuleDNSFailure))
	if n, _ := db.OpenAlertCount(ctx); n != 0 {
		t.Fatalf("after 1 failure: want 0 open alerts, got %d", n)
	}
	eng.Observe(ctx, failing(RuleDNSFailure))
	if n, _ := db.OpenAlertCount(ctx); n != 1 {
		t.Fatalf("after 2 failures: want 1 open alert, got %d", n)
	}

	// §8.4.2: further failures update, never duplicate.
	eng.Observe(ctx, failing(RuleDNSFailure))
	if n, _ := db.OpenAlertCount(ctx); n != 1 {
		t.Fatalf("no duplicate rows: want 1, got %d", n)
	}
	a, _ := db.FindOpenAlert(ctx, RuleDNSFailure, "src")
	if a.OccurrenceCount != 2 {
		t.Errorf("occurrence_count: want 2, got %d", a.OccurrenceCount)
	}
}

// §8.4.1 exception: new-device fires on a single observation.
func TestNewDeviceImmediate(t *testing.T) {
	eng, db := newTestEngine(t)
	ctx := context.Background()
	eng.Observe(ctx, failing(RuleNewDevice))
	if n, _ := db.OpenAlertCount(ctx); n != 1 {
		t.Fatalf("new device must fire immediately, got %d open", n)
	}
}

// §8.4.3: M consecutive healthy checks before resolution.
func TestAlertResolveDebounce(t *testing.T) {
	eng, db := newTestEngine(t)
	ctx := context.Background()

	eng.Observe(ctx, failing(RuleHighMemory))
	eng.Observe(ctx, failing(RuleHighMemory))
	if n, _ := db.OpenAlertCount(ctx); n != 1 {
		t.Fatal("setup: alert should be open")
	}

	eng.Observe(ctx, healthy(RuleHighMemory))
	if n, _ := db.OpenAlertCount(ctx); n != 1 {
		t.Fatal("after 1 healthy check the alert must stay open")
	}
	eng.Observe(ctx, healthy(RuleHighMemory))
	if n, _ := db.OpenAlertCount(ctx); n != 0 {
		t.Fatal("after 2 healthy checks the alert must resolve")
	}

	// Flapping: a single failure then single success must not reopen.
	eng.Observe(ctx, failing(RuleHighMemory))
	eng.Observe(ctx, healthy(RuleHighMemory))
	if n, _ := db.OpenAlertCount(ctx); n != 0 {
		t.Fatal("flapping condition must not reopen the alert")
	}
}

func TestBusFanOut(t *testing.T) {
	bus := NewBus()
	a, b := bus.Subscribe(), bus.Subscribe()
	bus.Publish("test.event", map[string]int{"x": 1})
	for _, ch := range []chan Event{a, b} {
		select {
		case ev := <-ch:
			if ev.Type != "test.event" {
				t.Errorf("wrong type: %s", ev.Type)
			}
		default:
			t.Error("subscriber did not receive event")
		}
	}
	bus.Unsubscribe(a)
	bus.Publish("test.event2", nil)
	select {
	case ev := <-b:
		if ev.Type != "test.event2" {
			t.Errorf("wrong type: %s", ev.Type)
		}
	default:
		t.Error("remaining subscriber did not receive event")
	}
}
