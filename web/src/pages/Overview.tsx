import { useQuery } from "@tanstack/react-query";
import { api, formatBytes, formatRate, timeAgo } from "../lib/api";
import { Card, StatTile, StatusPill, Empty, InfoTip } from "../components/ui";
import { Sparkline } from "../components/charts";

const scoreColor: Record<string, string> = {
  healthy: "var(--status-good)",
  degraded: "var(--status-warning)",
  critical: "var(--status-critical)",
};

export default function OverviewPage() {
  const { data: o, error } = useQuery({ queryKey: ["overview"], queryFn: api.overview });
  const { data: hist } = useQuery({
    queryKey: ["healthHistory"],
    queryFn: () => api.healthHistory("24h"),
  });

  if (error) return <Empty text={`Could not reach the daemon: ${String(error)}`} />;
  if (!o) return <Empty text="Loading…" />;

  const score = o.health;
  return (
    <div className="flex flex-col gap-4">
      <h1 className="text-xl font-semibold">Overview</h1>

      <div className="grid grid-cols-1 gap-4 md:grid-cols-3">
        <Card className="md:col-span-1">
          <div className="flex items-start justify-between">
            <div>
              <div className="flex items-center gap-1.5 text-xs" style={{ color: "var(--ink-muted)" }}>
                <span>Health score</span>
                <InfoTip text="A 0–100 rating of your internet connection and your computer's health. 100 means all checks pass; the Troubleshoot page explains any points that were taken off." />
              </div>
              <div className="mt-1 text-5xl font-semibold" style={{ color: scoreColor[score.status] }}>
                {score.score}
              </div>
              <div className="mt-2">
                <StatusPill status={score.status} />
              </div>
            </div>
          </div>
          <div className="mt-3">
            <Sparkline
              points={hist?.points ?? []}
              dataKey="score"
              color={scoreColor[score.status]}
            />
            <div className="text-xs" style={{ color: "var(--ink-muted)" }}>
              last 24h
            </div>
          </div>
        </Card>

        <div className="grid grid-cols-2 gap-4 md:col-span-2">
          <StatTile
            label={`Download · ${o.primary_interface || "no primary interface"}`}
            value={o.bandwidth?.has_rates ? formatRate(o.bandwidth.rx_rate_bps) : "-"}
            sub={o.bandwidth ? `total ${formatBytes(o.bandwidth.rx_bytes)}` : undefined}
            tip="How fast data is coming into this computer right now. The total counts everything received since the computer was switched on."
          />
          <StatTile
            label={`Upload · ${o.primary_interface || "no primary interface"}`}
            value={o.bandwidth?.has_rates ? formatRate(o.bandwidth.tx_rate_bps) : "-"}
            sub={o.bandwidth ? `total ${formatBytes(o.bandwidth.tx_bytes)}` : undefined}
            tip="How fast data is leaving this computer right now. The total counts everything sent since the computer was switched on."
          />
          <StatTile
            label="Devices seen"
            value={o.device_count}
            sub="visible from this machine"
            tip="Other devices (phones, TVs, printers…) this computer has recently talked to on your home network. Quiet devices that never talk to this computer won't appear."
          />
          <StatTile
            label="Open alerts"
            value={o.open_alert_count}
            sub={o.open_alert_count === 0 ? "all clear" : "see Alerts page"}
            tip="Problems RouteViewNet can see right now. Alerts clear themselves once things return to normal, so there is nothing to dismiss."
          />
        </div>
      </div>

      <div className="grid grid-cols-1 gap-4 md:grid-cols-3">
        <Card
          title={`Gateway ${o.gateway_ip ? `· ${o.gateway_ip}` : ""}`}
          tip="Your router (the box your internet comes through) and how quickly it answers. If this fails, the problem is inside your home (Wi-Fi, cables, router), not with your internet provider."
        >
          <div className="flex items-center justify-between">
            <StatusPill status={o.gateway.status} />
            {o.gateway.status === "ok" && (
              <span className="tabular text-sm">{o.gateway.latency_ms.toFixed(1)} ms</span>
            )}
          </div>
        </Card>
        <Card
          title="Internet"
          tip="Whether the world beyond your router can be reached. If this fails while the Gateway check is fine, the problem is likely with your internet provider."
        >
          <div className="flex items-center justify-between">
            <StatusPill status={o.internet.status} />
            {o.internet.latency_ms > 0 && (
              <span className="tabular text-sm">{o.internet.latency_ms.toFixed(1)} ms</span>
            )}
          </div>
        </Card>
        <Card
          title="DNS"
          tip="DNS is the internet's address book: it turns website names into addresses. When it's slow, every page is slow to start loading, even on a fast connection."
        >
          <div className="flex items-center justify-between">
            <StatusPill status={o.dns.status} />
            {o.dns.latency_ms > 0 && <span className="tabular text-sm">{o.dns.latency_ms.toFixed(0)} ms</span>}
          </div>
        </Card>
      </div>

      <div className="grid grid-cols-1 gap-4 md:grid-cols-3">
        <StatTile
          label="Memory"
          value={o.memory ? `${o.memory.memory_usage_percent.toFixed(0)}%` : "-"}
          sub={
            o.memory
              ? `${formatBytes(o.memory.mem_used_bytes)} of ${formatBytes(o.memory.mem_total_bytes)}`
              : undefined
          }
          tip="How much of your computer's working memory (RAM) is truly busy. Fairly high numbers are normal; it only becomes a problem when this stays near 100%."
        />
        <StatTile
          label="Load average (1m)"
          value={o.load ? o.load.load_avg_1m.toFixed(2) : "-"}
          sub={o.load ? `${o.load.cpu_core_count} cores` : undefined}
          tip="How busy your processor has been over the last minute. Rule of thumb: trouble starts when this number stays above the number of cores shown below it."
        />
        <StatTile
          label="Daemon memory"
          value={o.daemon_memory ? formatBytes(o.daemon_memory.rss_bytes) : "-"}
          sub={o.daemon_memory ? `${o.daemon_memory.goroutines} goroutines` : undefined}
          tip="How much memory RouteViewNet itself uses, so you can check that the monitor isn't the thing slowing your computer down."
        />
      </div>

      <Card
        title="Recent events"
        tip="The latest things RouteViewNet noticed: problems appearing or clearing. The full history is on the Alerts page."
      >
        {o.recent_events?.length ? (
          <ul className="flex flex-col gap-2">
            {o.recent_events.map((e) => (
              <li key={e.id} className="flex items-center gap-3 text-sm">
                <StatusPill status={e.severity || "info"} label={e.type} />
                <span style={{ color: "var(--ink-muted)" }}>{timeAgo(e.created_at)}</span>
              </li>
            ))}
          </ul>
        ) : (
          <Empty text="No events yet." />
        )}
      </Card>
    </div>
  );
}
