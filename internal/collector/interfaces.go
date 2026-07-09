// Package collector gathers Linux network and memory/load signals from
// /proc, /sys, and `ip` (§7). Parsers are pure functions over file contents
// so they are unit-testable without a live system.
package collector

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"routeviewnet/internal/models"
)

// ifaceCounters is a raw /proc/net/dev row.
type ifaceCounters struct {
	Name      string
	RxBytes   uint64
	RxPackets uint64
	RxErrors  uint64
	RxDropped uint64
	TxBytes   uint64
	TxPackets uint64
	TxErrors  uint64
	TxDropped uint64
}

// ParseProcNetDev parses /proc/net/dev content (§7.1).
func ParseProcNetDev(content string) ([]ifaceCounters, error) {
	lines := strings.Split(content, "\n")
	if len(lines) < 2 {
		return nil, fmt.Errorf("malformed /proc/net/dev: %d lines", len(lines))
	}
	var out []ifaceCounters
	for _, line := range lines[2:] { // first two lines are headers
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		colon := strings.Index(line, ":")
		if colon < 0 {
			continue
		}
		name := strings.TrimSpace(line[:colon])
		fields := strings.Fields(line[colon+1:])
		if len(fields) < 16 {
			continue
		}
		u := func(i int) uint64 {
			v, _ := strconv.ParseUint(fields[i], 10, 64)
			return v
		}
		out = append(out, ifaceCounters{
			Name:    name,
			RxBytes: u(0), RxPackets: u(1), RxErrors: u(2), RxDropped: u(3),
			TxBytes: u(8), TxPackets: u(9), TxErrors: u(10), TxDropped: u(11),
		})
	}
	return out, nil
}

// DefaultExcluded reports whether an interface is excluded by the default
// rule (§7.1): loopback, docker bridges, virtual/VPN interfaces.
func DefaultExcluded(name string) bool {
	if name == "lo" {
		return true
	}
	for _, prefix := range []string{"docker", "br-", "veth", "virbr", "tun", "tap", "wg", "tailscale", "zt"} {
		if strings.HasPrefix(name, prefix) {
			return true
		}
	}
	return false
}

// routeEntry is a parsed /proc/net/route row.
type routeEntry struct {
	Iface       string
	Destination string // hex, little-endian
	Metric      int
}

// ParseProcNetRoute parses /proc/net/route content.
func ParseProcNetRoute(content string) []routeEntry {
	var out []routeEntry
	for i, line := range strings.Split(content, "\n") {
		if i == 0 {
			continue // header
		}
		fields := strings.Fields(line)
		if len(fields) < 8 {
			continue
		}
		metric, _ := strconv.Atoi(fields[6])
		out = append(out, routeEntry{Iface: fields[0], Destination: fields[1], Metric: metric})
	}
	return out
}

// SelectPrimaryInterface implements §7.1.5: default-route owner, lowest
// metric on ties, then first non-virtual UP interface, honoring the user
// override first.
func SelectPrimaryInterface(routes []routeEntry, counters []ifaceCounters, states map[string]string, override string) string {
	if override != "" {
		return override
	}
	best := ""
	bestMetric := int(^uint(0) >> 1)
	for _, r := range routes {
		if r.Destination != "00000000" || DefaultExcluded(r.Iface) {
			continue
		}
		if r.Metric < bestMetric {
			best, bestMetric = r.Iface, r.Metric
		}
	}
	if best != "" {
		return best
	}
	for _, c := range counters {
		if DefaultExcluded(c.Name) {
			continue
		}
		if st, ok := states[c.Name]; ok && strings.EqualFold(st, "up") {
			return c.Name
		}
	}
	return ""
}

// ParseGatewayFromRoute extracts the default gateway IP for an interface
// ("" iface = any). /proc/net/route stores it as little-endian hex.
func ParseGatewayFromRoute(content, iface string) string {
	for i, line := range strings.Split(content, "\n") {
		if i == 0 {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 3 || fields[1] != "00000000" {
			continue
		}
		if iface != "" && fields[0] != iface {
			continue
		}
		gwHex := fields[2]
		if len(gwHex) != 8 {
			continue
		}
		var b [4]uint64
		ok := true
		for j := 0; j < 4; j++ {
			v, err := strconv.ParseUint(gwHex[j*2:j*2+2], 16, 8)
			if err != nil {
				ok = false
				break
			}
			b[j] = v
		}
		if !ok {
			continue
		}
		// little-endian: bytes reversed
		return fmt.Sprintf("%d.%d.%d.%d", b[3], b[2], b[1], b[0])
	}
	return ""
}

// ComputeRates derives per-second rates from two samples (§7.1), guarding
// against counter regressions (§7.1 [v1.2]) and clock jumps (§14.4).
// Returns hasRates=false when the sample must be skipped/re-baselined.
func ComputeRates(prev, cur ifaceCounters, prevAt, curAt time.Time) (rxBps, txBps float64, hasRates bool) {
	dt := curAt.Sub(prevAt).Seconds()
	if dt <= 0 || dt > 3600 { // clock jump or implausible gap
		return 0, 0, false
	}
	if cur.RxBytes < prev.RxBytes || cur.TxBytes < prev.TxBytes { // counter reset/wrap
		return 0, 0, false
	}
	return float64(cur.RxBytes-prev.RxBytes) / dt, float64(cur.TxBytes-prev.TxBytes) / dt, true
}

// InterfaceCollector reads /proc/net/dev and /sys/class/net (§7.1).
type InterfaceCollector struct {
	ProcRoot string // "/proc" (overridable in tests)
	SysRoot  string // "/sys"

	prev   map[string]ifaceCounters
	prevAt time.Time
}

func NewInterfaceCollector() *InterfaceCollector {
	return &InterfaceCollector{ProcRoot: "/proc", SysRoot: "/sys", prev: map[string]ifaceCounters{}}
}

func (c *InterfaceCollector) Name() string { return "interfaces" }

// InterfaceSample is the result of one interface collection cycle.
type InterfaceSample struct {
	Metrics    []models.InterfaceMetric
	Interfaces []models.Interface
	Primary    string
	Gateway    string
}

func (c *InterfaceCollector) sysAttr(iface, attr string) string {
	b, err := os.ReadFile(filepath.Join(c.SysRoot, "class/net", iface, attr))
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(b))
}

// Collect reads counters, computes rates, selects the primary interface,
// and discovers the default gateway.
func (c *InterfaceCollector) Collect(override string) (*InterfaceSample, error) {
	raw, err := os.ReadFile(filepath.Join(c.ProcRoot, "net/dev"))
	if err != nil {
		return nil, fmt.Errorf("read /proc/net/dev: %w", err)
	}
	counters, err := ParseProcNetDev(string(raw))
	if err != nil {
		return nil, err
	}
	now := time.Now().UTC()

	routeRaw, _ := os.ReadFile(filepath.Join(c.ProcRoot, "net/route"))
	routes := ParseProcNetRoute(string(routeRaw))

	states := map[string]string{}
	sample := &InterfaceSample{}
	for _, ct := range counters {
		if DefaultExcluded(ct.Name) {
			continue
		}
		states[ct.Name] = c.sysAttr(ct.Name, "operstate")
	}
	sample.Primary = SelectPrimaryInterface(routes, counters, states, override)
	sample.Gateway = ParseGatewayFromRoute(string(routeRaw), sample.Primary)

	for _, ct := range counters {
		if DefaultExcluded(ct.Name) {
			continue
		}
		m := models.InterfaceMetric{
			Name:    ct.Name,
			RxBytes: ct.RxBytes, TxBytes: ct.TxBytes,
			RxPackets: ct.RxPackets, TxPackets: ct.TxPackets,
			RxErrors: ct.RxErrors, TxErrors: ct.TxErrors,
			RxDropped: ct.RxDropped, TxDropped: ct.TxDropped,
			CollectedAt: now,
		}
		if prev, ok := c.prev[ct.Name]; ok {
			m.RxRateBps, m.TxRateBps, m.HasRates = ComputeRates(prev, ct, c.prevAt, now)
		}
		sample.Metrics = append(sample.Metrics, m)

		speed := int64(0)
		if s := c.sysAttr(ct.Name, "speed"); s != "" {
			speed, _ = strconv.ParseInt(s, 10, 64)
			if speed < 0 {
				speed = 0
			}
		}
		sample.Interfaces = append(sample.Interfaces, models.Interface{
			Name:      ct.Name,
			MAC:       c.sysAttr(ct.Name, "address"),
			State:     states[ct.Name],
			SpeedMbps: speed,
			IsPrimary: ct.Name == sample.Primary,
		})
	}

	c.prev = map[string]ifaceCounters{}
	for _, ct := range counters {
		c.prev[ct.Name] = ct
	}
	c.prevAt = now
	return sample, nil
}
