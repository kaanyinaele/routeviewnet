package collector

import (
	"context"
	"fmt"
	"net"
	"os"
	"time"

	"golang.org/x/net/icmp"
	"golang.org/x/net/ipv4"

	"routeviewnet/internal/models"
)

// Ping modes (§7.3 privilege ladder [v1.2]).
const (
	PingICMPDgram = "icmp-dgram" // unprivileged, via net.ipv4.ping_group_range
	PingICMPRaw   = "icmp-raw"   // requires CAP_NET_RAW
	PingTCP       = "tcp"        // degraded fallback: TCP connect time
)

// LatencyCollector pings the gateway and internet targets (§7.3).
type LatencyCollector struct {
	Mode      string // resolved mode after DetectPingMode
	PingCount int
	Timeout   time.Duration
}

func NewLatencyCollector(configuredMode string) *LatencyCollector {
	c := &LatencyCollector{PingCount: 3, Timeout: 2 * time.Second}
	c.Mode = DetectPingMode(configuredMode)
	return c
}

func (c *LatencyCollector) Name() string { return "latency" }

// DetectPingMode walks the §7.3 ladder: unprivileged ICMP datagram socket,
// then raw ICMP, then TCP connect probing.
func DetectPingMode(configured string) string {
	if configured != "" && configured != "auto" {
		return configured
	}
	if conn, err := icmp.ListenPacket("udp4", "0.0.0.0"); err == nil {
		conn.Close()
		return PingICMPDgram
	}
	if conn, err := icmp.ListenPacket("ip4:icmp", "0.0.0.0"); err == nil {
		conn.Close()
		return PingICMPRaw
	}
	return PingTCP
}

// Check probes one target: PingCount echoes, reporting avg latency and loss.
func (c *LatencyCollector) Check(ctx context.Context, target, targetType string) models.LatencyCheck {
	res := models.LatencyCheck{
		Target: target, TargetType: targetType, Method: c.Mode,
		CollectedAt: time.Now().UTC(),
	}
	var latencies []float64
	sent := 0
	for i := 0; i < c.PingCount; i++ {
		if ctx.Err() != nil {
			break
		}
		sent++
		var (
			rtt time.Duration
			err error
		)
		switch c.Mode {
		case PingTCP:
			rtt, err = tcpProbe(ctx, target, c.Timeout)
		default:
			rtt, err = c.icmpPing(target, i)
		}
		if err == nil {
			latencies = append(latencies, float64(rtt.Microseconds())/1000.0)
		} else {
			res.Error = err.Error()
		}
	}
	if sent == 0 {
		res.Error = "cancelled"
		return res
	}
	res.PacketLoss = float64(sent-len(latencies)) / float64(sent) * 100
	if len(latencies) > 0 {
		sum := 0.0
		for _, l := range latencies {
			sum += l
		}
		res.LatencyMs = sum / float64(len(latencies))
		res.Success = true
		if res.PacketLoss == 0 {
			res.Error = ""
		}
	}
	return res
}

func (c *LatencyCollector) icmpPing(target string, seq int) (time.Duration, error) {
	network, listenAddr := "udp4", "0.0.0.0"
	if c.Mode == PingICMPRaw {
		network = "ip4:icmp"
	}
	conn, err := icmp.ListenPacket(network, listenAddr)
	if err != nil {
		return 0, fmt.Errorf("icmp listen: %w", err)
	}
	defer conn.Close()

	dst := net.Addr(&net.UDPAddr{IP: net.ParseIP(target)})
	if c.Mode == PingICMPRaw {
		dst = &net.IPAddr{IP: net.ParseIP(target)}
	}

	msg := icmp.Message{
		Type: ipv4.ICMPTypeEcho, Code: 0,
		Body: &icmp.Echo{ID: os.Getpid() & 0xffff, Seq: seq, Data: []byte("routeviewnet")},
	}
	wb, err := msg.Marshal(nil)
	if err != nil {
		return 0, err
	}

	start := time.Now()
	if _, err := conn.WriteTo(wb, dst); err != nil {
		return 0, fmt.Errorf("icmp send: %w", err)
	}
	if err := conn.SetReadDeadline(time.Now().Add(c.Timeout)); err != nil {
		return 0, err
	}
	rb := make([]byte, 1500)
	for {
		n, _, err := conn.ReadFrom(rb)
		if err != nil {
			return 0, fmt.Errorf("icmp timeout: %w", err)
		}
		rtt := time.Since(start)
		parsed, err := icmp.ParseMessage(1, rb[:n]) // 1 = ICMPv4
		if err != nil {
			continue
		}
		if parsed.Type == ipv4.ICMPTypeEchoReply {
			return rtt, nil
		}
	}
}

// tcpProbe measures TCP connect time to :443, falling back to :53 (§7.3.3).
func tcpProbe(ctx context.Context, target string, timeout time.Duration) (time.Duration, error) {
	var lastErr error
	for _, port := range []string{"443", "53"} {
		d := net.Dialer{Timeout: timeout}
		start := time.Now()
		conn, err := d.DialContext(ctx, "tcp4", net.JoinHostPort(target, port))
		if err == nil {
			rtt := time.Since(start)
			conn.Close()
			return rtt, nil
		}
		lastErr = err
	}
	return 0, fmt.Errorf("tcp probe: %w", lastErr)
}
