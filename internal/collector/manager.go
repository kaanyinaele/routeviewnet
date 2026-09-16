package collector

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"routeviewnet/internal/config"
	"routeviewnet/internal/engine"
	"routeviewnet/internal/models"
	"routeviewnet/internal/storage"
)

// Manager runs collection cycles: collect → persist → publish → evaluate
// alert rules (§7.10, §8.3). Every cycle tolerates individual failures —
// a collector error is logged, never fatal (§14.2).
type Manager struct {
	DB     *storage.DB
	Bus    *engine.Bus
	Alerts *engine.AlertEngine
	Cfg    *config.Store
	Log    *slog.Logger

	Interfaces *InterfaceCollector
	Devices    *DeviceCollector
	Latency    *LatencyCollector
	DNS        *DNSCollector
	Memory     *MemoryCollector
	Load       *LoadCollector
	ProcMem    *ProcessMemoryCollector
	DaemonMem  *DaemonMemoryCollector
	AppTraffic *AppTrafficCollector

	mu      sync.RWMutex
	primary string
	gateway string
	// lastPrimary is the most recent non-empty primary interface. It is what
	// link alerts watch when the primary goes blank, which is exactly what
	// happens when the only cable is pulled: the kernel drops the default
	// route with it, so "current primary" alone would stop watching the
	// interface at the moment it fails.
	lastPrimary string
	prevDrops   map[string]uint64
	memTotal    uint64
}

func NewManager(db *storage.DB, bus *engine.Bus, alerts *engine.AlertEngine, cfg *config.Store, log *slog.Logger) *Manager {
	c := cfg.Get()
	return &Manager{
		DB: db, Bus: bus, Alerts: alerts, Cfg: cfg, Log: log,
		Interfaces: NewInterfaceCollector(),
		Devices:    NewDeviceCollector(),
		Latency:    NewLatencyCollector(c.Checks.PingMode),
		DNS:        NewDNSCollector(),
		Memory:     NewMemoryCollector(),
		Load:       NewLoadCollector(),
		ProcMem:    NewProcessMemoryCollector(),
		DaemonMem:  &DaemonMemoryCollector{},
		AppTraffic: NewAppTrafficCollector(),
		prevDrops:  map[string]uint64{},
	}
}

func (m *Manager) Primary() string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.primary
}

func (m *Manager) Gateway() string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.gateway
}

func (m *Manager) PingMode() string { return m.Latency.Mode }

// RunNetworkCycle handles interface counters and rates (main interval).
func (m *Manager) RunNetworkCycle(ctx context.Context) {
	cfg := m.Cfg.Get()
	sample, err := m.Interfaces.Collect(cfg.Checks.PrimaryInterfaceOverride)
	if err != nil {
		m.Log.Warn("collector failed", "collector", "interfaces", "error", err)
		return
	}
	m.mu.Lock()
	m.primary = sample.Primary
	if sample.Gateway != "" {
		m.gateway = sample.Gateway
	}
	m.mu.Unlock()

	if err := m.DB.InsertInterfaceMetrics(ctx, sample.Metrics); err != nil {
		m.Log.Warn("persist interface metrics failed", "error", err)
	}
	for _, iface := range sample.Interfaces {
		if err := m.DB.UpsertInterface(ctx, iface); err != nil {
			m.Log.Warn("upsert interface failed", "interface", iface.Name, "error", err)
		}
	}
	m.Bus.Publish("metrics.interface.updated", sample.Metrics)

	// Alert rules: interface down, drops increasing, traffic spike (§8.3).
	m.observeLinks(ctx, sample)
	m.checkTrafficSpike(ctx, sample)
}

// watchedInterface records primary and returns the interface link alerts
// should watch: the current primary, or the last one seen when there is none.
func (m *Manager) watchedInterface(primary string) string {
	m.mu.Lock()
	defer m.mu.Unlock()
	if primary != "" {
		m.lastPrimary = primary
	}
	return m.lastPrimary
}

// observeLinks feeds the interface-down and dropped-packet rules for one
// sample.
func (m *Manager) observeLinks(ctx context.Context, sample *InterfaceSample) {
	watched := m.watchedInterface(sample.Primary)

	// Link rules only fire for the interface this machine depends on. They used
	// to fire for every interface, so a laptop on Wi-Fi with an ethernet port
	// it never uses reported that port as a fault on every cycle — a permanent
	// warning, 15 health points lost, and a Troubleshoot page telling the user
	// to check a cable that does not exist. An unused port being down is not a
	// problem; the connection you are actually using being down is.
	//
	// Every other interface is still observed, as healthy, rather than skipped:
	// that is what lets an alert raised before this rule existed (or before the
	// machine switched from ethernet to Wi-Fi) resolve through the engine's
	// normal hysteresis instead of staying open forever.
	//
	// Trade-off: a machine with two live uplinks is only warned about the one
	// carrying the default route. Restarting the daemon with the cable already
	// out also leaves nothing to watch until a route appears; the gateway and
	// internet rules still report the outage in both cases.
	for _, iface := range sample.Interfaces {
		m.Alerts.Observe(ctx, engine.Observation{
			RuleKey: engine.RuleInterfaceDown, Source: iface.Name,
			Severity: models.SeverityWarning,
			Title:    "Interface down",
			Message:  fmt.Sprintf("Interface %s is %s", iface.Name, orUnknown(iface.State)),
			Failing:  iface.Name == watched && iface.State == "down",
		})
	}
	// Rebuilt rather than updated in place: an interface that goes away
	// should stop being tracked instead of lingering in the map forever.
	drops := make(map[string]uint64, len(sample.Metrics))
	for _, met := range sample.Metrics {
		total := met.RxDropped + met.TxDropped
		prev, seen := m.prevDrops[met.Name]
		drops[met.Name] = total
		if !seen {
			continue
		}
		m.Alerts.Observe(ctx, engine.Observation{
			RuleKey: engine.RuleDroppedIncreasing, Source: met.Name,
			Severity: models.SeverityWarning,
			Title:    "Dropped packets increasing",
			Message:  fmt.Sprintf("Interface %s dropped %d packets since the last check", met.Name, total-prev),
			Failing:  met.Name == watched && total > prev,
		})
	}
	m.prevDrops = drops
}

func (m *Manager) checkTrafficSpike(ctx context.Context, sample *InterfaceSample) {
	cfg := m.Cfg.Get()
	for _, met := range sample.Metrics {
		if !met.HasRates || met.Name != sample.Primary {
			continue
		}
		avg, err := m.DB.AvgRxRate(ctx, met.Name, time.Now().Add(-10*time.Minute))
		if err != nil || avg < 10_000 { // no meaningful baseline yet
			continue
		}
		spiking := met.RxRateBps > avg*cfg.Alerts.TrafficSpikeMultiplier
		m.Alerts.Observe(ctx, engine.Observation{
			RuleKey: engine.RuleTrafficSpike, Source: met.Name,
			Severity: models.SeverityInfo,
			Title:    "Traffic spike",
			Message: fmt.Sprintf("Interface %s RX rate %.0f B/s exceeds %.1fx its 10-minute average (%.0f B/s)",
				met.Name, met.RxRateBps, cfg.Alerts.TrafficSpikeMultiplier, avg),
			Failing: spiking,
		})
	}
}

// RunChecksCycle handles latency + DNS checks (main interval).
func (m *Manager) RunChecksCycle(ctx context.Context) {
	cfg := m.Cfg.Get()

	if cfg.Collection.EnableLatencyChecks {
		gateway := cfg.Checks.Gateway
		if gateway == "auto" || gateway == "" {
			gateway = m.Gateway()
		}
		if gateway != "" {
			check := m.Latency.Check(ctx, gateway, "gateway")
			m.persistLatency(ctx, check)
			m.Alerts.Observe(ctx, engine.Observation{
				RuleKey: engine.RuleGatewayUnreachable, Source: gateway,
				Severity: models.SeverityCritical,
				Title:    "Gateway unreachable",
				Message:  fmt.Sprintf("Gateway %s is not responding to %s checks", gateway, check.Method),
				Failing:  !check.Success,
			})
			m.observeLoss(ctx, check, cfg.Alerts.PacketLossWarning)
		}

		allFailed := len(cfg.Checks.InternetTargets) > 0
		for _, target := range cfg.Checks.InternetTargets {
			check := m.Latency.Check(ctx, target, "internet")
			m.persistLatency(ctx, check)
			if check.Success {
				allFailed = false
			}
			m.observeLoss(ctx, check, cfg.Alerts.PacketLossWarning)
		}
		if len(cfg.Checks.InternetTargets) > 0 {
			m.Alerts.Observe(ctx, engine.Observation{
				RuleKey: engine.RuleInternetUnreachable, Source: "internet",
				Severity: models.SeverityCritical,
				Title:    "Internet unreachable",
				Message:  "All configured internet targets are unreachable",
				Failing:  allFailed,
			})
		}
	}

	if cfg.Collection.EnableDNSChecks {
		for _, domain := range cfg.Checks.DNSDomains {
			types := []string{"A"}
			if cfg.Checks.DNSCheckAAAA {
				types = append(types, "AAAA")
			}
			var aCheck models.DNSCheck
			for _, rt := range types {
				check := m.DNS.Check(ctx, domain, rt)
				if err := m.DB.InsertDNSCheck(ctx, check); err != nil {
					m.Log.Warn("persist dns check failed", "error", err)
				}
				m.Bus.Publish("dns.check.updated", check)
				if rt == "A" {
					aCheck = check
				}
			}
			// Alerts key off the A lookup — AAAA failures alone are common
			// on IPv4-only networks and shouldn't page anyone (§7.4).
			m.Alerts.Observe(ctx, engine.Observation{
				RuleKey: engine.RuleDNSFailure, Source: domain,
				Severity: models.SeverityWarning,
				Title:    "DNS failure",
				Message:  fmt.Sprintf("DNS lookup for %s failed: %s", domain, aCheck.Error),
				Failing:  !aCheck.Success,
			})
			m.Alerts.Observe(ctx, engine.Observation{
				RuleKey: engine.RuleHighDNSLatency, Source: domain,
				Severity: models.SeverityWarning,
				Title:    "High DNS latency",
				Message:  fmt.Sprintf("DNS lookup for %s took %.0fms (threshold %.0fms)", domain, aCheck.LatencyMs, cfg.Alerts.DNSLatencyWarningMs),
				Failing:  aCheck.Success && aCheck.LatencyMs > cfg.Alerts.DNSLatencyWarningMs,
			})
		}
	}
}

func (m *Manager) persistLatency(ctx context.Context, check models.LatencyCheck) {
	if err := m.DB.InsertLatencyCheck(ctx, check); err != nil {
		m.Log.Warn("persist latency check failed", "error", err)
	}
	m.Bus.Publish("latency.check.updated", check)
}

func (m *Manager) observeLoss(ctx context.Context, check models.LatencyCheck, threshold float64) {
	m.Alerts.Observe(ctx, engine.Observation{
		RuleKey: engine.RuleHighPacketLoss, Source: check.Target,
		Severity: models.SeverityWarning,
		Title:    "High packet loss",
		Message:  fmt.Sprintf("Packet loss to %s is %.0f%% (threshold %.0f%%)", check.Target, check.PacketLoss, threshold),
		Failing:  check.Success && check.PacketLoss > threshold,
	})
}

// RunDiscoveryCycle handles `ip neigh` device discovery (main interval).
func (m *Manager) RunDiscoveryCycle(ctx context.Context) {
	devices, err := m.Devices.Collect(ctx, m.Cfg.Get().Devices.ResolveHostnames)
	if err != nil {
		m.Log.Warn("collector failed", "collector", "discovery", "error", err)
		return
	}
	storedDevices := make([]models.Device, 0, len(devices))
	for _, dev := range devices {
		stored, isNew, err := m.DB.UpsertDevice(ctx, dev)
		if err != nil {
			m.Log.Warn("upsert device failed", "ip", dev.IP, "error", err)
			continue
		}
		storedDevices = append(storedDevices, stored)
		if isNew {
			m.Log.Info("device discovered", "ip", stored.IP, "mac", stored.MAC)
			_ = m.DB.InsertEvent(ctx, "device.discovered", models.SeverityInfo, engine.MarshalPayload(stored))
			m.Bus.Publish("device.discovered", stored)
			m.Alerts.Observe(ctx, engine.Observation{
				RuleKey: engine.RuleNewDevice, Source: stored.IP + "/" + stored.MAC,
				Severity: models.SeverityInfo,
				Title:    "New device detected",
				Message:  fmt.Sprintf("New device %s (%s) appeared on %s", stored.IP, orUnknown(stored.MAC), orUnknown(stored.InterfaceName)),
				Failing:  true,
			})
		} else {
			m.Bus.Publish("device.updated", stored)
		}
	}
	// New-device alerts self-resolve once the device is no longer "new":
	// re-observe healthy each cycle for known devices so they age out.
	m.resolveNewDeviceAlerts(ctx, storedDevices)
}

// resolveNewDeviceAlerts marks new-device alerts healthy on subsequent
// sightings so the info alert auto-resolves after the resolve debounce.
func (m *Manager) resolveNewDeviceAlerts(ctx context.Context, devices []models.Device) {
	for _, dev := range devices {
		if dev.FirstSeenAt != nil && time.Since(*dev.FirstSeenAt) < 10*time.Minute {
			continue // still fresh; keep the alert open
		}
		m.Alerts.Observe(ctx, engine.Observation{
			RuleKey: engine.RuleNewDevice, Source: dev.IP + "/" + dev.MAC,
			Severity: models.SeverityInfo,
			Title:    "New device detected",
			Failing:  false,
		})
	}
}

// RunMemoryCycle handles /proc/meminfo + daemon self-monitor.
func (m *Manager) RunMemoryCycle(ctx context.Context) {
	cfg := m.Cfg.Get()
	mem, err := m.Memory.Collect()
	if err != nil {
		m.Log.Warn("collector failed", "collector", "memory", "error", err)
	} else {
		m.mu.Lock()
		m.memTotal = mem.MemTotal
		m.mu.Unlock()
		if err := m.DB.InsertMemoryMetrics(ctx, mem); err != nil {
			m.Log.Warn("persist memory metrics failed", "error", err)
		}
		m.Bus.Publish("system.memory.updated", mem)
		m.Alerts.Observe(ctx, engine.Observation{
			RuleKey: engine.RuleHighMemory, Source: "system",
			Severity: models.SeverityWarning,
			Title:    "High system memory usage",
			Message:  fmt.Sprintf("Memory usage is %.0f%% (threshold %.0f%%)", mem.MemoryPercent, cfg.Alerts.MemoryUsageWarningPercent),
			Failing:  mem.MemoryPercent > cfg.Alerts.MemoryUsageWarningPercent,
		})
		m.Alerts.Observe(ctx, engine.Observation{
			RuleKey: engine.RuleHighSwap, Source: "system",
			Severity: models.SeverityWarning,
			Title:    "High swap usage",
			Message:  fmt.Sprintf("Swap usage is %.0f%% (threshold %.0f%%)", mem.SwapPercent, cfg.Alerts.SwapUsageWarningPercent),
			Failing:  mem.SwapTotal > 0 && mem.SwapPercent > cfg.Alerts.SwapUsageWarningPercent,
		})
	}

	dm, err := m.DaemonMem.Collect()
	if err != nil {
		m.Log.Warn("collector failed", "collector", "daemon_memory", "error", err)
		return
	}
	if err := m.DB.InsertDaemonMemory(ctx, dm); err != nil {
		m.Log.Warn("persist daemon memory failed", "error", err)
	}
	m.Bus.Publish("system.daemon_memory.updated", dm)
	thresholdBytes := cfg.Alerts.DaemonMemoryWarningMB * 1024 * 1024
	m.Alerts.Observe(ctx, engine.Observation{
		RuleKey: engine.RuleDaemonMemoryHigh, Source: "daemon",
		Severity: models.SeverityWarning,
		Title:    "Daemon memory high",
		Message:  fmt.Sprintf("RouteViewNet RSS is %.0fMB (threshold %.0fMB)", float64(dm.RSSBytes)/1024/1024, cfg.Alerts.DaemonMemoryWarningMB),
		Failing:  float64(dm.RSSBytes) > thresholdBytes,
	})
}

// RunLoadCycle handles /proc/loadavg (§7.7).
func (m *Manager) RunLoadCycle(ctx context.Context) {
	cfg := m.Cfg.Get()
	load, err := m.Load.Collect()
	if err != nil {
		m.Log.Warn("collector failed", "collector", "load", "error", err)
		return
	}
	if err := m.DB.InsertLoadMetrics(ctx, load); err != nil {
		m.Log.Warn("persist load metrics failed", "error", err)
	}
	m.Bus.Publish("system.load.updated", load)
	threshold := cfg.Alerts.LoadAvgMultiplierWarning * float64(load.CPUCores)
	m.Alerts.Observe(ctx, engine.Observation{
		RuleKey: engine.RuleHighLoad, Source: "system",
		Severity: models.SeverityWarning,
		Title:    "High system load",
		Message:  fmt.Sprintf("1-minute load average %.2f exceeds %.1f (%.1fx of %d cores)", load.Load1, threshold, cfg.Alerts.LoadAvgMultiplierWarning, load.CPUCores),
		Failing:  load.Load1 > threshold,
	})
}

// RunProcessMemoryCycle handles the top-N process scan (§7.8).
func (m *Manager) RunProcessMemoryCycle(ctx context.Context) {
	cfg := m.Cfg.Get()
	m.ProcMem.StoreCommand = cfg.Privacy.StoreProcessCommand
	m.mu.RLock()
	memTotal := m.memTotal
	m.mu.RUnlock()
	procs, err := m.ProcMem.Collect(memTotal)
	if err != nil {
		m.Log.Warn("collector failed", "collector", "process_memory", "error", err)
		return
	}
	if err := m.DB.InsertProcessMemory(ctx, procs); err != nil {
		m.Log.Warn("persist process memory failed", "error", err)
	}
	m.Bus.Publish("system.process_memory.updated", procs)
}

// RunAppTrafficCycle samples per-app TCP traffic deltas.
func (m *Manager) RunAppTrafficCycle(ctx context.Context) {
	rows, err := m.AppTraffic.Collect(ctx)
	if err != nil {
		m.Log.Warn("collector failed", "collector", "app_traffic", "error", err)
		return
	}
	if err := m.DB.InsertAppTraffic(ctx, rows); err != nil {
		m.Log.Warn("persist app traffic failed", "error", err)
		return
	}
	if len(rows) > 0 {
		m.Bus.Publish("traffic.apps.updated", rows)
	}
}

func orUnknown(s string) string {
	if s == "" {
		return "unknown"
	}
	return s
}
