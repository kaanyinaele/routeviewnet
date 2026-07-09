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

// tcpStates maps /proc/net/tcp hex state codes to names.
var tcpStates = map[string]string{
	"01": "ESTABLISHED", "02": "SYN_SENT", "03": "SYN_RECV", "04": "FIN_WAIT1",
	"05": "FIN_WAIT2", "06": "TIME_WAIT", "07": "CLOSE", "08": "CLOSE_WAIT",
	"09": "LAST_ACK", "0A": "LISTEN", "0B": "CLOSING",
}

// ConnectionCollector reads /proc/net/tcp and /proc/net/udp (§7.5).
type ConnectionCollector struct {
	ProcRoot string
}

func NewConnectionCollector() *ConnectionCollector { return &ConnectionCollector{ProcRoot: "/proc"} }

func (c *ConnectionCollector) Name() string { return "connections" }

// parseHexAddr decodes "0100007F:1F90" → "127.0.0.1", 8080.
func parseHexAddr(s string) (string, int) {
	parts := strings.Split(s, ":")
	if len(parts) != 2 || len(parts[0]) != 8 {
		return "", 0
	}
	var b [4]uint64
	for i := 0; i < 4; i++ {
		v, err := strconv.ParseUint(parts[0][i*2:i*2+2], 16, 8)
		if err != nil {
			return "", 0
		}
		b[i] = v
	}
	port, _ := strconv.ParseUint(parts[1], 16, 32)
	return fmt.Sprintf("%d.%d.%d.%d", b[3], b[2], b[1], b[0]), int(port)
}

// ParseProcNetTCP parses /proc/net/tcp or /proc/net/udp content.
func ParseProcNetTCP(content, protocol string, at time.Time) []models.Connection {
	var out []models.Connection
	for i, line := range strings.Split(content, "\n") {
		if i == 0 {
			continue
		}
		fields := strings.Fields(strings.TrimSpace(line))
		if len(fields) < 4 {
			continue
		}
		localIP, localPort := parseHexAddr(fields[1])
		remoteIP, remotePort := parseHexAddr(fields[2])
		if localIP == "" {
			continue
		}
		state := tcpStates[strings.ToUpper(fields[3])]
		if protocol == "udp" {
			state = ""
		}
		out = append(out, models.Connection{
			Protocol: protocol,
			LocalIP:  localIP, LocalPort: localPort,
			RemoteIP: remoteIP, RemotePort: remotePort,
			State: state, CollectedAt: at,
		})
	}
	return out
}

func (c *ConnectionCollector) Collect() ([]models.Connection, error) {
	now := time.Now().UTC()
	var out []models.Connection
	for _, proto := range []string{"tcp", "udp"} {
		b, err := os.ReadFile(filepath.Join(c.ProcRoot, "net", proto))
		if err != nil {
			continue // one missing file must not kill the collector (§7.10)
		}
		out = append(out, ParseProcNetTCP(string(b), proto, now)...)
	}
	return out, nil
}
