package collector

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"routeviewnet/internal/models"
)

// AppTrafficCollector attributes per-connection TCP byte counters
// (`ss -H -t -i -n -e`, kernel tcp_info) to the owning process via
// /proc/[pid]/fd socket inodes, and reports per-app byte deltas between
// samples. Reading other users' /proc/[pid]/fd requires CAP_SYS_PTRACE
// (granted by the systemd unit); without it, foreign connections are
// reported as "(unidentified)".
//
// Honest limits: TCP only. UDP sockets have no kernel byte counters, so
// QUIC/HTTP3 and most video-call traffic is not counted, and connections
// that open and close entirely between two samples are missed. The UI
// labels the numbers accordingly.
type AppTrafficCollector struct {
	ProcRoot string

	prev  map[string]SSConn // ino|local|peer → counters at last sample
	first bool              // baseline cycle: report nothing, just record
}

func NewAppTrafficCollector() *AppTrafficCollector {
	return &AppTrafficCollector{ProcRoot: "/proc", prev: map[string]SSConn{}, first: true}
}

func (c *AppTrafficCollector) Name() string { return "app_traffic" }

// SSConn is one TCP connection from `ss` output.
type SSConn struct {
	Ino      uint64
	Local    string
	Peer     string
	Acked    uint64 // bytes_acked: delivered to the peer (tx)
	Received uint64 // bytes_received: delivered from the peer (rx)
}

// ParseSSConnections parses `ss -H -t -i -n -e` output: one header line per
// connection (addresses, ino:) followed by an indented tcp_info line
// (bytes_acked:, bytes_received:). Loopback peers are skipped.
func ParseSSConnections(out string) []SSConn {
	var conns []SSConn
	var cur *SSConn
	for _, line := range strings.Split(out, "\n") {
		if line == "" {
			continue
		}
		indented := line[0] == ' ' || line[0] == '\t'
		if !indented {
			cur = nil
			fields := strings.Fields(line)
			if len(fields) < 5 {
				continue
			}
			local, peer := fields[3], fields[4]
			if strings.HasPrefix(peer, "127.") || strings.HasPrefix(peer, "[::1]") {
				continue // loopback is not network data usage
			}
			ino := uint64(0)
			for _, f := range fields[5:] {
				if v, ok := strings.CutPrefix(f, "ino:"); ok {
					ino, _ = strconv.ParseUint(v, 10, 64)
					break
				}
			}
			if ino == 0 {
				continue
			}
			cur = &SSConn{Ino: ino, Local: local, Peer: peer}
			continue
		}
		if cur == nil {
			continue
		}
		for _, f := range strings.Fields(line) {
			if v, ok := strings.CutPrefix(f, "bytes_acked:"); ok {
				cur.Acked, _ = strconv.ParseUint(v, 10, 64)
			} else if v, ok := strings.CutPrefix(f, "bytes_received:"); ok {
				cur.Received, _ = strconv.ParseUint(v, 10, 64)
			}
		}
		conns = append(conns, *cur)
		cur = nil
	}
	return conns
}

// appsForInodes maps socket inodes to process names by scanning
// /proc/[pid]/fd symlinks ("socket:[ino]"). Unreadable processes (other
// users, without CAP_SYS_PTRACE) are skipped silently.
func appsForInodes(procRoot string, inodes map[uint64]bool) map[uint64]string {
	found := map[uint64]string{}
	entries, err := os.ReadDir(procRoot)
	if err != nil {
		return found
	}
	for _, e := range entries {
		if len(found) == len(inodes) {
			break
		}
		pid, err := strconv.Atoi(e.Name())
		if err != nil {
			continue
		}
		fdDir := filepath.Join(procRoot, e.Name(), "fd")
		fds, err := os.ReadDir(fdDir)
		if err != nil {
			continue // process vanished or not readable
		}
		name := ""
		for _, fd := range fds {
			link, err := os.Readlink(filepath.Join(fdDir, fd.Name()))
			if err != nil || !strings.HasPrefix(link, "socket:[") {
				continue
			}
			ino, err := strconv.ParseUint(strings.TrimSuffix(strings.TrimPrefix(link, "socket:["), "]"), 10, 64)
			if err != nil || !inodes[ino] || found[ino] != "" {
				continue
			}
			if name == "" {
				name = procComm(procRoot, pid)
			}
			if name != "" {
				found[ino] = name
			}
		}
	}
	return found
}

func procComm(procRoot string, pid int) string {
	b, err := os.ReadFile(filepath.Join(procRoot, strconv.Itoa(pid), "comm"))
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(b))
}

// Accumulate applies delta accounting between samples and aggregates per
// app. The first cycle only records baselines (long-lived connections must
// not dump their whole pre-daemon history into the stats); afterwards a
// connection not seen before is counted in full, since it necessarily
// started after the previous sample.
func (c *AppTrafficCollector) Accumulate(conns []SSConn, apps map[uint64]string, now time.Time) []models.AppTraffic {
	agg := map[string]*models.AppTraffic{}
	cur := make(map[string]SSConn, len(conns))
	for _, sc := range conns {
		key := fmt.Sprintf("%d|%s|%s", sc.Ino, sc.Local, sc.Peer)
		cur[key] = sc
		var dtx, drx uint64
		if p, ok := c.prev[key]; ok {
			// Counters going backwards means the inode was reused by a new
			// socket; treat it as a fresh connection.
			if sc.Acked >= p.Acked && sc.Received >= p.Received {
				dtx, drx = sc.Acked-p.Acked, sc.Received-p.Received
			} else {
				dtx, drx = sc.Acked, sc.Received
			}
		} else if !c.first {
			dtx, drx = sc.Acked, sc.Received
		}
		if dtx == 0 && drx == 0 {
			continue
		}
		app := apps[sc.Ino]
		if app == "" {
			app = "(unidentified)"
		}
		a := agg[app]
		if a == nil {
			a = &models.AppTraffic{App: app, CollectedAt: now}
			agg[app] = a
		}
		a.TxBytes += dtx
		a.RxBytes += drx
	}
	c.prev = cur
	c.first = false
	out := make([]models.AppTraffic, 0, len(agg))
	for _, a := range agg {
		out = append(out, *a)
	}
	return out
}

// Collect samples all TCP connections and returns per-app deltas since the
// previous sample. Command execution follows §11.6: argument array with a
// timeout, never a shell.
func (c *AppTrafficCollector) Collect(ctx context.Context) ([]models.AppTraffic, error) {
	cctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	out, err := exec.CommandContext(cctx, "ss", "-H", "-t", "-i", "-n", "-e").Output()
	if err != nil {
		return nil, fmt.Errorf("run ss: %w", err)
	}
	conns := ParseSSConnections(string(out))
	inodes := make(map[uint64]bool, len(conns))
	for _, sc := range conns {
		inodes[sc.Ino] = true
	}
	apps := appsForInodes(c.ProcRoot, inodes)
	return c.Accumulate(conns, apps, time.Now().UTC()), nil
}
