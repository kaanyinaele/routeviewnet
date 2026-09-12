package config

import (
	"bytes"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"gopkg.in/yaml.v3"
)

// The shipped config.yaml documents "all values shown are the built-in
// defaults". Drift between the two is silent otherwise: yaml.Unmarshal
// ignores keys with no matching field, so a stale or renamed setting in the
// file looks like it works and simply does nothing.
func TestShippedConfigMatchesDefaults(t *testing.T) {
	const shipped = "../../packaging/config.yaml"
	cfg, err := Load(shipped)
	if err != nil {
		t.Fatalf("packaging/config.yaml must load cleanly: %v", err)
	}
	if !reflect.DeepEqual(cfg, Default()) {
		t.Errorf("packaging/config.yaml drifted from Default()\n file: %+v\n code: %+v", cfg, Default())
	}

	// Every key in the file must correspond to a real field.
	raw, err := os.ReadFile(shipped)
	if err != nil {
		t.Fatal(err)
	}
	dec := yaml.NewDecoder(bytes.NewReader(raw))
	dec.KnownFields(true)
	var strict Config
	if err := dec.Decode(&strict); err != nil {
		t.Errorf("packaging/config.yaml has a key with no matching setting: %v", err)
	}
}

func TestLoadMissingFileUsesDefaults(t *testing.T) {
	cfg, err := Load(filepath.Join(t.TempDir(), "nope.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Server.Port != 4545 || cfg.Server.Host != "127.0.0.1" || cfg.Storage.RetentionDays != 7 {
		t.Errorf("defaults wrong: %+v", cfg.Server)
	}
	if cfg.Privacy.Telemetry || cfg.Privacy.StoreProcessCommand {
		t.Error("privacy defaults must be off (§11.4, §11.5)")
	}
}

func TestLoadOverridesAndValidation(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	os.WriteFile(path, []byte("server:\n  port: 8080\nalerts:\n  dns_latency_warning_ms: 250\n"), 0o600)
	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Server.Port != 8080 || cfg.Alerts.DNSLatencyWarningMs != 250 {
		t.Errorf("overrides not applied: %+v", cfg)
	}
	// Untouched values keep defaults.
	if cfg.Collection.IntervalSeconds != 5 {
		t.Errorf("default lost: %d", cfg.Collection.IntervalSeconds)
	}

	os.WriteFile(path, []byte("server:\n  port: 99999\n"), 0o600)
	if _, err := Load(path); err == nil {
		t.Error("invalid port must be rejected")
	}
	os.WriteFile(path, []byte("checks:\n  ping_mode: banana\n"), 0o600)
	if _, err := Load(path); err == nil {
		t.Error("invalid ping_mode must be rejected")
	}
	os.WriteFile(path, []byte(":::not yaml"), 0o600)
	if _, err := Load(path); err == nil {
		t.Error("malformed yaml must be rejected")
	}
}

func TestStoreApplyRestartRequired(t *testing.T) {
	store := NewStore(Default())

	// Hot-reloadable change: no restart.
	next := store.Get()
	next.Alerts.DNSLatencyWarningMs = 900
	restart, err := store.Apply(next)
	if err != nil || restart {
		t.Errorf("threshold change: want no restart, got restart=%v err=%v", restart, err)
	}
	if store.Get().Alerts.DNSLatencyWarningMs != 900 {
		t.Error("hot change not applied")
	}

	// Restart-required change: flagged, and the running value is retained.
	next = store.Get()
	next.Server.Port = 9999
	restart, err = store.Apply(next)
	if err != nil || !restart {
		t.Errorf("port change: want restart, got restart=%v err=%v", restart, err)
	}
	if store.Get().Server.Port != 4545 {
		t.Error("running port must not change until restart")
	}

	// Invalid config rejected.
	next = store.Get()
	next.Storage.RetentionDays = 0
	if _, err := store.Apply(next); err == nil {
		t.Error("invalid retention must be rejected")
	}
}

// Regression: restart-required settings used to be stripped from the config
// before it was persisted, so a host/port/db-path change was reported as
// "restart required" and then silently thrown away — the restart delivered
// nothing. Apply must keep the running values live but preserve the
// requested ones for persistence.
func TestApplyKeepsRestartOnlyChangesForPersistence(t *testing.T) {
	store := NewStore(Default())

	next := Default()
	next.Server.Host = "0.0.0.0"
	next.Server.Port = 9999
	next.Storage.Path = "/tmp/other.db"
	next.Checks.PingMode = "tcp"
	next.Alerts.PacketLossWarning = 12 // hot-reloadable, for contrast

	restart, err := store.Apply(next)
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if !restart {
		t.Error("changing host/port/path/ping_mode must report restart_required")
	}

	// The running config keeps the values the process is actually using.
	live := store.Get()
	if live.Server.Host != "127.0.0.1" || live.Server.Port != 4545 {
		t.Errorf("live config must keep the bound address, got %s:%d", live.Server.Host, live.Server.Port)
	}
	if live.Storage.Path != Default().Storage.Path {
		t.Errorf("live config must keep the open database path, got %q", live.Storage.Path)
	}
	if live.Checks.PingMode != "auto" {
		t.Errorf("live config must keep the resolved ping mode, got %q", live.Checks.PingMode)
	}
	// Hot-reloadable settings apply immediately.
	if live.Alerts.PacketLossWarning != 12 {
		t.Errorf("hot-reloadable setting should apply at once, got %v", live.Alerts.PacketLossWarning)
	}

	// The requested document — what gets persisted and what the Settings
	// page shows — keeps the change, so the restart actually delivers it.
	want := store.Desired()
	if want.Server.Host != "0.0.0.0" || want.Server.Port != 9999 {
		t.Errorf("desired config lost the requested address: %s:%d", want.Server.Host, want.Server.Port)
	}
	if want.Storage.Path != "/tmp/other.db" {
		t.Errorf("desired config lost the requested db path: %q", want.Storage.Path)
	}
	if want.Checks.PingMode != "tcp" {
		t.Errorf("desired config lost the requested ping mode: %q", want.Checks.PingMode)
	}
}

// Adopt is the startup path: nothing is bound or open yet, so a stored
// override takes full effect, restart-only fields included.
func TestAdoptAppliesRestartOnlyFields(t *testing.T) {
	store := NewStore(Default())
	next := Default()
	next.Server.Port = 9999
	next.Server.Host = "0.0.0.0"

	if err := store.Adopt(next); err != nil {
		t.Fatalf("Adopt: %v", err)
	}
	if got := store.Get(); got.Server.Port != 9999 || got.Server.Host != "0.0.0.0" {
		t.Errorf("Adopt must install restart-only fields, got %s:%d", got.Server.Host, got.Server.Port)
	}
	if got := store.Desired(); got.Server.Port != 9999 {
		t.Errorf("Adopt must sync the desired document too, got port %d", got.Server.Port)
	}
}

func TestAdoptRejectsInvalidConfig(t *testing.T) {
	store := NewStore(Default())
	bad := Default()
	bad.Server.Port = 0
	if err := store.Adopt(bad); err == nil {
		t.Error("Adopt must validate")
	}
	if store.Get().Server.Port != 4545 {
		t.Error("a rejected Adopt must not disturb the live config")
	}
}

// Settings that the running daemon re-reads every cycle must not be reported
// as needing a restart — the old list claimed CORS origins and the primary
// interface override did, while both actually hot-reload.
func TestHotReloadableSettingsDoNotRequireRestart(t *testing.T) {
	store := NewStore(Default())
	next := Default()
	next.Server.CORSAllowedOrigins = []string{"http://localhost:3000"}
	next.Checks.PrimaryInterfaceOverride = "eth1"
	next.Devices.ResolveHostnames = false
	next.Logging.Level = "debug"
	next.Collection.IntervalSeconds = 30

	restart, err := store.Apply(next)
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if restart {
		t.Error("hot-reloadable settings must not report restart_required")
	}
	live := store.Get()
	if live.Checks.PrimaryInterfaceOverride != "eth1" || live.Collection.IntervalSeconds != 30 {
		t.Error("hot-reloadable settings must be live immediately")
	}
}
