package collector

import (
	"context"
	"net"
	"os/exec"
	"strings"
	"time"

	"routeviewnet/internal/models"
)

// DeviceCollector discovers IPv4 neighbors via `ip neigh` (§7.2). Command
// execution uses an argument array with a timeout — never a shell (§11.6).
type DeviceCollector struct {
	ResolveHostnames bool
}

func (c *DeviceCollector) Name() string { return "discovery" }

// ParseIPNeigh parses `ip -4 neigh show` output lines like:
//
//	192.168.1.10 dev eth0 lladdr aa:bb:cc:dd:ee:ff REACHABLE
func ParseIPNeigh(output string) []models.Device {
	var out []models.Device
	for _, line := range strings.Split(output, "\n") {
		fields := strings.Fields(strings.TrimSpace(line))
		if len(fields) < 2 {
			continue
		}
		ip := net.ParseIP(fields[0])
		if ip == nil || ip.To4() == nil {
			continue // IPv4 only in v1 (§7 IPv6 stance)
		}
		d := models.Device{IP: fields[0]}
		for i := 1; i < len(fields)-1; i++ {
			switch fields[i] {
			case "dev":
				d.InterfaceName = fields[i+1]
			case "lladdr":
				d.MAC = strings.ToLower(fields[i+1])
			}
		}
		state := fields[len(fields)-1]
		switch state {
		case "REACHABLE", "STALE", "DELAY", "PROBE", "FAILED", "INCOMPLETE", "PERMANENT", "NOARP":
			d.State = state
		}
		if d.State == "FAILED" || d.State == "INCOMPLETE" {
			continue // not a confirmed neighbor
		}
		out = append(out, d)
	}
	return out
}

// Collect runs `ip neigh` and enriches results with best-effort reverse DNS
// (§7.2 [v1.2]) and offline OUI vendor lookup.
func (c *DeviceCollector) Collect(ctx context.Context) ([]models.Device, error) {
	cctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	out, err := exec.CommandContext(cctx, "ip", "-4", "neigh", "show").Output()
	if err != nil {
		return nil, err
	}
	devices := ParseIPNeigh(string(out))
	for i := range devices {
		devices[i].Vendor = VendorForMAC(devices[i].MAC)
		if c.ResolveHostnames {
			devices[i].Hostname = reverseLookup(cctx, devices[i].IP)
		}
	}
	return devices, nil
}

// reverseLookup is a best-effort PTR lookup with a short timeout — it must
// never block discovery for long (§7.2).
func reverseLookup(ctx context.Context, ip string) string {
	lctx, cancel := context.WithTimeout(ctx, 1500*time.Millisecond)
	defer cancel()
	names, err := net.DefaultResolver.LookupAddr(lctx, ip)
	if err != nil || len(names) == 0 {
		return ""
	}
	return strings.TrimSuffix(names[0], ".")
}

// ouiVendors is a small embedded OUI-prefix subset (§7.2 [v1.2]): offline
// only, no network MAC-vendor lookups. Unknown prefixes stay blank.
var ouiVendors = map[string]string{
	"00:1a:11": "Google", "3c:5a:b4": "Google", "f4:f5:d8": "Google",
	"b8:27:eb": "Raspberry Pi", "dc:a6:32": "Raspberry Pi", "e4:5f:01": "Raspberry Pi", "28:cd:c1": "Raspberry Pi",
	"00:03:93": "Apple", "a4:83:e7": "Apple", "f0:18:98": "Apple", "3c:22:fb": "Apple", "88:66:5a": "Apple",
	"00:15:5d": "Microsoft (Hyper-V)", "28:18:78": "Microsoft",
	"00:50:56": "VMware", "00:0c:29": "VMware",
	"52:54:00": "QEMU/KVM",
	"08:00:27": "VirtualBox",
	"18:e8:29": "Ubiquiti", "74:ac:b9": "Ubiquiti", "f0:9f:c2": "Ubiquiti",
	"a0:21:b7": "Netgear", "9c:3d:cf": "Netgear",
	"14:cc:20": "TP-Link", "50:c7:bf": "TP-Link", "60:e3:27": "TP-Link", "d8:07:b6": "TP-Link",
	"00:14:bf": "Linksys", "c0:56:27": "Belkin",
	"b0:be:76": "TP-Link", "ec:08:6b": "TP-Link",
	"fc:ec:da": "Ubiquiti",
	"00:11:32": "Synology",
	"00:e0:4c": "Realtek", "52:54:4c": "Realtek",
	"d0:50:99": "ASRock", "30:9c:23": "Micro-Star (MSI)",
	"94:de:80": "GIGA-BYTE", "1c:69:7a": "EliteGroup",
	"3c:07:54": "Samsung", "8c:71:f8": "Samsung", "cc:6e:a4": "Samsung",
	"ac:37:43": "HTC", "40:4e:36": "HTC",
	"64:16:66": "Amazon", "0c:47:c9": "Amazon", "44:65:0d": "Amazon", "fc:65:de": "Amazon",
	"18:b4:30": "Nest", "64:16:7f": "Polycom",
	"b4:75:0e": "Belkin", "94:10:3e": "Belkin",
	"00:17:88": "Philips Hue", "ec:b5:fa": "Philips",
	"d4:81:d7": "Dell", "f8:bc:12": "Dell", "18:a9:9b": "Dell",
	"3c:d9:2b": "Hewlett Packard", "94:57:a5": "Hewlett Packard",
	"00:1b:21": "Intel", "a0:36:9f": "Intel", "8c:16:45": "Intel",
	"fc:aa:14": "GIGA-BYTE", "e0:d5:5e": "GIGA-BYTE",
}

// VendorForMAC maps a MAC's OUI prefix to a vendor name, or "".
func VendorForMAC(mac string) string {
	mac = strings.ToLower(mac)
	if len(mac) < 8 {
		return ""
	}
	return ouiVendors[mac[:8]]
}
