package collector

import (
	"context"
	"net"
	"testing"
	"time"

	"golang.org/x/net/dns/dnsmessage"
)

func TestResolverAddr(t *testing.T) {
	for in, want := range map[string]string{
		"192.168.1.1":      "192.168.1.1:53",
		"127.0.0.53":       "127.0.0.53:53",
		"192.168.1.1:5353": "192.168.1.1:5353",
		"[2001:db8::1]:53": "[2001:db8::1]:53",
	} {
		if got := resolverAddr(in); got != want {
			t.Errorf("resolverAddr(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestDNSFQDN(t *testing.T) {
	if got := dnsFQDN("example.com"); got != "example.com." {
		t.Errorf("want trailing dot, got %q", got)
	}
	if got := dnsFQDN("example.com."); got != "example.com." {
		t.Errorf("must not double the dot, got %q", got)
	}
}

// fakeResolver answers one query per test with the supplied handler and
// returns the address to point the collector at.
func fakeResolver(t *testing.T, reply func(q dnsmessage.Message) dnsmessage.Message) string {
	t.Helper()
	conn, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { conn.Close() })

	go func() {
		buf := make([]byte, 1500)
		for {
			n, peer, err := conn.ReadFrom(buf)
			if err != nil {
				return
			}
			var q dnsmessage.Message
			if err := q.Unpack(buf[:n]); err != nil {
				continue
			}
			resp := reply(q)
			out, err := resp.Pack()
			if err != nil {
				continue
			}
			_, _ = conn.WriteTo(out, peer)
		}
	}()
	return conn.LocalAddr().String()
}

func answerFor(q dnsmessage.Message, rcode dnsmessage.RCode, answers []dnsmessage.Resource) dnsmessage.Message {
	return dnsmessage.Message{
		Header: dnsmessage.Header{
			ID:            q.Header.ID,
			Response:      true,
			RCode:         rcode,
			Authoritative: true,
		},
		Questions: q.Questions,
		Answers:   answers,
	}
}

func aRecord(name string) dnsmessage.Resource {
	n, _ := dnsmessage.NewName(name)
	return dnsmessage.Resource{
		Header: dnsmessage.ResourceHeader{Name: n, Type: dnsmessage.TypeA, Class: dnsmessage.ClassINET, TTL: 60},
		Body:   &dnsmessage.AResource{A: [4]byte{93, 184, 216, 34}},
	}
}

// The check queries the configured resolver directly rather than going
// through net.Resolver, so it measures a real round trip instead of a stub
// cache hit that stays flat whether the resolver is healthy or on fire.
func TestCheckQueriesConfiguredResolver(t *testing.T) {
	addr := fakeResolver(t, func(q dnsmessage.Message) dnsmessage.Message {
		return answerFor(q, dnsmessage.RCodeSuccess, []dnsmessage.Resource{aRecord("example.com.")})
	})

	c := NewDNSCollector()
	c.resolver, c.resolverAt = addr, time.Now()

	res := c.Check(context.Background(), "example.com", "A")
	if !res.Success {
		t.Fatalf("want success, got error %q", res.Error)
	}
	if res.Resolver != addr {
		t.Errorf("want resolver %q, got %q", addr, res.Resolver)
	}
	if res.LatencyMs <= 0 {
		t.Errorf("want a measured round trip, got %v ms", res.LatencyMs)
	}
}

// An empty answer section is not a success: the name resolved to nothing of
// the type asked for.
func TestCheckReportsNoRecords(t *testing.T) {
	addr := fakeResolver(t, func(q dnsmessage.Message) dnsmessage.Message {
		return answerFor(q, dnsmessage.RCodeSuccess, nil)
	})
	c := NewDNSCollector()
	c.resolver, c.resolverAt = addr, time.Now()

	res := c.Check(context.Background(), "example.com", "A")
	if res.Success {
		t.Error("an empty answer section must not count as success")
	}
	if res.Error != "no A records" {
		t.Errorf("want a clear error, got %q", res.Error)
	}
}

// SERVFAIL is the signal the DNS-failure alert exists to catch.
func TestCheckReportsResolverFailure(t *testing.T) {
	addr := fakeResolver(t, func(q dnsmessage.Message) dnsmessage.Message {
		return answerFor(q, dnsmessage.RCodeServerFailure, nil)
	})
	c := NewDNSCollector()
	c.resolver, c.resolverAt = addr, time.Now()

	res := c.Check(context.Background(), "example.com", "A")
	if res.Success {
		t.Error("SERVFAIL must not count as success")
	}
	if res.Error == "" {
		t.Error("SERVFAIL should be reported with a reason")
	}
}

// An answer carrying someone else's transaction ID is not ours; accepting it
// would let an off-path responder fake a healthy resolver.
func TestCheckIgnoresMismatchedTransactionID(t *testing.T) {
	addr := fakeResolver(t, func(q dnsmessage.Message) dnsmessage.Message {
		m := answerFor(q, dnsmessage.RCodeSuccess, []dnsmessage.Resource{aRecord("example.com.")})
		m.Header.ID = q.Header.ID + 1 // not the ID we asked with
		return m
	})
	c := NewDNSCollector()
	c.Timeout = 300 * time.Millisecond
	c.resolver, c.resolverAt = addr, time.Now()

	res := c.Check(context.Background(), "example.com", "A")
	if res.Success {
		t.Error("a reply with the wrong transaction ID must not be accepted")
	}
}

func TestCheckTimesOutOnSilentResolver(t *testing.T) {
	// A port nobody is listening on: the query goes nowhere.
	conn, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := conn.LocalAddr().String()
	conn.Close()

	c := NewDNSCollector()
	c.Timeout = 300 * time.Millisecond
	c.resolver, c.resolverAt = addr, time.Now()

	start := time.Now()
	res := c.Check(context.Background(), "example.com", "A")
	if res.Success {
		t.Error("a silent resolver must not report success")
	}
	if elapsed := time.Since(start); elapsed > 2*time.Second {
		t.Errorf("check should honor its timeout, took %v", elapsed)
	}
}
