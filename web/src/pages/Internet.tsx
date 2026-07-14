import { useMemo, useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { api, LatencyCheck, Range } from "../lib/api";
import { Card, Empty, InfoTip, RangePicker, StatusPill, Td, Th } from "../components/ui";
import { Sparkline } from "../components/charts";

// Fixed slot assignment: color follows the target, never its rank.
const seriesColors = ["var(--series-1)", "var(--series-2)", "var(--series-3)"];

interface TargetStats {
  target: string;
  friendly: string;
  median: number;
  worst: number;
  successRate: number;
  method: string;
  trend: { v: number }[];
  checks: number;
}

// verdictFor turns numbers into a word a newcomer can act on. The median
// is used (not the average) so a single spike does not change the verdict.
function verdictFor(s: TargetStats): { status: string; label: string } {
  if (s.checks === 0) return { status: "unknown", label: "no data" };
  if (s.successRate < 90) return { status: "failing", label: "unstable" };
  if (s.median < 30) return { status: "ok", label: "excellent" };
  if (s.median < 100) return { status: "ok", label: "good" };
  if (s.median < 200) return { status: "degraded", label: "fair" };
  return { status: "critical", label: "slow" };
}

function friendlyTarget(target: string, gatewayIP?: string): string {
  if (gatewayIP && target === gatewayIP) return "Your router";
  if (target === "1.1.1.1") return "Internet (Cloudflare)";
  if (target === "8.8.8.8") return "Internet (Google)";
  return "Internet";
}

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
  const { data: overview } = useQuery({ queryKey: ["overview"], queryFn: api.overview });
  const gatewayIP = overview?.gateway_ip;

  // Per-target summaries: median and worst reply, reliability, and a
  // smoothed trend (bucket averages), instead of a raw multi-line chart.
  const targets = useMemo<TargetStats[]>(() => {
    const items = latency?.items ?? [];
    const by = new Map<string, LatencyCheck[]>();
    for (const c of items) {
      const arr = by.get(c.target) ?? [];
      arr.push(c);
      by.set(c.target, arr);
    }
    const out: TargetStats[] = [];
    for (const [target, checks] of by) {
      const ok = checks
        .filter((c) => c.success)
        .map((c) => c.latency_ms)
        .sort((a, b) => a - b);
      const times = checks.map((c) => Date.parse(c.collected_at));
      const minT = Math.min(...times);
      const maxT = Math.max(...times);
      const bucketMs = Math.max((maxT - minT) / 48, 1);
      const sums = new Map<number, { total: number; n: number }>();
      for (const c of checks) {
        if (!c.success) continue;
        const b = Math.floor((Date.parse(c.collected_at) - minT) / bucketMs);
        const cur = sums.get(b) ?? { total: 0, n: 0 };
        cur.total += c.latency_ms;
        cur.n++;
        sums.set(b, cur);
      }
      const trend = Array.from(sums.entries())
        .sort(([a], [b]) => a - b)
        .map(([, s]) => ({ v: s.total / s.n }));
      out.push({
        target,
        friendly: friendlyTarget(target, gatewayIP),
        median: ok.length ? ok[Math.floor(ok.length / 2)] : 0,
        worst: ok.length ? ok[ok.length - 1] : 0,
        successRate: checks.length
          ? (checks.filter((c) => c.success).length / checks.length) * 100
          : 0,
        method: checks[0]?.method ?? "",
        trend,
        checks: checks.length,
      });
    }
    return out.sort((a, b) => a.target.localeCompare(b.target));
  }, [latency, gatewayIP]);

  return (
    <div className="flex flex-col gap-4">
      <div className="flex items-center justify-between">
        <h1 className="text-xl font-semibold">Internet Health</h1>
        <RangePicker value={range} onChange={setRange} />
      </div>

      <div className="flex items-center gap-1.5">
        <h2 className="text-sm font-medium" style={{ color: "var(--ink-secondary)" }}>
          Response times
        </h2>
        <InfoTip text="How quickly each tested address replies. 'Typically' is the middle of all replies in the selected window, so one bad spike does not change it. 'Answered' is the share of tests that got a reply. The small line shows the trend." />
      </div>

      {targets.length ? (
        <div className="grid grid-cols-1 gap-4 md:grid-cols-3">
          {targets.map((s, i) => {
            const v = verdictFor(s);
            return (
              <Card key={s.target}>
                <div className="flex items-start justify-between gap-2">
                  <div>
                    <div className="font-medium">{s.friendly}</div>
                    <div className="text-xs" style={{ color: "var(--ink-muted)" }}>
                      {s.target}
                    </div>
                  </div>
                  <StatusPill status={v.status} label={v.label} />
                </div>
                <div className="mt-3 text-2xl font-semibold">
                  {s.checks ? (
                    <>
                      {s.median < 100 ? s.median.toFixed(1) : s.median.toFixed(0)}
                      <span className="text-sm font-normal" style={{ color: "var(--ink-muted)" }}>
                        {" "}
                        ms typically
                      </span>
                    </>
                  ) : (
                    "-"
                  )}
                </div>
                <div className="mt-1 text-xs" style={{ color: "var(--ink-secondary)" }}>
                  worst {s.worst < 100 ? s.worst.toFixed(1) : s.worst.toFixed(0)} ms ·{" "}
                  {s.successRate.toFixed(0)}% answered · via {s.method}
                </div>
                <div className="mt-3">
                  <Sparkline points={s.trend} dataKey="v" color={seriesColors[i % 3]} domain={[0, "auto"]} />
                  <div className="text-xs" style={{ color: "var(--ink-muted)" }}>
                    lower is better
                  </div>
                </div>
              </Card>
            );
          })}
        </div>
      ) : (
        <Card>
          <Empty text="No latency checks yet. Give the collector a minute." />
        </Card>
      )}

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
