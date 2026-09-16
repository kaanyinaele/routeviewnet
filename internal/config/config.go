// Package config loads /etc/routeviewnet/config.yaml, applies defaults,
// validates values (§12), and provides a mutex-guarded hot-reload store
// so the scheduler and engines read current values each cycle (§12.4).
package config

import (
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"gopkg.in/yaml.v3"
)

type Server struct {
	Host                string   `yaml:"host" json:"host"`
	Port                int      `yaml:"port" json:"port"`
	CORSAllowedOrigins  []string `yaml:"cors_allowed_origins" json:"cors_allowed_origins"`
	MaxWebsocketClients int      `yaml:"max_websocket_clients" json:"max_websocket_clients"`
	MaxHistoryPoints    int      `yaml:"max_history_points" json:"max_history_points"`
	AllowedHosts        []string `yaml:"allowed_hosts" json:"allowed_hosts"`
}

type Logging struct {
	Level string `yaml:"level" json:"level"`
}

type Storage struct {
	Path          string `yaml:"path" json:"path"`
	RetentionDays int    `yaml:"retention_days" json:"retention_days"`
}

type Collection struct {
	IntervalSeconds               int  `yaml:"interval_seconds" json:"interval_seconds"`
	EnableDeviceDiscovery         bool `yaml:"enable_device_discovery" json:"enable_device_discovery"`
	EnableDNSChecks               bool `yaml:"enable_dns_checks" json:"enable_dns_checks"`
	EnableLatencyChecks           bool `yaml:"enable_latency_checks" json:"enable_latency_checks"`
	EnableMemoryMonitoring        bool `yaml:"enable_memory_monitoring" json:"enable_memory_monitoring"`
	EnableProcessMemoryMonitoring bool `yaml:"enable_process_memory_monitoring" json:"enable_process_memory_monitoring"`
	EnableLoadMonitoring          bool `yaml:"enable_load_monitoring" json:"enable_load_monitoring"`
	EnableAppTrafficMonitoring    bool `yaml:"enable_app_traffic_monitoring" json:"enable_app_traffic_monitoring"`
	MemoryIntervalSeconds         int  `yaml:"memory_interval_seconds" json:"memory_interval_seconds"`
	ProcessMemoryIntervalSeconds  int  `yaml:"process_memory_interval_seconds" json:"process_memory_interval_seconds"`
	LoadIntervalSeconds           int  `yaml:"load_interval_seconds" json:"load_interval_seconds"`
	AppTrafficIntervalSeconds     int  `yaml:"app_traffic_interval_seconds" json:"app_traffic_interval_seconds"`
}

type Checks struct {
	Gateway                  string   `yaml:"gateway" json:"gateway"`
	PrimaryInterfaceOverride string   `yaml:"primary_interface_override" json:"primary_interface_override"`
	InternetTargets          []string `yaml:"internet_targets" json:"internet_targets"`
	DNSDomains               []string `yaml:"dns_domains" json:"dns_domains"`
	DNSCheckAAAA             bool     `yaml:"dns_check_aaaa" json:"dns_check_aaaa"`
	PingMode                 string   `yaml:"ping_mode" json:"ping_mode"` // auto | icmp-dgram | icmp-raw | tcp
}

type Alerts struct {
	PacketLossWarning         float64 `yaml:"packet_loss_warning" json:"packet_loss_warning"`
	DNSLatencyWarningMs       float64 `yaml:"dns_latency_warning_ms" json:"dns_latency_warning_ms"`
	GatewayLatencyWarningMs   float64 `yaml:"gateway_latency_warning_ms" json:"gateway_latency_warning_ms"`
	TrafficSpikeMultiplier    float64 `yaml:"traffic_spike_multiplier" json:"traffic_spike_multiplier"`
	MemoryUsageWarningPercent float64 `yaml:"memory_usage_warning_percent" json:"memory_usage_warning_percent"`
	SwapUsageWarningPercent   float64 `yaml:"swap_usage_warning_percent" json:"swap_usage_warning_percent"`
	DaemonMemoryWarningMB     float64 `yaml:"daemon_memory_warning_mb" json:"daemon_memory_warning_mb"`
	LoadAvgMultiplierWarning  float64 `yaml:"load_avg_multiplier_warning" json:"load_avg_multiplier_warning"`
	TriggerDebounceChecks     int     `yaml:"trigger_debounce_checks" json:"trigger_debounce_checks"`
	ResolveDebounceChecks     int     `yaml:"resolve_debounce_checks" json:"resolve_debounce_checks"`

	// WebhookURL receives a POST when an alert opens or resolves. Empty
	// disables delivery. This is the only channel that works while nobody
	// has the dashboard open, so it is the one that makes the alert engine
	// useful rather than decorative.
	WebhookURL string `yaml:"webhook_url" json:"webhook_url"`
	// WebhookTimeoutSeconds bounds one delivery attempt.
	WebhookTimeoutSeconds int `yaml:"webhook_timeout_seconds" json:"webhook_timeout_seconds"`
}

type Privacy struct {
	Telemetry           bool `yaml:"telemetry" json:"telemetry"`
	StoreProcessCommand bool `yaml:"store_process_command" json:"store_process_command"`
}

type Devices struct {
	ResolveHostnames bool `yaml:"resolve_hostnames" json:"resolve_hostnames"`
}

type Config struct {
	Server     Server     `yaml:"server" json:"server"`
	Logging    Logging    `yaml:"logging" json:"logging"`
	Storage    Storage    `yaml:"storage" json:"storage"`
	Collection Collection `yaml:"collection" json:"collection"`
	Checks     Checks     `yaml:"checks" json:"checks"`
	Alerts     Alerts     `yaml:"alerts" json:"alerts"`
	Privacy    Privacy    `yaml:"privacy" json:"privacy"`
	Devices    Devices    `yaml:"devices" json:"devices"`
}

// Default returns the documented default configuration (§12.2).
func Default() Config {
	return Config{
		Server: Server{
			Host: "127.0.0.1",
			Port: 4545,
			// Empty, not nil: these serialize into GET /settings, and the
			// dashboard treats list settings as arrays. A nil slice would
			// marshal to null and break editing them.
			CORSAllowedOrigins:  []string{},
			AllowedHosts:        []string{},
			MaxWebsocketClients: 20,
			MaxHistoryPoints:    500,
		},
		Logging: Logging{Level: "info"},
		Storage: Storage{
			Path:          "/var/lib/routeviewnet/routeviewnet.db",
			RetentionDays: 7,
		},
		Collection: Collection{
			IntervalSeconds:               5,
			EnableDeviceDiscovery:         true,
			EnableDNSChecks:               true,
			EnableLatencyChecks:           true,
			EnableMemoryMonitoring:        true,
			EnableProcessMemoryMonitoring: true,
			EnableLoadMonitoring:          true,
			EnableAppTrafficMonitoring:    true,
			MemoryIntervalSeconds:         10,
			ProcessMemoryIntervalSeconds:  30,
			LoadIntervalSeconds:           10,
			AppTrafficIntervalSeconds:     30,
		},
		Checks: Checks{
			Gateway:         "auto",
			InternetTargets: []string{"1.1.1.1", "8.8.8.8"},
			DNSDomains:      []string{"google.com", "cloudflare.com"},
			DNSCheckAAAA:    true,
			PingMode:        "auto",
		},
		Alerts: Alerts{
			PacketLossWarning:         5,
			DNSLatencyWarningMs:       500,
			GatewayLatencyWarningMs:   100,
			TrafficSpikeMultiplier:    3,
			MemoryUsageWarningPercent: 90,
			SwapUsageWarningPercent:   50,
			DaemonMemoryWarningMB:     300,
			LoadAvgMultiplierWarning:  2.0,
			TriggerDebounceChecks:     2,
			ResolveDebounceChecks:     2,
			WebhookURL:                "",
			WebhookTimeoutSeconds:     5,
		},
		Privacy: Privacy{Telemetry: false, StoreProcessCommand: false},
		Devices: Devices{ResolveHostnames: true},
	}
}

// Load reads the YAML file at path over the defaults. A missing file is not
// an error (defaults apply); a malformed or invalid file is.
func Load(path string) (Config, error) {
	cfg := Default()
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return cfg, nil
		}
		return cfg, fmt.Errorf("read config %s: %w", path, err)
	}
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return cfg, fmt.Errorf("parse config %s: %w", path, err)
	}
	if err := cfg.Validate(); err != nil {
		return cfg, err
	}
	return cfg, nil
}

// Validate rejects invalid values with actionable messages (§12.3).
func (c *Config) Validate() error {
	if c.Server.Port < 1 || c.Server.Port > 65535 {
		return fmt.Errorf("server.port must be 1-65535, got %d", c.Server.Port)
	}
	if c.Storage.RetentionDays < 1 {
		return fmt.Errorf("storage.retention_days must be >= 1, got %d", c.Storage.RetentionDays)
	}
	// storage.path is concatenated into a SQLite URI DSN
	// ("file:" + path + "?_pragma=..."), so a path carrying its own query or
	// fragment would append pragmas of its own choosing. Requiring an absolute
	// path with no URI punctuation keeps the DSN meaning what it says.
	if strings.ContainsAny(c.Storage.Path, "?#\n\r") {
		return fmt.Errorf("storage.path must not contain '?', '#' or newlines, got %q", c.Storage.Path)
	}
	if c.Storage.Path != "" && !filepath.IsAbs(c.Storage.Path) && !strings.HasPrefix(c.Storage.Path, "tmp/") {
		// Relative paths are allowed only for the in-repo dev config that
		// `make run` writes; anything installed should name an absolute path.
		return fmt.Errorf("storage.path must be absolute, got %q", c.Storage.Path)
	}
	if c.Storage.Path == "" {
		return fmt.Errorf("storage.path must not be empty")
	}
	for name, v := range map[string]int{
		"collection.interval_seconds":                c.Collection.IntervalSeconds,
		"collection.memory_interval_seconds":         c.Collection.MemoryIntervalSeconds,
		"collection.process_memory_interval_seconds": c.Collection.ProcessMemoryIntervalSeconds,
		"collection.load_interval_seconds":           c.Collection.LoadIntervalSeconds,
		"collection.app_traffic_interval_seconds":    c.Collection.AppTrafficIntervalSeconds,
	} {
		if v < 1 {
			return fmt.Errorf("%s must be >= 1, got %d", name, v)
		}
	}
	switch c.Checks.PingMode {
	case "auto", "icmp-dgram", "icmp-raw", "tcp":
	default:
		return fmt.Errorf("checks.ping_mode must be auto|icmp-dgram|icmp-raw|tcp, got %q", c.Checks.PingMode)
	}
	switch c.Logging.Level {
	case "debug", "info", "warn", "error":
	default:
		return fmt.Errorf("logging.level must be debug|info|warn|error, got %q", c.Logging.Level)
	}
	if c.Alerts.TriggerDebounceChecks < 1 || c.Alerts.ResolveDebounceChecks < 1 {
		return fmt.Errorf("alerts.trigger_debounce_checks and resolve_debounce_checks must be >= 1")
	}
	if c.Alerts.WebhookURL != "" {
		u, err := url.Parse(c.Alerts.WebhookURL)
		switch {
		case err != nil:
			return fmt.Errorf("alerts.webhook_url is not a valid URL: %w", err)
		case u.Scheme != "http" && u.Scheme != "https":
			// The daemon POSTs to whatever this names. Restricting the
			// scheme keeps a typo from turning into a file:// or unix://
			// request against the machine being monitored.
			return fmt.Errorf("alerts.webhook_url must be http or https, got %q", u.Scheme)
		case u.Host == "":
			return fmt.Errorf("alerts.webhook_url must include a host, got %q", c.Alerts.WebhookURL)
		}
	}
	if c.Alerts.WebhookTimeoutSeconds < 1 {
		return fmt.Errorf("alerts.webhook_timeout_seconds must be >= 1, got %d", c.Alerts.WebhookTimeoutSeconds)
	}
	if c.Server.MaxHistoryPoints < 10 {
		return fmt.Errorf("server.max_history_points must be >= 10, got %d", c.Server.MaxHistoryPoints)
	}
	return nil
}

// Store is the mutex-guarded live config (§12.4). Hot-reloadable settings
// are swapped in place; restart-required settings are recorded as desired
// but only take effect on the next start.
type Store struct {
	mu sync.RWMutex
	// cfg is what is actually in effect right now.
	cfg Config
	// desired is cfg plus any restart-only changes that are not in effect
	// yet. It is what gets persisted and what the Settings page edits, so a
	// restart actually delivers what the user asked for.
	desired Config
}

func NewStore(cfg Config) *Store { return &Store{cfg: cfg, desired: cfg} }

// Get returns the configuration in effect right now. Collectors, engines,
// and middleware read this every cycle/request.
func (s *Store) Get() Config {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.cfg
}

// Desired returns the configuration the user has asked for, including
// restart-only changes that have not taken effect yet. This is what the
// API persists and serves, so restart-required settings are not lost.
func (s *Store) Desired() Config {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.desired
}

// restartOnly lists the settings a running daemon cannot change: the
// listener address is already bound, the database is already open, and the
// ping mode was resolved against the process's privileges at startup.
// Everything else is re-read from the store each cycle and hot-reloads.
func restartOnly(cur, next Config) bool {
	return cur.Server.Host != next.Server.Host ||
		cur.Server.Port != next.Server.Port ||
		cur.Storage.Path != next.Storage.Path ||
		cur.Checks.PingMode != next.Checks.PingMode
}

// Apply validates next and installs it as the live config, keeping
// restart-only fields at their running values. The full requested document
// is retained (see Desired) so persisting it survives a restart. It reports
// whether a restart is needed for the change to take full effect.
func (s *Store) Apply(next Config) (restart bool, err error) {
	if err := next.Validate(); err != nil {
		return false, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	restart = restartOnly(s.cfg, next)
	s.desired = next
	// Restart-only fields keep their running values in the live config;
	// s.desired still carries the requested ones.
	next.Server.Host = s.cfg.Server.Host
	next.Server.Port = s.cfg.Server.Port
	next.Storage.Path = s.cfg.Storage.Path
	next.Checks.PingMode = s.cfg.Checks.PingMode
	s.cfg = next
	return restart, nil
}

// Adopt installs next wholesale, restart-only fields included. It is the
// startup path: nothing is bound or open yet, so a stored override can take
// full effect. Callers must use it before the listener and DB are live.
func (s *Store) Adopt(next Config) error {
	if err := next.Validate(); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.cfg = next
	s.desired = next
	return nil
}
