package api

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"routeviewnet/internal/config"
	"routeviewnet/internal/storage"
)

func settingsServer(t *testing.T) *Server {
	t.Helper()
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	db, err := storage.Open(filepath.Join(t.TempDir(), "settings.db"), log)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	return &Server{DB: db, Cfg: config.NewStore(config.Default()), Log: log}
}

func postSettings(t *testing.T, s *Server, body string) *httptest.ResponseRecorder {
	t.Helper()
	r := httptest.NewRequest(http.MethodPost, "/api/v1/settings", strings.NewReader(body))
	r.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.handlePostSettings(w, r)
	return w
}

// Regression: POST /settings persisted the *live* config, which Apply had
// already stripped of restart-only changes. The response promised
// restart_required, the daemon logged it, and the change was gone — a
// restart re-read the old value. The blob written to the settings table must
// carry what the user asked for.
func TestPostSettingsPersistsRestartOnlyChange(t *testing.T) {
	s := settingsServer(t)

	w := postSettings(t, s, `{"server":{"host":"0.0.0.0","port":4545,"cors_allowed_origins":null,"max_websocket_clients":20,"max_history_points":500,"allowed_hosts":null}}`)
	if w.Code != http.StatusOK {
		t.Fatalf("POST /settings = %d: %s", w.Code, w.Body.String())
	}

	var resp struct {
		Settings        config.Config `json:"settings"`
		RestartRequired bool          `json:"restart_required"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if !resp.RestartRequired {
		t.Error("changing server.host must report restart_required")
	}
	// The response is the document the user asked for, so the Settings page
	// does not show their change snapping back.
	if resp.Settings.Server.Host != "0.0.0.0" {
		t.Errorf("response should echo the requested host, got %q", resp.Settings.Server.Host)
	}
	// The running daemon is still on the address it actually bound.
	if live := s.Cfg.Get().Server.Host; live != "127.0.0.1" {
		t.Errorf("live config must keep the bound host, got %q", live)
	}

	// What survives a restart is what matters.
	blob, err := s.DB.GetSetting(context.Background(), SettingsOverrideKey)
	if err != nil {
		t.Fatal(err)
	}
	var persisted config.Config
	if err := json.Unmarshal([]byte(blob), &persisted); err != nil {
		t.Fatal(err)
	}
	if persisted.Server.Host != "0.0.0.0" {
		t.Errorf("persisted settings must carry the requested host, got %q", persisted.Server.Host)
	}

	// And a restart must actually pick it up.
	restarted := config.NewStore(config.Default())
	if err := restarted.Adopt(persisted); err != nil {
		t.Fatal(err)
	}
	if got := restarted.Get().Server.Host; got != "0.0.0.0" {
		t.Errorf("after restart the daemon should bind %q, got %q", "0.0.0.0", got)
	}
}

// GET /settings serves the document being edited, so a pending restart-only
// change is still visible after a page reload.
func TestGetSettingsShowsPendingChange(t *testing.T) {
	s := settingsServer(t)
	if w := postSettings(t, s, `{"server":{"host":"0.0.0.0","port":4545,"cors_allowed_origins":null,"max_websocket_clients":20,"max_history_points":500,"allowed_hosts":null}}`); w.Code != http.StatusOK {
		t.Fatalf("POST: %d %s", w.Code, w.Body.String())
	}

	w := httptest.NewRecorder()
	s.handleGetSettings(w, httptest.NewRequest(http.MethodGet, "/api/v1/settings", nil))
	var got config.Config
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.Server.Host != "0.0.0.0" {
		t.Errorf("GET /settings should show the pending host, got %q", got.Server.Host)
	}
}

// A hot-reloadable change applies immediately and needs no restart.
func TestPostSettingsHotReload(t *testing.T) {
	s := settingsServer(t)
	w := postSettings(t, s, `{"collection":{"interval_seconds":30,"enable_device_discovery":true,"enable_dns_checks":true,"enable_latency_checks":true,"enable_memory_monitoring":true,"enable_process_memory_monitoring":true,"enable_load_monitoring":true,"enable_app_traffic_monitoring":true,"memory_interval_seconds":10,"process_memory_interval_seconds":30,"load_interval_seconds":10,"app_traffic_interval_seconds":30}}`)
	if w.Code != http.StatusOK {
		t.Fatalf("POST: %d %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), `"restart_required":false`) {
		t.Errorf("interval change should not need a restart: %s", w.Body.String())
	}
	if got := s.Cfg.Get().Collection.IntervalSeconds; got != 30 {
		t.Errorf("interval should be live immediately, got %d", got)
	}
}

func TestPostSettingsRejectsInvalid(t *testing.T) {
	s := settingsServer(t)
	w := postSettings(t, s, `{"storage":{"path":"/tmp/x.db","retention_days":0}}`)
	if w.Code != http.StatusBadRequest {
		t.Errorf("retention_days 0 should be rejected, got %d", w.Code)
	}
	w = postSettings(t, s, `{"nope":1}`)
	if w.Code != http.StatusBadRequest {
		t.Errorf("unknown field should be rejected, got %d", w.Code)
	}
}
