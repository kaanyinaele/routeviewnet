// routeviewnetd is the RouteViewNet daemon: collectors, engines, storage,
// API, and the embedded dashboard in a single local process (§5).
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"routeviewnet/internal/api"
	"routeviewnet/internal/collector"
	"routeviewnet/internal/config"
	"routeviewnet/internal/engine"
	"routeviewnet/internal/scheduler"
	"routeviewnet/internal/storage"
	"routeviewnet/internal/web"
)

var version = "1.0.0"

func main() {
	configPath := flag.String("config", "/etc/routeviewnet/config.yaml", "path to config.yaml")
	flag.Parse()

	if err := run(*configPath); err != nil {
		fmt.Fprintln(os.Stderr, "fatal:", err)
		os.Exit(1)
	}
}

func run(configPath string) error {
	cfg, err := config.Load(configPath)
	if err != nil {
		return err
	}

	level := new(slog.LevelVar)
	level.Set(parseLevel(cfg.Logging.Level))
	log := newLogger(level)
	store := config.NewStore(cfg)

	db, err := storage.Open(cfg.Storage.Path, log)
	if err != nil {
		return err
	}
	// Closes whichever DB is current, including one reopened at an
	// overridden storage path below.
	defer func() { db.Close() }()

	// Re-apply settings changed via the API in a previous run (§12.4).
	// Nothing is bound or served yet, so restart-only settings (host, port,
	// storage path, ping mode) take effect here — this is the "restart" that
	// POST /settings tells the user about.
	if blob, err := db.GetSetting(context.Background(), api.SettingsOverrideKey); err == nil && blob != "" {
		override := cfg
		switch err := json.Unmarshal([]byte(blob), &override); {
		case err != nil:
			log.Warn("stored settings override unreadable, ignoring", "error", err)
		default:
			if err := store.Adopt(override); err != nil {
				log.Warn("stored settings override invalid, ignoring", "error", err)
				break
			}
			// A changed database path can only be honored on the way up.
			if override.Storage.Path != cfg.Storage.Path {
				next, err := storage.Open(override.Storage.Path, log)
				if err != nil {
					return fmt.Errorf("reopen database at configured path %s: %w", override.Storage.Path, err)
				}
				db.Close()
				db = next
			}
			cfg = store.Get()
			level.Set(parseLevel(cfg.Logging.Level))
			log.Info("applied stored settings override")
		}
	}

	bus := engine.NewBus()
	alerts := engine.NewAlertEngine(db, bus, log)
	alerts.TriggerN = func() int { return store.Get().Alerts.TriggerDebounceChecks }
	alerts.ResolveM = func() int { return store.Get().Alerts.ResolveDebounceChecks }
	health := engine.NewHealthEngine(db, bus, log)
	alerts.OnChange = health.Recompute

	mgr := collector.NewManager(db, bus, alerts, store, log)
	log.Info("starting routeviewnetd",
		"version", version, "ping_mode", mgr.PingMode(),
		"db", cfg.Storage.Path, "listen", fmt.Sprintf("%s:%d", cfg.Server.Host, cfg.Server.Port))
	if mgr.PingMode() == collector.PingTCP {
		log.Warn("ICMP unavailable (no ping_group_range membership and no CAP_NET_RAW); latency checks degrade to TCP connect probes (§7.3)")
	}
	if cfg.Server.Host == "0.0.0.0" {
		log.Warn("server.host is 0.0.0.0: the dashboard and API are LAN-accessible and v1 has no authentication (§11.2)")
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGINT)
	defer stop()

	// Startup retention pass, then hourly via the scheduler (§9.3).
	if err := db.Cleanup(ctx, store.Get().Storage.RetentionDays); err != nil {
		log.Warn("startup retention cleanup failed", "error", err)
	}
	// Seed the health score so the Overview has one before any alert fires.
	health.Recompute(ctx)

	sched := &scheduler.Scheduler{Cfg: store, Mgr: mgr, DB: db, Log: log}
	sched.Start(ctx)

	srv := &api.Server{
		DB: db, Bus: bus, Cfg: store, Mgr: mgr, Health: health,
		Log: log, Version: version, Started: time.Now(),
		Shutdown:    ctx,
		Static:      web.Handler(),
		SetLogLevel: func(l string) { level.Set(parseLevel(l)) },
	}
	httpServer := &http.Server{
		Addr:              net.JoinHostPort(cfg.Server.Host, fmt.Sprint(cfg.Server.Port)),
		Handler:           srv.Router(),
		ReadHeaderTimeout: 10 * time.Second,
		BaseContext:       func(net.Listener) context.Context { return ctx },
	}

	errCh := make(chan error, 1)
	go func() {
		if err := httpServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
	}()
	log.Info("dashboard available", "url", fmt.Sprintf("http://localhost:%d", cfg.Server.Port))

	select {
	case err := <-errCh:
		stop()
		sched.Wait()
		return err
	case <-ctx.Done():
	}

	// Graceful shutdown (§14.9): drain HTTP ≤10s, wait for collector loops.
	log.Info("shutting down")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := httpServer.Shutdown(shutdownCtx); err != nil {
		log.Warn("http shutdown incomplete", "error", err)
	}
	sched.Wait()
	log.Info("shutdown complete")
	return nil
}

// newLogger logs at whatever level holds right now, so a logging.level
// change from the Settings page takes effect without a restart.
func newLogger(level slog.Leveler) *slog.Logger {
	return slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: level}))
}

func parseLevel(level string) slog.Level {
	switch level {
	case "debug":
		return slog.LevelDebug
	case "warn":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}
