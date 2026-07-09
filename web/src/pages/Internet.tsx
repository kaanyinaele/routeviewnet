import { useMemo, useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { api, Range } from "../lib/api";
import { Card, Empty, RangePicker, StatusPill, Td, Th } from "../components/ui";
import { SeriesSpec, TimeSeriesChart } from "../components/charts";

// Fixed slot assignment: color follows the target, never its rank.
const seriesColors = ["var(--series-1)", "var(--series-2)", "var(--series-3)"];

export default function InternetPage() {
  const [range, setRange] = useState<Range>("1h");
  const { data: latency } = useQuery({
    queryKey: ["latencyChecks", range],
    queryFn: () => api.latencyChecks(range),
  });
  const { data: dns } = useQuery({
    queryKey: ["dnsChecks", range],
    queryFn: () => api.dnsChecks(range),
  });

  // Pivot checks into one row per timestamp with a column per target.
  const { chartData, series } = useMemo(() => {
    const items = latency?.items ?? [];
    const targets: string[] = [];
    for (const c of items) if (!targets.includes(c.target)) targets.push(c.target);
    targets.sort();
    const byTime = new Map<string, any>();
    for (const c of [...items].reverse()) {
      const row = byTime.get(c.collected_at) ?? { time: c.collected_at };
      if (c.success) row[c.target] = c.latency_ms;
      byTime.set(c.collected_at, row);
    }
    const series: SeriesSpec[] = targets.slice(0, 3).map((t, i) => ({
      key: t,
      name: t,
      color: seriesColors[i],
    }));
    return { chartData: Array.from(byTime.values()), series };
  }, [latency]);

  const latest = useMemo(() => {
    const seen = new Set<string>();
    return (latency?.items ?? []).filter((c) => {
      if (seen.has(c.target)) return false;
      seen.add(c.target);
      return true;
    });
  }, [latency]);

  return (
    <div className="flex flex-col gap-4">
      <div className="flex items-center justify-between">
        <h1 className="text-xl font-semibold">Internet Health</h1>
        <RangePicker value={range} onChange={setRange} />
      </div>

      <div className="grid grid-cols-1 gap-4 md:grid-cols-3">
        {latest.map((c) => (
          <Card
            key={c.target}
            title={`${c.target} · ${c.target_type}`}
            tip="The latest test of this address: how long a round trip took, and whether any of the tiny test messages got lost on the way. Lower times are better."
          >
            <div className="flex items-center justify-between">
              <StatusPill status={c.success ? "ok" : "failing"} />
              <span className="tabular text-sm">
                {c.success ? `${c.latency_ms.toFixed(1)} ms · ${c.packet_loss.toFixed(0)}% loss` : "unreachable"}
              </span>
            </div>
            <div className="mt-1 text-xs" style={{ color: "var(--ink-muted)" }}>
              via {c.method}
              {c.method === "tcp" && " (TCP connect fallback, ICMP unavailable)"}
            </div>
          </Card>
        ))}
      </div>

      <Card
        title="Latency"
        tip="How long round trips to each tested address took, over the chosen window. A calm flat line is good; spikes mean moments of lag, and gaps mean a test failed."
      >
        {chartData.length ? (
          <TimeSeriesChart data={chartData} series={series} format={(v) => `${v.toFixed(1)} ms`} />
        ) : (
          <Empty text="No latency checks yet." />
        )}
      </Card>

      <Card
        title="DNS checks (A and AAAA)"
        tip="Each row is one test of the internet's address book: how long it took to turn a website name into an address. A and AAAA are just the older and newer address formats; both are normal."
      >
        {dns?.items?.length ? (
          <div className="max-h-80 overflow-auto">
            <table className="w-full">
              <thead>
                <tr className="border-b" style={{ borderColor: "var(--border)" }}>
                  <Th>Domain</Th>
                  <Th>Type</Th>
                  <Th>Resolver</Th>
                  <Th>Latency</Th>
                  <Th>Result</Th>
                  <Th>When</Th>
                </tr>
              </thead>
              <tbody>
                {dns.items.slice(0, 40).map((c, i) => (
                  <tr key={i} className="border-b" style={{ borderColor: "var(--border)" }}>
                    <Td>{c.domain}</Td>
                    <Td>{c.record_type}</Td>
                    <Td className="tabular">{c.resolver || "-"}</Td>
                    <Td className="tabular">{c.latency_ms.toFixed(0)} ms</Td>
                    <Td>
                      <StatusPill status={c.success ? "ok" : "failing"} />
                    </Td>
                    <Td className="tabular">{new Date(c.collected_at).toLocaleTimeString()}</Td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        ) : (
          <Empty text="No DNS checks yet." />
        )}
      </Card>
    </div>
  );
}
