// Package models defines the shared data types passed between collectors,
// engines, storage, and the API.
package models

import "time"

// InterfaceMetric is one sample of counters for a network interface,
// optionally enriched with computed rates (§7.1).
type InterfaceMetric struct {
	Name        string    `json:"interface_name"`
	RxBytes     uint64    `json:"rx_bytes"`
	TxBytes     uint64    `json:"tx_bytes"`
	RxPackets   uint64    `json:"rx_packets"`
	TxPackets   uint64    `json:"tx_packets"`
	RxErrors    uint64    `json:"rx_errors"`
	TxErrors    uint64    `json:"tx_errors"`
	RxDropped   uint64    `json:"rx_dropped"`
	TxDropped   uint64    `json:"tx_dropped"`
	RxRateBps   float64   `json:"rx_rate_bps"`
	TxRateBps   float64   `json:"tx_rate_bps"`
	HasRates    bool      `json:"has_rates"`
	CollectedAt time.Time `json:"collected_at"`
}

// Interface is a known interface with enrichment from /sys/class/net (§9.2.1).
type Interface struct {
	ID        int64  `json:"id"`
	Name      string `json:"name"`
	MAC       string `json:"mac_address"`
	IP        string `json:"ip_address"`
	State     string `json:"state"`
	SpeedMbps int64  `json:"speed_mbps"`
	IsPrimary bool   `json:"is_primary"`
}

// Device is a neighbor discovered via `ip neigh` (§7.2, §9.2.3).
type Device struct {
	ID            int64      `json:"id"`
	IP            string     `json:"ip_address"`
	MAC           string     `json:"mac_address"`
	Hostname      string     `json:"hostname"`
	Vendor        string     `json:"vendor"`
	Nickname      string     `json:"nickname"`
	Trusted       bool       `json:"trusted"`
	InterfaceName string     `json:"interface_name"`
	State         string     `json:"state"`
	FirstSeenAt   *time.Time `json:"first_seen_at"`
	LastSeenAt    *time.Time `json:"last_seen_at"`
}

// LatencyCheck is one ping (or TCP-probe) result (§7.3).
type LatencyCheck struct {
	Target      string    `json:"target"`
	TargetType  string    `json:"target_type"` // gateway | internet | custom
	Method      string    `json:"method"`      // icmp-dgram | icmp-raw | tcp
	LatencyMs   float64   `json:"latency_ms"`
	PacketLoss  float64   `json:"packet_loss"`
	Success     bool      `json:"success"`
	Error       string    `json:"error_message,omitempty"`
	CollectedAt time.Time `json:"collected_at"`
}

// DNSCheck is one resolution attempt (§7.4).
type DNSCheck struct {
	Domain      string    `json:"domain"`
	RecordType  string    `json:"record_type"` // A | AAAA
	Resolver    string    `json:"resolver"`
	LatencyMs   float64   `json:"latency_ms"`
	Success     bool      `json:"success"`
	Error       string    `json:"error_message,omitempty"`
	CollectedAt time.Time `json:"collected_at"`
}

// Connection is one row from /proc/net/tcp|udp (§7.5).
type Connection struct {
	Protocol    string    `json:"protocol"`
	LocalIP     string    `json:"local_ip"`
	LocalPort   int       `json:"local_port"`
	RemoteIP    string    `json:"remote_ip"`
	RemotePort  int       `json:"remote_port"`
	State       string    `json:"state"`
	CollectedAt time.Time `json:"collected_at"`
}

// MemoryMetrics is a /proc/meminfo sample (§7.6).
type MemoryMetrics struct {
	MemTotal      uint64    `json:"mem_total_bytes"`
	MemAvailable  uint64    `json:"mem_available_bytes"`
	MemFree       uint64    `json:"mem_free_bytes"`
	MemUsed       uint64    `json:"mem_used_bytes"`
	Buffers       uint64    `json:"buffers_bytes"`
	Cached        uint64    `json:"cached_bytes"`
	SwapTotal     uint64    `json:"swap_total_bytes"`
	SwapFree      uint64    `json:"swap_free_bytes"`
	SwapUsed      uint64    `json:"swap_used_bytes"`
	MemoryPercent float64   `json:"memory_usage_percent"`
	SwapPercent   float64   `json:"swap_usage_percent"`
	CollectedAt   time.Time `json:"collected_at"`
}

// LoadMetrics is a /proc/loadavg sample (§7.7).
type LoadMetrics struct {
	Load1       float64   `json:"load_avg_1m"`
	Load5       float64   `json:"load_avg_5m"`
	Load15      float64   `json:"load_avg_15m"`
	CPUCores    int       `json:"cpu_core_count"`
	CollectedAt time.Time `json:"collected_at"`
}

// ProcessMemory is one process from the top-N scan (§7.8).
type ProcessMemory struct {
	PID           int       `json:"pid"`
	Name          string    `json:"process_name"`
	Command       string    `json:"command,omitempty"`
	RSSBytes      uint64    `json:"rss_bytes"`
	VirtualBytes  uint64    `json:"virtual_memory_bytes"`
	MemoryPercent float64   `json:"memory_percent"`
	CollectedAt   time.Time `json:"collected_at"`
}

// AppTraffic is one app's TCP byte delta for a single sample window.
type AppTraffic struct {
	App         string    `json:"app"`
	RxBytes     uint64    `json:"rx_bytes"`
	TxBytes     uint64    `json:"tx_bytes"`
	CollectedAt time.Time `json:"collected_at"`
}

// AppTrafficTotal is an app's aggregated usage over a query window.
type AppTrafficTotal struct {
	App        string `json:"app"`
	RxBytes    uint64 `json:"rx_bytes"`
	TxBytes    uint64 `json:"tx_bytes"`
	TotalBytes uint64 `json:"total_bytes"`
}

// DaemonMemory is the daemon's self-report (§7.9).
type DaemonMemory struct {
	RSSBytes    uint64    `json:"rss_bytes"`
	HeapAlloc   uint64    `json:"heap_alloc_bytes"`
	HeapSys     uint64    `json:"heap_sys_bytes"`
	SysBytes    uint64    `json:"sys_bytes"`
	Goroutines  int       `json:"goroutines"`
	CollectedAt time.Time `json:"collected_at"`
}

// Alert severities and statuses (§8.1, §8.2).
const (
	SeverityInfo     = "info"
	SeverityWarning  = "warning"
	SeverityCritical = "critical"

	StatusOpen     = "open"
	StatusResolved = "resolved"
)

// Alert is a deduplicated alert row (§9.2.10).
type Alert struct {
	ID              int64      `json:"id"`
	RuleKey         string     `json:"rule_key"`
	Severity        string     `json:"severity"`
	Title           string     `json:"title"`
	Message         string     `json:"message"`
	Source          string     `json:"source"`
	Status          string     `json:"status"`
	OccurrenceCount int64      `json:"occurrence_count"`
	CreatedAt       time.Time  `json:"created_at"`
	UpdatedAt       time.Time  `json:"updated_at"`
	ResolvedAt      *time.Time `json:"resolved_at"`
}

// Event is a persisted event row (§9.2.11).
type Event struct {
	ID        int64     `json:"id"`
	Type      string    `json:"type"`
	Severity  string    `json:"severity"`
	Payload   string    `json:"payload"`
	CreatedAt time.Time `json:"created_at"`
}

// HealthScore statuses (§6.1).
const (
	HealthHealthy  = "healthy"
	HealthDegraded = "degraded"
	HealthCritical = "critical"
)

// HealthScore is one computed score (§6).
type HealthScore struct {
	Score      int       `json:"score"`
	Status     string    `json:"status"`
	ComputedAt time.Time `json:"computed_at"`
}

// HealthStatusFor maps a score to its band (§6.1).
func HealthStatusFor(score int) string {
	switch {
	case score >= 90:
		return HealthHealthy
	case score >= 60:
		return HealthDegraded
	default:
		return HealthCritical
	}
}

// Diagnosis is the explanation engine output (§8.5).
type Diagnosis struct {
	Status           string   `json:"status"`
	Summary          string   `json:"summary"`
	LikelyCause      string   `json:"likely_cause"`
	Evidence         []string `json:"evidence"`
	SuggestedActions []string `json:"suggested_actions"`
}
