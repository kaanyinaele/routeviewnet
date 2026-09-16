package storage

import "fmt"

// migrations are applied in order and tracked in schema_migrations (§9.4).
// Never edit an applied migration — append a new one.
var migrations = []string{
	// 1: full v1 schema including all indexes (§9.2).
	`
CREATE TABLE IF NOT EXISTS interfaces (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  name TEXT NOT NULL UNIQUE,
  mac_address TEXT,
  ip_address TEXT,
  state TEXT,
  speed_mbps INTEGER,
  is_primary BOOLEAN DEFAULT 0,
  is_primary_override BOOLEAN DEFAULT 0,
  created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
  updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS interface_metrics (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  interface_name TEXT NOT NULL,
  rx_bytes INTEGER NOT NULL,
  tx_bytes INTEGER NOT NULL,
  rx_packets INTEGER,
  tx_packets INTEGER,
  rx_errors INTEGER,
  tx_errors INTEGER,
  rx_dropped INTEGER,
  tx_dropped INTEGER,
  rx_rate_bps REAL,
  tx_rate_bps REAL,
  collected_at DATETIME NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_iface_metrics_time ON interface_metrics(collected_at);
CREATE INDEX IF NOT EXISTS idx_iface_metrics_name_time ON interface_metrics(interface_name, collected_at);

CREATE TABLE IF NOT EXISTS devices (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  ip_address TEXT NOT NULL,
  mac_address TEXT,
  hostname TEXT,
  vendor TEXT,
  nickname TEXT,
  trusted BOOLEAN DEFAULT 0,
  interface_name TEXT,
  state TEXT,
  status TEXT DEFAULT 'unknown',
  first_seen_at DATETIME,
  last_seen_at DATETIME,
  UNIQUE(ip_address, mac_address)
);
CREATE INDEX IF NOT EXISTS idx_devices_last_seen ON devices(last_seen_at);

CREATE TABLE IF NOT EXISTS latency_checks (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  target TEXT NOT NULL,
  target_type TEXT,
  method TEXT,
  latency_ms REAL,
  packet_loss REAL,
  success BOOLEAN NOT NULL,
  error_message TEXT,
  collected_at DATETIME NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_latency_time ON latency_checks(collected_at);

CREATE TABLE IF NOT EXISTS dns_checks (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  domain TEXT NOT NULL,
  record_type TEXT DEFAULT 'A',
  resolver TEXT,
  latency_ms REAL,
  success BOOLEAN NOT NULL,
  error_message TEXT,
  collected_at DATETIME NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_dns_time ON dns_checks(collected_at);

CREATE TABLE IF NOT EXISTS connections (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  protocol TEXT NOT NULL,
  local_ip TEXT,
  local_port INTEGER,
  remote_ip TEXT,
  remote_port INTEGER,
  state TEXT,
  collected_at DATETIME NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_connections_time ON connections(collected_at);

CREATE TABLE IF NOT EXISTS system_memory_metrics (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  mem_total_bytes INTEGER NOT NULL,
  mem_available_bytes INTEGER,
  mem_free_bytes INTEGER,
  mem_used_bytes INTEGER,
  buffers_bytes INTEGER,
  cached_bytes INTEGER,
  swap_total_bytes INTEGER,
  swap_free_bytes INTEGER,
  swap_used_bytes INTEGER,
  memory_usage_percent REAL,
  swap_usage_percent REAL,
  collected_at DATETIME NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_sysmem_time ON system_memory_metrics(collected_at);

CREATE TABLE IF NOT EXISTS process_memory_metrics (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  pid INTEGER NOT NULL,
  process_name TEXT NOT NULL,
  command TEXT,
  rss_bytes INTEGER,
  virtual_memory_bytes INTEGER,
  memory_percent REAL,
  collected_at DATETIME NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_procmem_time ON process_memory_metrics(collected_at);

CREATE TABLE IF NOT EXISTS daemon_memory_metrics (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  rss_bytes INTEGER,
  heap_alloc_bytes INTEGER,
  heap_sys_bytes INTEGER,
  sys_bytes INTEGER,
  goroutines INTEGER,
  collected_at DATETIME NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_daemonmem_time ON daemon_memory_metrics(collected_at);

CREATE TABLE IF NOT EXISTS alerts (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  rule_key TEXT NOT NULL,
  severity TEXT NOT NULL,
  title TEXT NOT NULL,
  message TEXT NOT NULL,
  source TEXT,
  status TEXT DEFAULT 'open',
  occurrence_count INTEGER DEFAULT 1,
  consecutive_failures INTEGER DEFAULT 0,
  consecutive_successes INTEGER DEFAULT 0,
  created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
  updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
  resolved_at DATETIME
);
CREATE INDEX IF NOT EXISTS idx_alerts_status ON alerts(status);
CREATE INDEX IF NOT EXISTS idx_alerts_rule_source ON alerts(rule_key, source);

CREATE TABLE IF NOT EXISTS events (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  type TEXT NOT NULL,
  severity TEXT,
  payload TEXT,
  payload_schema_version INTEGER DEFAULT 1,
  created_at DATETIME DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX IF NOT EXISTS idx_events_time ON events(created_at);

CREATE TABLE IF NOT EXISTS settings (
  key TEXT PRIMARY KEY,
  value TEXT NOT NULL,
  updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS health_score_history (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  score INTEGER NOT NULL,
  status TEXT NOT NULL,
  computed_at DATETIME NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_healthscore_time ON health_score_history(computed_at);

CREATE TABLE IF NOT EXISTS system_load_metrics (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  load_avg_1m REAL,
  load_avg_5m REAL,
  load_avg_15m REAL,
  cpu_core_count INTEGER,
  collected_at DATETIME NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_load_time ON system_load_metrics(collected_at);
`,
	// 2: per-app TCP traffic deltas (Data usage by app).
	`
CREATE TABLE IF NOT EXISTS app_traffic_metrics (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  app_name TEXT NOT NULL,
  rx_bytes INTEGER NOT NULL DEFAULT 0,
  tx_bytes INTEGER NOT NULL DEFAULT 0,
  collected_at DATETIME NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_apptraffic_time ON app_traffic_metrics(collected_at);
`,
	// 3: drop the connections table and index the "latest row per key"
	// lookups that /overview runs on every request.
	//
	// connections was written every collection cycle and read by nothing —
	// no endpoint, no engine, no rule — so on a busy host it accumulated
	// millions of rows and hundreds of megabytes purely to be deleted again
	// by the retention job. The collector is gone; so is the table.
	//
	// The two indexes serve `WHERE id IN (SELECT MAX(id) ... GROUP BY ...)`,
	// which previously had to scan the whole table on every /overview call.
	`
DROP TABLE IF EXISTS connections;

CREATE INDEX IF NOT EXISTS idx_iface_metrics_name_id ON interface_metrics(interface_name, id DESC);
CREATE INDEX IF NOT EXISTS idx_latency_target_id ON latency_checks(target, id DESC);
`,
}

func (d *DB) migrate() error {
	if _, err := d.write.Exec(`CREATE TABLE IF NOT EXISTS schema_migrations (
		version INTEGER PRIMARY KEY,
		applied_at DATETIME DEFAULT CURRENT_TIMESTAMP
	)`); err != nil {
		return fmt.Errorf("create schema_migrations: %w", err)
	}
	var current int
	if err := d.write.QueryRow(`SELECT COALESCE(MAX(version),0) FROM schema_migrations`).Scan(&current); err != nil {
		return err
	}
	for i, m := range migrations {
		v := i + 1
		if v <= current {
			continue
		}
		tx, err := d.write.Begin()
		if err != nil {
			return err
		}
		if _, err := tx.Exec(m); err != nil {
			tx.Rollback()
			return fmt.Errorf("migration %d: %w", v, err)
		}
		if _, err := tx.Exec(`INSERT INTO schema_migrations (version) VALUES (?)`, v); err != nil {
			tx.Rollback()
			return err
		}
		if err := tx.Commit(); err != nil {
			return err
		}
		d.log.Info("applied migration", "version", v)
	}
	return nil
}
