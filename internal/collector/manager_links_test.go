package collector

import (
	"context"
	"io"
	"log/slog"
	"path/filepath"
	"testing"

	"routeviewnet/internal/engine"
	"routeviewnet/internal/models"
	"routeviewnet/internal/storage"
)

// linkHarness runs observeLinks against a real alert engine and database, so
// these tests exercise the same debounce and resolve behaviour as the daemon.
type linkHarness struct {
	t   *testing.T
	m   *Manager
	db  *storage.DB
	ctx context.Context
}

func newLinkHarness(t *testing.T) *linkHarness {
	t.Helper()
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	db, err := storage.Open(filepath.Join(t.TempDir(), "test.db"), log)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	alerts := engine.NewAlertEngine(db, engine.NewBus(), log)
	return &linkHarness{
		t:   t,
		m:   &Manager{DB: db, Alerts: alerts, Log: log, prevDrops: map[string]uint64{}},
		db:  db,
		ctx: context.Background(),
	}
}

type ifaceState struct {
	name, state string
	dropped     uint64
}

// cycle feeds n identical samples, which is what it takes to get through the
// engine's trigger or resolve debounce.
func (h *linkHarness) cycle(n int, primary string, ifaces ...ifaceState) {
	h.t.Helper()
	sample := &InterfaceSample{Primary: primary}
	for _, i := range ifaces {
		sample.Interfaces = append(sample.Interfaces, models.Interface{Name: i.name, State: i.state})
		sample.Metrics = append(sample.Metrics, models.InterfaceMetric{Name: i.name, RxDropped: i.dropped})
	}
	for range n {
		h.m.observeLinks(h.ctx, sample)
	}
}

func (h *linkHarness) open(rule string) []string {
	h.t.Helper()
	alerts, err := h.db.OpenAlerts(h.ctx)
	if err != nil {
		h.t.Fatal(err)
	}
	var sources []string
	for _, a := range alerts {
		if a.RuleKey == rule {
			sources = append(sources, a.Source)
		}
	}
	return sources
}

// The reported false alarm: a laptop on Wi-Fi with an ethernet port it never
// uses. That port being down is not a fault and must never raise an alert.
func TestUnusedInterfaceDoesNotAlert(t *testing.T) {
	h := newLinkHarness(t)
	h.cycle(10, "wlp2s0",
		ifaceState{"wlp2s0", "up", 0},
		ifaceState{"enp0s31f6", "down", 0},
	)
	if got := h.open(engine.RuleInterfaceDown); len(got) != 0 {
		t.Errorf("unused interface raised interface_down for %v", got)
	}
}

// Installs that already carry the false alarm must recover on upgrade. If the
// unused interface were skipped rather than reported healthy, the engine would
// never see a resolving observation and the alert would stay open forever.
func TestExistingFalseAlarmResolves(t *testing.T) {
	h := newLinkHarness(t)
	if _, err := h.db.CreateAlert(h.ctx, models.Alert{
		RuleKey: engine.RuleInterfaceDown, Source: "enp0s31f6",
		Severity: models.SeverityWarning, Title: "Interface down",
	}); err != nil {
		t.Fatal(err)
	}
	h.cycle(3, "wlp2s0",
		ifaceState{"wlp2s0", "up", 0},
		ifaceState{"enp0s31f6", "down", 0},
	)
	if got := h.open(engine.RuleInterfaceDown); len(got) != 0 {
		t.Errorf("pre-existing false alarm still open for %v", got)
	}
}

// The real fault must still be caught. Pulling the only cable also removes the
// default route, so the primary goes blank at the very moment the interface
// fails; watching only the current primary would miss it.
func TestActiveInterfaceGoingDownStillAlerts(t *testing.T) {
	h := newLinkHarness(t)
	h.cycle(3, "eth0", ifaceState{"eth0", "up", 0})
	h.cycle(3, "", ifaceState{"eth0", "down", 0})
	got := h.open(engine.RuleInterfaceDown)
	if len(got) != 1 || got[0] != "eth0" {
		t.Errorf("unplugging the interface in use: open alerts %v, want [eth0]", got)
	}
}

// Undocking a laptop: ethernet goes down but Wi-Fi takes over the route. The
// user is still online, so this is not a fault either.
func TestSwitchingToAnotherInterfaceDoesNotAlert(t *testing.T) {
	h := newLinkHarness(t)
	h.cycle(3, "eth0",
		ifaceState{"eth0", "up", 0},
		ifaceState{"wlan0", "up", 0},
	)
	h.cycle(5, "wlan0",
		ifaceState{"eth0", "down", 0},
		ifaceState{"wlan0", "up", 0},
	)
	if got := h.open(engine.RuleInterfaceDown); len(got) != 0 {
		t.Errorf("switching uplinks raised interface_down for %v", got)
	}
}

// Dropped packets on an interface nobody is using are equally irrelevant;
// on the one in use they still count.
func TestDroppedPacketsOnlyWatchActiveInterface(t *testing.T) {
	h := newLinkHarness(t)
	for i := uint64(1); i <= 4; i++ {
		h.cycle(1, "wlp2s0",
			ifaceState{"wlp2s0", "up", 0},
			ifaceState{"enp0s31f6", "down", i * 10},
		)
	}
	if got := h.open(engine.RuleDroppedIncreasing); len(got) != 0 {
		t.Errorf("drops on an unused interface raised an alert for %v", got)
	}
	for i := uint64(1); i <= 4; i++ {
		h.cycle(1, "wlp2s0", ifaceState{"wlp2s0", "up", i * 10})
	}
	if got := h.open(engine.RuleDroppedIncreasing); len(got) != 1 || got[0] != "wlp2s0" {
		t.Errorf("drops on the active interface: open alerts %v, want [wlp2s0]", got)
	}
}
