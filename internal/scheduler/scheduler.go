// Package scheduler runs collection cycles on their configured intervals.
// Intervals are re-read from the config store after every run, so settings
// changes hot-reload without restart (§12.4).
package scheduler

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"routeviewnet/internal/collector"
	"routeviewnet/internal/config"
	"routeviewnet/internal/storage"
)

type Scheduler struct {
	Cfg *config.Store
	Mgr *collector.Manager
	DB  *storage.DB
	Log *slog.Logger

	wg sync.WaitGroup
}

// loop runs fn immediately, then every interval() seconds until ctx ends.
func (s *Scheduler) loop(ctx context.Context, name string, interval func() time.Duration, enabled func() bool, fn func(context.Context)) {
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		defer func() {
			if r := recover(); r != nil {
				// A panicking collector must never take down the daemon
				// (§7.10). Log and let systemd-visible logs surface it.
				s.Log.Error("collector loop panicked", "loop", name, "panic", r)
			}
		}()
		timer := time.NewTimer(0)
		defer timer.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-timer.C:
			}
			if enabled() {
				fn(ctx)
			}
			timer.Reset(interval())
		}
	}()
}

// Start launches all collection loops plus the hourly retention job (§9.3).
func (s *Scheduler) Start(ctx context.Context) {
	cfg := func() config.Config { return s.Cfg.Get() }
	always := func() bool { return true }

	s.loop(ctx, "network",
		func() time.Duration { return time.Duration(cfg().Collection.IntervalSeconds) * time.Second },
		always, s.Mgr.RunNetworkCycle)

	s.loop(ctx, "checks",
		func() time.Duration { return time.Duration(cfg().Collection.IntervalSeconds) * time.Second },
		func() bool {
			c := cfg().Collection
			return c.EnableLatencyChecks || c.EnableDNSChecks
		}, s.Mgr.RunChecksCycle)

	s.loop(ctx, "discovery",
		func() time.Duration { return time.Duration(cfg().Collection.IntervalSeconds) * time.Second },
		func() bool { return cfg().Collection.EnableDeviceDiscovery }, s.Mgr.RunDiscoveryCycle)

	s.loop(ctx, "memory",
		func() time.Duration { return time.Duration(cfg().Collection.MemoryIntervalSeconds) * time.Second },
		func() bool { return cfg().Collection.EnableMemoryMonitoring }, s.Mgr.RunMemoryCycle)

	s.loop(ctx, "load",
		func() time.Duration { return time.Duration(cfg().Collection.LoadIntervalSeconds) * time.Second },
		func() bool { return cfg().Collection.EnableLoadMonitoring }, s.Mgr.RunLoadCycle)

	s.loop(ctx, "process_memory",
		func() time.Duration {
			return time.Duration(cfg().Collection.ProcessMemoryIntervalSeconds) * time.Second
		},
		func() bool { return cfg().Collection.EnableProcessMemoryMonitoring }, s.Mgr.RunProcessMemoryCycle)

	s.loop(ctx, "app_traffic",
		func() time.Duration { return time.Duration(cfg().Collection.AppTrafficIntervalSeconds) * time.Second },
		func() bool { return cfg().Collection.EnableAppTrafficMonitoring }, s.Mgr.RunAppTrafficCycle)

	s.loop(ctx, "retention",
		func() time.Duration { return time.Hour },
		always, func(ctx context.Context) {
			if err := s.DB.Cleanup(ctx, cfg().Storage.RetentionDays); err != nil {
				s.Log.Warn("retention cleanup failed", "error", err)
			}
		})
}

// Wait blocks until all loops have exited (graceful shutdown, §14.9).
func (s *Scheduler) Wait() { s.wg.Wait() }
