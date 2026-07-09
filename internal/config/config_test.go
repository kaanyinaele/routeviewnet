package config

import (
	"os"
	"path/filepath"
	"testing"
)

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
