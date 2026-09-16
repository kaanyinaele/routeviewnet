package engine

import (
	"context"
	"io"
	"log/slog"
	"testing"
	"time"

	"routeviewnet/internal/models"
)

// --- SweepStale: which alerts get resolved ---

// Alerts for sources that are no longer observed must resolve; an alert whose
// source is still being observed failing must not. Timestamps are stored to
// the second, so the test waits across a second boundary to separate them.
func TestSweepStaleResolvesOnlyUnobservedAlerts(t *testing.T) {
	e, db := newTestEngine(t)
	ctx := context.Background()
	e.TriggerN = func() int { return 1 }

	failing := func(src string) Observation {
		return Observation{RuleKey: RuleGatewayUnreachable, Source: src, Severity: models.SeverityCritical,
			Title: "Gateway unreachable", Message: "no reply", Failing: true}
	}
	e.Observe(ctx, failing("192.168.0.1")) // a router on a network we have since left
	e.Observe(ctx, failing("172.20.10.1")) // the router we are on, genuinely down

	time.Sleep(1100 * time.Millisecond)
	cutoff := time.Now()
	e.Observe(ctx, failing("172.20.10.1")) // still being checked, still failing

	changes := 0
	e.OnChange = func(context.Context) { changes++ }
	events := e.bus.Subscribe()

	n, err := e.SweepStale(ctx, cutoff)
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("swept %d alerts, want 1", n)
	}

	open, _ := db.OpenAlerts(ctx)
	if len(open) != 1 || open[0].Source != "172.20.10.1" {
		t.Errorf("open after sweep = %v, want only the still-observed 172.20.10.1", sources(open))
	}
	if changes != 1 {
		t.Errorf("OnChange ran %d times, want once so the health score recomputes", changes)
	}
	select {
	case ev := <-events:
		if ev.Type != "alert.resolved" {
			t.Errorf("published %q, want alert.resolved so the dashboard and webhook hear about it", ev.Type)
		}
	default:
		t.Error("sweep resolved an alert without publishing alert.resolved")
	}
}

// Sweeping nothing must not trigger a pointless health recompute.
func TestSweepStaleNoOpWhenNothingStale(t *testing.T) {
	e, _ := newTestEngine(t)
	ctx := context.Background()
	e.TriggerN = func() int { return 1 }
	e.Observe(ctx, Observation{RuleKey: RuleHighMemory, Source: "system", Failing: true})

	changes := 0
	e.OnChange = func(context.Context) { changes++ }
	n, err := e.SweepStale(ctx, time.Now().Add(-time.Hour))
	if err != nil || n != 0 || changes != 0 {
		t.Errorf("sweep with nothing stale: n=%d err=%v onChange=%d, want 0/nil/0", n, err, changes)
	}
}

// A swept source that comes back and fails again must reopen through the
// normal debounce, not be blocked by leftover counters.
func TestSweptAlertReopensIfSourceFailsAgain(t *testing.T) {
	e, db := newTestEngine(t)
	ctx := context.Background()
	obs := Observation{RuleKey: RuleInterfaceDown, Source: "enx0", Severity: models.SeverityWarning, Failing: true}
	e.Observe(ctx, obs)
	e.Observe(ctx, obs) // default TriggerN is 2: opens here

	if _, err := e.SweepStale(ctx, time.Now().Add(2*time.Second)); err != nil {
		t.Fatal(err)
	}
	if open, _ := db.OpenAlerts(ctx); len(open) != 0 {
		t.Fatalf("expected the alert swept, still open: %v", sources(open))
	}

	e.Observe(ctx, obs)
	if open, _ := db.OpenAlerts(ctx); len(open) != 0 {
		t.Error("reopened on the first failing observation; the trigger debounce should restart")
	}
	e.Observe(ctx, obs)
	if open, _ := db.OpenAlerts(ctx); len(open) != 1 {
		t.Error("did not reopen after the debounce once the source failed again")
	}
}

// --- StaleSweeper: when sweeping is safe ---

type fakeSweep struct {
	cutoffs []time.Time
}

func (f *fakeSweep) sweep(_ context.Context, cutoff time.Time) (int, error) {
	f.cutoffs = append(f.cutoffs, cutoff)
	return 0, nil
}

func newTestSweeper(clock *time.Time) (*StaleSweeper, *fakeSweep) {
	f := &fakeSweep{}
	return &StaleSweeper{
		Sweep: f.sweep,
		Log:   slog.New(slog.NewTextHandler(io.Discard, nil)),
		Now:   func() time.Time { return *clock },
	}, f
}

const (
	testEvery = time.Minute
	testStale = 15 * time.Minute
	testGrace = 2 * time.Minute
)

// Right after startup every alert looks old, including ones about to be
// re-observed failing. No sweep may run until the grace period has passed.
func TestSweeperWaitsOutGraceAfterStartup(t *testing.T) {
	clock := time.Date(2026, 9, 16, 12, 0, 0, 0, time.UTC)
	s, f := newTestSweeper(&clock)

	s.Tick(context.Background(), testEvery, testStale, testGrace) // 12:00 startup
	clock = clock.Add(time.Minute)
	s.Tick(context.Background(), testEvery, testStale, testGrace) // 12:01, still in grace
	if len(f.cutoffs) != 0 {
		t.Fatalf("swept %d times during the startup grace period", len(f.cutoffs))
	}
	clock = clock.Add(time.Minute)
	s.Tick(context.Background(), testEvery, testStale, testGrace) // 12:02, grace over
	if len(f.cutoffs) != 1 {
		t.Fatalf("swept %d times after grace, want 1", len(f.cutoffs))
	}
	if want := clock.Add(-testStale); !f.cutoffs[0].Equal(want) {
		t.Errorf("cutoff = %v, want now-staleAfter = %v", f.cutoffs[0], want)
	}
}

// Waking from sleep is the same hazard as startup. The gap is only visible on
// the wall clock, which the fake clock stands in for here.
func TestSweeperWaitsOutGraceAfterResume(t *testing.T) {
	clock := time.Date(2026, 9, 16, 12, 0, 0, 0, time.UTC)
	s, f := newTestSweeper(&clock)
	for range 4 { // 12:00 .. 12:03, past the startup grace
		s.Tick(context.Background(), testEvery, testStale, testGrace)
		clock = clock.Add(time.Minute)
	}
	before := len(f.cutoffs)

	clock = clock.Add(45 * time.Minute) // laptop lid closed
	s.Tick(context.Background(), testEvery, testStale, testGrace)
	if len(f.cutoffs) != before {
		t.Fatal("swept immediately after resuming from sleep")
	}
	clock = clock.Add(testGrace)
	s.Tick(context.Background(), testEvery, testStale, testGrace)
	if len(f.cutoffs) != before+1 {
		t.Error("did not resume sweeping once the post-sleep grace had passed")
	}
}

func TestSweepTimingScalesWithSlowCollectors(t *testing.T) {
	for _, c := range []struct {
		longest, stale, grace time.Duration
	}{
		{5 * time.Second, 15 * time.Minute, time.Minute},       // defaults: floors apply
		{10 * time.Minute, 50 * time.Minute, 20 * time.Minute}, // slow custom interval
		{time.Hour, 5 * time.Hour, 2 * time.Hour},              // hourly checks
	} {
		stale, grace := SweepTiming(c.longest)
		if stale != c.stale || grace != c.grace {
			t.Errorf("SweepTiming(%v) = %v, %v; want %v, %v", c.longest, stale, grace, c.stale, c.grace)
		}
		if stale < 10*time.Minute+time.Minute {
			t.Errorf("stale window %v would sweep a new-device alert inside its 10-minute freshness window", stale)
		}
	}
}

func sources(alerts []models.Alert) []string {
	var out []string
	for _, a := range alerts {
		out = append(out, a.RuleKey+"/"+a.Source)
	}
	return out
}
