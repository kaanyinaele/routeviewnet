package collector

import (
	"testing"
	"time"
)

const sampleNetDev = `Inter-|   Receive                                                |  Transmit
 face |bytes    packets errs drop fifo frame compressed multicast|bytes    packets errs drop fifo colls carrier compressed
    lo:  528040    5290    0    0    0     0          0         0   528040    5290    0    0    0     0       0          0
  eth0: 1000000   10000    2    5    0     0          0         0  2000000   15000    1    3    0     0       0          0
 wlan0:  400000    3000    0    0    0     0          0         0   100000    1200    0    0    0     0       0          0
docker0:     100       1    0    0    0     0          0         0      200       2    0    0    0     0       0          0
`

func TestParseProcNetDev(t *testing.T) {
	got, err := ParseProcNetDev(sampleNetDev)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 4 {
		t.Fatalf("want 4 interfaces, got %d", len(got))
	}
	eth := got[1]
	if eth.Name != "eth0" || eth.RxBytes != 1000000 || eth.TxBytes != 2000000 {
		t.Errorf("eth0 counters wrong: %+v", eth)
	}
	if eth.RxErrors != 2 || eth.RxDropped != 5 || eth.TxErrors != 1 || eth.TxDropped != 3 {
		t.Errorf("eth0 error/drop counters wrong: %+v", eth)
	}
}

func TestParseProcNetDevMalformed(t *testing.T) {
	if _, err := ParseProcNetDev(""); err == nil {
		t.Error("want error for empty content")
	}
}

func TestDefaultExcluded(t *testing.T) {
	for name, want := range map[string]bool{
		"lo": true, "docker0": true, "br-abc123": true, "veth1a2b": true,
		"virbr0": true, "tun0": true, "wg0": true, "tailscale0": true,
		"eth0": false, "wlan0": false, "enp3s0": false, "wlp2s0": false,
	} {
		if got := DefaultExcluded(name); got != want {
			t.Errorf("DefaultExcluded(%q) = %v, want %v", name, got, want)
		}
	}
}

const sampleRoute = `Iface	Destination	Gateway 	Flags	RefCnt	Use	Metric	Mask		MTU	Window	IRTT
eth0	00000000	0100A8C0	0003	0	0	100	00000000	0	0	0
wlan0	00000000	0100A8C0	0003	0	0	600	00000000	0	0	0
eth0	0000A8C0	00000000	0001	0	0	100	00FFFFFF	0	0	0
`

func TestSelectPrimaryInterface(t *testing.T) {
	routes := ParseProcNetRoute(sampleRoute)
	counters, _ := ParseProcNetDev(sampleNetDev)
	states := map[string]string{"eth0": "up", "wlan0": "up"}

	// Rule 1+2: default route with lowest metric wins.
	if got := SelectPrimaryInterface(routes, counters, states, ""); got != "eth0" {
		t.Errorf("want eth0 (lowest-metric default route), got %q", got)
	}
	// Override wins over everything.
	if got := SelectPrimaryInterface(routes, counters, states, "wlan0"); got != "wlan0" {
		t.Errorf("want override wlan0, got %q", got)
	}
	// Rule 3: no default route → first non-virtual UP interface.
	if got := SelectPrimaryInterface(nil, counters, states, ""); got != "eth0" {
		t.Errorf("want eth0 (first up), got %q", got)
	}
}

func TestParseGatewayFromRoute(t *testing.T) {
	// 0100A8C0 little-endian = 192.168.0.1
	if got := ParseGatewayFromRoute(sampleRoute, "eth0"); got != "192.168.0.1" {
		t.Errorf("want 192.168.0.1, got %q", got)
	}
	if got := ParseGatewayFromRoute(sampleRoute, ""); got != "192.168.0.1" {
		t.Errorf("any-interface lookup: want 192.168.0.1, got %q", got)
	}
}

func TestComputeRates(t *testing.T) {
	t0 := time.Now()
	prev := ifaceCounters{Name: "eth0", RxBytes: 1000, TxBytes: 2000}
	cur := ifaceCounters{Name: "eth0", RxBytes: 6000, TxBytes: 4000}

	rx, tx, ok := ComputeRates(prev, cur, t0, t0.Add(5*time.Second))
	if !ok || rx != 1000 || tx != 400 {
		t.Errorf("want (1000, 400, true), got (%v, %v, %v)", rx, tx, ok)
	}

	// Counter regression (interface bounce) must skip the sample [v1.2].
	if _, _, ok := ComputeRates(cur, prev, t0, t0.Add(5*time.Second)); ok {
		t.Error("counter regression must not produce rates")
	}
	// Clock jump backward must skip the sample (§14.4).
	if _, _, ok := ComputeRates(prev, cur, t0, t0.Add(-time.Second)); ok {
		t.Error("negative time delta must not produce rates")
	}
	// Implausible gap must skip the sample.
	if _, _, ok := ComputeRates(prev, cur, t0, t0.Add(2*time.Hour)); ok {
		t.Error("implausible time delta must not produce rates")
	}
}

func TestParseIPNeigh(t *testing.T) {
	out := `192.168.0.10 dev eth0 lladdr AA:BB:CC:DD:EE:FF REACHABLE
192.168.0.11 dev eth0 lladdr 11:22:33:44:55:66 STALE
192.168.0.12 dev eth0  FAILED
fe80::1 dev eth0 lladdr aa:aa:aa:aa:aa:aa router REACHABLE
`
	devices := ParseIPNeigh(out)
	if len(devices) != 2 {
		t.Fatalf("want 2 devices (FAILED and IPv6 excluded), got %d", len(devices))
	}
	if devices[0].MAC != "aa:bb:cc:dd:ee:ff" || devices[0].InterfaceName != "eth0" || devices[0].State != "REACHABLE" {
		t.Errorf("device 0 wrong: %+v", devices[0])
	}
}

const sampleMeminfo = `MemTotal:       16384000 kB
MemFree:         2048000 kB
MemAvailable:    8192000 kB
Buffers:          512000 kB
Cached:          4096000 kB
SwapTotal:       4096000 kB
SwapFree:        3072000 kB
`

func TestParseMeminfo(t *testing.T) {
	m, err := ParseMeminfo(sampleMeminfo, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if m.MemTotal != 16384000*1024 {
		t.Errorf("MemTotal wrong: %d", m.MemTotal)
	}
	// used = MemTotal - MemAvailable (§7.6)
	if m.MemUsed != (16384000-8192000)*1024 {
		t.Errorf("MemUsed wrong: %d", m.MemUsed)
	}
	if m.MemoryPercent != 50 {
		t.Errorf("MemoryPercent: want 50, got %v", m.MemoryPercent)
	}
	if m.SwapUsed != (4096000-3072000)*1024 || m.SwapPercent != 25 {
		t.Errorf("swap wrong: used=%d pct=%v", m.SwapUsed, m.SwapPercent)
	}
}

func TestParseMeminfoMissingTotal(t *testing.T) {
	if _, err := ParseMeminfo("MemFree: 5 kB\n", time.Now()); err == nil {
		t.Error("want error when MemTotal missing")
	}
}

func TestParseLoadavg(t *testing.T) {
	m, err := ParseLoadavg("0.52 0.58 0.59 1/389 12345\n", 8, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if m.Load1 != 0.52 || m.Load5 != 0.58 || m.Load15 != 0.59 || m.CPUCores != 8 {
		t.Errorf("loadavg wrong: %+v", m)
	}
	if _, err := ParseLoadavg("garbage", 8, time.Now()); err == nil {
		t.Error("want error for malformed loadavg")
	}
}

func TestParseProcStatus(t *testing.T) {
	content := "Name:\tfirefox\nVmSize:\t  4096000 kB\nVmRSS:\t  1024000 kB\n"
	name, rss, vsize := ParseProcStatus(content)
	if name != "firefox" || rss != 1024000*1024 || vsize != 4096000*1024 {
		t.Errorf("got name=%q rss=%d vsize=%d", name, rss, vsize)
	}
}

func TestParseProcNetTCP(t *testing.T) {
	content := `  sl  local_address rem_address   st tx_queue rx_queue tr tm->when retrnsmt   uid  timeout inode
   0: 0100007F:11C1 00000000:0000 0A 00000000:00000000 00:00000000 00000000  1000        0 12345 1 0000000000000000 100 0 0 10 0
   1: 9500A8C0:D2A4 5DB8D822:01BB 01 00000000:00000000 00:00000000 00000000  1000        0 12346 1 0000000000000000 20 4 30 10 -1
`
	conns := ParseProcNetTCP(content, "tcp", time.Now())
	if len(conns) != 2 {
		t.Fatalf("want 2 connections, got %d", len(conns))
	}
	if conns[0].LocalIP != "127.0.0.1" || conns[0].LocalPort != 4545 || conns[0].State != "LISTEN" {
		t.Errorf("conn 0 wrong: %+v", conns[0])
	}
	if conns[1].State != "ESTABLISHED" {
		t.Errorf("conn 1 state wrong: %+v", conns[1])
	}
}

func TestVendorForMAC(t *testing.T) {
	if v := VendorForMAC("b8:27:eb:12:34:56"); v != "Raspberry Pi" {
		t.Errorf("want Raspberry Pi, got %q", v)
	}
	if v := VendorForMAC("ff:ff:ff:00:00:00"); v != "" {
		t.Errorf("unknown OUI must be empty, got %q", v)
	}
}
