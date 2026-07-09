package collector

import (
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"routeviewnet/internal/models"
)

// ProcessMemoryCollector scans /proc/[pid]/status for the top-N processes
// by RSS (§7.8). Command lines are only captured when explicitly enabled
// (privacy default, §11.5).
type ProcessMemoryCollector struct {
	ProcRoot     string
	TopN         int
	StoreCommand bool
}

func NewProcessMemoryCollector() *ProcessMemoryCollector {
	return &ProcessMemoryCollector{ProcRoot: "/proc", TopN: 10}
}

func (c *ProcessMemoryCollector) Name() string { return "process_memory" }

// ParseProcStatus extracts Name, VmRSS, and VmSize (kB → bytes) from a
// /proc/[pid]/status document.
func ParseProcStatus(content string) (name string, rss, vsize uint64) {
	for _, line := range strings.Split(content, "\n") {
		switch {
		case strings.HasPrefix(line, "Name:"):
			name = strings.TrimSpace(line[5:])
		case strings.HasPrefix(line, "VmRSS:"):
			rss = parseKB(line[6:])
		case strings.HasPrefix(line, "VmSize:"):
			vsize = parseKB(line[7:])
		}
	}
	return
}

func parseKB(s string) uint64 {
	fields := strings.Fields(s)
	if len(fields) < 1 {
		return 0
	}
	v, _ := strconv.ParseUint(fields[0], 10, 64)
	return v * 1024
}

// Collect scans all numeric /proc entries. Processes disappearing mid-scan
// are skipped silently (§14.2). memTotal contextualizes memory_percent.
func (c *ProcessMemoryCollector) Collect(memTotal uint64) ([]models.ProcessMemory, error) {
	entries, err := os.ReadDir(c.ProcRoot)
	if err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	var procs []models.ProcessMemory
	for _, e := range entries {
		pid, err := strconv.Atoi(e.Name())
		if err != nil {
			continue
		}
		b, err := os.ReadFile(filepath.Join(c.ProcRoot, e.Name(), "status"))
		if err != nil {
			continue // process vanished mid-scan
		}
		name, rss, vsize := ParseProcStatus(string(b))
		if rss == 0 {
			continue // kernel threads etc.
		}
		p := models.ProcessMemory{
			PID: pid, Name: name, RSSBytes: rss, VirtualBytes: vsize, CollectedAt: now,
		}
		if memTotal > 0 {
			p.MemoryPercent = float64(rss) / float64(memTotal) * 100
		}
		if c.StoreCommand {
			if cmd, err := os.ReadFile(filepath.Join(c.ProcRoot, e.Name(), "cmdline")); err == nil {
				p.Command = strings.TrimSpace(strings.ReplaceAll(string(cmd), "\x00", " "))
			}
		}
		procs = append(procs, p)
	}
	sort.Slice(procs, func(i, j int) bool { return procs[i].RSSBytes > procs[j].RSSBytes })
	if len(procs) > c.TopN {
		procs = procs[:c.TopN]
	}
	return procs, nil
}
