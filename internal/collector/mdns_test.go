package collector

import (
	"encoding/binary"
	"testing"
)

func TestBuildPTRQuery(t *testing.T) {
	q, err := buildPTRQuery("1.0.168.192.in-addr.arpa.")
	if err != nil {
		t.Fatal(err)
	}
	if binary.BigEndian.Uint16(q[4:6]) != 1 {
		t.Error("QDCOUNT must be 1")
	}
	// First label after the 12-byte header is "1".
	if q[12] != 1 || q[13] != '1' {
		t.Errorf("first label wrong: % x", q[12:14])
	}
	// Ends with root, QTYPE=PTR, QCLASS=IN|QU.
	n := len(q)
	if binary.BigEndian.Uint16(q[n-4:n-2]) != typePTR || binary.BigEndian.Uint16(q[n-2:]) != classINQU {
		t.Errorf("trailer wrong: % x", q[n-4:])
	}
}

// buildAnswer crafts a response: question echoed, one PTR answer whose
// name is a compression pointer back to the question name.
func buildAnswer(t *testing.T, qname, target string) []byte {
	t.Helper()
	q, err := buildPTRQuery(qname)
	if err != nil {
		t.Fatal(err)
	}
	msg := append([]byte{}, q...)
	msg[2] = 0x84 // QR + AA
	binary.BigEndian.PutUint16(msg[6:8], 1)

	msg = append(msg, 0xC0, 12) // name: pointer to question at offset 12
	msg = binary.BigEndian.AppendUint16(msg, typePTR)
	msg = binary.BigEndian.AppendUint16(msg, 0x0001) // IN
	msg = append(msg, 0, 0, 0, 120)                  // TTL

	var rdata []byte
	for _, label := range []string{target, "local"} {
		rdata = append(rdata, byte(len(label)))
		rdata = append(rdata, label...)
	}
	rdata = append(rdata, 0)
	msg = binary.BigEndian.AppendUint16(msg, uint16(len(rdata)))
	return append(msg, rdata...)
}

func TestParsePTRAnswer(t *testing.T) {
	const qname = "1.0.168.192.in-addr.arpa."
	msg := buildAnswer(t, qname, "my-router")
	if got := parsePTRAnswer(msg, qname); got != "my-router.local" {
		t.Errorf("want my-router.local, got %q", got)
	}
	// A query (QR=0) must be ignored.
	q, _ := buildPTRQuery(qname)
	if got := parsePTRAnswer(q, qname); got != "" {
		t.Errorf("query parsed as answer: %q", got)
	}
	// An answer for a different name must be ignored.
	if got := parsePTRAnswer(msg, "2.0.168.192.in-addr.arpa."); got != "" {
		t.Errorf("mismatched name accepted: %q", got)
	}
	// Truncated packets must not panic or match.
	for i := 0; i < len(msg); i += 7 {
		_ = parsePTRAnswer(msg[:i], qname)
	}
}

func TestDecodeNameCompressionLoop(t *testing.T) {
	// A pointer that points at itself must terminate, not spin.
	msg := make([]byte, 14)
	msg[12], msg[13] = 0xC0, 12
	if _, _, ok := decodeName(msg, 12); ok {
		t.Error("self-referencing pointer must fail")
	}
}
