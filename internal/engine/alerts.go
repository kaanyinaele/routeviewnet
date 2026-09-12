package engine

import (
	"context"
	"log/slog"
	"sync"

	"routeviewnet/internal/models"
	"routeviewnet/internal/storage"
)

// Rule keys (stable identifiers, §9.2.10). The health-score penalty table
// (§6.2) is keyed off these.
const (
	RuleNewDevice           = "new_device"
	RuleGatewayUnreachable  = "gateway_unreachable"
	RuleInternetUnreachable = "internet_unreachable"
	RuleHighPacketLoss      = "high_packet_loss"
	RuleDNSFailure          = "dns_failure"
	RuleHighDNSLatency      = "high_dns_latency"
	RuleInterfaceDown       = "interface_down"
	RuleDroppedIncreasing   = "dropped_packets_increasing"
	RuleTrafficSpike        = "traffic_spike"
	RuleHighMemory          = "high_memory_usage"
	RuleHighSwap            = "high_swap_usage"
	RuleDaemonMemoryHigh    = "daemon_memory_high"
	RuleHighLoad            = "high_system_load"
)

// immediateRules skip trigger debounce (§8.4.1 exception).
var immediateRules = map[string]bool{RuleNewDevice: true}

// Observation is one health check outcome fed to the engine by a collector.
type Observation struct {
	RuleKey  string
	Source   string // e.g. interface name, target IP, domain; "" for system-wide
	Severity string
	Title    string
	Message  string
	Failing  bool
}

// AlertEngine implements dedup + hysteresis (§8.4). Pre-trigger counters are
// in-memory per rule+source and reset on restart (§8.4.5, documented).
type AlertEngine struct {
	db  *storage.DB
	bus *Bus
	log *slog.Logger

	// TriggerN / ResolveM are read via the getter funcs each observation so
	// settings changes hot-reload (§12.4).
	TriggerN func() int
	ResolveM func() int

	// OnChange fires after an alert opens or resolves, so the health score
	// recomputes event-driven (§6.3).
	OnChange func(ctx context.Context)

	mu       sync.Mutex
	failures map[string]int // consecutive failing observations, pre-trigger
	healthy  map[string]int // consecutive healthy observations, pre-resolve
}

func NewAlertEngine(db *storage.DB, bus *Bus, log *slog.Logger) *AlertEngine {
	return &AlertEngine{
		db: db, bus: bus, log: log,
		TriggerN: func() int { return 2 },
		ResolveM: func() int { return 2 },
		failures: map[string]int{},
		healthy:  map[string]int{},
	}
}

func key(rule, source string) string { return rule + "|" + source }

// Observe applies §8.4 semantics to one observation.
func (e *AlertEngine) Observe(ctx context.Context, obs Observation) {
	open, err := e.db.FindOpenAlert(ctx, obs.RuleKey, obs.Source)
	if err != nil {
		e.log.Warn("alert lookup failed", "rule", obs.RuleKey, "error", err)
		return
	}

	e.mu.Lock()
	k := key(obs.RuleKey, obs.Source)
	var fails, oks int
	switch {
	case obs.Failing:
		delete(e.healthy, k)
		e.failures[k]++
		fails = e.failures[k]
	case open != nil:
		delete(e.failures, k)
		e.healthy[k]++
		oks = e.healthy[k]
	default:
		// Healthy with nothing open: there is no debounce in progress, so
		// there is no counter worth keeping. Dropping it bounds these maps,
		// which are keyed per rule+source — and "source" for the new-device
		// rule is an ip/mac pair, one more entry for every address a DHCP
		// lease ever hands out.
		delete(e.failures, k)
		delete(e.healthy, k)
	}
	e.mu.Unlock()

	switch {
	case obs.Failing && open != nil:
		// Already open: no duplicate row, just re-observe (§8.4.2).
		if err := e.db.TouchAlert(ctx, open.ID, obs.Message); err != nil {
			e.log.Warn("alert touch failed", "rule", obs.RuleKey, "error", err)
		}

	case obs.Failing && open == nil:
		n := e.TriggerN()
		if immediateRules[obs.RuleKey] {
			n = 1
		}
		if fails < n {
			return // still debouncing (§8.4.1)
		}
		a, err := e.db.CreateAlert(ctx, models.Alert{
			RuleKey: obs.RuleKey, Severity: obs.Severity,
			Title: obs.Title, Message: obs.Message, Source: obs.Source,
		})
		if err != nil {
			e.log.Warn("alert create failed", "rule", obs.RuleKey, "error", err)
			return
		}
		e.log.Info("alert opened", "rule", obs.RuleKey, "source", obs.Source, "severity", obs.Severity)
		_ = e.db.InsertEvent(ctx, "alert.created", obs.Severity, MarshalPayload(a))
		e.bus.Publish("alert.created", a)
		if e.OnChange != nil {
			e.OnChange(ctx)
		}

	case !obs.Failing && open != nil:
		if oks < e.ResolveM() {
			return // resolve debounce (§8.4.3)
		}
		if err := e.db.ResolveAlert(ctx, open.ID); err != nil {
			e.log.Warn("alert resolve failed", "rule", obs.RuleKey, "error", err)
			return
		}
		e.log.Info("alert resolved", "rule", obs.RuleKey, "source", obs.Source)
		open.Status = models.StatusResolved
		_ = e.db.InsertEvent(ctx, "alert.resolved", open.Severity, MarshalPayload(open))
		e.bus.Publish("alert.resolved", open)
		if e.OnChange != nil {
			e.OnChange(ctx)
		}
	}
}
