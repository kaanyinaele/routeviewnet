package collector

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"routeviewnet/internal/models"
)

// LoadCollector reads /proc/loadavg — the deliberately narrow CPU signal
// of §7.7. Core count comes from runtime.NumCPU (logical cores).
type LoadCollector struct {
	ProcRoot string
}

func NewLoadCollector() *LoadCollector { return &LoadCollector{ProcRoot: "/proc"} }

func (c *LoadCollector) Name() string { return "load" }

// ParseLoadavg parses "0.52 0.58 0.59 1/389 12345".
func ParseLoadavg(content string, cores int, at time.Time) (models.LoadMetrics, error) {
	fields := strings.Fields(strings.TrimSpace(content))
	if len(fields) < 3 {
		return models.LoadMetrics{}, fmt.Errorf("malformed /proc/loadavg: %q", content)
	}
	var vals [3]float64
	for i := 0; i < 3; i++ {
		v, err := strconv.ParseFloat(fields[i], 64)
		if err != nil {
			return models.LoadMetrics{}, fmt.Errorf("malformed loadavg field %d: %w", i, err)
		}
		vals[i] = v
	}
	return models.LoadMetrics{
		Load1: vals[0], Load5: vals[1], Load15: vals[2],
		CPUCores: cores, CollectedAt: at,
	}, nil
}

func (c *LoadCollector) Collect() (models.LoadMetrics, error) {
	b, err := os.ReadFile(filepath.Join(c.ProcRoot, "loadavg"))
	if err != nil {
		return models.LoadMetrics{}, fmt.Errorf("read /proc/loadavg: %w", err)
	}
	return ParseLoadavg(string(b), runtime.NumCPU(), time.Now().UTC())
}
