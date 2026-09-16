package collector

import (
	"context"
	"crypto/rand"
	"encoding/binary"
	"fmt"
	"net"
	"os"
	"strings"
	"sync"
	"time"

	"golang.org/x/net/dns/dnsmessage"

	"routeviewnet/internal/models"
)

// DNSCollector times A and AAAA lookups against configured domains (§7.4).
//
// It queries the configured resolver directly over UDP rather than going
// through net.Resolver. The library call resolves through nsswitch and the
// local stub (systemd-resolved, nscd), so re-asking for the same domain every
// few seconds mostly measures a process-local or stub cache hit — a number
// that stays flat at a fraction of a millisecond whether the resolver is
// healthy or on fire, which makes the latency alert unable to fire. A direct
// query measures a real round trip to the resolver named in /etc/resolv.conf.
// (That resolver may still answer from its own cache; that is genuinely what
// applications on this machine experience.)
type DNSCollector struct {
	Timeout time.Duration

	mu          sync.Mutex
	resolver    string
	resolverAt  time.Time
	resolverTTL time.Duration
}

func NewDNSCollector() *DNSCollector {
	return &DNSCollector{Timeout: 3 * time.Second, resolverTTL: 30 * time.Second}
}

func (c *DNSCollector) Name() string { return "dns" }

// SystemResolver returns the first nameserver in /etc/resolv.conf, best effort.
func SystemResolver() string {
	b, err := os.ReadFile("/etc/resolv.conf")
	if err != nil {
		return ""
	}
	for _, line := range strings.Split(string(b), "\n") {
		fields := strings.Fields(strings.TrimSpace(line))
		if len(fields) >= 2 && fields[0] == "nameserver" {
			return fields[1]
		}
	}
	return ""
}

// cachedResolver avoids re-reading /etc/resolv.conf on every single check
// (four reads per cycle at the default settings) while still noticing a
// resolver change within the TTL.
func (c *DNSCollector) cachedResolver() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	if time.Since(c.resolverAt) < c.resolverTTL {
		return c.resolver
	}
	c.resolver = SystemResolver()
	c.resolverAt = time.Now()
	return c.resolver
}

// Check performs one lookup of the given record type ("A" or "AAAA").
func (c *DNSCollector) Check(ctx context.Context, domain, recordType string) models.DNSCheck {
	res := models.DNSCheck{
		Domain: domain, RecordType: recordType,
		Resolver:    c.cachedResolver(),
		CollectedAt: time.Now().UTC(),
	}
	lctx, cancel := context.WithTimeout(ctx, c.Timeout)
	defer cancel()

	if res.Resolver != "" {
		latency, answers, err := queryResolver(lctx, res.Resolver, domain, recordType)
		res.LatencyMs = latency
		switch {
		case err != nil:
			res.Error = err.Error()
		case answers == 0:
			res.Error = fmt.Sprintf("no %s records", recordType)
		default:
			res.Success = true
		}
		return res
	}

	// No nameserver in /etc/resolv.conf: fall back to the system resolver so
	// the check still reports something, cache effects and all.
	network := "ip4"
	if recordType == "AAAA" {
		network = "ip6"
	}
	start := time.Now()
	ips, err := net.DefaultResolver.LookupIP(lctx, network, domain)
	res.LatencyMs = float64(time.Since(start).Microseconds()) / 1000.0
	switch {
	case err != nil:
		res.Error = err.Error()
	case len(ips) == 0:
		res.Error = fmt.Sprintf("no %s records", recordType)
	default:
		res.Success = true
	}
	return res
}

// queryResolver sends one UDP query to resolver and returns the round-trip
// time in milliseconds plus the number of answer records of the wanted type.
func queryResolver(ctx context.Context, resolver, domain, recordType string) (float64, int, error) {
	want := dnsmessage.TypeA
	if recordType == "AAAA" {
		want = dnsmessage.TypeAAAA
	}
	name, err := dnsmessage.NewName(dnsFQDN(domain))
	if err != nil {
		return 0, 0, fmt.Errorf("bad domain %q: %w", domain, err)
	}
	id, err := queryID()
	if err != nil {
		return 0, 0, err
	}
	query, err := (&dnsmessage.Message{
		Header: dnsmessage.Header{ID: id, RecursionDesired: true},
		Questions: []dnsmessage.Question{{
			Name:  name,
			Type:  want,
			Class: dnsmessage.ClassINET,
		}},
	}).Pack()
	if err != nil {
		return 0, 0, err
	}

	var dialer net.Dialer
	conn, err := dialer.DialContext(ctx, "udp", resolverAddr(resolver))
	if err != nil {
		return 0, 0, fmt.Errorf("dial resolver %s: %w", resolver, err)
	}
	defer conn.Close()
	if deadline, ok := ctx.Deadline(); ok {
		_ = conn.SetDeadline(deadline)
	}

	start := time.Now()
	if _, err := conn.Write(query); err != nil {
		return 0, 0, fmt.Errorf("query %s: %w", resolver, err)
	}
	buf := make([]byte, 1500)
	for {
		n, err := conn.Read(buf)
		if err != nil {
			return ms(time.Since(start)), 0, fmt.Errorf("no answer from %s: %w", resolver, err)
		}
		latency := ms(time.Since(start))

		var resp dnsmessage.Message
		if err := resp.Unpack(buf[:n]); err != nil {
			continue // not a DNS message; keep waiting until the deadline
		}
		if resp.Header.ID != id || !resp.Header.Response {
			continue // a stray or spoofed answer, not ours
		}
		if resp.Header.RCode != dnsmessage.RCodeSuccess {
			return latency, 0, fmt.Errorf("resolver returned %s", resp.Header.RCode)
		}
		answers := 0
		for _, a := range resp.Answers {
			if a.Header.Type == want {
				answers++
			}
		}
		return latency, answers, nil
	}
}

// queryID returns an unpredictable DNS transaction ID, so an off-path answer
// cannot be passed off as the resolver's.
func queryID() (uint16, error) {
	var b [2]byte
	if _, err := rand.Read(b[:]); err != nil {
		return 0, fmt.Errorf("generate query id: %w", err)
	}
	return binary.BigEndian.Uint16(b[:]), nil
}

func dnsFQDN(domain string) string {
	if strings.HasSuffix(domain, ".") {
		return domain
	}
	return domain + "."
}

// resolverAddr accepts a bare address from resolv.conf or one that already
// carries a port.
func resolverAddr(resolver string) string {
	if _, _, err := net.SplitHostPort(resolver); err == nil {
		return resolver
	}
	return net.JoinHostPort(resolver, "53")
}

func ms(d time.Duration) float64 { return float64(d.Microseconds()) / 1000.0 }
