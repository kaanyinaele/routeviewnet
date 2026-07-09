package collector

import (
	"os"
	"runtime"
	"time"

	"routeviewnet/internal/models"
)

// DaemonMemoryCollector reports RouteViewNet's own footprint (§7.9) from
// /proc/self/status (RSS) and runtime.ReadMemStats (heap).
type DaemonMemoryCollector struct{}

func (c *DaemonMemoryCollector) Name() string { return "daemon_memory" }

func (c *DaemonMemoryCollector) Collect() (models.DaemonMemory, error) {
	var ms runtime.MemStats
	runtime.ReadMemStats(&ms)
	m := models.DaemonMemory{
		HeapAlloc:   ms.HeapAlloc,
		HeapSys:     ms.HeapSys,
		SysBytes:    ms.Sys,
		Goroutines:  runtime.NumGoroutine(),
		CollectedAt: time.Now().UTC(),
	}
	if b, err := os.ReadFile("/proc/self/status"); err == nil {
		_, rss, _ := ParseProcStatus(string(b))
		m.RSSBytes = rss
	}
	return m, nil
}
