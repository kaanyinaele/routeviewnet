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

// MemoryCollector reads /proc/meminfo (§7.6).
type MemoryCollector struct {
	ProcRoot string
}

func NewMemoryCollector() *MemoryCollector { return &MemoryCollector{ProcRoot: "/proc"} }

func (c *MemoryCollector) Name() string { return "memory" }

// ParseMeminfo parses /proc/meminfo content. Values are kB in the file and
// converted to bytes. used = MemTotal - MemAvailable (§7.6).
func ParseMeminfo(content string, at time.Time) (models.MemoryMetrics, error) {
	kv := map[string]uint64{}
	for _, line := range strings.Split(content, "\n") {
		colon := strings.Index(line, ":")
		if colon < 0 {
			continue
		}
		key := strings.TrimSpace(line[:colon])
		fields := strings.Fields(line[colon+1:])
		if len(fields) < 1 {
			continue
		}
		v, err := strconv.ParseUint(fields[0], 10, 64)
		if err != nil {
			continue
		}
		kv[key] = v * 1024 // kB → bytes
	}
	total, ok := kv["MemTotal"]
	if !ok || total == 0 {
		return models.MemoryMetrics{}, fmt.Errorf("meminfo missing MemTotal")
	}
	m := models.MemoryMetrics{
		MemTotal:     total,
		MemAvailable: kv["MemAvailable"],
		MemFree:      kv["MemFree"],
		Buffers:      kv["Buffers"],
		Cached:       kv["Cached"],
		SwapTotal:    kv["SwapTotal"],
		SwapFree:     kv["SwapFree"],
		CollectedAt:  at,
	}
	m.MemUsed = total - m.MemAvailable
	m.MemoryPercent = float64(m.MemUsed) / float64(total) * 100
	if m.SwapTotal > 0 {
		m.SwapUsed = m.SwapTotal - m.SwapFree
		m.SwapPercent = float64(m.SwapUsed) / float64(m.SwapTotal) * 100
	}
	return m, nil
}

func (c *MemoryCollector) Collect() (models.MemoryMetrics, error) {
	b, err := os.ReadFile(filepath.Join(c.ProcRoot, "meminfo"))
	if err != nil {
		return models.MemoryMetrics{}, fmt.Errorf("read /proc/meminfo: %w", err)
	}
	return ParseMeminfo(string(b), time.Now().UTC())
}
