package engine

import (
	"context"

	"routeviewnet/internal/models"
	"routeviewnet/internal/storage"
)

// Diagnose implements the rule-based explanation engine (§8.5): it maps the
// current set of open alerts into a plain-English diagnosis with evidence
// and suggested actions. Rules are evaluated in priority order — the first
// matching rule wins, with supporting conditions appended as evidence.
func Diagnose(ctx context.Context, db *storage.DB) models.Diagnosis {
	open, err := db.OpenAlerts(ctx)
	if err != nil {
		return models.Diagnosis{
			Status:           "unknown",
			Summary:          "Could not read current alerts.",
			LikelyCause:      "The alert store was unavailable when the diagnosis ran.",
			Evidence:         []string{err.Error()},
			SuggestedActions: []string{"Check the daemon logs (journalctl -u routeviewnetd)"},
		}
	}

	has := map[string]*models.Alert{}
	for i := range open {
		a := open[i]
		if _, ok := has[a.RuleKey]; !ok {
			has[a.RuleKey] = &a
		}
	}
	evidence := func(keys ...string) []string {
		var ev []string
		for _, k := range keys {
			if a, ok := has[k]; ok {
				ev = append(ev, a.Message)
			}
		}
		return ev
	}

	switch {
	// Rule 2: Local Network Problem — gateway unreachable/high loss.
	case has[RuleGatewayUnreachable] != nil:
		return models.Diagnosis{
			Status:      "critical",
			Summary:     "Your local network gateway is unreachable.",
			LikelyCause: "This machine cannot reach the router, so nothing beyond the local machine will work.",
			Evidence:    evidence(RuleGatewayUnreachable, RuleInterfaceDown, RuleHighPacketLoss),
			SuggestedActions: []string{
				"Check the network cable or Wi-Fi connection",
				"Check whether the router is powered on and responding",
				"Check interface status on the Traffic page",
				"Run `ip route` to confirm a default route exists",
			},
		}

	// Rule 3: External Internet Problem — gateway OK, internet fails.
	case has[RuleInternetUnreachable] != nil:
		return models.Diagnosis{
			Status:      "critical",
			Summary:     "Your router is reachable, but the internet is not.",
			LikelyCause: "The problem is likely between your router and your ISP, not inside your local network.",
			Evidence:    append([]string{"Gateway checks are passing"}, evidence(RuleInternetUnreachable)...),
			SuggestedActions: []string{
				"Check your ISP's status page or outage reports",
				"Check the router's WAN/internet indicator",
				"Try restarting the router",
			},
		}

	// Rule 1: DNS Problem — connectivity OK, DNS slow/failing.
	case has[RuleDNSFailure] != nil || has[RuleHighDNSLatency] != nil:
		status := "warning"
		summary := "DNS appears slow."
		if has[RuleDNSFailure] != nil {
			summary = "DNS lookups are failing."
		}
		return models.Diagnosis{
			Status:      status,
			Summary:     summary,
			LikelyCause: "Your gateway is reachable, but DNS resolution is slow or failing. The local network path is fine.",
			Evidence:    append([]string{"Gateway latency is normal"}, evidence(RuleDNSFailure, RuleHighDNSLatency)...),
			SuggestedActions: []string{
				"Check your configured DNS resolver",
				"Try a different DNS server (e.g. 1.1.1.1 or 9.9.9.9)",
				"Check /etc/resolv.conf",
				"Check whether raw IP connections are faster than domain lookups",
			},
		}

	// Rule 4: Traffic Congestion — spike + latency/loss together.
	case has[RuleTrafficSpike] != nil && has[RuleHighPacketLoss] != nil:
		return models.Diagnosis{
			Status:      "warning",
			Summary:     "Heavy traffic appears to be congesting your connection.",
			LikelyCause: "Bandwidth usage is well above its recent average at the same time as elevated latency/packet loss.",
			Evidence:    evidence(RuleTrafficSpike, RuleHighPacketLoss),
			SuggestedActions: []string{
				"Check which devices are active on the Devices page",
				"Pause large downloads, uploads, or sync jobs",
			},
		}

	// Rule 5: Interface Error.
	case has[RuleDroppedIncreasing] != nil || has[RuleInterfaceDown] != nil:
		return models.Diagnosis{
			Status:      "warning",
			Summary:     "A network interface is reporting problems.",
			LikelyCause: "Errors or dropped packets are increasing, which usually points at the physical link or driver.",
			Evidence:    evidence(RuleInterfaceDown, RuleDroppedIncreasing),
			SuggestedActions: []string{
				"Check the cable and connector",
				"Check NIC driver messages with `dmesg`",
				"Check negotiated interface speed",
				"Check system load on the System page",
			},
		}

	// Rule 8 [v1.1]: System Load + elevated latency.
	case has[RuleHighLoad] != nil && has[RuleHighPacketLoss] != nil:
		return models.Diagnosis{
			Status:      "warning",
			Summary:     "High system load may be affecting network performance.",
			LikelyCause: "System load is high relative to available CPU cores, which may be slowing down network-dependent processes.",
			Evidence:    evidence(RuleHighLoad, RuleHighPacketLoss),
			SuggestedActions: []string{
				"Check top processes on the System page",
				"Consider load-related causes before assuming a network fault",
			},
		}

	// Rule 6: Memory Pressure.
	case has[RuleHighMemory] != nil || has[RuleHighSwap] != nil:
		return models.Diagnosis{
			Status:      "warning",
			Summary:     "The system is under memory pressure.",
			LikelyCause: "High RAM or swap usage can slow everything down, including network-dependent applications.",
			Evidence:    evidence(RuleHighMemory, RuleHighSwap),
			SuggestedActions: []string{
				"Close unused applications",
				"Check top memory consumers on the System page",
				"Check swap usage; sustained swapping indicates too little RAM",
			},
		}

	// Rule 8 (load alone).
	case has[RuleHighLoad] != nil:
		return models.Diagnosis{
			Status:      "warning",
			Summary:     "System load is high.",
			LikelyCause: "System load is high relative to available CPU cores, which may be slowing down network-dependent processes.",
			Evidence:    evidence(RuleHighLoad),
			SuggestedActions: []string{
				"Check top processes on the System page",
				"Consider load-related causes before assuming a network fault",
			},
		}

	// Rule 7: Daemon Memory.
	case has[RuleDaemonMemoryHigh] != nil:
		return models.Diagnosis{
			Status:      "warning",
			Summary:     "RouteViewNet itself is using more memory than expected.",
			LikelyCause: "The monitoring daemon's memory footprint is above its configured threshold.",
			Evidence:    evidence(RuleDaemonMemoryHigh),
			SuggestedActions: []string{
				"Reduce retention days in Settings",
				"Restart the daemon (systemctl restart routeviewnetd)",
				"Check daemon logs for anomalies",
			},
		}

	case len(open) > 0:
		return models.Diagnosis{
			Status:           "warning",
			Summary:          "Some alerts are open, but no known failure pattern matches.",
			LikelyCause:      "See the open alerts for details.",
			Evidence:         evidence(RuleNewDevice, RuleHighPacketLoss, RuleTrafficSpike),
			SuggestedActions: []string{"Review the Alerts page"},
		}

	default:
		return models.Diagnosis{
			Status:           "healthy",
			Summary:          "Everything looks healthy.",
			LikelyCause:      "No open alerts: gateway, internet, DNS, memory, and load are all within normal ranges.",
			Evidence:         []string{"No open alerts"},
			SuggestedActions: []string{},
		}
	}
}
