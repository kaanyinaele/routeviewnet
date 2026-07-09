import { useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { api, formatBytes, Range } from "../lib/api";
import { Card, Empty, InfoTip, RangePicker, StatusPill, Td, Th } from "../components/ui";
import { BandwidthChart } from "../components/charts";

// "View all" shows every app at or above this much data in the window.
const MIN_APP_BYTES = 20 * 1024 * 1024;

export default function TrafficPage() {
  const [range, setRange] = useState<Range>("1h");
  const [selected, setSelected] = useState<string | undefined>();
  const [showAllApps, setShowAllApps] = useState(false);

  const { data: ifaces } = useQuery({ queryKey: ["interfaces"], queryFn: api.interfaces });
  const { data: bw } = useQuery({
    queryKey: ["bandwidth", range, selected],
    queryFn: () => api.bandwidth(range, selected),
  });
  const { data: apps } = useQuery({
    queryKey: ["appTraffic", range],
    queryFn: () => api.appTraffic(range),
  });

  const appItems = apps?.items ?? [];
  const bigApps = appItems.filter((a) => a.total_bytes >= MIN_APP_BYTES);
  const shownApps = showAllApps ? bigApps : appItems.slice(0, 5);

  return (
    <div className="flex flex-col gap-4">
      <div className="flex items-center justify-between">
        <h1 className="text-xl font-semibold">Traffic</h1>
        <RangePicker value={range} onChange={setRange} />
      </div>

      <Card
        title={`Bandwidth · ${bw?.interface || "…"}`}
        tip="How much data went in (download) and out (upload) through the selected connection, over the time window picked at the top right."
      >
        {bw?.points?.length ? (
          <BandwidthChart points={bw.points} height={280} />
        ) : (
          <Empty text="No bandwidth samples yet. Give the collector a minute." />
        )}
      </Card>

      <div className="flex items-center gap-1.5">
        <h2 className="text-sm font-medium" style={{ color: "var(--ink-secondary)" }}>
          Interfaces
        </h2>
        <InfoTip text="The network connections this computer has: Wi-Fi, wired, and sometimes virtual ones created by software. Click one to chart it; “primary” is the one your internet traffic actually uses." />
      </div>
      <div className="grid grid-cols-1 gap-4 md:grid-cols-2 lg:grid-cols-3">
        {ifaces?.items?.map((i) => (
          <button
            key={i.id}
            onClick={() => setSelected(i.name)}
            className={`card p-4 text-left ${bw?.interface === i.name ? "ring-1 ring-[var(--series-1)]" : ""}`}
          >
            <div className="flex items-center justify-between">
              <span className="font-medium">{i.name}</span>
              <span className="flex gap-2">
                {i.is_primary && <StatusPill status="info" label="primary" />}
                <StatusPill status={i.state === "up" ? "ok" : "unknown"} label={i.state || "unknown"} />
              </span>
            </div>
            <div className="mt-2 text-xs" style={{ color: "var(--ink-secondary)" }}>
              <div>MAC {i.mac_address || "-"}</div>
              <div>{i.speed_mbps > 0 ? `${i.speed_mbps} Mb/s link` : "link speed unknown"}</div>
            </div>
          </button>
        )) ?? <Empty text="No interfaces yet." />}
      </div>

      <Card
        title="Data usage by app"
        tip="Which programs on this computer moved the most data over the selected time window. Counted from the computer's direct connections; traffic using other protocols (like some video calls) is not included."
      >
        {appItems.length ? (
          <>
            <table className="w-full">
              <thead>
                <tr className="border-b" style={{ borderColor: "var(--border)" }}>
                  <Th>App</Th>
                  <Th>Downloaded</Th>
                  <Th>Uploaded</Th>
                  <Th>Total</Th>
                </tr>
              </thead>
              <tbody>
                {shownApps.map((a) => (
                  <tr key={a.app} className="border-b" style={{ borderColor: "var(--border)" }}>
                    <Td>{a.app}</Td>
                    <Td className="tabular">{formatBytes(a.rx_bytes)}</Td>
                    <Td className="tabular">{formatBytes(a.tx_bytes)}</Td>
                    <Td className="tabular">{formatBytes(a.total_bytes)}</Td>
                  </tr>
                ))}
              </tbody>
            </table>
            {showAllApps && bigApps.length === 0 && (
              <Empty text="No app has used 20 MB or more in this window." />
            )}
            <div className="mt-3 flex items-center justify-between">
              <button
                onClick={() => setShowAllApps(!showAllApps)}
                className="rounded-md border px-2.5 py-1 text-xs"
                style={{ borderColor: "var(--border)", color: "var(--ink-secondary)" }}
              >
                {showAllApps ? "Show top 5" : `View all with 20 MB or more (${bigApps.length})`}
              </button>
              <span className="text-xs" style={{ color: "var(--ink-muted)" }}>
                direct connections only
              </span>
            </div>
          </>
        ) : (
          <Empty text="No app data usage measured yet. Give the collector a minute." />
        )}
      </Card>

      {bw?.points?.length ? (
        <Card
          title="Errors & drops (same window)"
          tip="Counts of network hiccups since the computer was switched on. A few are normal; numbers that keep climbing can mean a bad cable, weak Wi-Fi, or an overloaded connection."
        >
          <div className="grid grid-cols-2 gap-4 text-sm md:grid-cols-4">
            {(["rx_errors", "tx_errors", "rx_dropped", "tx_dropped"] as const).map((k) => {
              const last = bw.points![bw.points!.length - 1];
              return (
                <div key={k}>
                  <div className="text-xs" style={{ color: "var(--ink-muted)" }}>
                    {k.replace("_", " ").toUpperCase()}
                  </div>
                  <div className="tabular mt-1 text-lg font-medium">{formatCount(last[k])}</div>
                </div>
              );
            })}
          </div>
        </Card>
      ) : null}
    </div>
  );
}

function formatCount(n: number): string {
  if (n >= 1_000_000) return formatBytes(n, "");
  return String(Math.round(n));
}
