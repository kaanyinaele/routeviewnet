package collector

import (
	"testing"
	"time"

	"routeviewnet/internal/models"
)

const ssSample = `ESTAB      0      0      192.168.0.149:48082  142.251.216.35:443   timer:(keepalive,3.094ms,0) uid:1000 ino:432437 sk:1 cgroup:/user.slice <->
	 ts sack cubic wscale:8,10 rto:231 mss:1348 bytes_sent:3520 bytes_acked:3521 bytes_received:7226 segs_out:21
ESTAB      0      0      127.0.0.1:41000  127.0.0.1:4545   uid:999 ino:555 sk:2 <->
	 ts sack cubic bytes_acked:900 bytes_received:900
ESTAB      0      0      192.168.0.149:46514    4.225.11.192:443   uid:1000 ino:433867 sk:3 <->
	 ts sack cubic wscale:10,10 bytes_sent:2513 bytes_acked:2514 bytes_received:7818
LISTEN     0      128    0.0.0.0:22  0.0.0.0:*   uid:0 sk:4
`

func TestParseSSConnections(t *testing.T) {
	conns := ParseSSConnections(ssSample)
	if len(conns) != 2 {
		t.Fatalf("want 2 connections (loopback and counter-less skipped), got %d: %+v", len(conns), conns)
	}
	if conns[0].Ino != 432437 || conns[0].Acked != 3521 || conns[0].Received != 7226 {
		t.Errorf("first conn parsed wrong: %+v", conns[0])
	}
	if conns[1].Ino != 433867 || conns[1].Peer != "4.225.11.192:443" {
		t.Errorf("second conn parsed wrong: %+v", conns[1])
	}
}

func TestAppTrafficAccumulate(t *testing.T) {
	c := NewAppTrafficCollector()
	now := time.Now().UTC()
	apps := map[uint64]string{1: "firefox", 2: "firefox", 3: "curl"}

	// Cycle 1: baseline only; long-lived connections must not dump their
	// pre-daemon history into the stats.
	rows := c.Accumulate([]SSConn{
		{Ino: 1, Local: "a:1", Peer: "b:443", Acked: 1000, Received: 5000},
	}, apps, now)
	if len(rows) != 0 {
		t.Fatalf("first cycle must report nothing, got %+v", rows)
	}

	// Cycle 2: existing connection grows, a new one appears (counted in
	// full), and both belong to the same app so they aggregate.
	rows = c.Accumulate([]SSConn{
		{Ino: 1, Local: "a:1", Peer: "b:443", Acked: 1500, Received: 9000},
		{Ino: 2, Local: "a:2", Peer: "c:443", Acked: 200, Received: 300},
		{Ino: 3, Local: "a:3", Peer: "d:80", Acked: 50, Received: 70},
	}, apps, now)
	got := map[string]models.AppTraffic{}
	for _, r := range rows {
		got[r.App] = r
	}
	ff := got["firefox"]
	if ff.TxBytes != 500+200 || ff.RxBytes != 4000+300 {
		t.Errorf("firefox delta wrong: %+v", ff)
	}
	if got["curl"].RxBytes != 70 {
		t.Errorf("curl delta wrong: %+v", got["curl"])
	}

	// Cycle 3: counters going backwards means inode reuse; count as fresh.
	rows = c.Accumulate([]SSConn{
		{Ino: 1, Local: "a:1", Peer: "b:443", Acked: 100, Received: 200},
	}, apps, now)
	if len(rows) != 1 || rows[0].TxBytes != 100 || rows[0].RxBytes != 200 {
		t.Errorf("inode reuse not handled: %+v", rows)
	}

	// Unattributed inodes fall into the labeled bucket.
	rows = c.Accumulate([]SSConn{
		{Ino: 99, Local: "a:9", Peer: "e:443", Acked: 10, Received: 20},
	}, apps, now)
	if len(rows) != 1 || rows[0].App != "(unidentified)" {
		t.Errorf("unattributed bucket wrong: %+v", rows)
	}
}
