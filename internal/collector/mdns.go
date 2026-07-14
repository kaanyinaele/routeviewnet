package collector

import (
	"context"
	"encoding/binary"
	"fmt"
	"net"
	"strings"
	"time"
)

// One-shot multicast DNS reverse lookup (RFC 6762 §5.1, §6.7): a PTR
// query for x.x.x.x.in-addr.arpa sent to 224.0.0.251:5353 from an
// ephemeral port. Phones, TVs, and printers often answer this directly
// even when the router serves no reverse DNS. Because the source port is
// not 5353, responders treat it as a legacy query and unicast the answer
// back to us, so no multicast group membership is needed.

const (
	mdnsAddr    = "224.0.0.251:5353"
	typePTR     = 12
	classINQU   = 0x8001 // IN, unicast-response requested
	mdnsTimeout = 1200 * time.Millisecond
)

// mdnsReverseLookup returns the mDNS hostname for an IPv4 address
// (without the trailing dot, e.g. "living-room-tv.local"), or "".
func mdnsReverseLookup(ctx context.Context, ip string) string {
	a := net.ParseIP(ip)
	if a == nil || a.To4() == nil {
		return ""
	}
	v4 := a.To4()
	qname := fmt.Sprintf("%d.%d.%d.%d.in-addr.arpa.", v4[3], v4[2], v4[1], v4[0])

	query, err := buildPTRQuery(qname)
	if err != nil {
		return ""
	}
	raddr, err := net.ResolveUDPAddr("udp4", mdnsAddr)
	if err != nil {
		return ""
	}
	conn, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.IPv4zero})
	if err != nil {
		return ""
	}
	defer conn.Close()

	deadline := time.Now().Add(mdnsTimeout)
	if d, ok := ctx.Deadline(); ok && d.Before(deadline) {
		deadline = d
	}
	_ = conn.SetDeadline(deadline)
	if _, err := conn.WriteToUDP(query, raddr); err != nil {
		return ""
	}
	buf := make([]byte, 1500)
	for {
		n, _, err := conn.ReadFromUDP(buf)
		if err != nil {
			return "" // timeout: nobody on the network claims this address
		}
		if name := parsePTRAnswer(buf[:n], qname); name != "" {
			return strings.TrimSuffix(name, ".")
		}
	}
}

// buildPTRQuery assembles a minimal DNS query message. mDNS multicast
// queries use ID 0 and no flags.
func buildPTRQuery(qname string) ([]byte, error) {
	msg := make([]byte, 12, 12+len(qname)+6)
	binary.BigEndian.PutUint16(msg[4:6], 1) // QDCOUNT
	for _, label := range strings.Split(strings.TrimSuffix(qname, "."), ".") {
		if len(label) == 0 || len(label) > 63 {
			return nil, fmt.Errorf("bad label %q", label)
		}
		msg = append(msg, byte(len(label)))
		msg = append(msg, label...)
	}
	msg = append(msg, 0) // root
	msg = binary.BigEndian.AppendUint16(msg, typePTR)
	msg = binary.BigEndian.AppendUint16(msg, classINQU)
	return msg, nil
}

// parsePTRAnswer extracts the first PTR record matching qname from a DNS
// response, handling name compression. Returns "" if none match.
func parsePTRAnswer(msg []byte, qname string) string {
	if len(msg) < 12 || msg[2]&0x80 == 0 { // not a response
		return ""
	}
	qd := int(binary.BigEndian.Uint16(msg[4:6]))
	an := int(binary.BigEndian.Uint16(msg[6:8]))
	off := 12
	for i := 0; i < qd; i++ { // skip echoed questions
		_, next, ok := decodeName(msg, off)
		if !ok || next+4 > len(msg) {
			return ""
		}
		off = next + 4
	}
	want := strings.ToLower(strings.TrimSuffix(qname, "."))
	for i := 0; i < an; i++ {
		name, next, ok := decodeName(msg, off)
		if !ok || next+10 > len(msg) {
			return ""
		}
		rtype := binary.BigEndian.Uint16(msg[next : next+2])
		rdlen := int(binary.BigEndian.Uint16(msg[next+8 : next+10]))
		rdata := next + 10
		if rdata+rdlen > len(msg) {
			return ""
		}
		if rtype == typePTR && strings.ToLower(strings.TrimSuffix(name, ".")) == want {
			target, _, ok := decodeName(msg, rdata)
			if ok {
				return target
			}
		}
		off = rdata + rdlen
	}
	return ""
}

// decodeName reads a possibly-compressed DNS name at off, returning the
// dotted name, the offset just past it in the original stream, and ok.
func decodeName(msg []byte, off int) (string, int, bool) {
	var parts []string
	next := -1 // offset after the name in the original (pre-jump) stream
	for hops := 0; ; hops++ {
		if hops > 32 || off >= len(msg) {
			return "", 0, false // corrupt or looping compression
		}
		b := int(msg[off])
		switch {
		case b == 0:
			if next < 0 {
				next = off + 1
			}
			return strings.Join(parts, "."), next, true
		case b&0xC0 == 0xC0: // compression pointer
			if off+1 >= len(msg) {
				return "", 0, false
			}
			if next < 0 {
				next = off + 2
			}
			off = (b&0x3F)<<8 | int(msg[off+1])
		default:
			if off+1+b > len(msg) {
				return "", 0, false
			}
			parts = append(parts, string(msg[off+1:off+1+b]))
			off += 1 + b
		}
	}
}
