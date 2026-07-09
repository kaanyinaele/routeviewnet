package collector

import (
	"context"
	"fmt"
	"net"
	"os"
	"strings"
	"time"

	"routeviewnet/internal/models"
)

// DNSCollector times A and AAAA lookups against configured domains (§7.4).
type DNSCollector struct {
	Timeout time.Duration
}

func NewDNSCollector() *DNSCollector { return &DNSCollector{Timeout: 3 * time.Second} }

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

// Check performs one lookup of the given record type ("A" or "AAAA").
func (c *DNSCollector) Check(ctx context.Context, domain, recordType string) models.DNSCheck {
	res := models.DNSCheck{
		Domain: domain, RecordType: recordType,
		Resolver:    SystemResolver(),
		CollectedAt: time.Now().UTC(),
	}
	lctx, cancel := context.WithTimeout(ctx, c.Timeout)
	defer cancel()

	network := "ip4"
	if recordType == "AAAA" {
		network = "ip6"
	}
	start := time.Now()
	ips, err := net.DefaultResolver.LookupIP(lctx, network, domain)
	res.LatencyMs = float64(time.Since(start).Microseconds()) / 1000.0
	if err != nil {
		res.Error = err.Error()
		return res
	}
	if len(ips) == 0 {
		res.Error = fmt.Sprintf("no %s records", recordType)
		return res
	}
	res.Success = true
	return res
}
